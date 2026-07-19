package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// apiKeyHeader is the header name expected by ServiceNow for inbound REST API key authentication.
const apiKeyHeader = "x-sn-apikey"

// Client is a ServiceNow REST API client.
type Client struct {
	BaseURL    string
	Username   string
	Password   string
	APIKey     string
	httpClient *http.Client
}

// NewClient creates a new API client.
//
// If apiKey is set, the client authenticates via the
// ServiceNow API key header; otherwise via HTTP Basic Auth with
// username/password.
func NewClient(baseURL, username, password, apiKey string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")

	c := &Client{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
		APIKey:   apiKey,
	}

	c.httpClient = &http.Client{
		Timeout: defaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			c.authenticate(req)
			return nil
		},
	}

	return c
}

// authenticate sets auth headers on a request, depending on whether
// the client is configured with an API key or username/password.
func (c *Client) authenticate(req *http.Request) {
	if c.APIKey != "" {
		req.Header.Set(apiKeyHeader, c.APIKey)
		return
	}
	req.SetBasicAuth(c.Username, c.Password)
}

// ── Response shapes ───────────────────────────────────────────────────────────

type tableResponse struct {
	Result []map[string]interface{} `json:"result"`
}

type recordResponse struct {
	Result map[string]interface{} `json:"result"`
}

// GroupStat represents an entry from the /api/now/stats endpoint.
type GroupStat struct {
	Field        string // technical column name, e.g., "state"
	DisplayValue string // display value, e.g., "New"
	Value        string // raw value for queries, e.g., "1"
	Count        int
}

// dumpErrorToLog appends either the client error or the full HTTP response dump to snowtui.log
func dumpErrorToLog(resp *http.Response, reqErr error, contextMsg string) {
	f, err := os.OpenFile("snowtui.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // Silently return if we cannot open the log file
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf("=== ERROR DUMP: %s [%s] ===\n", contextMsg, time.Now().Format(time.RFC3339)))

	if reqErr != nil {
		f.WriteString(fmt.Sprintf("Client Request Error: %v\n", reqErr))
	}

	if resp != nil {
		// Dump the response including the body (true)
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			f.WriteString(fmt.Sprintf("Could not dump response: %v\n", err))
		} else {
			f.Write(dump)
			f.WriteString("\n")
		}
	}
	f.WriteString("=========================================\n\n")
}

// ── Request helpers ───────────────────────────────────────────────────────────

// newRequest baut einen authentifizierten Request mit den übergebenen
// Query-Parametern und optionalem JSON-Body (nil für Requests ohne Body).
func (c *Client) newRequest(method, endpoint string, params url.Values, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("Request-Body kodieren: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("Request erstellen: %w", err)
	}
	c.authenticate(req)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.URL.RawQuery = params.Encode()
	return req, nil
}

// doRequest führt den Request aus und loggt Netzwerkfehler bzw. Fehler-Antworten.
func (c *Client) doRequest(req *http.Request, context string) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		dumpErrorToLog(nil, err, context+" - Network/Client Error")
		return nil, fmt.Errorf("Request ausführen: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		dumpErrorToLog(resp, nil, fmt.Sprintf("%s - HTTP %d", context, resp.StatusCode))
	}
	return resp, nil
}

// parseAPIError reads the ServiceNow error body from a non-2xx response and
// returns a descriptive error. ServiceNow reports validation and permission
// failures as {"error":{"message","detail"},"status":"failure"}; this is the
// only validation surface the client relies on for writes.
func parseAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		if errResp.Error.Detail != "" && errResp.Error.Detail != errResp.Error.Message {
			return fmt.Errorf("%s: %s", errResp.Error.Message, errResp.Error.Detail)
		}
		return fmt.Errorf("%s", errResp.Error.Message)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("authentication failed (401): check credentials")
	case http.StatusForbidden:
		return fmt.Errorf("access denied (403): insufficient permissions")
	case http.StatusNotFound:
		return fmt.Errorf("record not found (404)")
	}
	return fmt.Errorf("API error: HTTP %d", resp.StatusCode)
}

// ── API methods ───────────────────────────────────────────────────────────────

// GetRecords ruft Datensätze aus einer ServiceNow-Tabelle ab.
func (c *Client) GetRecords(table string, limit, offset int, query string) ([]map[string]interface{}, int, error) {
	endpoint := fmt.Sprintf("%s/api/now/table/%s", c.BaseURL, table)

	params := url.Values{}
	params.Set("sysparm_limit", strconv.Itoa(limit))
	params.Set("sysparm_offset", strconv.Itoa(offset))
	params.Set("sysparm_display_value", "true")
	params.Set("sysparm_exclude_reference_link", "true")
	if query != "" {
		params.Set("sysparm_query", query)
	}

	req, err := c.newRequest(http.MethodGet, endpoint, params, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.doRequest(req, "GetRecords")
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, 0, fmt.Errorf("authentication failed (401): check credentials")
	case http.StatusForbidden:
		return nil, 0, fmt.Errorf("access denied (403): insufficient permissions")
	case http.StatusNotFound:
		return nil, 0, fmt.Errorf("table not found (404): %s", table)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	total, _ := strconv.Atoi(resp.Header.Get("X-Total-Count"))

	var result tableResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}
	return result.Result, total, nil
}

// GetRecord retrieves a single record by sys_id.
func (c *Client) GetRecord(table, sysID string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/now/table/%s/%s", c.BaseURL, table, sysID)

	params := url.Values{}
	params.Set("sysparm_display_value", "true")
	params.Set("sysparm_exclude_reference_link", "true")

	req, err := c.newRequest(http.MethodGet, endpoint, params, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, "GetRecord")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API-Fehler: HTTP %d", resp.StatusCode)
	}

	var result recordResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Antwort dekodieren: %w", err)
	}
	return result.Result, nil
}

// GetGroupStats ruft Gruppen-Statistiken vom /api/now/stats Endpoint ab.
//
// ServiceNow gruppiert die Ergebnisse nach groupCol und gibt für jeden
// eindeutigen Wert die Anzahl der passenden Datensätze zurück.
// Die Ergebnisse werden absteigend nach Anzahl sortiert zurückgegeben.
//
// Hinweis: Der Endpoint erfordert auf manchen Instanzen die Rolle
// "stats_api" oder "itil". Bei Zugriffsproblemen kommt HTTP 403.
func (c *Client) GetGroupStats(table, groupCol, query string) ([]GroupStat, error) {
	endpoint := fmt.Sprintf("%s/api/now/stats/%s", c.BaseURL, table)

	params := url.Values{}
	params.Set("sysparm_group_by", groupCol)
	params.Set("sysparm_count", "true")
	// "all" (not "true") is required so groupby_fields includes both the raw
	// value (used to build the filter query) and the display_value (used to
	// label groups) — with "true" ServiceNow collapses "value" to the display
	// text and omits display_value entirely, breaking group filtering.
	params.Set("sysparm_display_value", "all")
	params.Set("sysparm_exclude_reference_link", "true")
	if query != "" {
		params.Set("sysparm_query", query)
	}

	req, err := c.newRequest(http.MethodGet, endpoint, params, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, "GetGroupStats")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("authentication failed (401)")
	case http.StatusForbidden:
		return nil, fmt.Errorf("access denied (403): stats_api role required?")
	case http.StatusNotFound:
		return nil, fmt.Errorf("table not found (404): %s", table)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	// ServiceNow stats response shape:
	// {"result": [{"groupby_fields": [{"field":"state","display_value":"New","value":"1"}],
	//              "stats": {"count": "42"}}]}
	var raw struct {
		Result []struct {
			GroupByFields []struct {
				Field        string `json:"field"`
				DisplayValue string `json:"display_value"`
				Value        string `json:"value"`
			} `json:"groupby_fields"`
			Stats struct {
				Count string `json:"count"`
			} `json:"stats"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("Antwort dekodieren: %w", err)
	}

	stats := make([]GroupStat, 0, len(raw.Result))
	for _, entry := range raw.Result {
		if len(entry.GroupByFields) == 0 {
			continue
		}
		f := entry.GroupByFields[0]
		count, _ := strconv.Atoi(entry.Stats.Count)
		stats = append(stats, GroupStat{
			Field:        f.Field,
			DisplayValue: f.DisplayValue,
			Value:        f.Value,
			Count:        count,
		})
	}

	// Sort by count descending so the largest groups appear first.
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats, nil
}

// writeParams returns the query parameters shared by all write requests.
// sysparm_input_display_value mirrors sysparm_display_value (used for reads)
// so that submitted field values are interpreted as display values rather
// than raw values, matching what the UI shows and edits.
func writeParams() url.Values {
	params := url.Values{}
	params.Set("sysparm_display_value", "true")
	params.Set("sysparm_input_display_value", "true")
	params.Set("sysparm_exclude_reference_link", "true")
	return params
}

// CreateRecord creates a new record in the given table with the supplied
// fields and returns the record as stored by ServiceNow.
func (c *Client) CreateRecord(table string, fields map[string]interface{}) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/now/table/%s", c.BaseURL, table)

	req, err := c.newRequest(http.MethodPost, endpoint, writeParams(), fields)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, "CreateRecord")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	var result recordResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result.Result, nil
}

// UpdateRecord applies a partial update to a record and returns the record
// as stored by ServiceNow after the update.
func (c *Client) UpdateRecord(table, sysID string, fields map[string]interface{}) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/now/table/%s/%s", c.BaseURL, table, sysID)

	req, err := c.newRequest(http.MethodPatch, endpoint, writeParams(), fields)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, "UpdateRecord")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	var result recordResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result.Result, nil
}

// DeleteRecord deletes a record by sys_id.
func (c *Client) DeleteRecord(table, sysID string) error {
	endpoint := fmt.Sprintf("%s/api/now/table/%s/%s", c.BaseURL, table, sysID)

	req, err := c.newRequest(http.MethodDelete, endpoint, url.Values{}, nil)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req, "DeleteRecord")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return parseAPIError(resp)
	}
	return nil
}
