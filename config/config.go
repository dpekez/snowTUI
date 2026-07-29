package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Instance represents a ServiceNow instance connection.
//
// Authentication is done either via API key (recommended) or
// via username/password (HTTP Basic Auth). If api_key is set, it
// takes precedence over username/password.
type Instance struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
	Active   *bool  `yaml:"active,omitempty"`
}

// UsesAPIKey indicates whether the instance is authenticated via API key instead of Basic Auth.
func (i Instance) UsesAPIKey() bool {
	return i.APIKey != ""
}

// IsActive reports whether the instance should be offered for selection.
// Instances are active by default; set active: false to hide one without
// deleting its configuration.
func (i Instance) IsActive() bool {
	return i.Active == nil || *i.Active
}

// ListConfig configures the record list view for a table.
type ListConfig struct {
	Columns []string `yaml:"columns,omitempty"`
}

// RecordConfig configures the record detail view for a table. The detail
// view always shows every field of a record; this only controls which
// fields are called out in the "Important Fields" section.
type RecordConfig struct {
	ImportantFields []string `yaml:"important_fields,omitempty"`
}

// TableConfig defines how a ServiceNow table is presented: its entry in the
// table browser, its record list columns, and its record detail important
// fields.
type TableConfig struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description,omitempty"`
	Options     []string     `yaml:"options,omitempty"`
	List        ListConfig   `yaml:"list,omitempty"`
	Record      RecordConfig `yaml:"record,omitempty"`
}

// hasOption reports whether the table entry has the given option set.
func (t TableConfig) hasOption(opt string) bool {
	for _, o := range t.Options {
		if o == opt {
			return true
		}
	}
	return false
}

const (
	// DefaultTableName is the reserved table entry name used as a catch-all
	// fallback for list columns and important fields when no exact or
	// wildcard match is found for a table.
	DefaultTableName = "_default"

	// TableOptionWildcard, set via a table entry's options, treats its name
	// as a prefix: it matches any table name starting with it when no exact
	// entry exists for that table.
	TableOptionWildcard = "wildcard"
)

// Config contains all configured instances and tables.
type Config struct {
	Instances []Instance    `yaml:"instances"`
	Tables    []TableConfig `yaml:"tables"`
}

// ActiveInstances returns the configured instances that are active, i.e. not
// explicitly marked with active: false.
func (cfg Config) ActiveInstances() []Instance {
	var active []Instance
	for _, inst := range cfg.Instances {
		if inst.IsActive() {
			active = append(active, inst)
		}
	}
	return active
}

// Table returns the configuration to use for the given table name. Lookup
// order: an exact name match; the longest wildcard-prefix match among
// entries with the "wildcard" option; the reserved "_default" catch-all
// entry, if configured.
func (cfg Config) Table(name string) (TableConfig, bool) {
	for _, t := range cfg.Tables {
		if t.Name == name {
			return t, true
		}
	}

	var best TableConfig
	found := false
	for _, t := range cfg.Tables {
		if t.Name == DefaultTableName || !t.hasOption(TableOptionWildcard) {
			continue
		}
		if strings.HasPrefix(name, t.Name) && (!found || len(t.Name) > len(best.Name)) {
			best = t
			found = true
		}
	}
	if found {
		return best, true
	}

	for _, t := range cfg.Tables {
		if t.Name == DefaultTableName {
			return t, true
		}
	}

	return TableConfig{}, false
}

// BrowsableTables returns the configured tables shown in the table browser,
// excluding the reserved "_default" fallback entry.
func (cfg Config) BrowsableTables() []TableConfig {
	var out []TableConfig
	for _, t := range cfg.Tables {
		if t.Name == DefaultTableName {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Load reads, parses, and validates the configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg Config) validate() error {
	for i, inst := range cfg.Instances {
		if inst.Name == "" {
			return fmt.Errorf("instance #%d: 'name' missing", i+1)
		}
		if inst.URL == "" {
			return fmt.Errorf("instance %q: 'url' missing", inst.Name)
		}
		if !inst.UsesAPIKey() && (inst.Username == "" || inst.Password == "") {
			return fmt.Errorf("instance %q: either 'api_key' or 'username'+'password' required", inst.Name)
		}
	}
	for i, tbl := range cfg.Tables {
		if tbl.Name == "" {
			return fmt.Errorf("table #%d: 'name' missing", i+1)
		}
		for _, opt := range tbl.Options {
			if opt != TableOptionWildcard {
				return fmt.Errorf("table %q: unknown option %q", tbl.Name, opt)
			}
		}
	}
	return nil
}
