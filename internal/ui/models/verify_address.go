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
// the same number of rows in every state so the surrounding screen does not
// shift while the device is checked.
type AddressVerifyView struct {
	Address       string
	Path          string
	DeviceAddress string
	ErrMessage    string
	State         AddressVerifyState
	Width         int
	Frame         int
	StartedAt     time.Time
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
}

// Begin starts a device confirmation.
func (v *AddressVerifyView) Begin() {
	v.DeviceAddress = ""
	v.ErrMessage = ""
	v.State = AddressVerifyWaiting
	v.Frame = 0
	v.StartedAt = time.Now()
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

// IsWaiting reports whether a device confirmation is in progress.
func (v AddressVerifyView) IsWaiting() bool {
	return v.State == AddressVerifyWaiting
}

// Height is the number of terminal rows the panel occupies.
func (v AddressVerifyView) Height() int {
	return 2 + 2*v.reservedRows() + 2 // border rows, address blocks, title and status
}

func (v AddressVerifyView) innerWidth() int {
	panelWidth := v.Width
	if panelWidth <= 0 {
		panelWidth = addressVerifyMaxWidth
	}
	if panelWidth > addressVerifyMaxWidth {
		panelWidth = addressVerifyMaxWidth
	}
	if panelWidth < 24 {
		panelWidth = 24
	}
	innerWidth := panelWidth - 4 // rounded border and one column of padding each side
	if innerWidth < 8 {
		innerWidth = 8
	}
	return innerWidth
}

func (v AddressVerifyView) reservedRows() int {
	reserved := len(wrapAddressChunks(v.Address, v.innerWidth()))
	if deviceRows := len(wrapAddressChunks(v.DeviceAddress, v.innerWidth())); deviceRows > reserved {
		reserved = deviceRows
	}
	if reserved == 0 {
		reserved = 1
	}
	return reserved
}

// View renders the fixed-height panel.
func (v AddressVerifyView) View() string {
	innerWidth := v.innerWidth()
	expectedLines := wrapAddressChunks(v.Address, innerWidth)
	deviceLines := wrapAddressChunks(v.DeviceAddress, innerWidth)
	reserved := v.reservedRows()

	lines := make([]string, 0, 2+2*reserved)
	lines = append(lines, v.titleLine(innerWidth))
	for i := 0; i < reserved; i++ {
		if i < len(expectedLines) {
			lines = append(lines, styles.NormalText.Render(expectedLines[i]))
		} else {
			lines = append(lines, "")
		}
	}
	deviceGroup := 0
	for i := 0; i < reserved; i++ {
		if v.State == AddressVerifyMismatch && i < len(deviceLines) {
			lines = append(lines, v.deviceLine(deviceLines[i], deviceGroup))
			deviceGroup += len(strings.Split(deviceLines[i], " "))
		} else {
			lines = append(lines, "")
		}
	}
	lines = append(lines, v.statusLine(innerWidth))

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
		return line + strings.Repeat(" ", width-titleWidth)
	}
	gap := width - titleWidth - lipgloss.Width(path)
	if gap < 1 {
		gap = 1
	}
	return line + strings.Repeat(" ", gap) + styles.DimText.Render(path)
}

func (v AddressVerifyView) deviceLine(line string, offset int) string {
	expected := chunkAddress(v.Address)
	parts := strings.Split(line, " ")
	rendered := make([]string, len(parts))
	for i, group := range parts {
		groupIndex := offset + i
		if groupIndex >= len(expected) || group != expected[groupIndex] {
			rendered[i] = styles.ErrorText.Render(group)
		} else {
			rendered[i] = styles.NormalText.Render(group)
		}
	}
	return strings.Join(rendered, " ")
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
	default:
		text = "v confirm on device"
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
	groups := make([]string, 0, (len(runes)+3)/4)
	for len(runes) > 0 {
		n := 4
		if len(runes) < n {
			n = len(runes)
		}
		groups = append(groups, string(runes[:n]))
		runes = runes[n:]
	}
	return groups
}

// wrapAddressChunks groups the address by four and wraps the groups to width.
func wrapAddressChunks(address string, width int) []string {
	if width < 1 {
		width = 1
	}

	var lines []string
	var current strings.Builder
	currentWidth := 0
	for _, group := range chunkAddress(address) {
		groupWidth := lipgloss.Width(group)
		if currentWidth == 0 {
			current.WriteString(group)
			currentWidth = groupWidth
			continue
		}
		if currentWidth+1+groupWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(group)
			currentWidth = groupWidth
			continue
		}
		current.WriteString(" ")
		current.WriteString(group)
		currentWidth += 1 + groupWidth
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}
