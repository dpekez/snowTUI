# ServiceNow TUI

A Text User Interface applicatoin for accessing the ServiceNow Table API – built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

Tired of browser tabs? Now you can browse ServiceNow incidents from your terminal like it's 1985 and *not* deal anymore with Chrome eating up your RAM.

```
┌──  ServiceNow TUI ────────────────────────────────────────────────────────┐
│  dev  ›  incident                                                         │
├───────────────────────────────────────────────────────────────────────────┤
│  Table: incident  │  1243 records  │  Page 1/63                           │
│ ┌──────────────┬──────────────────────────────┬──────────┬─────────────┐  │
│ │ number       │ short_description            │ state    │ priority    │  │
│ ├──────────────┼──────────────────────────────┼──────────┼─────────────┤  │
│ │ INC0010042   │ Cannot access VPN            │ In Pro…  │ 2 - High    │  │
│ │ INC0010041   │ Email not working            │ New      │ 3 - Modera… │  │
│ │ INC0010040   │ Printer offline              │ Resolv…  │ 4 - Low     │  │
│ └──────────────┴──────────────────────────────┴──────────┴─────────────┘  │
│  p/← previous   Page 1/63   n/→ next                                      │
├───────────────────────────────────────────────────────────────────────────┤
│  ↑/↓ navigate  enter detail  / search  n/p page back/forward  esc back    │
└───────────────────────────────────────────────────────────────────────────┘
```

## Features

- **Instance Selection** – Manage multiple ServiceNow instances via config
- **Table Browser** – List of 18 pre-configured tables
- **Choose Table** – Open any table through custom input
- **Record List** – Paginated table view with configurable column selection
- **Group By** – Group records by a specific field
- **ServiceNow Query** – Enter query syntax directly (e.g., `state=1^priority=1`)
- **Detail View** – Browse and edit any field of a record
- **Detail View Important Fields** – Pick specific fields to display on top via config
- **Full Data** – Uses `sysparm_display_value=true` for human-readable display values
- **Record Creation** – Start a blank record from the list view, fill in any field (including arbitrary ones beyond the configured set), and submit
- **Record Update** – Edit any field on an existing record and save the changes
- **Record Deletion** – Delete the selected record from the list or detail view

## Requirements

- Go 1.22+
- Access to a ServiceNow instance via Basic Auth or API Key
- REST API Policy for Table and Aggregate API if you're using API Key
- `x-sn-apikey` header configured for API key auth

## Installation

```bash
# Clone repository
git clone <repo-url>
cd snowtui

# Create config
cp config.yaml.example config.yaml
# → fill config.yaml with your instance details

# Download dependencies and build
go mod tidy
go build -o snowtui .

# Run
./snowtui

# Optional: provide custom config path
./snowtui /path/to/config.yaml
```

## Configuration

An example config `config.yaml.example` is located in the repository.

```yaml
instances:
  - name: "dev"
    url: "https://dev12345.service-now.com"
    api_key: "your-api-key"

  - name: "production"
    url: "https://mycompany.service-now.com"
    username: "readonly"
    password: "your-password"

tables:
  - name: "incident"
    description: "IT incidents and outages"
    list:
      columns: ["number", "short_description", "state", "priority", "impact",
                "urgency", "category", "assigned_to", "caller_id", "sys_updated_on", "active"]
    record:
      important_fields: ["number", "sys_id", "short_description", "description",
                          "state", "priority", "impact", "urgency", "category", "subcategory",
                          "assigned_to", "assignment_group", "caller_id", "opened_by",
                          "opened_at", "resolved_at", "closed_at",
                          "sys_created_on", "sys_updated_on", "active", "escalation"]
```

Instance Fields:

| Field      | Description                                                       |
|------------|-------------------------------------------------------------------|
| `name`     | Display name in the TUI                                           |
| `url`      | Base URL of the instance (without trailing `/`)                   |
| `api_key`  | ServiceNow REST API key (recommended, takes precedence over username/password) |
| `username` | ServiceNow username (only if `api_key` is not set)                |
| `password` | Password or app token (only if `api_key` is not set)              |

Each instance requires either `api_key` **or** `username`+`password`. If
`api_key` is set, the client sends the `x-sn-apikey` header instead of HTTP
Basic Auth.

Table Fields:

| Field        | Description                                                       |
|--------------|-------------------------------------------------------------------|
| `name`       | ServiceNow table name                                             |
| `description`| Description of the table                                          |
| `list`       | Columns to display in the list view                               |
| `record`     | Fields to display in the record important fields view             |

## Keyboard Shortcuts

### Instance Selection
| Key        | Action            |
|------------|-------------------|
| `↑ / ↓`    | Navigate          |
| `Enter`    | Open instance     |
| `/`        | Filter list       |
| `Ctrl+C`   | Quit              |

### Table Selection
| Key        | Action                    |
|------------|---------------------------|
| `↑ / ↓`    | Navigate                  |
| `Enter`    | Open table                |
| `/`        | Filter list               |
| `Ctrl+N`   | Enter custom table name   |
| `Esc`      | Back                      |

### Record List
| Key          | Action                    |
|--------------|---------------------------|
| `↑ / ↓`      | Navigate                  |
| `Enter`      | Open detail view          |
| `/`          | Enter ServiceNow query    |
| `n` / `→`    | Next page                 |
| `p` / `←`    | Previous page             |
| `Shift+[A-Z]`| Sort by selected column   |
| `c`          | Create new record         |
| `d`          | Delete selected record (y/n confirm) |
| `r`          | Reload                    |
| `g`          | Open group view           |
| `Esc`        | Back                      |

### Group View
| Key        | Action                    |
|------------|---------------------------|
| `↑ / ↓`    | Select column / group     |
| `Enter`    | Group by column / apply group filter |
| `Ctrl+N`   | Group by a custom field name |
| `/`        | Filter group results      |
| `Esc`      | Back                      |

### Detail View
| Key         | Action                                              |
|-------------|------------------------------------------------------|
| `↑ / ↓`     | Move between fields                                 |
| `Enter`     | Edit the selected field                              |
| `Enter` / `Esc` (while editing) | Commit / cancel that field's edit       |
| `a`         | Add an arbitrary new field (only when creating a record) |
| `Ctrl+S`    | Save changes / create the record (y/n confirm)       |
| `d`         | Delete the record (y/n confirm, only for existing records) |
| `Esc` / `q` | Back (discards unsaved edits)                        |

Editing a field replaces its value with a text input; every field of the
record is editable, not just the "Important Fields". Edited-but-unsaved
fields are marked with `*` until saved. Creating a record starts from the
table's configured `important_fields` (empty) and lets you add any other
field by name before submitting — nothing is restricted to that list.

## ServiceNow Query Syntax

Use native ServiceNow Encoded Query syntax in the search prompt:

```
state=1                         → Exact match queries
state=1^priority=1              → AND queries  
assigned_toISEMPTY              → Filter for empty fields
sys_updated_on>=2024-01-01      → Updated since Jan 1, 2024
short_descriptionLIKEvpn        → Short description contains "vpn"
nameSTARTSWITHtest              → Name starts with "test"
```

## API Reference

snowtui talks to the ServiceNow Table API and Aggregate API (Stats API), via `api/client.go`. Every request and header sent by the app is listed below.

### Endpoints

| Method | Endpoint | Used by | Purpose |
|--------|----------|---------|---------|
| `GET`    | `/api/now/table/{table}` | `GetRecords` | List records in a table (paginated) |
| `GET`    | `/api/now/table/{table}/{sys_id}` | `GetRecord` | Fetch a single record |
| `POST`   | `/api/now/table/{table}` | `CreateRecord` | Create a new record |
| `PATCH`  | `/api/now/table/{table}/{sys_id}` | `UpdateRecord` | Partially update a record |
| `DELETE` | `/api/now/table/{table}/{sys_id}` | `DeleteRecord` | Delete a record |
| `GET`    | `/api/now/stats/{table}` | `GetGroupStats` | Grouped counts used for the group-by view; some instances require the Aggregate Api `stats_api` or `itil` role, returning HTTP 403 otherwise |

### Query Parameters

| Parameter | Value | Sent by | Meaning |
|-----------|-------|---------|---------|
| `sysparm_limit` | page size | `GetRecords` | Maximum number of records to return |
| `sysparm_offset` | page offset | `GetRecords` | Number of records to skip, for pagination |
| `sysparm_query` | ServiceNow encoded query | `GetRecords`, `GetGroupStats` | Filter, e.g. `state=1^priority=1`; omitted entirely when empty |
| `sysparm_display_value` | `true` | `GetRecords`, `GetRecord`, `CreateRecord`, `UpdateRecord` | Return/accept human-readable display values instead of raw values |
| `sysparm_display_value` | `all` | `GetGroupStats` | Return both the raw value and the display value per group, so raw values can build filter queries while display values label the group |
| `sysparm_input_display_value` | `true` | `CreateRecord`, `UpdateRecord` | Interpret submitted field values as display values (matches what the UI shows/edits) rather than raw values |
| `sysparm_exclude_reference_link` | `true` | `GetRecords`, `GetRecord`, `GetGroupStats`, `CreateRecord`, `UpdateRecord` | Omit the `link` metadata field on reference fields |
| `sysparm_group_by` | column name | `GetGroupStats` | Table column to group counts by |
| `sysparm_count` | `true` | `GetGroupStats` | Include the aggregate record count per group |

`DeleteRecord` sends no query parameters.

### Headers

Request headers:

| Header | Value | Sent on | Notes |
|--------|-------|---------|-------|
| `Accept` | `application/json` | every request | |
| `Content-Type` | `application/json` | requests with a body (`CreateRecord`, `UpdateRecord`) | |
| `Authorization` | `Basic <base64(username:password)>` | every request | only when the instance has no `api_key` configured |
| `x-sn-apikey` | the instance's `api_key` | every request | only when the instance has `api_key` configured; takes precedence over username/password |

Response headers:

| Header | Read by | Meaning |
|--------|---------|---------|
| `X-Total-Count` | `GetRecords` | Total number of matching records across all pages, used to compute page count |

## Project Structure

```
snowtui/
├── main.go               # Entry point
├── config.yaml.example   # Example configuration
├── config/
│   └── config.go         # Load YAML configuration
├── api/
│   └── client.go         # ServiceNow Table API client
└── ui/
    ├── app.go            # Root model, state machine
    ├── messages.go       # Bubble Tea message types
    ├── styles.go         # Lipgloss styles and helpers
    ├── instance_view.go  # Instance selection
    ├── table_view.go     # Table selection
    ├── list_view.go      # Record list with pagination
    └── detail_view.go    # Record detail view
```

## Known Issues

- top header disappears/UI layout is broken if confirmation dialog is answered with no
- list view column sort does not work for some fields
- terminal widths != 115 result in buggy UI layout

## Future Features

- two-lane recordview
- customizable/dynamic pagination sizes
