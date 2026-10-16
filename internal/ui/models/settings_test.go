package models

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/config"
)

// ─── Constructor ────────────────────────────────────────────────────────────

func TestNewSettingsModel_DefaultConfig(t *testing.T) {
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	m := NewSettingsModel(nil, cm)

	if m.displayUnit != config.DisplayUnitBTC {
		t.Errorf("expected default display unit BTC, got %q", m.displayUnit)
	}
	if m.showMnemonic {
		t.Error("expected showMnemonic=false initially")
	}
	if m.showMnemonicWarning {
		t.Error("expected showMnemonicWarning=false initially")
	}
	if m.showCosignerList {
		t.Error("expected showCosignerList=false initially")
	}
	if m.isActive {
		t.Error("expected isActive=false initially")
	}
}

func TestNewSettingsModel_NilConfigManager(t *testing.T) {
	m := NewSettingsModel(nil, nil)

	// Should still create a valid model with defaults
	if m.displayUnit != config.DisplayUnitBTC {
		t.Errorf("expected default display unit BTC, got %q", m.displayUnit)
	}
}

// ─── Message Handlers ───────────────────────────────────────────────────────

func TestSettings_WindowSizeMsg(t *testing.T) {
	m := NewSettingsModel(nil, nil)

	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := model.(SettingsModel)

	if updated.width != 120 {
		t.Errorf("expected width=120, got %d", updated.width)
	}
	if updated.height != 40 {
		t.Errorf("expected height=40, got %d", updated.height)
	}
}

func TestSettings_TickMsg_Inactive(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.isActive = false

	model, cmd := m.Update(TickMsg(time.Now()))
	_ = model.(SettingsModel)

	// Should return nil cmd when inactive
	if cmd != nil {
		t.Error("expected nil command when settings is inactive")
	}
}

func TestSettings_TickMsg_CopiedTimeout(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.isActive = true
	m.showCopied = true
	m.copiedTime = time.Now().Add(-3 * time.Second) // expired

	model, _ := m.Update(TickMsg(time.Now()))
	updated := model.(SettingsModel)

	if updated.showCopied {
		t.Error("expected showCopied=false after timeout")
	}
}

func TestSettings_TickMsg_CopiedStillVisible(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.isActive = true
	m.showCopied = true
	m.copiedTime = time.Now() // just now

	model, cmd := m.Update(TickMsg(time.Now()))
	updated := model.(SettingsModel)

	if !updated.showCopied {
		t.Error("expected showCopied=true (not expired)")
	}
	if cmd == nil {
		t.Error("expected animation tick command")
	}
}

// ─── Mnemonic Warning State ─────────────────────────────────────────────────

func TestSettings_MnemonicWarning_Cancel_N(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonicWarning = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := model.(SettingsModel)

	if updated.showMnemonicWarning {
		t.Error("expected showMnemonicWarning=false after 'n'")
	}
}

func TestSettings_MnemonicWarning_Cancel_Esc(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonicWarning = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(SettingsModel)

	if updated.showMnemonicWarning {
		t.Error("expected showMnemonicWarning=false after esc")
	}
}

func TestSettings_MnemonicWarning_IgnoresOtherKeys(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonicWarning = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated := model.(SettingsModel)

	// Should stay in warning state (only y/n/esc close it)
	if !updated.showMnemonicWarning {
		t.Error("expected showMnemonicWarning=true after unrelated key")
	}
}

// ─── Mnemonic Viewing State ─────────────────────────────────────────────────

func TestSettings_MnemonicView_EscCloses(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonic = true
	m.mnemonic = "test mnemonic words"

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(SettingsModel)

	if updated.showMnemonic {
		t.Error("expected showMnemonic=false after esc")
	}
	if updated.mnemonic != "" {
		t.Error("expected mnemonic cleared after esc")
	}
}

func TestSettings_MnemonicView_EnterCloses(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonic = true
	m.mnemonic = "test mnemonic words"

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(SettingsModel)

	if updated.showMnemonic {
		t.Error("expected showMnemonic=false after enter")
	}
}

func TestSettings_MnemonicView_QCloses(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonic = true
	m.mnemonic = "test mnemonic words"

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := model.(SettingsModel)

	if updated.showMnemonic {
		t.Error("expected showMnemonic=false after 'q'")
	}
}

// ─── Cosigner List State ────────────────────────────────────────────────────

func TestSettings_CosignerList_EscCloses(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showCosignerList = true
	m.cosignerImportError = "old error"
	m.cosignerImportSuccess = "old success"

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(SettingsModel)

	if updated.showCosignerList {
		t.Error("expected showCosignerList=false after esc")
	}
	if updated.cosignerImportError != "" {
		t.Error("expected cosignerImportError cleared")
	}
	if updated.cosignerImportSuccess != "" {
		t.Error("expected cosignerImportSuccess cleared")
	}
}

func TestSettings_CosignerList_QCloses(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showCosignerList = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := model.(SettingsModel)

	if updated.showCosignerList {
		t.Error("expected showCosignerList=false after 'q'")
	}
}

func TestSettings_CosignerList_OpenSeedInput(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showCosignerList = true

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	updated := model.(SettingsModel)

	if updated.showCosignerList {
		t.Error("expected showCosignerList=false after 'i'")
	}
	if !updated.showCosignerSeedInput {
		t.Error("expected showCosignerSeedInput=true")
	}
	if updated.cosignerImportError != "" {
		t.Error("expected cosignerImportError cleared")
	}
}

// ─── Cosigner Edit State ────────────────────────────────────────────────────

func TestSettings_CosignerEdit_EscCancels(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showCosignerEdit = true
	m.editingCosignerIndex = 1

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(SettingsModel)

	if updated.showCosignerEdit {
		t.Error("expected showCosignerEdit=false after esc")
	}
	if !updated.showCosignerList {
		t.Error("expected showCosignerList=true (go back to list)")
	}
}

func TestSettings_CosignerEdit_TabSwitchesFields(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showCosignerEdit = true
	m.cosignerEditField = 0
	m.cosignerNameInput.Focus()

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := model.(SettingsModel)

	if updated.cosignerEditField != 1 {
		t.Errorf("expected cosignerEditField=1 after tab, got %d", updated.cosignerEditField)
	}

	// Tab again goes back to name
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = model.(SettingsModel)

	if updated.cosignerEditField != 0 {
		t.Errorf("expected cosignerEditField=0 after second tab, got %d", updated.cosignerEditField)
	}
}

// ─── Descriptor Menu State ──────────────────────────────────────────────────

func TestSettings_DescriptorMenu_EscCloses(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showDescriptorMenu = true
	m.descriptorText = "wpkh([fingerprint/84'/1'/0']tpub...)"

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(SettingsModel)

	if updated.showDescriptorMenu {
		t.Error("expected showDescriptorMenu=false after esc")
	}
	if updated.descriptorText != "" {
		t.Error("expected descriptorText cleared")
	}
}

// ─── View Rendering (overlay views that don't need walletManager) ───────────

func TestSettings_View_MnemonicWarning(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonicWarning = true
	m.width = 80
	m.height = 40

	view := m.View()

	// Mnemonic warning overlay should contain warning text
	if view == "" {
		t.Error("expected non-empty view for mnemonic warning")
	}
}

func TestSettings_View_MnemonicDisplay(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonic = true
	m.mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	m.width = 80
	m.height = 40

	view := m.View()

	if !strings.Contains(view, "abandon") {
		t.Error("expected mnemonic words in view")
	}
}

// Note: CosignerSeedInput, CosignerEdit, and SingleSigSeedInput views
// call walletManager methods (GetMultisigConfig, etc.) and cannot be tested
// with nil walletManager. These require integration tests with a real wallet.

// ─── AnimateTick ────────────────────────────────────────────────────────────

func TestSettings_AnimateTick_Inactive(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.isActive = false

	cmd := m.animateTick()
	if cmd != nil {
		t.Error("expected nil command when inactive")
	}
}

func TestSettings_AnimateTick_Active(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.isActive = true

	cmd := m.animateTick()
	if cmd == nil {
		t.Error("expected tick command when active")
	}
}

// ─── Init ───────────────────────────────────────────────────────────────────

func TestSettings_Init_NoAnimation(t *testing.T) {
	m := NewSettingsModel(nil, nil)

	cmd := m.Init()
	// Should return nil when not showing copy message
	if cmd != nil {
		t.Error("expected nil command from Init when not showing copy")
	}
}
