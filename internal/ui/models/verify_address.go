package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	addressVerifyMaxGroups    = 8
	addressVerifyLongestAddr  = 62
	addressVerifyGroupSize    = 4
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

// groupsPerRow keeps the historical eight groups per row when there is room
// and drops to fewer groups on narrow panels.
func (v AddressVerifyView) groupsPerRow() int {
	available := v.innerWidth() - addressVerifyLabelWidth
	if available < addressVerifyGroupSize {
		available = addressVerifyGroupSize
	}
	perRow := (available + 1) / (addressVerifyGroupSize + 1)
	if perRow > addressVerifyMaxGroups {
		perRow = addressVerifyMaxGroups
	}
	if perRow < 1 {
		perRow = 1
	}
	return perRow
}

// reservedRows is based on the longest supported address so P2WPKH and P2WSH
// addresses render at the same height.
func (v AddressVerifyView) reservedRows() int {
	groups := (addressVerifyLongestAddr + addressVerifyGroupSize - 1) / addressVerifyGroupSize
	perRow := v.groupsPerRow()
	rows := (groups + perRow - 1) / perRow
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

// addressRows renders an address in fixed-size groups, using only the reserved
// number of rows. Device rows stay blank until a mismatch is available.
func (v AddressVerifyView) addressRows(address, label string, highlight bool) []string {
	rows := v.reservedRows()
	perRow := v.groupsPerRow()
	groups := chunkAddress(address)
	expected := chunkAddress(v.Address)

	lines := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		prefix := strings.Repeat(" ", addressVerifyLabelWidth)
		if row == 0 {
			prefix = label + strings.Repeat(" ", max(0, addressVerifyLabelWidth-lipgloss.Width(label)))
		}

		var line strings.Builder
		line.WriteString(prefix)

		start := row * perRow
		end := min(start+perRow, len(groups))
		for i := start; i < end; i++ {
			if i > start {
				line.WriteString(" ")
			}
			group := groups[i]
			if highlight && (i >= len(expected) || group != expected[i]) {
				line.WriteString(styles.ErrorText.Render(group))
			} else {
				line.WriteString(styles.NormalText.Render(group))
			}
		}
		lines = append(lines, padLine(line.String(), v.innerWidth()))
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

func (v AddressVerifyView) borderColor() lipgloss.Color {
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

// chunkAddress splits an address into groups of four characters.
func chunkAddress(address string) []string {
	runes := []rune(address)
	groups := make([]string, 0, (len(runes)+addressVerifyGroupSize-1)/addressVerifyGroupSize)
	for len(runes) > 0 {
		n := addressVerifyGroupSize
		if len(runes) < n {
			n = len(runes)
		}
		groups = append(groups, string(runes[:n]))
		runes = runes[n:]
	}
	return groups
}

// chunkAddressRows groups an address by four and wraps after perRow groups.
func chunkAddressRows(address string, perRow int) []string {
	if perRow < 1 {
		perRow = 1
	}

	groups := chunkAddress(address)
	lines := make([]string, 0, (len(groups)+perRow-1)/perRow)
	for start := 0; start < len(groups); start += perRow {
		end := min(start+perRow, len(groups))
		lines = append(lines, strings.Join(groups[start:end], " "))
	}
	return lines
}

func padLine(line string, width int) string {
	if w := lipgloss.Width(line); w < width {
		return line + strings.Repeat(" ", width-w)
	}
	return line
}
