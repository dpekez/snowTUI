package ui

import (
	"snowtui/api"
	"snowtui/config"
)

// instanceSelectedMsg is sent when the user selects an instance.
type instanceSelectedMsg struct {
	instance *config.Instance
}

// tableSelectedMsg is sent when the user selects a table.
type tableSelectedMsg struct {
	table string
}

// recordSelectedMsg is sent when the user selects a record.
type recordSelectedMsg struct {
	record map[string]interface{}
}

// recordsLoadedMsg is sent when records have been loaded.
type recordsLoadedMsg struct {
	records []map[string]interface{}
	total   int
}

// groupViewRequestedMsg is sent when the user wants to open the group view.
// Carries the context of the current ListView so that GroupView knows the correct
// column picker and already active filter.
type groupViewRequestedMsg struct {
	tableName string   // ServiceNow table name
	query     string   // currently active filter (without sort directive)
	colNames  []string // visible columns for column picker
}

// groupStatsLoadedMsg is sent when group statistics are loaded from API.
type groupStatsLoadedMsg struct {
	stats []api.GroupStat
}

// groupSelectedMsg is sent when the user selects a group.
// query is the complete combined filter that applies to ListView.
type groupSelectedMsg struct {
	query        string // combined filter query for ListView
	groupCol     string // technical column name, e.g., "state"
	displayValue string // readable value, e.g., "New"
}

// goBackMsg is sent when the user wants to navigate back.
type goBackMsg struct{}

// errMsg is sent when an error has occurred.
type errMsg struct {
	err error
}

// newRecordRequestedMsg is sent when the user wants to create a new record
// in the currently open table.
type newRecordRequestedMsg struct{}

// recordSavedMsg is sent when a record has been successfully created or
// updated. record is the record as returned by the server.
type recordSavedMsg struct {
	record map[string]interface{}
}

// recordDeletedMsg is sent when a record has been successfully deleted.
type recordDeletedMsg struct {
	sysID string
}

// mutationErrMsg is sent when a create/update/delete request fails. It is
// distinct from errMsg so views can stay on the current form/list instead
// of treating it as a fatal load error.
type mutationErrMsg struct {
	err error
}
