package ui

import (
	"fmt"

	"snowtui/api"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Internal modes ────────────────────────────────────────────────────────────

type groupMode int

const (
	groupPickingColumn groupMode = iota // Select column
	groupLoading                        // Loading statistics
	groupBrowsing                       // Display group results
	groupError                          // Error state
)

// ── List items ────────────────────────────────────────────────────────────────

// colPickerItem represents a selectable column in the column picker.
type colPickerItem struct{ col string }

func (i colPickerItem) Title() string       { return i.col }
func (i colPickerItem) Description() string { return "" }
func (i colPickerItem) FilterValue() string { return i.col }

// groupStatItem represents a group entry in the results list.
type groupStatItem struct{ stat api.GroupStat }

func (i groupStatItem) Title() string {
	dv := i.stat.DisplayValue
	if dv == "" {
		dv = "(empty)"
	}
	return dv
}

func (i groupStatItem) Description() string {
	filterVal := i.stat.Value
	if filterVal == "" {
		filterVal = i.stat.DisplayValue
	}
	return fmt.Sprintf("%d records   ·   Filter: %s=%s",
		i.stat.Count, i.stat.Field, filterVal)
}

func (i groupStatItem) FilterValue() string { return i.stat.DisplayValue }

// ── Model ─────────────────────────────────────────────────────────────────────

// GroupView first shows a column picker and then displays aggregated
// group results from the ServiceNow Stats API.
type GroupView struct {
	mode      groupMode
	client    *api.Client
	tableName string
	baseQuery string     // Active filter from calling ListView (without sort)
	groupCol  string     // Selected group column
	colNames  []string   // Visible columns for picker
	colList   list.Model // Column picker
	groupList list.Model // Group results
	sp        spinner.Model
	err       error
	width     int
	height    int
}

// NewGroupView creates a GroupView with visible columns from ListView.
func NewGroupView(client *api.Client, tableName, baseQuery string, colNames []string) GroupView {
	delegate := newGroupDelegate()

	// ── Column picker ─────────────────────────────────────────────────────────
	colItems := make([]list.Item, len(colNames))
	for i, col := range colNames {
		colItems[i] = colPickerItem{col: col}
	}
	colList := list.New(colItems, delegate, 0, 0)
	colList.Title = fmt.Sprintf("Select column  (%d available)", len(colNames))
	colList.Styles.Title = titleStyle.Copy()
	colList.Styles.TitleBar = lipgloss.NewStyle()
	colList.SetShowStatusBar(false)
	colList.SetFilteringEnabled(false)
	colList.SetShowHelp(false)
	colList.KeyMap.Quit.SetEnabled(false)

	// ── Group results list ────────────────────────────────────────────────────
	groupList := list.New(nil, delegate, 0, 0)
	groupList.Title = ""
	groupList.Styles.Title = titleStyle.Copy()
	groupList.Styles.TitleBar = lipgloss.NewStyle()
	groupList.SetShowStatusBar(true)
	groupList.SetFilteringEnabled(true)
	groupList.SetShowHelp(false)
	groupList.KeyMap.Quit.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = loadingStyle

	return GroupView{
		mode:      groupPickingColumn,
		client:    client,
		tableName: tableName,
		baseQuery: baseQuery,
		colNames:  colNames,
		colList:   colList,
		groupList: groupList,
		sp:        sp,
	}
}

// newGroupDelegate returns a styled list delegate shared by both inner lists.
func newGroupDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(white).Background(purple).BorderForeground(purple)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lightGray).Background(purple).BorderForeground(purple)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lightGray)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(gray)
	return d
}

// ── Init / fetch ──────────────────────────────────────────────────────────────

func (v GroupView) Init() tea.Cmd { return nil }

func (v GroupView) doFetchStats() tea.Cmd {
	client := v.client
	tableName := v.tableName
	groupCol := v.groupCol
	baseQuery := v.baseQuery
	return func() tea.Msg {
		stats, err := client.GetGroupStats(tableName, groupCol, baseQuery)
		if err != nil {
			return errMsg{err: err}
		}
		return groupStatsLoadedMsg{stats: stats}
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (v GroupView) Update(msg tea.Msg) (GroupView, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Async results ─────────────────────────────────────────────────────────

	case groupStatsLoadedMsg:
		items := make([]list.Item, len(msg.stats))
		for i, stat := range msg.stats {
			items[i] = groupStatItem{stat: stat}
		}
		v.groupList.SetItems(items)
		v.groupList.Title = fmt.Sprintf(
			"Groups by »%s«  ·  %d values found",
			v.groupCol, len(msg.stats))
		v.mode = groupBrowsing
		return v, nil

	case errMsg:
		v.err = msg.err
		v.mode = groupError
		return v, nil

	case spinner.TickMsg:
		if v.mode == groupLoading {
			var cmd tea.Cmd
			v.sp, cmd = v.sp.Update(msg)
			return v, cmd
		}

	// ── Key events ────────────────────────────────────────────────────────────

	case tea.KeyMsg:
		switch v.mode {

		case groupPickingColumn:
			switch msg.String() {
			case "enter":
				if item, ok := v.colList.SelectedItem().(colPickerItem); ok {
					v.groupCol = item.col
					v.mode = groupLoading
					return v, tea.Batch(v.sp.Tick, v.doFetchStats())
				}
			case "esc":
				// Verlasse den GroupView → zurück zur ListView
				return v, func() tea.Msg { return goBackMsg{} }
			}

		case groupBrowsing:
			switch msg.String() {
			case "enter":
				if v.groupList.SettingFilter() {
					break // Filter confirmation goes to bubbles/list
				}
				if item, ok := v.groupList.SelectedItem().(groupStatItem); ok {
					return v, v.buildGroupSelectedCmd(item.stat)
				}
			case "esc":
				if v.groupList.SettingFilter() {
					break // Filter cancellation goes to bubbles/list
				}
				// Back to column picker (internal navigation, no goBackMsg)
				v.mode = groupPickingColumn
				return v, nil
			}

		case groupError:
			switch msg.String() {
			case "r":
				v.mode = groupLoading
				v.err = nil
				return v, tea.Batch(v.sp.Tick, v.doFetchStats())
			case "esc":
				v.mode = groupPickingColumn
				return v, nil
			}
		}
	}

	// Delegate remaining messages to active bubble
	var cmd tea.Cmd
	switch v.mode {
	case groupPickingColumn:
		v.colList, cmd = v.colList.Update(msg)
	case groupBrowsing:
		v.groupList, cmd = v.groupList.Update(msg)
	}
	return v, cmd
}

// buildGroupSelectedCmd builds the combined query and returns the Cmd.
func (v GroupView) buildGroupSelectedCmd(stat api.GroupStat) tea.Cmd {
	// Prefer raw value; fallback to display value if empty.
	filterVal := stat.Value
	if filterVal == "" {
		filterVal = stat.DisplayValue
	}

	// Build combined query: existing filter + group filter.
	var combinedQuery string
	groupClause := fmt.Sprintf("%s=%s", v.groupCol, filterVal)
	if v.baseQuery == "" {
		combinedQuery = groupClause
	} else {
		combinedQuery = v.baseQuery + "^" + groupClause
	}

	displayValue := stat.DisplayValue
	if displayValue == "" {
		displayValue = "(empty)"
	}
	groupCol := v.groupCol

	return func() tea.Msg {
		return groupSelectedMsg{
			query:        combinedQuery,
			groupCol:     groupCol,
			displayValue: displayValue,
		}
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (v GroupView) View() string {
	switch v.mode {
	case groupLoading:
		return "\n" + loadingStyle.Render(
			fmt.Sprintf("  %s  Loading groups for »%s«…",
				v.sp.View(), v.groupCol))

	case groupError:
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+errorStyle.Render("Error: "+v.err.Error()),
			"",
			"  "+helpStyle.Render(
				keyStyle.Render("r")+" retry   "+
					keyStyle.Render("esc")+" back to column selection"),
		)

	case groupBrowsing:
		return v.groupList.View()

	default: // groupPickingColumn
		return v.colList.View()
	}
}

func (v *GroupView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.colList.SetSize(w, h)
	v.groupList.SetSize(w, h)
}

// ActiveGroupCol returns the currently selected grouping column.
func (v GroupView) ActiveGroupCol() string { return v.groupCol }

// IsPickingColumn returns true if no column has been selected yet.
func (v GroupView) IsPickingColumn() bool { return v.mode == groupPickingColumn }
