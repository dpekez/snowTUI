package ui

import (
	"fmt"
	"sort"
	"strings"

	"snowtui/api"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type detailMode int

const (
	detailLoading       detailMode = iota
	detailReady                    // browsing fields, cursor moves between them
	detailEditingField             // textinput editing the value of the field at cursor
	detailAddFieldName             // (create mode only) textinput for a new field's name
	detailAddFieldValue            // (create mode only) textinput for that field's value
	detailConfirm                  // inline y/n confirm before save/create/delete
	detailSaving                   // create/update/delete request in flight
	detailError
)

// pendingAction identifies which mutation a detailConfirm/detailSaving cycle
// is carrying out.
type pendingAction int

const (
	actionNone pendingAction = iota
	actionSave
	actionCreate
	actionDelete
)

// DetailView displays and edits all fields of a single ServiceNow record. In
// "create" mode (isNew) it instead builds a brand-new record from scratch.
type DetailView struct {
	mode            detailMode
	client          *api.Client
	table           string
	record          map[string]interface{}
	sysID           string
	importantFields []string // configured fields shown in the "Important Fields" section
	isNew           bool     // true when this view is creating a record rather than viewing one

	fieldOrder []string          // navigable field keys, important fields first
	cursor     int               // index into fieldOrder
	dirty      map[string]string // field -> edited value, pending save

	input            textinput.Model // active textinput for editing/add-field flows
	pendingFieldName string          // (add-field flow) name entered before its value
	pendingAction    pendingAction
	confirmPrompt    string
	saveErr          error

	vp     viewport.Model
	sp     spinner.Model
	err    error
	width  int
	height int
}

// NewDetailView creates a DetailView for viewing/editing an existing record.
// importantFields are the fields configured for this table's "Important
// Fields" section; the detail view always shows every field of the record
// regardless, and every field is editable.
func NewDetailView(client *api.Client, table string, record map[string]interface{}, importantFields []string) DetailView {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = loadingStyle

	sysID := getStringValue(record, "sys_id")

	return DetailView{
		mode:            detailLoading,
		client:          client,
		table:           table,
		record:          record,
		sysID:           sysID,
		importantFields: importantFields,
		dirty:           make(map[string]string),
		sp:              sp,
	}
}

// NewDetailViewForCreate creates a DetailView in "create" mode: a blank
// record seeded with empty values for the table's configured important
// fields (so they're immediately visible/editable), with no network fetch.
// Additional arbitrary fields can be added via the 'a' key before creating.
func NewDetailViewForCreate(client *api.Client, table string, importantFields []string) DetailView {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = loadingStyle

	record := make(map[string]interface{}, len(importantFields))
	for _, f := range importantFields {
		record[f] = ""
	}

	v := DetailView{
		mode:            detailReady,
		client:          client,
		table:           table,
		record:          record,
		importantFields: importantFields,
		isNew:           true,
		dirty:           make(map[string]string),
		sp:              sp,
	}
	v.buildFieldOrder()
	return v
}

func (v DetailView) Init() tea.Cmd {
	if v.isNew {
		return nil
	}
	// Load full record from API
	return tea.Batch(v.sp.Tick, v.fetchFull())
}

func (v DetailView) fetchFull() tea.Cmd {
	client := v.client
	table := v.table
	sysID := v.sysID
	existing := v.record
	return func() tea.Msg {
		if sysID == "" {
			// No sys_id → use existing record
			return recordsLoadedMsg{records: []map[string]interface{}{existing}, total: 1}
		}
		rec, err := client.GetRecord(table, sysID)
		if err != nil {
			// Fallback auf bereits geladenen Record
			return recordsLoadedMsg{records: []map[string]interface{}{existing}, total: 1}
		}
		return recordsLoadedMsg{records: []map[string]interface{}{rec}, total: 1}
	}
}

// IsNew reports whether this DetailView is creating a record rather than
// viewing/editing an existing one.
func (v DetailView) IsNew() bool { return v.isNew }

// recordLabel returns a human-readable identifier for the current record,
// used in confirm prompts.
func (v DetailView) recordLabel() string {
	if n := getStringValue(v.record, "number"); n != "" {
		return n
	}
	if sd := getStringValue(v.record, "short_description"); sd != "" {
		return sd
	}
	return v.sysID
}

// buildFieldOrder computes the navigable field list: configured important
// fields first (in their configured order), then every remaining field of
// the record, sorted alphabetically.
func (v *DetailView) buildFieldOrder() {
	seen := make(map[string]bool, len(v.record))
	order := make([]string, 0, len(v.record))
	for _, f := range v.importantFields {
		if _, ok := v.record[f]; ok && !seen[f] {
			order = append(order, f)
			seen[f] = true
		}
	}
	rest := make([]string, 0, len(v.record))
	for k := range v.record {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)
	v.fieldOrder = order
	if v.cursor >= len(order) {
		v.cursor = 0
	}
}

// fieldValue returns the current (possibly-dirty) display value for a field.
func (v DetailView) fieldValue(key string) (value string, isDirty bool) {
	if val, ok := v.dirty[key]; ok {
		return val, true
	}
	return getStringValue(v.record, key), false
}

func newFieldInput(placeholder, value string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 500
	ti.Prompt = "▶  "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(purple)
	ti.TextStyle = lipgloss.NewStyle().Foreground(white)
	if width > 0 {
		ti.Width = width
	}
	ti.SetValue(value)
	ti.CursorEnd()
	return ti
}

func (v DetailView) Update(msg tea.Msg) (DetailView, tea.Cmd) {
	switch msg := msg.(type) {
	case recordsLoadedMsg:
		if len(msg.records) > 0 {
			v.record = msg.records[0]
			v.sysID = getStringValue(v.record, "sys_id")
		}
		v.mode = detailReady
		v.buildFieldOrder()
		v.buildViewport()
		return v, nil

	case recordSavedMsg:
		v.record = msg.record
		if v.sysID == "" {
			v.sysID = getStringValue(msg.record, "sys_id")
		}
		v.isNew = false
		v.dirty = make(map[string]string)
		v.saveErr = nil
		v.cursor = 0
		v.pendingAction = actionNone
		v.buildFieldOrder()
		v.mode = detailReady
		v.refreshContent()
		return v, nil

	case mutationErrMsg:
		v.saveErr = msg.err
		v.pendingAction = actionNone
		v.mode = detailReady
		v.refreshContent()
		return v, nil

	case errMsg:
		v.err = msg.err
		v.mode = detailError
		return v, nil

	case spinner.TickMsg:
		if v.mode == detailLoading || v.mode == detailSaving {
			var cmd tea.Cmd
			v.sp, cmd = v.sp.Update(msg)
			return v, cmd
		}

	case tea.KeyMsg:
		switch v.mode {

		case detailReady:
			switch msg.String() {
			case "esc", "q":
				return v, func() tea.Msg { return goBackMsg{} }

			case "up", "k":
				if v.cursor > 0 {
					v.cursor--
					v.refreshContent()
				}
				return v, nil

			case "down", "j":
				if v.cursor < len(v.fieldOrder)-1 {
					v.cursor++
					v.refreshContent()
				}
				return v, nil

			case "enter":
				if v.cursor >= 0 && v.cursor < len(v.fieldOrder) {
					key := v.fieldOrder[v.cursor]
					current, _ := v.fieldValue(key)
					v.input = newFieldInput(key, current, v.inputWidth())
					v.mode = detailEditingField
					v.refreshContent()
					return v, v.input.Focus()
				}
				return v, nil

			case "a":
				if v.isNew {
					v.input = newFieldInput("field_name", "", v.inputWidth())
					v.mode = detailAddFieldName
					return v, v.input.Focus()
				}
				return v, nil

			case "ctrl+s":
				if v.isNew {
					v.pendingAction = actionCreate
					v.confirmPrompt = fmt.Sprintf("Create new %s record?", v.table)
					v.mode = detailConfirm
				} else if len(v.dirty) > 0 {
					v.pendingAction = actionSave
					v.confirmPrompt = fmt.Sprintf("Save changes to %s?", v.recordLabel())
					v.mode = detailConfirm
				}
				return v, nil

			case "d":
				if !v.isNew && v.sysID != "" {
					v.pendingAction = actionDelete
					v.confirmPrompt = fmt.Sprintf("Delete %s?", v.recordLabel())
					v.mode = detailConfirm
				}
				return v, nil
			}

		case detailEditingField:
			switch msg.String() {
			case "enter":
				key := v.fieldOrder[v.cursor]
				newVal := v.input.Value()
				if newVal == getStringValue(v.record, key) {
					delete(v.dirty, key)
				} else {
					v.dirty[key] = newVal
				}
				v.input.Blur()
				v.mode = detailReady
				v.refreshContent()
				return v, nil
			case "esc":
				v.input.Blur()
				v.mode = detailReady
				v.refreshContent()
				return v, nil
			}

		case detailAddFieldName:
			switch msg.String() {
			case "enter":
				name := strings.TrimSpace(v.input.Value())
				v.input.Blur()
				if name == "" {
					v.mode = detailReady
					v.refreshContent()
					return v, nil
				}
				for i, existing := range v.fieldOrder {
					if existing == name {
						// Field already present → just edit it instead of adding a duplicate.
						v.cursor = i
						v.mode = detailReady
						v.refreshContent()
						return v, nil
					}
				}
				v.pendingFieldName = name
				v.input = newFieldInput("value", "", v.inputWidth())
				v.mode = detailAddFieldValue
				return v, v.input.Focus()
			case "esc":
				v.input.Blur()
				v.mode = detailReady
				v.refreshContent()
				return v, nil
			}

		case detailAddFieldValue:
			switch msg.String() {
			case "enter":
				name := v.pendingFieldName
				v.record[name] = ""
				v.dirty[name] = v.input.Value()
				v.buildFieldOrder()
				for i, k := range v.fieldOrder {
					if k == name {
						v.cursor = i
						break
					}
				}
				v.pendingFieldName = ""
				v.input.Blur()
				v.mode = detailReady
				v.refreshContent()
				return v, nil
			case "esc":
				v.pendingFieldName = ""
				v.input.Blur()
				v.mode = detailReady
				v.refreshContent()
				return v, nil
			}

		case detailConfirm:
			switch msg.String() {
			case "y", "enter":
				v.mode = detailSaving
				return v, tea.Batch(v.sp.Tick, v.performAction())
			case "n", "esc":
				v.pendingAction = actionNone
				v.mode = detailReady
				return v, nil
			}

		case detailError:
			switch msg.String() {
			case "esc", "q":
				return v, func() tea.Msg { return goBackMsg{} }
			}
		}
	}

	var cmd tea.Cmd
	switch v.mode {
	case detailEditingField:
		v.input, cmd = v.input.Update(msg)
		v.refreshContent()
	case detailAddFieldName, detailAddFieldValue:
		v.input, cmd = v.input.Update(msg)
	case detailReady:
		v.vp, cmd = v.vp.Update(msg)
	}
	return v, cmd
}

// performAction fires the network request for the pending create/update/
// delete action and translates the result into a Bubble Tea message.
func (v DetailView) performAction() tea.Cmd {
	client := v.client
	table := v.table
	sysID := v.sysID
	action := v.pendingAction

	fields := make(map[string]interface{}, len(v.dirty))
	for k, val := range v.dirty {
		fields[k] = val
	}

	return func() tea.Msg {
		switch action {
		case actionCreate:
			rec, err := client.CreateRecord(table, fields)
			if err != nil {
				return mutationErrMsg{err: err}
			}
			return recordSavedMsg{record: rec}
		case actionSave:
			rec, err := client.UpdateRecord(table, sysID, fields)
			if err != nil {
				return mutationErrMsg{err: err}
			}
			return recordSavedMsg{record: rec}
		case actionDelete:
			if err := client.DeleteRecord(table, sysID); err != nil {
				return mutationErrMsg{err: err}
			}
			return recordDeletedMsg{sysID: sysID}
		default:
			return nil
		}
	}
}

// ── Views ─────────────────────────────────────────────────────────────────────

func (v DetailView) View() string {
	switch v.mode {
	case detailLoading:
		return "\n" + loadingStyle.Render(fmt.Sprintf("  %s  Loading full record…", v.sp.View()))

	case detailError:
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+errorStyle.Render("Error: "+v.err.Error()),
			"  "+helpStyle.Render(keyStyle.Render("esc")+" back"),
		)

	case detailSaving:
		label := "Saving…"
		switch v.pendingAction {
		case actionCreate:
			label = "Creating…"
		case actionDelete:
			label = "Deleting…"
		}
		return "\n" + loadingStyle.Render(fmt.Sprintf("  %s  %s", v.sp.View(), label))

	case detailConfirm:
		bar := "  " + warningStyle.Render(v.confirmPrompt) + "  " +
			helpStyle.Render(keyStyle.Render("y")+" yes   "+keyStyle.Render("n")+" no")
		return lipgloss.JoinVertical(lipgloss.Left, v.vp.View(), bar)

	case detailAddFieldName, detailAddFieldValue:
		title := "Add field — name"
		help := "Enter to continue   ·   Esc to cancel"
		if v.mode == detailAddFieldValue {
			title = fmt.Sprintf("Add field — value for %q", v.pendingFieldName)
			help = "Enter to add   ·   Esc to cancel"
		}
		content := lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(title),
			subtitleStyle.Render(help),
			"",
			v.input.View(),
		)
		box := boxStyle.Copy().Width(v.width / 2).Render(content)
		return lipgloss.Place(v.width, v.height, lipgloss.Center, lipgloss.Center, box)

	default:
		return v.vp.View()
	}
}

func (v *DetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
	if v.mode != detailLoading {
		v.buildViewport()
	}
}

// inputWidth returns a sensible textinput width given the current terminal
// width.
func (v DetailView) inputWidth() int {
	w := v.width - 40
	if w < 20 {
		w = 20
	}
	return w
}

// buildViewport (re)creates the viewport and fills it with freshly rendered
// content. Used on resize and on record load, where a clean scroll position
// is appropriate.
func (v *DetailView) buildViewport() {
	if v.width == 0 || v.height == 0 {
		return
	}
	v.vp = viewport.New(v.width, v.height)
	v.refreshContent()
}

// refreshContent re-renders field content into the existing viewport and
// scrolls just enough to keep the cursor's field visible.
func (v *DetailView) refreshContent() {
	if v.width == 0 || v.height == 0 {
		return
	}
	content, cursorLine, cursorTop := v.renderContent()
	v.vp.SetContent(content)
	if cursorLine < v.vp.YOffset {
		// Scroll up to cursorTop rather than cursorLine, so any header or
		// section divider directly above the cursor's field (which has no
		// other way back into view) is revealed along with it.
		v.vp.SetYOffset(cursorTop)
	} else if cursorLine >= v.vp.YOffset+v.vp.Height {
		v.vp.SetYOffset(cursorLine - v.vp.Height + 1)
	}
}

// renderContent renders the header and every navigable field, returning the
// full content, the line index at which the cursor's field starts (used to
// keep it scrolled into view from below), and the line index that an
// upward scroll should reveal (the field's own line, except for the very
// first field or the first field of a section, where it's the header/
// divider immediately above it).
func (v DetailView) renderContent() (string, int, int) {
	if v.record == nil {
		return errorStyle.Render("No record available."), 0, 0
	}

	keyW := 28
	valW := v.width - keyW - 6
	if valW < 20 {
		valW = 20
	}

	var lines []string
	cursorLine := 0
	cursorTop := 0

	if v.saveErr != nil {
		lines = append(lines, "  "+errorStyle.Render("Error: "+v.saveErr.Error()), "")
	}

	// ── Kopfzeile ─────────────────────────────────────────────
	if v.isNew {
		lines = append(lines, titleStyle.Render("New "+v.table+" record"), "")
	} else {
		number := getStringValue(v.record, "number")
		shortDesc := getStringValue(v.record, "short_description")
		if number != "" || shortDesc != "" {
			header := ""
			if number != "" {
				header = titleStyle.Render(number)
			}
			if shortDesc != "" {
				if header != "" {
					header += "   "
				}
				header += subtitleStyle.Render(shortDesc)
			}
			lines = append(lines, header, "")
		}
	}

	importantSet := make(map[string]bool, len(v.importantFields))
	for _, f := range v.importantFields {
		importantSet[f] = true
	}
	numImportant := 0
	for _, k := range v.fieldOrder {
		if importantSet[k] {
			numImportant++
		}
	}
	if numImportant > 0 {
		lines = append(lines, selectedItemStyle.Render("  Important Fields  "), "")
	}

	for i, key := range v.fieldOrder {
		dividerStart := -1
		if i == numImportant {
			dividerStart = len(lines)
			lines = append(lines, "", selectedItemStyle.Render("  All Fields  "), "")
		}
		if i == v.cursor {
			cursorLine = len(lines)
			switch {
			case i == 0:
				cursorTop = 0
			case dividerStart >= 0:
				cursorTop = dividerStart
			default:
				cursorTop = cursorLine
			}
		}
		lines = append(lines, v.renderFieldLines(i, key, keyW, valW)...)
	}

	return strings.Join(lines, "\n"), cursorLine, cursorTop
}

// renderFieldLines renders one field as one or more lines (wrapped for long
// values), marking the cursor and any unsaved edit.
func (v DetailView) renderFieldLines(idx int, key string, keyW, valW int) []string {
	selected := idx == v.cursor
	cursorMark := "  "
	if selected {
		cursorMark = "▸ "
	}

	if v.mode == detailEditingField && selected {
		keyStr := fieldKeyStyle.Copy().Width(keyW).Render(key + "*")
		return []string{cursorMark + keyStr + "  " + v.input.View()}
	}

	value, isDirty := v.fieldValue(key)
	empty := value == ""
	if empty {
		value = "(empty)"
	}

	label := key
	if isDirty {
		label += "*"
	}
	keyStr := fieldKeyStyle.Copy().Width(keyW).Render(label)

	wrapped := wrapString(value, valW)
	indent := strings.Repeat(" ", keyW+2)

	parts := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		var rendered string
		if empty {
			rendered = subtitleStyle.Render(line)
		} else {
			rendered = fieldValStyle.Render(line)
		}
		if i == 0 {
			parts = append(parts, cursorMark+keyStr+"  "+rendered)
		} else {
			parts = append(parts, "  "+indent+rendered)
		}
	}
	return parts
}

// wrapString bricht einen String auf maxLen Zeichen pro Zeile um.
func wrapString(s string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = 40
	}
	// Newlines im Originalwert respektieren
	rawLines := strings.Split(s, "\n")
	var result []string
	for _, raw := range rawLines {
		runes := []rune(raw)
		for len(runes) > maxLen {
			result = append(result, string(runes[:maxLen]))
			runes = runes[maxLen:]
		}
		result = append(result, string(runes))
	}
	return result
}
