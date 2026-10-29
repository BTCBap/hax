package models

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/BTCBap/hax/internal/ui/styles"
)

type choiceOption struct {
	name  string
	desc  string
	check string // appended to the name while this row is highlighted
}

// renderChoiceMenu lays out a menu with a fixed width so that centering it
// does not shift it when the cursor moves. Only the highlighted row shows its
// description, to the right of the name; the other rows are padded in place.
func renderChoiceMenu(options []choiceOption, cursor int) string {
	nameWidth, descWidth := 0, 0
	for _, opt := range options {
		nameWidth = max(nameWidth, lipgloss.Width(opt.name)+lipgloss.Width(opt.check))
		descWidth = max(descWidth, lipgloss.Width(opt.desc))
	}

	var b strings.Builder
	for i, opt := range options {
		marker := "  "
		style := styles.UnselectedItemStyle
		name := opt.name + strings.Repeat(" ", nameWidth-lipgloss.Width(opt.name))
		desc := ""
		if i == cursor {
			marker = "▶ "
			style = styles.SelectedItemStyle
			padding := nameWidth - lipgloss.Width(opt.name) - lipgloss.Width(opt.check)
			name = opt.name + opt.check + strings.Repeat(" ", padding)
			desc = opt.desc
		}
		b.WriteString(style.Render(marker + name))
		if descWidth > 0 {
			descPadding := strings.Repeat(" ", descWidth-lipgloss.Width(desc))
			b.WriteString(styles.DimText.Render("  " + desc + descPadding))
		}
		b.WriteString("\n")
	}
	return b.String()
}
