package models

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// newTextInput returns a text input configured with the Bubbles v1 default
// styles. Bubbles v2 changed the defaults (colored prompt and cursor), so the
// styles are set explicitly here to keep the rendered UI unchanged.
func newTextInput() textinput.Model {
	ti := textinput.New()
	plain := lipgloss.NewStyle()
	placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	state := textinput.StyleState{
		Text:        plain,
		Placeholder: placeholder,
		Suggestion:  placeholder,
		Prompt:      plain,
	}
	ti.SetStyles(textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor: textinput.CursorStyle{
			Color: nil,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	})
	return ti
}
