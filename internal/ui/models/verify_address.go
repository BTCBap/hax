package models

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// AddressVerifyState is the outcome shown by an AddressVerifyView.
type AddressVerifyState int

const (
	AddressVerifyIdle AddressVerifyState = iota
	AddressVerifyWaiting
	AddressVerifyMatch
	AddressVerifyMismatch
	AddressVerifyError
)

const (
	addressVerifyTickInterval = 120 * time.Millisecond
	addressVerifyMaxWidth     = 80
	addressVerifyMinWidth     = 24
	addressVerifyLabelWidth   = 8
	addressVerifyLongestAddr  = 62
)

var addressVerifySpinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// AddressVerifyTickMsg drives the waiting spinner.
type AddressVerifyTickMsg struct{}

// AddressVerifyTickCmd schedules the next spinner tick.
func AddressVerifyTickCmd() tea.Cmd {
	return tea.Tick(addressVerifyTickInterval, func(time.Time) tea.Msg {
		return AddressVerifyTickMsg{}
	})
}

// AddressVerifyView renders the on-device address verification panel. It keeps
// the same number of rows in every active state so the surrounding screen does
// not shift while the device is checked. Idle renders nothing.
type AddressVerifyView struct {
	Address       string
	Path          string
	DeviceAddress string
	ErrMessage    string
	State         AddressVerifyState
	Width         int
	Frame         int
	StartedAt     time.Time

	attempt uint64
}

// Reset clears the result and points the panel at a new address.
func (v *AddressVerifyView) Reset(address, path string) {
	v.Address = sanitizeLine(address)
	v.Path = sanitizeLine(path)
	v.DeviceAddress = ""
	v.ErrMessage = ""
	v.State = AddressVerifyIdle
	v.Frame = 0
	v.StartedAt = time.Time{}
	v.attempt++
}

// Begin starts a device confirmation.
func (v *AddressVerifyView) Begin() {
	v.DeviceAddress = ""
	v.ErrMessage = ""
	v.State = AddressVerifyWaiting
	v.Frame = 0
	v.StartedAt = time.Now()
	v.attempt++
}

// Cancel leaves verification mode and discards the current result.
func (v *AddressVerifyView) Cancel() {
	v.DeviceAddress = ""
	v.ErrMessage = ""
	v.State = AddressVerifyIdle
	v.Frame = 0
	v.StartedAt = time.Time{}
	v.attempt++
}

// Complete records the device result.
func (v *AddressVerifyView) Complete(err error) {
	v.DeviceAddress = ""
	v.ErrMessage = ""
	if err == nil {
		v.State = AddressVerifyMatch
		return
	}

	var mismatch *wallet.AddressMismatchError
	if errors.As(err, &mismatch) {
		v.State = AddressVerifyMismatch
		v.DeviceAddress = sanitizeLine(mismatch.Displayed)
		return
	}

	v.State = AddressVerifyError
	v.ErrMessage = sanitizeLine(err.Error())
}

// Advance moves the waiting spinner forward one frame.
func (v *AddressVerifyView) Advance() {
	v.Frame++
}

// Active reports whether the panel is visible.
func (v AddressVerifyView) Active() bool {
	return v.State != AddressVerifyIdle
}

// IsWaiting reports whether a device confirmation is in progress.
func (v AddressVerifyView) IsWaiting() bool {
	return v.State == AddressVerifyWaiting
}

// Attempt identifies the current confirmation so stale device results can be
// discarded after a retry or after leaving verification mode.
func (v AddressVerifyView) Attempt() uint64 {
	return v.attempt
}

// Height is the number of terminal rows the panel occupies. Idle occupies none.
func (v AddressVerifyView) Height() int {
	if !v.Active() {
		return 0
	}
	return 2*v.reservedRows() + 8 // border, title, wallet/device blocks, status, hint
}

func (v AddressVerifyView) panelWidth() int {
	panelWidth := v.Width
	if panelWidth <= 0 {
		panelWidth = addressVerifyMaxWidth
	}
	if panelWidth > addressVerifyMaxWidth {
		panelWidth = addressVerifyMaxWidth
	}
	if panelWidth < addressVerifyMinWidth {
		panelWidth = addressVerifyMinWidth
	}
	return panelWidth
}

func (v AddressVerifyView) innerWidth() int {
	return v.panelWidth() - 4 // rounded border and one column of padding each side
}

func (v AddressVerifyView) addressRowWidth() int {
	width := v.innerWidth() - addressVerifyLabelWidth
	if width < 1 {
		width = 1
	}
	return width
}

// reservedRows is based on the longest supported address plus the two
// separator spaces so P2WPKH and P2WSH addresses render at the same height.
func (v AddressVerifyView) reservedRows() int {
	cells := addressVerifyLongestAddr + 2
	rows := (cells + v.addressRowWidth() - 1) / v.addressRowWidth()
	if rows < 1 {
		rows = 1
	}
	return rows
}

// View renders the fixed-height panel. Idle state renders nothing.
func (v AddressVerifyView) View() string {
	if !v.Active() {
		return ""
	}

	innerWidth := v.innerWidth()
	rows := v.reservedRows()

	lines := make([]string, 0, 2*rows+6)
	lines = append(lines, v.titleLine(innerWidth))
	lines = append(lines, padLine("", innerWidth))
	lines = append(lines, v.addressRows(v.Address, "Wallet", false)...)
	lines = append(lines, v.addressRows(v.DeviceAddress, "Device", v.State == AddressVerifyMismatch)...)
	lines = append(lines, padLine("", innerWidth))
	lines = append(lines, padLine(v.statusLine(innerWidth), innerWidth))
	// The screen's own hint row carries the retry/back keys; this row keeps
	// the panel's reserved height so the outer row does not move.
	lines = append(lines, padLine("", innerWidth))
	lines = append(lines, padLine("", innerWidth))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(v.borderColor()).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (v AddressVerifyView) titleLine(width int) string {
	const title = "Verify on device"
	titleWidth := lipgloss.Width(title)

	path := v.Path
	available := width - titleWidth - 1
	if available < 4 {
		path = ""
	} else {
		path = truncateRunes(path, available, "…")
	}

	line := styles.HighlightText.Render(title)
	if path == "" {
		return padLine(line, width)
	}
	gap := width - titleWidth - lipgloss.Width(path)
	if gap < 1 {
		gap = 1
	}
	return line + strings.Repeat(" ", gap) + styles.DimText.Render(path)
}

// addressRows renders an address with the shared format, wrapping it over the
// reserved rows. Device rows stay blank until a mismatch is available.
func (v AddressVerifyView) addressRows(address, label string, highlight bool) []string {
	rows := v.reservedRows()

	var addressLines []string
	if address != "" {
		cells := styledAddressCells(ui.FormatAddress(address, 0))
		if highlight {
			markAddressMismatches(cells, v.Address, address)
		}
		addressLines = wrapAddressCells(cells, v.addressRowWidth())
	}

	lines := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		prefix := strings.Repeat(" ", addressVerifyLabelWidth)
		if row == 0 {
			prefix = label + strings.Repeat(" ", max(0, addressVerifyLabelWidth-lipgloss.Width(label)))
		}

		content := ""
		if row < len(addressLines) {
			content = addressLines[row]
		}
		lines = append(lines, padLine(prefix+content, v.innerWidth()))
	}
	return lines
}

func (v AddressVerifyView) statusLine(width int) string {
	var text string
	style := styles.DimText
	switch v.State {
	case AddressVerifyWaiting:
		frame := addressVerifySpinner[v.Frame%len(addressVerifySpinner)]
		seconds := 0
		if !v.StartedAt.IsZero() {
			seconds = int(time.Since(v.StartedAt).Seconds())
			if seconds < 0 {
				seconds = 0
			}
		}
		text = fmt.Sprintf("%s Confirm on your device… %ds", frame, seconds)
		style = styles.NormalText
	case AddressVerifyMatch:
		text = "✓ Matches device"
		style = styles.SuccessText
	case AddressVerifyMismatch:
		text = "✗ Device shows a different address"
		style = styles.ErrorText
	case AddressVerifyError:
		text = v.ErrMessage
		style = styles.ErrorText
	}
	if lipgloss.Width(text) > width {
		text = truncateRunes(text, width, "…")
	}
	return style.Render(text)
}

func (v AddressVerifyView) borderColor() color.Color {
	switch v.State {
	case AddressVerifyWaiting:
		return styles.PastelOrange
	case AddressVerifyMatch:
		return styles.SuccessGreen
	case AddressVerifyMismatch, AddressVerifyError:
		return styles.ErrorRed
	default:
		return styles.DarkGray
	}
}

// addressCell is one visible character of a formatted address together with
// the SGR prefix that was active for it.
type addressCell struct {
	text  string
	style string
}

func (c addressCell) render() string {
	if c.style == "" {
		return c.text
	}
	return c.style + c.text + "\x1b[m"
}

// styledAddressCells splits a formatted address into visible cells so the
// panel can wrap it and recolor individual characters without losing styles.
func styledAddressCells(s string) []addressCell {
	var cells []addressCell
	var style strings.Builder

	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			end := i + 2
			for end < len(s) && (s[end] < 0x40 || s[end] > 0x7e) {
				end++
			}
			if end < len(s) {
				seq := s[i : end+1]
				if strings.HasSuffix(seq, "m") {
					if seq == "\x1b[0m" || seq == "\x1b[m" {
						style.Reset()
					} else {
						style.WriteString(seq)
					}
				}
				i = end + 1
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		cells = append(cells, addressCell{text: string(r), style: style.String()})
		i += size
	}
	return cells
}

// markAddressMismatches recolors every device character that differs from the
// wallet address at the same index.
func markAddressMismatches(cells []addressCell, wallet, device string) {
	errorStyle := sgrPrefix(styles.ErrorText)
	index := 0
	for i := range cells {
		if cells[i].text == " " || cells[i].text == "…" {
			continue
		}
		if index >= len(wallet) || index >= len(device) || device[index] != wallet[index] {
			cells[i].style = errorStyle
		}
		index++
	}
}

// wrapAddressCells breaks styled cells into lines of at most width visible
// cells, re-emitting the active style on each continuation line.
func wrapAddressCells(cells []addressCell, width int) []string {
	if width < 1 {
		width = 1
	}

	var lines []string
	var line strings.Builder
	lineWidth := 0
	for _, cell := range cells {
		cellWidth := lipgloss.Width(cell.text)
		if lineWidth > 0 && lineWidth+cellWidth > width {
			lines = append(lines, line.String())
			line.Reset()
			lineWidth = 0
		}
		line.WriteString(cell.render())
		lineWidth += cellWidth
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

// sgrPrefix returns the escape sequence a style writes before its text.
func sgrPrefix(style lipgloss.Style) string {
	rendered := style.Render("x")
	// Lip Gloss v1 resets with "\x1b[0m"; v2 uses "\x1b[m".
	rendered = strings.TrimSuffix(rendered, "\x1b[0m")
	rendered = strings.TrimSuffix(rendered, "\x1b[m")
	return strings.TrimSuffix(rendered, "x")
}

func padLine(line string, width int) string {
	if w := lipgloss.Width(line); w < width {
		return line + strings.Repeat(" ", width-w)
	}
	return line
}
