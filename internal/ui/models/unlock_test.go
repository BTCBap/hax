package models

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ─── Constructor ────────────────────────────────────────────────────────────

func TestNewUnlockModel_InitialState(t *testing.T) {
	m := NewUnlockModel(nil)

	if m.unlocking {
		t.Error("expected unlocking=false initially")
	}
	if m.syncing {
		t.Error("expected syncing=false initially")
	}
	if m.err != nil {
		t.Errorf("expected no error, got %v", m.err)
	}
	if m.walletName != "" {
		t.Errorf("expected empty walletName, got %q", m.walletName)
	}
	if m.selectedWallet != 0 {
		t.Errorf("expected selectedWallet=0, got %d", m.selectedWallet)
	}
}

func TestNewUnlockModel_PasswordInputConfigured(t *testing.T) {
	m := NewUnlockModel(nil)

	// EchoPassword hides input characters with the EchoCharacter
	if m.passwordInput.EchoCharacter != '•' {
		t.Errorf("expected echo character '•', got '%c'", m.passwordInput.EchoCharacter)
	}
}

// ─── Message Handlers ───────────────────────────────────────────────────────

func TestUnlock_WalletListMsg_Success(t *testing.T) {
	m := NewUnlockModel(nil)
	wallets := []string{"wallet1", "wallet2", "wallet3"}

	model, _ := m.Update(WalletListMsg{Wallets: wallets})
	updated := model.(UnlockModel)

	if len(updated.walletList) != 3 {
		t.Errorf("expected 3 wallets, got %d", len(updated.walletList))
	}
	if updated.walletName != "wallet1" {
		t.Errorf("expected walletName=wallet1, got %q", updated.walletName)
	}
}

func TestUnlock_WalletListMsg_Empty(t *testing.T) {
	m := NewUnlockModel(nil)

	model, _ := m.Update(WalletListMsg{Wallets: []string{}})
	updated := model.(UnlockModel)

	if len(updated.walletList) != 0 {
		t.Errorf("expected 0 wallets, got %d", len(updated.walletList))
	}
	if updated.walletName != "" {
		t.Errorf("expected empty walletName, got %q", updated.walletName)
	}
}

func TestUnlock_WalletListMsg_Error(t *testing.T) {
	m := NewUnlockModel(nil)

	model, _ := m.Update(WalletListMsg{Err: fmt.Errorf("disk error")})
	updated := model.(UnlockModel)

	if updated.err == nil {
		t.Fatal("expected error to be set")
	}
	if !strings.Contains(updated.err.Error(), "disk error") {
		t.Errorf("expected disk error, got %q", updated.err.Error())
	}
}

func TestUnlock_WalletListMsg_SingleWallet(t *testing.T) {
	m := NewUnlockModel(nil)

	model, _ := m.Update(WalletListMsg{Wallets: []string{"mywallet"}})
	updated := model.(UnlockModel)

	if updated.walletName != "mywallet" {
		t.Errorf("expected walletName=mywallet, got %q", updated.walletName)
	}
}

func TestUnlock_UnlockResultMsg_Success(t *testing.T) {
	m := NewUnlockModel(nil)
	m.unlocking = true

	model, cmd := m.Update(UnlockResultMsg{Success: true})
	updated := model.(UnlockModel)

	if updated.unlocking {
		t.Error("expected unlocking=false after success")
	}
	if !updated.syncing {
		t.Error("expected syncing=true after unlock success")
	}
	if cmd == nil {
		t.Error("expected commands for sync and spinner")
	}
}

func TestUnlock_UnlockResultMsg_Failure(t *testing.T) {
	m := NewUnlockModel(nil)
	m.unlocking = true
	m.passwordInput.SetValue("wrongpassword")

	model, cmd := m.Update(UnlockResultMsg{Success: false, Err: fmt.Errorf("wrong password")})
	updated := model.(UnlockModel)

	if updated.unlocking {
		t.Error("expected unlocking=false after failure")
	}
	if updated.err == nil {
		t.Fatal("expected error to be set")
	}
	if !strings.Contains(updated.err.Error(), "wrong password") {
		t.Errorf("expected wrong password error, got %q", updated.err.Error())
	}
	// Password should be cleared after failed attempt
	if updated.passwordInput.Value() != "" {
		t.Error("expected password input cleared after failure")
	}
	// Shake animation should be triggered
	if updated.shakeIntensity == 0 {
		t.Error("expected shake animation triggered")
	}
	// Should return animation tick command
	if cmd == nil {
		t.Error("expected animation tick command")
	}
}

func TestUnlock_SyncResultMsg_Success(t *testing.T) {
	m := NewUnlockModel(nil)
	m.syncing = true

	_, cmd := m.Update(SyncResultMsg{Success: true})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestUnlock_SyncResultMsg_Failure(t *testing.T) {
	m := NewUnlockModel(nil)
	m.syncing = true

	// Even on sync failure, navigates to dashboard (sync retried later)
	_, cmd := m.Update(SyncResultMsg{Success: false, Err: fmt.Errorf("sync failed")})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestUnlock_WindowSizeMsg(t *testing.T) {
	m := NewUnlockModel(nil)

	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	updated := model.(UnlockModel)

	if updated.width != 100 {
		t.Errorf("expected width=100, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height=50, got %d", updated.height)
	}
}

// ─── Key Handling - Single Wallet ───────────────────────────────────────────

func TestUnlock_CtrlC_Quits(t *testing.T) {
	m := NewUnlockModel(nil)
	// Focus password input to test password input key handling
	m.passwordInput.Focus()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg := extractMsg(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg from ctrl+c, got %T", msg)
	}
}

func TestUnlock_Esc_SingleWallet_Quits(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"mywallet"} // single wallet
	m.passwordInput.Focus()

	_, cmd := m.Update(keyCode(tea.KeyEsc))
	if cmd == nil {
		t.Fatal("expected a command from esc")
	}
	msg := extractMsg(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg from esc with single wallet, got %T", msg)
	}
}

func TestUnlock_Enter_EmptyPassword(t *testing.T) {
	m := NewUnlockModel(nil)
	m.passwordInput.Focus()

	model, _ := m.Update(keyCode(tea.KeyEnter))
	updated := model.(UnlockModel)

	if updated.err == nil {
		t.Fatal("expected error for empty password")
	}
	if !strings.Contains(updated.err.Error(), "enter a password") {
		t.Errorf("expected password required error, got %q", updated.err.Error())
	}
}

// ─── Key Handling - Multiple Wallets ────────────────────────────────────────

func TestUnlock_WalletSelection_Navigation(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"wallet1", "wallet2", "wallet3"}
	m.walletName = "wallet1"
	m.passwordInput.Blur() // Wallet selection mode

	// Down
	model, _ := m.Update(keyCode(tea.KeyDown))
	updated := model.(*UnlockModel)
	if updated.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", updated.cursor)
	}
	if updated.selectedWallet != 1 {
		t.Errorf("expected selectedWallet=1, got %d", updated.selectedWallet)
	}

	// Down again
	model, _ = updated.Update(keyCode(tea.KeyDown))
	updated = model.(*UnlockModel)
	if updated.cursor != 2 {
		t.Errorf("expected cursor=2, got %d", updated.cursor)
	}

	// Clamp at end
	model, _ = updated.Update(keyCode(tea.KeyDown))
	updated = model.(*UnlockModel)
	if updated.cursor != 2 {
		t.Errorf("expected cursor clamped at 2, got %d", updated.cursor)
	}

	// Up
	model, _ = updated.Update(keyCode(tea.KeyUp))
	updated = model.(*UnlockModel)
	if updated.cursor != 1 {
		t.Errorf("expected cursor=1 after up, got %d", updated.cursor)
	}
}

func TestUnlock_WalletSelection_VimKeys(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"wallet1", "wallet2"}
	m.passwordInput.Blur()

	// j for down
	model, _ := m.Update(keyRune('j'))
	updated := model.(*UnlockModel)
	if updated.cursor != 1 {
		t.Errorf("expected cursor=1 with 'j', got %d", updated.cursor)
	}

	// k for up
	model, _ = updated.Update(keyRune('k'))
	updated = model.(*UnlockModel)
	if updated.cursor != 0 {
		t.Errorf("expected cursor=0 with 'k', got %d", updated.cursor)
	}
}

func TestUnlock_WalletSelection_Enter(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"wallet1", "wallet2"}
	m.selectedWallet = 1
	m.passwordInput.Blur()

	model, cmd := m.Update(keyCode(tea.KeyEnter))
	updated := model.(*UnlockModel)

	if updated.walletName != "wallet2" {
		t.Errorf("expected walletName=wallet2, got %q", updated.walletName)
	}
	// Should focus password input
	if cmd == nil {
		t.Error("expected focus command for password input")
	}
}

func TestUnlock_WalletSelection_Esc_Quits(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"wallet1", "wallet2"}
	m.passwordInput.Blur()

	_, cmd := m.Update(keyRune('q'))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	msg := extractMsg(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestUnlock_MultipleWallets_EscFromPassword_GoesBack(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"wallet1", "wallet2"}
	m.passwordInput.Focus()

	model, _ := m.Update(keyCode(tea.KeyEsc))
	updated := model.(UnlockModel)

	if updated.passwordInput.Focused() {
		t.Error("expected password input blurred after esc")
	}
}

// ─── TickMsg (Animation) ────────────────────────────────────────────────────

func TestUnlock_TickMsg_ShakeDecays(t *testing.T) {
	m := NewUnlockModel(nil)
	m.shakeIntensity = 1.0
	m.unlocking = false

	model, cmd := m.Update(TickMsg(time.Now()))
	updated := model.(UnlockModel)

	// Shake should decay toward 0
	if updated.shakeIntensity >= 1.0 {
		t.Error("expected shake to decay")
	}
	// Should continue animation while shake > threshold
	if updated.shakeIntensity > 0.01 && cmd == nil {
		t.Error("expected continue animation command while shake > 0.01")
	}
}

func TestUnlock_TickMsg_UnlockingAnimation(t *testing.T) {
	m := NewUnlockModel(nil)
	m.unlocking = true

	model, cmd := m.Update(TickMsg(time.Now()))
	updated := model.(UnlockModel)

	// Pulse frame should increment
	if updated.pulseFrame != 1 {
		t.Errorf("expected pulseFrame=1, got %d", updated.pulseFrame)
	}
	// Should continue animation while unlocking
	if cmd == nil {
		t.Error("expected continue animation command while unlocking")
	}
}

// ─── View Rendering ─────────────────────────────────────────────────────────

func TestUnlock_View_PasswordEntry(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.walletName = "test-wallet"
	m.walletList = []string{"test-wallet"}
	m.passwordInput.Focus()

	view := m.View()

	if !strings.Contains(view, "Unlock Wallet") {
		t.Error("expected 'Unlock Wallet' in view")
	}
	if !strings.Contains(view, "test-wallet") {
		t.Error("expected wallet name in view")
	}
	if !strings.Contains(view, "Password") {
		t.Error("expected 'Password' in view")
	}
}

func TestUnlock_View_WalletSelection(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.walletList = []string{"wallet1", "wallet2", "wallet3"}
	m.passwordInput.Blur() // Show wallet selection

	view := m.View()

	if !strings.Contains(view, "Select a wallet") {
		t.Error("expected 'Select a wallet' in view")
	}
	if !strings.Contains(view, "wallet1") || !strings.Contains(view, "wallet2") {
		t.Error("expected wallet names in view")
	}
}

func TestUnlock_View_UnlockingState(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.unlocking = true

	view := m.View()

	if !strings.Contains(view, "Unlocking") {
		t.Error("expected 'Unlocking' in view")
	}
}

func TestUnlock_View_SyncingState(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.syncing = true

	view := m.View()

	if !strings.Contains(view, "Syncing") {
		t.Error("expected 'Syncing' in view")
	}
}

func TestUnlock_View_WithError(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.err = fmt.Errorf("wrong password")
	m.walletList = []string{"mywallet"}
	m.passwordInput.Focus()

	view := m.View()

	if !strings.Contains(view, "wrong password") {
		t.Error("expected error message in view")
	}
}

func TestUnlock_View_ZeroWidth_UsesDefault(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 0
	m.height = 0
	m.walletList = []string{"mywallet"}
	m.passwordInput.Focus()

	// Should not panic and use default width of 80
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view with zero width")
	}
}

func TestUnlock_View_Help_SingleWallet(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.walletList = []string{"mywallet"}
	m.passwordInput.Focus()

	view := m.View()

	if !strings.Contains(view, "unlock") {
		t.Error("expected 'unlock' in help text")
	}
	if !strings.Contains(view, "quit") {
		t.Error("expected 'quit' in help text for single wallet")
	}
}

func TestUnlock_View_Help_MultipleWallets_Selection(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.walletList = []string{"wallet1", "wallet2"}
	m.passwordInput.Blur()

	view := m.View()

	if !strings.Contains(view, "select") {
		t.Error("expected 'select' in help text for wallet selection")
	}
}

func TestUnlock_View_Help_MultipleWallets_Password(t *testing.T) {
	m := NewUnlockModel(nil)
	m.width = 80
	m.height = 40
	m.walletList = []string{"wallet1", "wallet2"}
	m.passwordInput.Focus()

	view := m.View()

	if !strings.Contains(view, "back") {
		t.Error("expected 'back' in help text for password entry with multiple wallets")
	}
}

// ─── Lock Icon Animation ────────────────────────────────────────────────────

func TestUnlock_RenderLockIcon_Normal(t *testing.T) {
	m := NewUnlockModel(nil)
	m.lockSpinAngle = 0

	icon := m.renderLockIcon()
	if icon == "" {
		t.Error("expected non-empty lock icon")
	}
}

func TestUnlock_RenderLockIcon_AllAngles(t *testing.T) {
	m := NewUnlockModel(nil)

	// Test all four angle ranges
	angles := []float64{0, 90, 180, 270}
	for _, angle := range angles {
		m.lockSpinAngle = angle
		icon := m.renderLockIcon()
		if icon == "" {
			t.Errorf("expected non-empty lock icon at angle %f", angle)
		}
	}
}

// ─── Spinner Handling ───────────────────────────────────────────────────────

func TestUnlock_SpinnerTick_WhileUnlocking(t *testing.T) {
	m := NewUnlockModel(nil)
	m.unlocking = true

	// Get a spinner tick from the model's spinner
	tickMsg := m.spinner.Tick()
	model, cmd := m.Update(tickMsg)
	_ = model.(UnlockModel)

	// Should propagate spinner tick while unlocking
	if cmd == nil {
		t.Error("expected spinner continuation while unlocking")
	}
}

func TestUnlock_SpinnerTick_WhileSyncing(t *testing.T) {
	m := NewUnlockModel(nil)
	m.syncing = true

	tickMsg := m.spinner.Tick()
	model, cmd := m.Update(tickMsg)
	_ = model.(UnlockModel)

	if cmd == nil {
		t.Error("expected spinner continuation while syncing")
	}
}

func TestUnlock_SpinnerTick_WhenIdle(t *testing.T) {
	m := NewUnlockModel(nil)
	m.unlocking = false
	m.syncing = false

	tickMsg := m.spinner.Tick()
	model, cmd := m.Update(tickMsg)
	_ = model.(UnlockModel)

	// Whether an idle spinner tick re-arms is implementation-dependent;
	// this test only checks that it doesn't panic.
	_ = cmd
}
