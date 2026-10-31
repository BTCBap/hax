package models

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

func TestGetAddressVisualState(t *testing.T) {
	tests := []struct {
		name     string
		addr     AddrDisplayInfo
		expected addressVisualState
	}{
		{
			name:     "unused address is standard",
			addr:     AddrDisplayInfo{Used: false, Balance: 0},
			expected: addressVisualUnused,
		},
		{
			name:     "used with zero balance is dim",
			addr:     AddrDisplayInfo{Used: true, Balance: 0},
			expected: addressVisualUsedNoBalance,
		},
		{
			name:     "used with balance is highlighted",
			addr:     AddrDisplayInfo{Used: true, Balance: api.Satoshi(1)},
			expected: addressVisualUsedWithBalance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAddressVisualState(tt.addr)
			if got != tt.expected {
				t.Fatalf("getAddressVisualState() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAddresses_VerifyOnDeviceMismatch(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsMultisigFn = func() bool { return false }
	mock.DisplayAddrOnDeviceFn = func(addr, path string) error {
		if addr != verifyTestAddress || path != verifyTestPath {
			t.Fatalf("device called with %q %q", addr, path)
		}
		return &wallet.AddressMismatchError{Displayed: verifyTestDevice, Expected: addr}
	}

	m := NewAddressesModel(mock, nil)
	m.width = 80
	m.height = 24
	m.receiveAddresses = []AddrDisplayInfo{{
		Address: verifyTestAddress,
		Path:    verifyTestPath,
		Index:   0,
	}}
	m.updateTable()

	model, cmd := m.Update(keyRunes("v"))
	waiting := model.(AddressesModel)
	if !waiting.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}
	if waiting.verify.Address != verifyTestAddress || waiting.verify.Path != verifyTestPath {
		t.Fatalf("verifying %q at %q", waiting.verify.Address, waiting.verify.Path)
	}

	result, ok := findMsgOfType[HWIVerifyMsg](collectMsgs(cmd))
	if !ok {
		t.Fatal("verification command did not produce HWIVerifyMsg")
	}
	model, _ = waiting.Update(result)
	updated := model.(AddressesModel)
	if updated.verify.State != AddressVerifyMismatch {
		t.Fatalf("state = %d, want mismatch", updated.verify.State)
	}
	if !strings.Contains(stripANSI(updated.View()), "✗ Device shows a different address") {
		t.Fatalf("view does not show mismatch:\n%s", stripANSI(updated.View()))
	}
}

func newHardwareAddresses(t *testing.T, width, height int) (AddressesModel, *MockWalletManager) {
	t.Helper()
	mock := NewMockWalletManager()
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsMultisigFn = func() bool { return false }

	m := NewAddressesModel(mock, nil)
	m.receiveAddresses = []AddrDisplayInfo{{
		Address: verifyTestAddress,
		Path:    verifyTestPath,
		Index:   0,
	}}
	m.updateTable()
	model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model.(AddressesModel), mock
}

func TestAddresses_VerifyModeReplacesDetailAndRestores(t *testing.T) {
	m, _ := newHardwareAddresses(t, 80, 24)
	before := m.View()
	if strings.Contains(stripANSI(before), "Verify on device") {
		t.Fatalf("idle addresses view already shows the panel:\n%s", stripANSI(before))
	}
	if !strings.Contains(stripANSI(before), "UNUSED") {
		t.Fatalf("idle addresses view is missing the list:\n%s", stripANSI(before))
	}

	model, _ := m.Update(keyRunes("v"))
	m = model.(AddressesModel)
	if !m.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Verify on device") {
		t.Fatalf("verify mode did not replace the list detail area:\n%s", plain)
	}
	if strings.Contains(plain, "UNUSED") {
		t.Fatalf("verify mode still shows the address list:\n%s", plain)
	}
	if strings.Contains(plain, "No label") {
		t.Fatalf("verify mode still shows the selected address label line:\n%s", plain)
	}
	assertWithinTerminal(t, m.View(), 80, 24)

	model, _ = m.Update(keyCode(tea.KeyEsc))
	restored := model.(AddressesModel)
	if restored.verify.Active() {
		t.Fatal("esc should leave verify mode")
	}
	if got := restored.View(); got != before {
		t.Fatalf("esc did not restore the addresses view exactly:\nbefore:\n%q\nafter:\n%q", before, got)
	}
}

func TestAddresses_VerifyModeLineCountStable(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}} {
		t.Run(size.name, func(t *testing.T) {
			m, _ := newHardwareAddresses(t, size.w, size.h)

			states := []AddressVerifyState{
				AddressVerifyWaiting,
				AddressVerifyMatch,
				AddressVerifyMismatch,
				AddressVerifyError,
			}
			var wantLines int
			for i, state := range states {
				m.verify.Reset(verifyTestAddress, verifyTestPath)
				m.verify.Begin()
				switch state {
				case AddressVerifyWaiting:
					m.verify.StartedAt = time.Now().Add(-2 * time.Second)
				case AddressVerifyMatch:
					m.verify.Complete(nil)
				case AddressVerifyMismatch:
					m.verify.Complete(&wallet.AddressMismatchError{
						Displayed: verifyTestDevice,
						Expected:  verifyTestAddress,
					})
				case AddressVerifyError:
					m.verify.Complete(errors.New("device is in mainnet mode"))
				}
				if m.verify.State != state {
					t.Fatalf("state = %d, want %d", m.verify.State, state)
				}

				view := m.View()
				lines := requireLineCount(t, view)
				if i == 0 {
					wantLines = lines
				}
				if lines != wantLines {
					t.Fatalf("state %d rendered %d lines, want %d", state, lines, wantLines)
				}
				assertWithinTerminal(t, view, size.w, size.h)
			}
		})
	}
}

// The verification panel no longer draws its own hint line; the screen footer
// carries the retry/back keys on the same row in every panel state.
func TestAddresses_VerifyModeOuterHintRow(t *testing.T) {
	m, _ := newHardwareAddresses(t, 80, 24)
	model, _ := m.Update(keyRunes("v"))
	m = model.(AddressesModel)
	if !m.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}

	states := []struct {
		state AddressVerifyState
		err   error
	}{
		{AddressVerifyWaiting, nil},
		{AddressVerifyMatch, nil},
		{AddressVerifyMismatch, &wallet.AddressMismatchError{Displayed: verifyTestDevice, Expected: verifyTestAddress}},
		{AddressVerifyError, errors.New("device is in mainnet mode")},
	}

	var wantLine int
	for i, tc := range states {
		m.verify.Reset(verifyTestAddress, verifyTestPath)
		m.verify.Begin()
		if tc.state != AddressVerifyWaiting {
			m.verify.Complete(tc.err)
		}

		view := m.View()
		plain := stripANSI(view)
		if got := strings.Count(plain, "v retry • esc back"); got != 1 {
			t.Fatalf("state %d: hint appears %d times, want once:\n%s", tc.state, got, plain)
		}
		if strings.Contains(plain, "↑/↓ navigate") || strings.Contains(plain, "v verify on device") {
			t.Fatalf("state %d: list hints still shown in verify mode:\n%s", tc.state, plain)
		}

		pos := requireTextPosition(t, view, "v retry • esc back")
		if i == 0 {
			wantLine = pos.line
		} else if pos.line != wantLine {
			t.Fatalf("state %d: outer hint moved from line %d to %d", tc.state, wantLine, pos.line)
		}
	}
}

func TestAddressTableAddressFormatting(t *testing.T) {
	const address = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7"
	m := NewAddressesModel(NewMockWalletManager(), nil)
	m.width = 110
	m.resizeColumnsForWidth()
	addrs := []AddrDisplayInfo{{Address: address, Used: true, Balance: api.Satoshi(1000)}}
	m.receiveAddresses = addrs
	m.updateTable()

	addrWidth := m.table.Columns()[1].Width
	plain := ansi.Strip(ui.FormatAddress(address, addrWidth))
	m.loading = false
	m.updateTable()
	view := m.View()
	if !strings.Contains(ansi.Strip(view), plain) {
		t.Fatalf("address table does not show the formatted address %q:\n%s", plain, ansi.Strip(view))
	}
	if !strings.Contains(view, ui.FormatAddress(address, addrWidth)) {
		t.Fatalf("address table lost the address styling: %q", view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Errorf("line %d is %d cells wide, max %d", i, w, m.width)
		}
	}

	row := " 0     " + plain + "   USED"

	got := m.colorizeAddressTable(row, addrs)
	if stripped := ansi.Strip(got); stripped != row {
		t.Fatalf("colorized table text = %q, want %q", stripped, row)
	}
	if !strings.Contains(got, ui.FormatAddress(address, addrWidth)) {
		t.Fatalf("table row does not carry the formatted address: %q", got)
	}
	if gotW, wantW := ansi.StringWidth(got), ansi.StringWidth(row); gotW != wantW {
		t.Fatalf("colorized row width = %d, want %d", gotW, wantW)
	}

	selected := styles.NewTableStyles().Selected.Render(row)
	got = m.colorizeAddressTable(selected, addrs)
	if stripped := ansi.Strip(got); stripped != row {
		t.Fatalf("selected table text = %q, want %q", stripped, row)
	}
	if gotW, wantW := ansi.StringWidth(got), ansi.StringWidth(selected); gotW != wantW {
		t.Fatalf("selected row width = %d, want %d", gotW, wantW)
	}
	if !strings.Contains(got, ui.FormatAddress(address, addrWidth)+sgrPrefix(styles.NewTableStyles().Selected)) {
		t.Fatalf("selected row did not restore its highlight after the address: %q", got)
	}
}
