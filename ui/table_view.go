package ui

import (
	"strings"

	"snowtui/config"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tableListItem struct{ name, desc string }

func (t tableListItem) Title() string       { return t.name }
func (t tableListItem) Description() string { return t.desc }
func (t tableListItem) FilterValue() string { return t.name + " " + t.desc }

type tableViewMode int

const (
	tableViewList  tableViewMode = iota
	tableViewInput               // enter custom table
)

// TableView allows selection of a ServiceNow table.
type TableView struct {
	mode   tableViewMode
	list   list.Model
	input  textinput.Model
	width  int
	height int
}

// NewTableView creates a TableView listing the tables configured in the
// config file.
func NewTableView(tables []config.TableConfig) TableView {
	items := make([]list.Item, len(tables))
	for i, t := range tables {
		items[i] = tableListItem{name: t.Name, desc: t.Description}
	}

	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(white).Background(green).BorderForeground(green)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lightGray).Background(green).BorderForeground(green)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lightGray)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(gray)

	l := list.New(items, d, 0, 0)
	l.Title = "Select table  (ctrl+n → custom table)"
	l.Styles.Title = titleStyle.Copy()
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.KeyMap.Quit.SetEnabled(false)

	ti := textinput.New()
	ti.Placeholder = "e.g.  incident  or  u_custom_table"
	ti.CharLimit = 100
	ti.Width = 50
	ti.Prompt = "▶  "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(green)
	ti.TextStyle = lipgloss.NewStyle().Foreground(white)

	return TableView{mode: tableViewList, list: l, input: ti}
}

func (v TableView) Init() tea.Cmd { return nil }

func (v TableView) Update(msg tea.Msg) (TableView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch v.mode {
		case tableViewList:
			switch msg.String() {
			case "esc":
				if !v.list.SettingFilter() {
					return v, func() tea.Msg { return goBackMsg{} }
				}
			case "ctrl+n":
				v.mode = tableViewInput
				return v, v.input.Focus()
			case "enter":
				if !v.list.SettingFilter() {
					if item, ok := v.list.SelectedItem().(tableListItem); ok {
						tbl := item.name
						return v, func() tea.Msg { return tableSelectedMsg{table: tbl} }
					}
				}
			}

		case tableViewInput:
			switch msg.String() {
			case "esc":
				v.mode = tableViewList
				v.input.Blur()
				v.input.SetValue("")
				return v, nil
			case "enter":
				tbl := strings.TrimSpace(v.input.Value())
				if tbl != "" {
					v.input.SetValue("")
					v.input.Blur()
					return v, func() tea.Msg { return tableSelectedMsg{table: tbl} }
				}
			}
		}
	}

	var cmd tea.Cmd
	switch v.mode {
	case tableViewList:
		v.list, cmd = v.list.Update(msg)
	case tableViewInput:
		v.input, cmd = v.input.Update(msg)
	}
	return v, cmd
}

func (v TableView) View() string {
	if v.mode == tableViewInput {
		content := lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Enter custom table"),
			subtitleStyle.Render("Enter to confirm  ·  Esc to cancel"),
			"",
			v.input.View(),
		)
		box := boxStyle.Copy().Width(v.width / 2).Render(content)
		return lipgloss.Place(v.width, v.height, lipgloss.Center, lipgloss.Center, box)
	}
	return v.list.View()
}

func (v *TableView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.list.SetSize(w, h)
	if w > 12 {
		v.input.Width = w/2 - 8
	}
}
