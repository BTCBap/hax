package models

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

const (
	// deviceStatusWidth keeps the status column the same size no matter how
	// long a device error is, so rows never reflow.
	deviceStatusWidth  = 32
	deviceNameMaxWidth = 32
	devicePinHelper    = "unlock device first"
)

// deviceMenuOption is a hardware device row with its already-styled status.
// The status is rendered for every row, not only the selected one.
type deviceMenuOption struct {
	name     string
	status   string
	needsPin bool
}

// renderDeviceMenu draws hardware devices with a fixed name column, a fixed
// status column and a helper column reserved for the selected row. The row
// width depends only on the device names, never on the selected status or the
// length of an error, so the menu cannot shift as the cursor moves.
func renderDeviceMenu(options []deviceMenuOption, cursor int) string {
	return renderDeviceMenuFit(options, cursor, 80)
}

// renderDeviceMenuFit is renderDeviceMenu constrained to maxWidth cells.
// A narrow frame shrinks the status column, keeping the selected row's helper
// and the "…" that marks a truncated error, so rows stay on one line.
func renderDeviceMenuFit(options []deviceMenuOption, cursor, maxWidth int) string {
	nameWidth := 0
	for _, opt := range options {
		nameWidth = max(nameWidth, lipgloss.Width(opt.name))
	}

	helperWidth := 0
	for _, opt := range options {
		if opt.needsPin {
			helperWidth = lipgloss.Width(devicePinHelper)
			break
		}
	}
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// Drop the helper column when it leaves no room for a status column.
	if 2+nameWidth+1+1+helperWidth > maxWidth {
		helperWidth = 0
	}
	statusWidth := min(deviceStatusWidth, max(0, maxWidth-(2+nameWidth+1+1+helperWidth)))

	var b strings.Builder
	for i, opt := range options {
		marker := "  "
		style := styles.UnselectedItemStyle
		if i == cursor {
			marker = "▶ "
			style = styles.SelectedItemStyle
		}

		name := opt.name + strings.Repeat(" ", nameWidth-lipgloss.Width(opt.name))
		statusText := opt.status
		if lipgloss.Width(statusText) > statusWidth {
			statusText = ansi.Truncate(statusText, statusWidth, "…")
		}
		status := statusText + strings.Repeat(" ", max(0, statusWidth-lipgloss.Width(statusText)))

		b.WriteString(style.Render(marker + name))
		b.WriteString(" " + status)
		if helperWidth > 0 {
			helper := ""
			if i == cursor && opt.needsPin {
				helper = styles.DimText.Render(devicePinHelper)
			}
			b.WriteString(" " + helper + strings.Repeat(" ", helperWidth-lipgloss.Width(helper)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// hardwareDeviceChoices turns detected devices into menu rows. Statuses are
// part of the row instead of helper text so every device's state stays
// visible while the cursor moves.
func hardwareDeviceChoices(devices []wallet.HardwareDevice) []deviceMenuOption {
	options := make([]deviceMenuOption, len(devices))
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

		var status string
		switch {
		case device.Error != "":
			errorText := strings.Join(strings.Fields(device.Error), " ")
			status = styles.ErrorText.Render(truncateDisplay(errorText, deviceStatusWidth))
		case device.NeedsPin && device.NeedsPass:
			status = styles.WarningText.Render("⚠ needs PIN") + " " +
				styles.DimText.Render("· (needs passphrase)")
		case device.NeedsPin:
			status = styles.WarningText.Render("⚠ needs PIN")
		case device.NeedsPass:
			status = styles.DimText.Render("(needs passphrase)")
		}

		options[i] = deviceMenuOption{
			name:     truncateDisplay(fmt.Sprintf("%s [%s]", name, fp), deviceNameMaxWidth),
			status:   status,
			needsPin: device.NeedsPin,
		}
	}
	return options
}

// truncateDisplay keeps s within width terminal cells and marks the cut with
// an ellipsis. ansi.Truncate measures cell width, so wide runes stay safe.
func truncateDisplay(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}
