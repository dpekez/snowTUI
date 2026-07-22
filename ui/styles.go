package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const appName = "ServiceNow TUI"

// Colors
var (
	purple     = lipgloss.Color("#7C3AED")
	darkPurple = lipgloss.Color("#4C1D95")
	green      = lipgloss.Color("#10B981")
	darkGreen  = lipgloss.Color("#06402B")
	red        = lipgloss.Color("#EF4444")
	yellow     = lipgloss.Color("#F59E0B")
	gray       = lipgloss.Color("#6B7280")
	lightGray  = lipgloss.Color("#D1D5DB")
	white      = lipgloss.Color("#F9FAFB")
	surfaceBg  = lipgloss.Color("#1F2937")
	borderClr  = lipgloss.Color("#374151")
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(white).
			Background(green).
			Padding(0, 2)

	breadcrumbStyle = lipgloss.NewStyle().
			Foreground(lightGray).
			Background(darkGreen).
			Padding(0, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(green)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(gray)

	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(white).
				Background(green).
				Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(red)

	successStyle = lipgloss.NewStyle().
			Foreground(green)

	warningStyle = lipgloss.NewStyle().
			Foreground(yellow)

	helpStyle = lipgloss.NewStyle().
			Foreground(gray).
			Background(surfaceBg).
			Padding(0, 1)

	keyStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderClr).
			Padding(1, 2)

	loadingStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	fieldKeyStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	fieldValStyle = lipgloss.NewStyle().
			Foreground(lightGray)
)

// renderHelp rendert die Hilfszeile unten mit Tastenkombinationen.
func renderHelp(keys [][2]string) string {
	parts := make([]string, 0, len(keys))
	for _, kv := range keys {
		parts = append(parts,
			keyStyle.Render(kv[0])+" "+
				lipgloss.NewStyle().Foreground(gray).Render(kv[1]),
		)
	}
	return strings.Join(parts, "  ")
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// getStringValue extrahiert den Anzeigewert aus einem ServiceNow-Feld.
// ServiceNow gibt mit sysparm_display_value=true Felder als plain string
// oder als Objekt {"display_value": "...", "value": "..."} zurück.
func getStringValue(record map[string]interface{}, key string) string {
	val, ok := record[key]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		if dv, ok := v["display_value"]; ok {
			if s, ok := dv.(string); ok {
				return s
			}
		}
		if value, ok := v["value"]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	}
	return fmt.Sprintf("%v", val)
}
