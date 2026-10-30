package models

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
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

// hardwareDeviceChoices turns detected devices into menu rows. Device state
// that used to sit on its own row below the list becomes the selected row's
// helper text, so moving between devices never changes the line count.
func hardwareDeviceChoices(devices []wallet.HardwareDevice) []choiceOption {
	options := make([]choiceOption, len(devices))
	for i, device := range devices {
		name := string(device.Type)
		if device.Model != "" {
			name = device.Model
		}

		fp := device.Fingerprint
		switch {
		case fp == "" || fp == "00000000":
			fp = "unknown"
		case len(fp) > 8:
			fp = fp[:8]
		}

		var details []string
		if device.NeedsPin {
			details = append(details, "⚠ needs PIN — unlock device first")
		}
		if device.NeedsPass {
			details = append(details, "needs passphrase")
		}
		if device.Error != "" {
			details = append(details, strings.Join(strings.Fields(device.Error), " "))
		}

		options[i] = choiceOption{
			name: fmt.Sprintf("%s [%s]", name, fp),
			desc: strings.Join(details, " · "),
		}
	}
	return options
}
