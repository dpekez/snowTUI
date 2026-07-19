package ui

import (
	"fmt"

	"snowtui/config"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// instanceItem implements the list.DefaultItem interface.
type instanceItem struct {
	instance config.Instance
}

func (i instanceItem) Title() string { return i.instance.Name }
func (i instanceItem) Description() string {
	auth := i.instance.Username
	if i.instance.UsesAPIKey() {
		auth = "API-Key"
	}
	return fmt.Sprintf("%s  ·  %s", i.instance.URL, auth)
}
func (i instanceItem) FilterValue() string {
	return i.instance.Name + " " + i.instance.URL
}

// InstanceView displays the list of configured instances.
type InstanceView struct {
	list   list.Model
	width  int
	height int
}

// NewInstanceView creates an InstanceView from a slice of instances.
func NewInstanceView(instances []config.Instance) InstanceView {
	items := make([]list.Item, len(instances))
	for i, inst := range instances {
		items[i] = instanceItem{instance: inst}
	}

	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(white).Background(purple).BorderForeground(purple)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lightGray).Background(purple).BorderForeground(purple)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lightGray)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(gray)

	l := list.New(items, d, 0, 0)
	l.Title = "Select ServiceNow Instance"
	l.Styles.Title = titleStyle.Copy()
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.KeyMap.Quit.SetEnabled(false)

	return InstanceView{list: l}
}

func (v InstanceView) Init() tea.Cmd { return nil }

func (v InstanceView) Update(msg tea.Msg) (InstanceView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Only intercept Enter outside of filter mode
		if msg.String() == "enter" && !v.list.SettingFilter() {
			if item, ok := v.list.SelectedItem().(instanceItem); ok {
				inst := item.instance // Kopie
				return v, func() tea.Msg {
					return instanceSelectedMsg{instance: &inst}
				}
			}
		}
	}

	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v InstanceView) View() string {
	return v.list.View()
}

func (v *InstanceView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.list.SetSize(w, h)
}
