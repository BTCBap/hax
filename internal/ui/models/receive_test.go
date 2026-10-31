package models

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
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

// receiveAddressAt returns an address of fixed length for an index. The length
// is fixed so layout tests can compare renders across indexes.
func receiveAddressAt(index uint32) string {
	return fmt.Sprintf("tb1q%037d", index)
}

func receivePathAt(index uint32) string {
	return fmt.Sprintf("m/84'/1'/0'/0/%d", index)
}

func TestReceive_Right_LoadsNextAddress(t *testing.T) {
	m, mock := newTestReceive(t)
	m.currentIndex = 3
	m.firstUnusedIndex = 3
	m.selectedAddress = receiveAddressAt(3)
	m.derivationPath = receivePathAt(3)

	var gotAccount, gotIndex uint32
	mock.GetAddressByIndexFn = func(account, index uint32) (string, string, error) {
		gotAccount, gotIndex = account, index
		return receiveAddressAt(index), receivePathAt(index), nil
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	loading := model.(ReceiveModel)
	if !loading.generatingNew {
		t.Fatal("right should mark the model as loading")
	}

	msg, ok := findMsgOfType[AddressGeneratedMsg](collectMsgs(cmd))
	if !ok {
		t.Fatal("right should load the next address")
	}
	if gotAccount != 0 || gotIndex != 4 {
		t.Fatalf("GetAddressByIndex called with account %d index %d, want account 0 index 4", gotAccount, gotIndex)
	}

	model, _ = loading.Update(msg)
	updated := model.(ReceiveModel)
	if updated.currentIndex != 4 {
		t.Fatalf("currentIndex = %d, want 4", updated.currentIndex)
	}
	if updated.selectedAddress != receiveAddressAt(4) {
		t.Fatalf("selectedAddress = %q, want %q", updated.selectedAddress, receiveAddressAt(4))
	}
	if updated.derivationPath != receivePathAt(4) {
		t.Fatalf("derivationPath = %q, want %q", updated.derivationPath, receivePathAt(4))
	}

	updated.revealProgress = 1.0
	plain := stripANSI(updated.View())
	wants := []string{
		"Address #4",
		receivePathAt(4),
		stripANSI(ui.FormatAddress(receiveAddressAt(4), 0)),
	}
	for _, want := range wants {
		if !strings.Contains(plain, want) {
			t.Fatalf("view is missing %q:\n%s", want, plain)
		}
	}
}

func TestReceive_Left_LoadsPreviousAddress(t *testing.T) {
	m, mock := newTestReceive(t)
	m.currentIndex = 4
	m.firstUnusedIndex = 4
	m.selectedAddress = receiveAddressAt(4)
	m.derivationPath = receivePathAt(4)

	mock.GetAddressByIndexFn = func(_, index uint32) (string, string, error) {
		return receiveAddressAt(index), receivePathAt(index), nil
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	loading := model.(ReceiveModel)
	if !loading.generatingNew {
		t.Fatal("left should mark the model as loading")
	}

	msg, ok := findMsgOfType[AddressGeneratedMsg](collectMsgs(cmd))
	if !ok {
		t.Fatal("left should load the previous address")
	}
	if msg.Index != 3 {
		t.Fatalf("loaded index %d, want 3", msg.Index)
	}

	model, _ = loading.Update(msg)
	updated := model.(ReceiveModel)
	if updated.currentIndex != 3 {
		t.Fatalf("currentIndex = %d, want 3", updated.currentIndex)
	}
	if updated.selectedAddress != receiveAddressAt(3) {
		t.Fatalf("selectedAddress = %q, want %q", updated.selectedAddress, receiveAddressAt(3))
	}
	if updated.derivationPath != receivePathAt(3) {
		t.Fatalf("derivationPath = %q, want %q", updated.derivationPath, receivePathAt(3))
	}

	updated.revealProgress = 1.0
	plain := stripANSI(updated.View())
	if !strings.Contains(plain, "Address #3") || !strings.Contains(plain, receivePathAt(3)) {
		t.Fatalf("view does not show address #3:\n%s", plain)
	}
}

func TestReceive_NonArrowKeysDoNotNavigate(t *testing.T) {
	for _, key := range []string{"h", "l", "n"} {
		t.Run(key, func(t *testing.T) {
			m, mock := newTestReceive(t)
			m.currentIndex = 3
			m.firstUnusedIndex = 3
			m.selectedAddress = receiveAddressAt(3)

			loaded := false
			mock.GetAddressByIndexFn = func(_, index uint32) (string, string, error) {
				loaded = true
				return receiveAddressAt(index), receivePathAt(index), nil
			}

			model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			updated := model.(ReceiveModel)
			if updated.generatingNew {
				t.Fatalf("%q should not start a load", key)
			}
			if containsMsgOfType[AddressGeneratedMsg](collectMsgs(cmd)) {
				t.Fatalf("%q produced an AddressGeneratedMsg", key)
			}
			if loaded {
				t.Fatalf("%q called GetAddressByIndex", key)
			}
		})
	}
}

func TestReceive_Right_IgnoredWhileLoading(t *testing.T) {
	m, _ := newTestReceive(t)
	m.currentIndex = 3
	m.firstUnusedIndex = 3
	m.generatingNew = true

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := model.(ReceiveModel)
	if !updated.generatingNew {
		t.Fatal("right while loading should keep generatingNew true")
	}
	if containsMsgOfType[AddressGeneratedMsg](collectMsgs(cmd)) {
		t.Fatal("right while loading should not start another load")
	}
}

func TestReceive_Right_StopsAtGapLimit(t *testing.T) {
	m, mock := newTestReceive(t)
	m.firstUnusedIndex = 5
	lastIndex := m.firstUnusedIndex + uint32(wallet.AddressGapLimit) - 1
	m.currentIndex = lastIndex
	mock.GetAddressByIndexFn = func(_, index uint32) (string, string, error) {
		return receiveAddressAt(index), receivePathAt(index), nil
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := model.(ReceiveModel)
	if updated.generatingNew {
		t.Fatal("right at the gap cap should not start a load")
	}
	if containsMsgOfType[AddressGeneratedMsg](collectMsgs(cmd)) {
		t.Fatal("right at the gap cap should not load an address")
	}

	updated.currentIndex = lastIndex - 1
	_, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRight})
	msg, ok := findMsgOfType[AddressGeneratedMsg](collectMsgs(cmd))
	if !ok {
		t.Fatal("right one below the gap cap should load the last allowed address")
	}
	if msg.Index != lastIndex {
		t.Fatalf("loaded index %d, want %d", msg.Index, lastIndex)
	}
}

func TestReceive_Init_SetsFirstUnusedIndex(t *testing.T) {
	m, mock := newTestReceive(t)
	mock.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return receiveAddressAt(7), receivePathAt(7), 7, nil
	}

	msg, ok := findMsgOfType[AddressGeneratedMsg](collectMsgs(m.Init()))
	if !ok {
		t.Fatal("Init should load the next unused address")
	}
	if !msg.NextUnused {
		t.Fatal("loadAddresses result should be marked NextUnused")
	}

	model, _ := m.Update(msg)
	updated := model.(ReceiveModel)
	if updated.firstUnusedIndex != 7 {
		t.Fatalf("firstUnusedIndex = %d, want 7", updated.firstUnusedIndex)
	}
}

func TestReceive_View_StableAcrossAddressIndexes(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetAddressByIndexFn = func(_, index uint32) (string, string, error) {
		return receiveAddressAt(index), receivePathAt(index), nil
	}

	base := NewReceiveModel(mock)
	base.width = 100
	base.height = 40
	base.viewport.Width = 96
	base.viewport.Height = 30

	var wantLines, wantHintLine, wantBorderLine int
	for i, index := range []uint32{0, 1, 9, 10} {
		m := base
		model, _ := m.Update(AddressGeneratedMsg{
			Address: receiveAddressAt(index),
			Path:    receivePathAt(index),
			Index:   index,
		})
		m = model.(ReceiveModel)
		m.revealProgress = 1.0

		view := m.View()
		lines := requireLineCount(t, view)
		hint := requireTextPosition(t, view, "c copy")
		border := requireTextPosition(t, view, "╭")

		if i == 0 {
			wantLines, wantHintLine, wantBorderLine = lines, hint.line, border.line
		} else {
			if lines != wantLines {
				t.Fatalf("index %d rendered %d lines, want %d", index, lines, wantLines)
			}
			if hint.line != wantHintLine {
				t.Fatalf("index %d moved the hint from line %d to %d", index, wantHintLine, hint.line)
			}
			if border.line != wantBorderLine {
				t.Fatalf("index %d moved the QR box top from line %d to %d", index, wantBorderLine, border.line)
			}
		}
		assertWithinTerminal(t, view, 100, 40)
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

// The verification panel no longer draws its own hint line; the screen footer
// carries the retry/back keys on the same row in every panel state.
func TestReceive_VerifyModeOuterHintRow(t *testing.T) {
	m, _ := newHardwareReceive(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = model.(ReceiveModel)
	m = pressReceiveKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})

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
		if strings.Contains(plain, "←/→ address") || strings.Contains(plain, "v verify on device") {
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
