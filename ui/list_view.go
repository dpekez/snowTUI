package ui

import (
	"fmt"
	"strings"
	"unicode"

	"snowtui/api"

	"github.com/charmbracelet/bubbles/spinner"
	btable "github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// underlineCombining is a combining low line (U+0332). Appending it after a
// rune renders that rune underlined without ANSI escapes, so it survives
// bubbles/table's (non-ANSI-aware) width and truncation logic untouched.
const underlineCombining = "̲"

// underlineFirstRune marks the first rune of s as underlined.
func underlineFirstRune(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(r[0]) + underlineCombining + string(r[1:])
}

const pageSize = 20

// ── Encoded-query operator legend ───────────────────────────────────────────────

// opEntry documents one ServiceNow encoded-query operator: the literal
// token to type into the query and a short human-readable description.
type opEntry struct {
	token string
	desc  string
}

// numericOperators are legal for numeric (and date) field comparisons.
var numericOperators = []opEntry{
	{"=", "equals"},
	{"!=", "not equal to"},
	{">", "greater than"},
	{">=", "greater or equal"},
	{"<", "less than"},
	{"<=", "less or equal"},
}

// stringOperators are legal for string field comparisons. LIKE and NOTLIKE
// are ServiceNow's actual encoded tokens for "contains" / "does not
// contain" (the condition builder's display labels, not literal keywords).
var stringOperators = []opEntry{
	{"=", "equals"},
	{"!=", "not equal to"},
	{"STARTSWITH", "starts with"},
	{"ENDSWITH", "ends with"},
	{"LIKE", "contains"},
	{"NOTLIKE", "does not contain"},
}

// renderOperatorGroup renders a titled, column-aligned list of operators.
func renderOperatorGroup(title string, ops []opEntry) string {
	tokenW := 0
	for _, op := range ops {
		if w := lipgloss.Width(op.token); w > tokenW {
			tokenW = w
		}
	}
	lines := make([]string, 0, len(ops)+1)
	lines = append(lines, titleStyle.Render(title))
	for _, op := range ops {
		pad := strings.Repeat(" ", tokenW-lipgloss.Width(op.token))
		lines = append(lines,
			"  "+keyStyle.Render(op.token)+pad+"  "+subtitleStyle.Render(op.desc))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderOperatorLegend shows every legal encoded-query operator for numeric
// and string comparisons, side by side when width allows or stacked when
// the terminal is too narrow.
func renderOperatorLegend(width int) string {
	numeric := renderOperatorGroup("Numeric", numericOperators)
	str := renderOperatorGroup("String", stringOperators)
	if lipgloss.Width(numeric)+4+lipgloss.Width(str) <= width {
		return lipgloss.JoinHorizontal(lipgloss.Top, numeric, "    ", str)
	}
	return lipgloss.JoinVertical(lipgloss.Left, numeric, "", str)
}

// ── Sort state ────────────────────────────────────────────────────────────────

type sortDir int

const (
	sortNone sortDir = iota
	sortAsc          // ORDERBY    → ↑
	sortDesc         // ORDERBYDESC → ↓
)

// ── List mode ─────────────────────────────────────────────────────────────────

type listMode int

const (
	listLoading listMode = iota
	listBrowsing
	listSearching
	listConfirmDelete
	listError
)

// ── Model ─────────────────────────────────────────────────────────────────────

// ListView displays records of a ServiceNow table in table form.
type ListView struct {
	mode        listMode
	client      *api.Client
	tableName   string
	listColumns []string // preferred columns from the table's config
	sp          spinner.Model
	records     []map[string]interface{}
	colNames    []string
	tbl         btable.Model
	search      textinput.Model
	query       string  // raw user query (without sort)
	sortCol     string  // column currently sorted on
	sortAmt     sortDir // direction of that sort
	offset      int
	total       int
	width       int
	height      int
	err         error

	mutationErr        error // last create/delete failure, shown until the next reload
	pendingDeleteSysID string
	pendingDeleteLabel string
}

// NewListView creates a new ListView. columns are the preferred list
// columns configured for this table (in order); any that aren't present on
// the loaded records are skipped.
func NewListView(client *api.Client, tableName string, columns []string) ListView {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = loadingStyle

	ti := textinput.New()
	ti.Placeholder = "ServiceNow query  (e.g., state=1^priority=1^active=true)"
	ti.CharLimit = 300
	ti.Prompt = "  🔍  "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(purple)
	ti.TextStyle = lipgloss.NewStyle().Foreground(white)

	return ListView{
		mode:        listLoading,
		client:      client,
		tableName:   tableName,
		listColumns: columns,
		sp:          sp,
		search:      ti,
	}
}

// ── Sort helpers ──────────────────────────────────────────────────────────────

// sortKeyMap maps the capitalized first letter of a column name (its
// shift-key shortcut, e.g. shift+n → 'N' for "number") to that column name.
// Columns whose first letter is shared with another visible column are
// ambiguous and get no shortcut at all.
func (v ListView) sortKeyMap() map[rune]string {
	counts := make(map[rune]int, len(v.colNames))
	for _, col := range v.colNames {
		if len(col) == 0 {
			continue
		}
		counts[unicode.ToUpper(rune(col[0]))]++
	}

	m := make(map[rune]string, len(v.colNames))
	for _, col := range v.colNames {
		if len(col) == 0 {
			continue
		}
		r := unicode.ToUpper(rune(col[0]))
		if counts[r] == 1 {
			m[r] = col
		}
	}
	return m
}

// effectiveQuery merges the user's filter query with the current sort
// directive using ServiceNow encoded-query syntax.
func (v ListView) effectiveQuery() string {
	if v.sortCol == "" || v.sortAmt == sortNone {
		return v.query
	}
	var directive string
	if v.sortAmt == sortAsc {
		directive = "ORDERBY" + v.sortCol
	} else {
		directive = "ORDERBYDESC" + v.sortCol
	}
	if v.query == "" {
		return directive
	}
	return v.query + "^" + directive
}

// applySortKey updates sortCol/sortAmt for the given column and returns true
// if a reload is needed.
func (v *ListView) applySortKey(col string) bool {
	switch {
	case v.sortCol != col:
		// New column → start ascending
		v.sortCol = col
		v.sortAmt = sortAsc
	case v.sortAmt == sortAsc:
		// Same column, was ascending → switch to descending
		v.sortAmt = sortDesc
	default:
		// Same column, was descending → clear sort
		v.sortCol = ""
		v.sortAmt = sortNone
	}
	return true
}

// ── Init / Update / View ──────────────────────────────────────────────────────

func (v ListView) Init() tea.Cmd {
	return tea.Batch(v.sp.Tick, v.doFetch())
}

func (v ListView) doFetch() tea.Cmd {
	client := v.client
	tbl := v.tableName
	offset := v.offset
	query := v.effectiveQuery() // includes sort directive
	return func() tea.Msg {
		records, total, err := client.GetRecords(tbl, pageSize, offset, query)
		if err != nil {
			return errMsg{err: err}
		}
		return recordsLoadedMsg{records: records, total: total}
	}
}

func (v ListView) Update(msg tea.Msg) (ListView, tea.Cmd) {
	switch msg := msg.(type) {

	case recordsLoadedMsg:
		v.records = msg.records
		v.total = msg.total
		v.mutationErr = nil
		v.mode = listBrowsing
		v.rebuildTable()
		return v, nil

	case mutationErrMsg:
		v.mutationErr = msg.err
		v.mode = listBrowsing
		return v, nil

	case errMsg:
		v.err = msg.err
		v.mode = listError
		return v, nil

	case spinner.TickMsg:
		if v.mode == listLoading {
			var cmd tea.Cmd
			v.sp, cmd = v.sp.Update(msg)
			return v, cmd
		}

	case tea.KeyMsg:
		switch v.mode {

		case listError:
			switch msg.String() {
			case "r":
				v.mode = listLoading
				v.err = nil
				return v, tea.Batch(v.sp.Tick, v.doFetch())
			case "esc":
				return v, func() tea.Msg { return goBackMsg{} }
			}

		case listConfirmDelete:
			switch msg.String() {
			case "y", "enter":
				client := v.client
				table := v.tableName
				sysID := v.pendingDeleteSysID
				v.mode = listLoading
				return v, tea.Batch(v.sp.Tick, func() tea.Msg {
					if err := client.DeleteRecord(table, sysID); err != nil {
						return mutationErrMsg{err: err}
					}
					return recordDeletedMsg{sysID: sysID}
				})
			case "n", "esc":
				v.mode = listBrowsing
				return v, nil
			}

		case listSearching:
			switch msg.String() {
			case "esc":
				v.mode = listBrowsing
				v.search.Blur()
				return v, nil
			case "enter":
				v.query = strings.TrimSpace(v.search.Value())
				v.offset = 0
				v.mode = listLoading
				v.search.Blur()
				return v, tea.Batch(v.sp.Tick, v.doFetch())
			}

		case listBrowsing:
			switch msg.String() {
			case "/":
				v.mode = listSearching
				v.search.SetValue(v.query)
				return v, v.search.Focus()

			case "r":
				v.mode = listLoading
				return v, tea.Batch(v.sp.Tick, v.doFetch())

			case "enter":
				idx := v.tbl.Cursor()
				if idx >= 0 && idx < len(v.records) {
					rec := v.records[idx]
					return v, func() tea.Msg { return recordSelectedMsg{record: rec} }
				}

			case "n", "right":
				if v.offset+pageSize < v.total {
					v.offset += pageSize
					v.mode = listLoading
					return v, tea.Batch(v.sp.Tick, v.doFetch())
				}

			case "p", "left":
				if v.offset >= pageSize {
					v.offset -= pageSize
					v.mode = listLoading
					return v, tea.Batch(v.sp.Tick, v.doFetch())
				}

			case "c":
				return v, func() tea.Msg { return newRecordRequestedMsg{} }

			case "d":
				idx := v.tbl.Cursor()
				if idx >= 0 && idx < len(v.records) {
					rec := v.records[idx]
					sysID := getStringValue(rec, "sys_id")
					if sysID == "" {
						break
					}
					label := getStringValue(rec, "number")
					if label == "" {
						label = getStringValue(rec, "short_description")
					}
					if label == "" {
						label = sysID
					}
					v.pendingDeleteSysID = sysID
					v.pendingDeleteLabel = label
					v.mutationErr = nil
					v.mode = listConfirmDelete
				}
				return v, nil

			case "g":
				// Gruppenansicht öffnen – nur wenn Spalten bereits bekannt sind.
				if len(v.colNames) > 0 {
					tableName := v.tableName
					query := v.query
					cols := append([]string(nil), v.colNames...) // defensive copy
					return v, func() tea.Msg {
						return groupViewRequestedMsg{
							tableName: tableName,
							query:     query,
							colNames:  cols,
						}
					}
				}

			case "esc":
				return v, func() tea.Msg { return goBackMsg{} }

			default:
				// ── Sort key handling ────────────────────────────────────────
				// A capitalized letter (shift+letter) matching the unique first
				// character of a visible column toggles sort for that column.
				key := msg.String()
				if len([]rune(key)) == 1 {
					r := []rune(key)[0]
					if col, ok := v.sortKeyMap()[r]; ok {
						v.applySortKey(col)
						v.offset = 0 // reset to first page
						v.mode = listLoading
						return v, tea.Batch(v.sp.Tick, v.doFetch())
					}
				}
			}
		}
	}

	var cmd tea.Cmd
	switch v.mode {
	case listSearching:
		v.search, cmd = v.search.Update(msg)
	case listBrowsing:
		v.tbl, cmd = v.tbl.Update(msg)
	}
	return v, cmd
}

// ── Views ─────────────────────────────────────────────────────────────────────

func (v ListView) View() string {
	switch v.mode {
	case listLoading:
		return "\n" + loadingStyle.Render(
			fmt.Sprintf("  %s  Loading '%s'…", v.sp.View(), v.tableName))

	case listError:
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+errorStyle.Render("Error: "+v.err.Error()),
			"",
			"  "+helpStyle.Render(
				keyStyle.Render("r")+" retry   "+
					keyStyle.Render("esc")+" back"),
		)

	case listSearching:
		boxW := v.width - 4
		box := boxStyle.Copy().Width(boxW).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				titleStyle.Render("ServiceNow Query"),
				subtitleStyle.Render(
					"state=1^priority=1 joins conditions\n"+
						"state=1^ORpriority=1 or conditions\n"+
						"short_description!=null filters empty values\n"+
						"Enter = search   Esc = cancel"),
				"",
				v.search.View(),
				"",
				renderOperatorLegend(boxW-4),
			),
		)
		return lipgloss.JoinVertical(lipgloss.Left, v.statusBar(), "", box)

	case listConfirmDelete:
		parts := []string{v.tbl.View(), v.statusBar()}
		parts = append(parts, "  "+warningStyle.Render("Delete "+v.pendingDeleteLabel+"?")+"  "+
			helpStyle.Render(keyStyle.Render("y")+" yes   "+keyStyle.Render("n")+" no"))
		return lipgloss.JoinVertical(lipgloss.Left, parts...)

	default: // listBrowsing
		if len(v.records) == 0 {
			return lipgloss.JoinVertical(lipgloss.Left,
				v.statusBar(),
				"",
				"  "+warningStyle.Render("No records found."),
				"  "+helpStyle.Render(
					keyStyle.Render("/")+" new search   "+keyStyle.Render("esc")+" back"),
			)
		}
		parts := []string{v.tbl.View(), v.statusBar()}
		if v.mutationErr != nil {
			parts = append(parts, "  "+errorStyle.Render("Error: "+v.mutationErr.Error()))
		}
		if p := v.pagination(); p != "" {
			parts = append(parts, p)
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
}

func (v ListView) statusBar() string {
	page := v.offset/pageSize + 1
	totalPages := (v.total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	s := fmt.Sprintf("  Table: %s  │  %d records  │  Page %d/%d",
		v.tableName, v.total, page, totalPages)
	if v.query != "" {
		s += fmt.Sprintf("  │  Filter: %s", truncate(v.query, 30))
	}
	if v.sortCol != "" && v.sortAmt != sortNone {
		arrow := "↑"
		if v.sortAmt == sortDesc {
			arrow = "↓"
		}
		s += fmt.Sprintf("  │  Sort: %s %s", v.sortCol, arrow)
	}
	return subtitleStyle.Render(s)
}

// pagination renders the prev/next navigation hints. The page number itself
// is shown in statusBar, directly above this bar, so it isn't repeated here.
func (v ListView) pagination() string {
	var parts []string
	if v.offset > 0 {
		parts = append(parts, keyStyle.Render("p/←")+" previous")
	}
	if v.offset+pageSize < v.total {
		parts = append(parts, keyStyle.Render("n/→")+" next")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + helpStyle.Render(strings.Join(parts, "   ·   "))
}

// ── Table builder ─────────────────────────────────────────────────────────────

// rebuildTable rebuilds the bubbles/table model from the current records.
// The active sort column gets an ↑ or ↓ appended to its header title.
func (v *ListView) rebuildTable() {
	if len(v.records) == 0 {
		v.tbl = btable.New()
		return
	}

	// Determine visible columns
	existing := make(map[string]bool)
	for k := range v.records[0] {
		existing[k] = true
	}
	var cols []string
	for _, c := range v.listColumns {
		if existing[c] {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		for k := range v.records[0] {
			cols = append(cols, k)
			if len(cols) >= 6 {
				break
			}
		}
	}
	v.colNames = cols

	// Column widths
	available := v.width - len(cols) - 1
	base := available / len(cols)
	if base < 8 {
		base = 8
	}

	sortableCols := v.sortKeyMap() // rune -> col, only for unique first letters

	tableCols := make([]btable.Column, len(cols))
	for i, name := range cols {
		w := base
		switch name {
		case "number":
			w = 14
		case "short_description", "description":
			w = base + 12
		case "sys_updated_on", "sys_created_on", "opened_at":
			w = 20
		case "state", "priority", "impact", "urgency", "active":
			w = 10
		}

		// Build header title; append sort indicator for the active column.
		// Reserve 2 chars for the indicator so the title doesn't overflow.
		title := name
		if name == v.sortCol {
			switch v.sortAmt {
			case sortAsc:
				title = truncate(title, w-2) + " ↑"
			case sortDesc:
				title = truncate(title, w-2) + " ↓"
			}
		}

		// Underline the shift-key shortcut letter, but only when this
		// column's first letter is unique (see sortKeyMap).
		for _, sortCol := range sortableCols {
			if sortCol == name {
				title = underlineFirstRune(title)
				break
			}
		}

		tableCols[i] = btable.Column{Title: title, Width: w}
	}

	// Rows
	rows := make([]btable.Row, len(v.records))
	for i, rec := range v.records {
		row := make(btable.Row, len(cols))
		for j, col := range cols {
			row[j] = truncate(getStringValue(rec, col), tableCols[j].Width)
		}
		rows[i] = row
	}

	// Height: total minus the table's own header block (title row + its
	// bottom border, 2 lines), the status bar (1), and the pagination bar
	// (1, only when there's more than one page).
	paginationLines := 0
	if (v.total+pageSize-1)/pageSize > 1 {
		paginationLines = 1
	}
	tblH := v.height - 2 - 1 - paginationLines
	if tblH < 3 {
		tblH = 3
	}

	s := btable.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderClr).
		BorderBottom(true).
		Foreground(purple).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(white).
		Background(purple).
		Bold(true)
	s.Cell = s.Cell.Foreground(lightGray)

	v.tbl = btable.New(
		btable.WithColumns(tableCols),
		btable.WithRows(rows),
		btable.WithFocused(true),
		btable.WithHeight(tblH),
		btable.WithStyles(s),
	)
}

func (v *ListView) SetSize(w, h int) {
	v.width = w
	v.height = h
	if w > 12 {
		v.search.Width = w - 12
	}
	if v.mode == listBrowsing && len(v.records) > 0 {
		v.rebuildTable()
	}
}
