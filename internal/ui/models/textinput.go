package models

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/ui/styles"
)

// textInputStyles returns the palette-aware text input styles. Bubbles v2
// changed the defaults (colored prompt and cursor), so the styles are set
// explicitly here to keep the rendered UI unchanged.
func textInputStyles() textinput.Styles {
	plain := lipgloss.NewStyle()
	placeholder := lipgloss.NewStyle().Foreground(styles.Placeholder)
	state := textinput.StyleState{
		Text:        plain,
		Placeholder: placeholder,
		Suggestion:  placeholder,
		Prompt:      plain,
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor: textinput.CursorStyle{
			Color: nil,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	}
}

func newTextInput() textinput.Model {
	ti := textinput.New()
	ti.SetStyles(textInputStyles())
	return ti
}
