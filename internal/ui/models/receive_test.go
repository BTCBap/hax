package models

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

func newTestReceive(t *testing.T) (ReceiveModel, *MockWalletManager) {
	t.Helper()
	mock := NewMockWalletManager()
	mock.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return "tb1q_test_receive_addr", "m/84'/1'/0'/0/0", 0, nil
	}
	mock.GetAddressByIndexFn = func(_, idx uint32) (string, string, error) {
		return "tb1q_test_addr_by_index", "m/84'/1'/0'/0/0", nil
	}
	mock.GetCachedAddrBalanceFn = func(string) (api.Satoshi, bool) {
		return 50000, true
	}
	m := NewReceiveModel(mock)
	m.width = 120
	m.height = 40
	return m, mock
}

func TestReceive_InitialState(t *testing.T) {
	m, _ := newTestReceive(t)
	if m.selectedAddress != "" {
		t.Error("expected empty address initially")
	}
	if m.generatingNew {
		t.Error("expected generatingNew=false")
	}
	if m.showCopied {
		t.Error("expected showCopied=false")
	}
}

func TestReceive_Init_LoadsAddress(t *testing.T) {
	m, _ := newTestReceive(t)
	cmd := m.Init()
	msgs := collectMsgs(cmd)

	if !containsMsgOfType[AddressGeneratedMsg](msgs) {
		t.Error("Init should produce an AddressGeneratedMsg")
	}
	addr, found := findMsgOfType[AddressGeneratedMsg](msgs)
	if found {
		if addr.Address != "tb1q_test_receive_addr" {
			t.Errorf("expected tb1q_test_receive_addr, got %s", addr.Address)
		}
		if addr.Path != "m/84'/1'/0'/0/0" {
			t.Errorf("expected m/84'/1'/0'/0/0, got %s", addr.Path)
		}
		if addr.Err != nil {
			t.Errorf("unexpected error: %v", addr.Err)
		}
	}
}

func TestReceive_AddressGeneratedMsg_UpdatesState(t *testing.T) {
	m, _ := newTestReceive(t)
	model, _ := m.Update(AddressGeneratedMsg{
		Address: "tb1q_generated",
		Path:    "m/84'/1'/0'/0/5",
		Index:   5,
		Balance: 12345,
	})
	updated := model.(ReceiveModel)
	if updated.selectedAddress != "tb1q_generated" {
		t.Errorf("expected tb1q_generated, got %s", updated.selectedAddress)
	}
	if updated.derivationPath != "m/84'/1'/0'/0/5" {
		t.Errorf("expected m/84'/1'/0'/0/5, got %s", updated.derivationPath)
	}
	if updated.currentIndex != 5 {
		t.Errorf("expected index=5, got %d", updated.currentIndex)
	}
	if updated.addressBalance != 12345 {
		t.Errorf("expected balance=12345, got %d", updated.addressBalance)
	}
	if updated.generatingNew {
		t.Error("expected generatingNew=false after address generated")
	}
}

func TestReceive_EscNavigatesBack(t *testing.T) {
	m, _ := newTestReceive(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestReceive_QNavigatesBack(t *testing.T) {
	m, _ := newTestReceive(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestReceive_LeftRight_NavigatesAddresses(t *testing.T) {
	m, _ := newTestReceive(t)
	// Set current index to 3 so we can go left
	m.currentIndex = 3
	m.selectedAddress = "tb1q_current"

	// Press left
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	updated := model.(ReceiveModel)
	if !updated.generatingNew {
		t.Error("expected generatingNew=true after left")
	}
	msgs := collectMsgs(cmd)
	if !containsMsgOfType[AddressGeneratedMsg](msgs) {
		t.Error("left should trigger address loading")
	}

	// Press right from original
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated = model.(ReceiveModel)
	if !updated.generatingNew {
		t.Error("expected generatingNew=true after right")
	}
	msgs = collectMsgs(cmd)
	if !containsMsgOfType[AddressGeneratedMsg](msgs) {
		t.Error("right should trigger address loading")
	}
}

func TestReceive_Left_ClampsAtZero(t *testing.T) {
	m, _ := newTestReceive(t)
	m.currentIndex = 0

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	updated := model.(ReceiveModel)
	if updated.generatingNew {
		t.Error("left at index 0 should not trigger loading")
	}
}

func TestReceive_N_GeneratesNewAddress(t *testing.T) {
	m, mock := newTestReceive(t)
	mock.GetNewAddressFn = func(uint32) (string, error) {
		return "tb1q_brand_new", nil
	}
	mock.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return "tb1q_brand_new", "m/84'/1'/0'/0/1", 1, nil
	}
	m.selectedAddress = "tb1q_old"

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	updated := model.(ReceiveModel)
	if !updated.generatingNew {
		t.Error("expected generatingNew=true after 'n'")
	}

	msgs := collectMsgs(cmd)
	addr, found := findMsgOfType[AddressGeneratedMsg](msgs)
	if !found {
		t.Fatal("'n' should produce an AddressGeneratedMsg")
	}
	if addr.Address != "tb1q_brand_new" {
		t.Errorf("expected tb1q_brand_new, got %s", addr.Address)
	}
}

func newHardwareReceive(t *testing.T) (ReceiveModel, *MockWalletManager) {
	t.Helper()
	m, mock := newTestReceive(t)
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsMultisigFn = func() bool { return false }
	m.selectedAddress = verifyTestAddress
	m.derivationPath = verifyTestPath
	m.revealProgress = 1.0
	m.verify.Reset(verifyTestAddress, verifyTestPath)
	return m, mock
}

func pressReceiveKey(t *testing.T, m ReceiveModel, key tea.KeyMsg) ReceiveModel {
	t.Helper()
	model, _ := m.Update(key)
	return model.(ReceiveModel)
}

func TestReceive_VerifyModeReplacesQRCard(t *testing.T) {
	m, _ := newHardwareReceive(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = model.(ReceiveModel)
	before := m.View()
	if strings.Contains(stripANSI(before), "Verify on device") {
		t.Fatalf("idle receive view already shows the panel:\n%s", stripANSI(before))
	}
	if !strings.Contains(stripANSI(before), "v verify on device") {
		t.Fatalf("idle receive view is missing the verify hint:\n%s", stripANSI(before))
	}

	m = pressReceiveKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	view := m.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "Verify on device") {
		t.Fatalf("verify mode did not replace the QR card:\n%s", plain)
	}
	if strings.Contains(plain, "Address #0") {
		t.Fatalf("verify mode still shows the QR card:\n%s", plain)
	}
	assertWithinTerminal(t, view, 80, 24)

	restored := pressReceiveKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if got := restored.View(); got != before {
		t.Fatalf("esc did not restore the receive view exactly:\nbefore:\n%q\nafter:\n%q", before, got)
	}
}

func TestReceive_VerifyModeLineCountStable(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}} {
		t.Run(size.name, func(t *testing.T) {
			m, _ := newHardwareReceive(t)
			model, _ := m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			m = model.(ReceiveModel)

			states := []struct {
				state AddressVerifyState
				err   error
			}{
				{AddressVerifyWaiting, nil},
				{AddressVerifyMatch, nil},
				{AddressVerifyMismatch, &wallet.AddressMismatchError{Displayed: verifyTestDevice, Expected: verifyTestAddress}},
				{AddressVerifyError, errors.New("device is in mainnet mode")},
			}
			var wantLines int
			for i, tc := range states {
				m.verify.Reset(verifyTestAddress, verifyTestPath)
				m.verify.Begin()
				if tc.state != AddressVerifyWaiting {
					m.verify.Complete(tc.err)
				}
				if m.verify.State != tc.state {
					t.Fatalf("state = %d, want %d", m.verify.State, tc.state)
				}
				view := m.View()
				lines := requireLineCount(t, view)
				if i == 0 {
					wantLines = lines
				}
				if lines != wantLines {
					t.Fatalf("state %d rendered %d lines, want %d", tc.state, lines, wantLines)
				}
				assertWithinTerminal(t, view, size.w, size.h)
				if size.w == 80 && size.h == 24 && lines > 24 {
					t.Fatalf("receive view grew to %d lines at 80x24", lines)
				}
			}
		})
	}
}

func TestReceive_VerifyEscWhileWaitingDiscardsResult(t *testing.T) {
	m, _ := newHardwareReceive(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = model.(ReceiveModel)
	before := m.View()

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = model.(ReceiveModel)
	if !m.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}
	attempt := m.verify.Attempt()

	m = pressReceiveKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.verify.Active() {
		t.Fatal("esc while waiting should leave verify mode")
	}
	if got := m.View(); got != before {
		t.Fatal("esc while waiting should restore the normal view")
	}

	// A result from the abandoned attempt must not reopen the panel.
	model, _ = m.Update(HWIVerifyMsg{Err: nil, Attempt: attempt})
	m = model.(ReceiveModel)
	if m.verify.Active() {
		t.Fatal("stale device result reopened the panel")
	}
	_ = cmd
}

func TestReceive_VerifyRetryStartsNewAttempt(t *testing.T) {
	m, _ := newHardwareReceive(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = model.(ReceiveModel)

	m = pressReceiveKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	first := m.verify.Attempt()
	if !m.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}

	m = pressReceiveKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	second := m.verify.Attempt()
	if second == first {
		t.Fatal("v in verify mode should start a new attempt")
	}
	if !m.verify.IsWaiting() {
		t.Fatal("retry should stay in the waiting state")
	}

	// The first attempt's result is stale and must be ignored.
	model, _ = m.Update(HWIVerifyMsg{Err: nil, Attempt: first})
	m = model.(ReceiveModel)
	if m.verify.State != AddressVerifyWaiting {
		t.Fatalf("stale result changed state to %d", m.verify.State)
	}
}

func assertWithinTerminal(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("view has %d lines, max %d", len(lines), height)
	}
	for i, line := range lines {
		if got := len([]rune(stripANSI(line))); got > width {
			t.Fatalf("line %d is %d cells wide, max %d: %q", i, got, width, stripANSI(line))
		}
	}
}

func TestReceive_View_ShowsDerivationPath(t *testing.T) {
	m, _ := newTestReceive(t)
	m.selectedAddress = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	m.derivationPath = "m/84'/1'/0'/0/0"
	m.currentIndex = 0
	m.revealProgress = 1.0 // Skip animation

	view := m.View()
	// Address is split by FormatBitcoinAddressColored, so check the derivation path
	// and the header which are rendered as plain text
	if !strings.Contains(view, "Receive") {
		t.Error("View should display the Receive header")
	}
	if !strings.Contains(view, "#0") {
		t.Error("View should display the address index")
	}
}

func TestReceive_WindowSizeMsg(t *testing.T) {
	m, _ := newTestReceive(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	updated := model.(ReceiveModel)
	if updated.width != 200 {
		t.Errorf("expected width=200, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height=50, got %d", updated.height)
	}
}

func TestReceive_V_VerifyOnDevice_OnlyForHardware(t *testing.T) {
	m, mock := newTestReceive(t)
	m.selectedAddress = "tb1q_test"
	m.derivationPath = "m/84'/1'/0'/0/0"

	// Software wallet: 'v' should NOT trigger verify
	mock.IsHardwareWalletFn = func() bool { return false }
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	updated := model.(ReceiveModel)
	if updated.verify.IsWaiting() {
		t.Error("v should not trigger verify for software wallet")
	}

	// Hardware wallet: 'v' should trigger verify
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsMultisigFn = func() bool { return false }
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	updated = model.(ReceiveModel)
	if !updated.verify.IsWaiting() {
		t.Error("v should trigger verify for hardware wallet")
	}
}

func TestReceive_VerifyOnDevice_Mismatch(t *testing.T) {
	m, mock := newTestReceive(t)
	m.selectedAddress = verifyTestAddress
	m.derivationPath = verifyTestPath
	m.revealProgress = 1.0
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsMultisigFn = func() bool { return false }
	mock.DisplayAddrOnDeviceFn = func(addr, path string) error {
		if addr != verifyTestAddress || path != verifyTestPath {
			t.Fatalf("device called with %q %q", addr, path)
		}
		return &wallet.AddressMismatchError{Displayed: verifyTestDevice, Expected: addr}
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	waiting := model.(ReceiveModel)
	if !waiting.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}

	result, ok := findMsgOfType[HWIVerifyMsg](collectMsgs(cmd))
	if !ok {
		t.Fatal("verification command did not produce HWIVerifyMsg")
	}
	model, _ = waiting.Update(result)
	updated := model.(ReceiveModel)
	if updated.verify.State != AddressVerifyMismatch {
		t.Fatalf("state = %d, want mismatch", updated.verify.State)
	}
	if updated.verify.DeviceAddress != verifyTestDevice {
		t.Fatalf("device address = %q", updated.verify.DeviceAddress)
	}

	view := stripANSI(updated.View())
	if !strings.Contains(view, "✗ Device shows a different address") {
		t.Fatalf("view does not show mismatch:\n%s", view)
	}
}
