package ui

import (
	"fmt"

	"snowtui/api"
	"snowtui/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── View states ───────────────────────────────────────────────────────────────

type viewState int

const (
	stateInstanceSelect viewState = iota
	stateTableSelect
	stateRecordList
	stateGroupView // Spalten-Picker + Gruppen-Ergebnisliste
	stateRecordDetail
)

// ── Model ─────────────────────────────────────────────────────────────────────

// Let's start by defining our model which will store our application's state. It can be any type, but a struct usually makes the most sense.
type AppModel struct {
	state  viewState
	config *config.Config
	width  int
	height int

	instanceView InstanceView
	tableView    TableView
	listView     ListView
	groupView    GroupView
	detailView   DetailView

	selectedInstance *config.Instance
	selectedTable    string
	client           *api.Client

	// Tracking for group navigation:
	// listFromGroup=true → esc from ListView goes back to GroupView,
	// not to table selection.
	listFromGroup    bool
	groupFilterLabel string   // Display label in breadcrumb, e.g., "state: New"
	savedListView    ListView // Original ListView, saved before GroupView opens
}

// Next, we’ll define our application’s initial state.
// Init can return a Cmd that could perform some initial I/O.
// For now, we don’t need to do any I/O, so for the command, we’ll just return nil, which translates to “no command.”
func NewApp(cfg *config.Config) AppModel {
	return AppModel{
		state:        stateInstanceSelect,
		config:       cfg,
		instanceView: NewInstanceView(cfg.ActiveInstances()),
	}
}

// ── BubbleTea interface ───────────────────────────────────────────────────────

// After that, we’ll define our application’s initial state in the Init method.
// Init can return a Cmd that could perform some initial I/O.
// For now, we don't need to do any I/O, so for the command, we'll just return nil, which translates to "no command."
func (m AppModel) Init() tea.Cmd {
	return m.instanceView.Init()
}

// The update function is called when “things happen.”
// Its job is to look at what has happened and return an updated model in response.
// It can also return a Cmd to make more things happen, but for now don't worry about that part.
// The “something happened” comes in the form of a Msg, which can be any type.
// Messages are the result of some I/O that took place, such as a keypress, timer tick, or a response from a server.
// We usually figure out which type of Msg we received with a type switch, but you could also use a type assertion.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Window resize ─────────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		ch := m.contentHeight()
		switch m.state {
		case stateInstanceSelect:
			m.instanceView.SetSize(m.width, ch)
		case stateTableSelect:
			m.tableView.SetSize(m.width, ch)
		case stateRecordList:
			m.listView.SetSize(m.width, ch)
		case stateGroupView:
			m.groupView.SetSize(m.width, ch)
		case stateRecordDetail:
			m.detailView.SetSize(m.width, ch)
		}
		return m, nil

	// ── Global keys ───────────────────────────────────────────────────────────
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	// ── Navigation messages ───────────────────────────────────────────────────

	case instanceSelectedMsg:
		m.selectedInstance = msg.instance
		m.client = api.NewClient(msg.instance.URL, msg.instance.Username, msg.instance.Password, msg.instance.APIKey)
		m.tableView = NewTableView(m.config.Tables)
		m.tableView.SetSize(m.width, m.contentHeight())
		m.state = stateTableSelect
		return m, m.tableView.Init()

	case tableSelectedMsg:
		m.selectedTable = msg.table
		tblCfg, _ := m.config.Table(msg.table)
		m.listView = NewListView(m.client, msg.table, tblCfg.List.Columns)
		m.listView.SetSize(m.width, m.contentHeight())
		m.listFromGroup = false
		m.groupFilterLabel = ""
		m.state = stateRecordList
		return m, m.listView.Init()

	// ── Group view messages ───────────────────────────────────────────────────

	case groupViewRequestedMsg:
		// Save original ListView before it could be overwritten by a group list
		// (happens in groupSelectedMsg).
		m.savedListView = m.listView
		m.groupView = NewGroupView(m.client, msg.tableName, msg.query, msg.colNames)
		m.groupView.SetSize(m.width, m.contentHeight())
		m.state = stateGroupView
		return m, m.groupView.Init()

	case groupSelectedMsg:
		// New ListView with the combined query built by GroupView.
		tblCfg, _ := m.config.Table(m.selectedTable)
		m.listView = NewListView(m.client, m.selectedTable, tblCfg.List.Columns)
		m.listView.query = msg.query
		m.listView.SetSize(m.width, m.contentHeight())
		m.listFromGroup = true
		m.groupFilterLabel = fmt.Sprintf("%s: %s",
			msg.groupCol, msg.displayValue)
		m.state = stateRecordList
		return m, m.listView.Init()

	// ── Record detail ─────────────────────────────────────────────────────────

	case recordSelectedMsg:
		tblCfg, _ := m.config.Table(m.selectedTable)
		m.detailView = NewDetailView(m.client, m.selectedTable, msg.record, tblCfg.Record.ImportantFields)
		m.detailView.SetSize(m.width, m.contentHeight())
		m.state = stateRecordDetail
		return m, m.detailView.Init()

	case newRecordRequestedMsg:
		tblCfg, _ := m.config.Table(m.selectedTable)
		m.detailView = NewDetailViewForCreate(m.client, m.selectedTable, tblCfg.Record.ImportantFields)
		m.detailView.SetSize(m.width, m.contentHeight())
		m.state = stateRecordDetail
		return m, m.detailView.Init()

	// ── Record mutations ──────────────────────────────────────────────────────

	case recordDeletedMsg:
		// Whether the delete came from the list (already in stateRecordList)
		// or the detail view (navigating back), reload so the deleted record
		// no longer appears.
		m.state = stateRecordList
		m.listView.mode = listLoading
		return m, tea.Batch(m.listView.sp.Tick, m.listView.doFetch())

	// ── Back navigation ───────────────────────────────────────────────────────

	case goBackMsg:
		return m.handleGoBack()
	}

	// ── Delegate to the active view ───────────────────────────────────────────
	var cmd tea.Cmd
	switch m.state {
	case stateInstanceSelect:
		m.instanceView, cmd = m.instanceView.Update(msg)
	case stateTableSelect:
		m.tableView, cmd = m.tableView.Update(msg)
	case stateRecordList:
		m.listView, cmd = m.listView.Update(msg)
	case stateGroupView:
		m.groupView, cmd = m.groupView.Update(msg)
	case stateRecordDetail:
		m.detailView, cmd = m.detailView.Update(msg)
	}
	return m, cmd
}

// At last, it’s time to render our UI. Of all the methods, the view is the simplest.
// We look at the model in its current state and use it to build a tea.View.
// The view declares our UI content and, optionally, terminal features like alt screen mode, mouse tracking, cursor position, and more.
func (m AppModel) View() string {
	if m.width == 0 {
		return "Initializing…"
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	var content string
	switch m.state {
	case stateInstanceSelect:
		content = m.instanceView.View()
	case stateTableSelect:
		content = m.tableView.View()
	case stateRecordList:
		content = m.listView.View()
	case stateGroupView:
		content = m.groupView.View()
	case stateRecordDetail:
		content = m.detailView.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m AppModel) contentHeight() int {
	h := m.height - 2 // 1 Header + 1 Footer
	if h < 1 {
		return 1
	}
	return h
}

func (m AppModel) renderHeader() string {
	title := fmt.Sprintf("  %s  ", appName)

	var crumb string
	switch m.state {
	case stateInstanceSelect:
		crumb = "  Select Instance"
	case stateTableSelect:
		if m.selectedInstance != nil {
			crumb = fmt.Sprintf("  %s  ›  Select Table", m.selectedInstance.Name)
		}
	case stateRecordList:
		if m.selectedInstance != nil {
			crumb = fmt.Sprintf("  %s  ›  %s", m.selectedInstance.Name, m.selectedTable)
			if m.groupFilterLabel != "" {
				crumb += fmt.Sprintf("  ›  %s", m.groupFilterLabel)
			}
		}
	case stateGroupView:
		if m.selectedInstance != nil {
			crumb = fmt.Sprintf("  %s  ›  %s  ›  Group View",
				m.selectedInstance.Name, m.selectedTable)
			if col := m.groupView.ActiveGroupCol(); col != "" {
				crumb += fmt.Sprintf(": %s", col)
			}
		}
	case stateRecordDetail:
		if m.selectedInstance != nil {
			crumb = fmt.Sprintf("  %s  ›  %s  ›  Detail",
				m.selectedInstance.Name, m.selectedTable)
		}
	}

	left := headerStyle.Render(title)
	rightW := m.width - lipgloss.Width(left)
	if rightW < 0 {
		rightW = 0
	}
	right := breadcrumbStyle.Copy().Width(rightW).Render(crumb)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m AppModel) renderFooter() string {
	var keys [][2]string
	switch m.state {
	case stateInstanceSelect:
		keys = [][2]string{
			{"↑/↓", "navigate"}, {"enter", "select"},
			{"/", "filter"}, {"ctrl+c", "quit"},
		}
	case stateTableSelect:
		keys = [][2]string{
			{"↑/↓", "navigate"}, {"enter", "select"},
			{"/", "filter"}, {"ctrl+n", "custom table"}, {"esc", "back"},
		}
	case stateRecordList:
		keys = [][2]string{
			{"↑/↓", "navigate"}, {"enter", "detail"},
			{"/", "search"}, {"g", "group"},
			{"n/p", "page"}, {"shift+[A-Z]", "sort"},
			{"c", "create"}, {"d", "delete"},
			{"r", "reload"}, {"esc", "back"},
		}
	case stateGroupView:
		if m.groupView.IsPickingColumn() {
			keys = [][2]string{
				{"↑/↓", "select column"}, {"enter", "group"},
				{"esc", "back"},
			}
		} else {
			keys = [][2]string{
				{"↑/↓", "navigate"}, {"enter", "apply filter"},
				{"/", "search"}, {"esc", "select column"},
			}
		}
	case stateRecordDetail:
		keys = [][2]string{
			{"↑/↓", "navigate"}, {"enter", "edit field"},
			{"ctrl+s", "save"},
		}
		if m.detailView.IsNew() {
			keys = append(keys, [2]string{"a", "add field"})
		} else {
			keys = append(keys, [2]string{"d", "delete"})
		}
		keys = append(keys, [2]string{"esc", "back"})
	}

	line := renderHelp(keys)
	return helpStyle.Copy().Width(m.width).Render(line)
}

func (m AppModel) handleGoBack() (tea.Model, tea.Cmd) {
	switch m.state {
	case stateTableSelect:
		m.state = stateInstanceSelect

	case stateRecordList:
		if m.listFromGroup {
			// From a group-filtered list → back to GroupView
			m.state = stateGroupView
		} else {
			// From a normal list → back to table selection
			m.tableView = NewTableView(m.config.Tables)
			m.tableView.SetSize(m.width, m.contentHeight())
			m.state = stateTableSelect
			return m, m.tableView.Init()
		}

	case stateGroupView:
		// GroupView → back to original (unfiltered) ListView.
		// Current m.listView may be a group-filtered list
		// (if user already selected a group), so restore
		// the saved original ListView.
		m.listView = m.savedListView
		m.listView.SetSize(m.width, m.contentHeight()) // Update size
		m.listFromGroup = false
		m.groupFilterLabel = ""
		m.state = stateRecordList

	case stateRecordDetail:
		m.state = stateRecordList
	}
	return m, nil
}
