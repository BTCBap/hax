package models

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewHelpModel(t *testing.T) {
	m := NewHelpModel(nil)
	if !m.ready {
		t.Error("expected ready=true")
	}
	if m.fadeIn != 0.0 {
		t.Errorf("expected fadeIn=0, got %f", m.fadeIn)
	}
	if m.isActive {
		t.Error("expected isActive=false initially")
	}
}

func TestHelpInit_ReturnsTickCmd(t *testing.T) {
	m := NewHelpModel(nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a tick command")
	}
}

func TestHelpKeyEsc_NavigatesToDashboard(t *testing.T) {
	m := NewHelpModel(nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestHelpKeyQ_NavigatesToDashboard(t *testing.T) {
	m := NewHelpModel(nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestHelpKeyQuestion_NavigatesToDashboard(t *testing.T) {
	m := NewHelpModel(nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestHelpScreenActiveMsg(t *testing.T) {
	m := NewHelpModel(nil)
	model, cmd := m.Update(ScreenActiveMsg{})
	updated := model.(HelpModel)
	if !updated.isActive {
		t.Error("expected isActive=true after ScreenActiveMsg")
	}
	if cmd == nil {
		t.Error("expected tick command for incomplete animation")
	}
}

func TestHelpWindowSizeMsg(t *testing.T) {
	m := NewHelpModel(nil)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	updated := model.(HelpModel)
	if updated.width != 100 {
		t.Errorf("expected width=100, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height=50, got %d", updated.height)
	}
}

func TestHelpTickMsg_AdvancesFadeIn(t *testing.T) {
	m := NewHelpModel(nil)
	m.isActive = true
	var model tea.Model = m
	for i := 0; i < 20; i++ {
		model, _ = model.Update(TickMsg(time.Now()))
	}
	updated := model.(HelpModel)
	if updated.fadeIn <= 0.0 {
		t.Error("expected fadeIn to advance after ticks")
	}
}

func TestHelpView_ContainsSections(t *testing.T) {
	m := NewHelpModel(nil)
	view := m.View()
	expected := []string{
		"Help",
		"GETTING STARTED",
	}
	for _, s := range expected {
		if !strings.Contains(view, s) {
			t.Errorf("help view should contain %q", s)
		}
	}
}

func TestHelpRenderContent_HasAllSections(t *testing.T) {
	m := NewHelpModel(nil)
	content := m.renderHelpContent()
	sections := []string{
		"GETTING STARTED",
		"KEYBOARD SHORTCUTS",
		"SENDING BITCOIN",
		"SECURITY",
		"BACKUP & RECOVERY",
		"TROUBLESHOOTING",
	}
	for _, s := range sections {
		if !strings.Contains(content, s) {
			t.Errorf("help content should contain section %q", s)
		}
	}
}

func TestHelpRenderContent_HasKeyShortcuts(t *testing.T) {
	m := NewHelpModel(nil)
	content := m.renderHelpContent()
	shortcuts := []string{
		"Navigation",
		"Dashboard",
		"Receive",
		"History",
		"Addresses",
		"UTXOs",
		"Settings",
		"Sign & Broadcast",
	}
	for _, s := range shortcuts {
		if !strings.Contains(content, s) {
			t.Errorf("help content should contain shortcut section %q", s)
		}
	}
}

func TestHelpRenderContent_IncludesAdvancedSettingsAndSendBindings(t *testing.T) {
	m := NewHelpModel(nil)
	content := m.renderHelpContent()

	expected := []string{
		"Copy / export / back",
		"Navigate / select / manual path / refresh",
		"Switch BC-UR / BBQr / back",
		"Confirm import/export or cancel",
		"some dialogs use esc only",
	}

	for _, s := range expected {
		if !strings.Contains(content, s) {
			t.Errorf("help content should contain advanced binding %q", s)
		}
	}
}

func TestHelpRenderContent_HardwareWalletSpecificGuidance(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsWatchOnlyFn = func() bool { return true }

	m := NewHelpModel(mock)
	content := m.renderHelpContent()

	if !strings.Contains(content, "v") || !strings.Contains(content, "Verify address on hardware device") {
		t.Error("hardware wallet help should document device verification")
	}
	if !strings.Contains(content, "Importing the matching seed in Settings (t) then (i) converts it to local signing.") {
		t.Error("hardware wallet help should explain local signing upgrade path")
	}
	if strings.Contains(content, "Reveal seed phrase after warning") {
		t.Error("hardware wallet help should not claim mnemonic reveal is available")
	}
}

func TestHelpRenderContent_MultisigSpecificGuidance(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsMultisigFn = func() bool { return true }

	m := NewHelpModel(mock)
	content := m.renderHelpContent()

	if !strings.Contains(content, "No single seed phrase restores this wallet.") {
		t.Error("multisig help should explain recovery requirements")
	}
	if !strings.Contains(content, "Export Coldcard multisig config") {
		t.Error("multisig help should document Coldcard export hotkey")
	}
	if !strings.Contains(content, "Cosigner list edit / scan local wallets") {
		t.Error("multisig help should document cosigner subview bindings")
	}
	if strings.Contains(content, "Reveal seed phrase after warning") {
		t.Error("multisig help should not claim mnemonic reveal is available")
	}
}

func TestHelpMatchesReceiveFooterShortcuts(t *testing.T) {
	software := NewMockWalletManager()
	softwareHelp := stripANSI(NewHelpModel(software).renderHelpContent())
	softwareFooter := stripANSI(NewReceiveModel(software).renderAnimatedHelp())

	assertFooterAndHelpShortcut(t, softwareFooter, softwareHelp, "n", "Generate new address")
	assertFooterAndHelpShortcut(t, softwareFooter, softwareHelp, "c", "Copy address to clipboard")
	assertFooterAndHelpShortcut(t, softwareFooter, softwareHelp, "←/→", "Navigate addresses")
	assertFooterAndHelpShortcut(t, softwareFooter, softwareHelp, "esc", "Go back / Cancel")

	hardware := NewMockWalletManager()
	hardware.IsHardwareWalletFn = func() bool { return true }
	hardware.IsWatchOnlyFn = func() bool { return true }
	hardwareHelp := stripANSI(NewHelpModel(hardware).renderHelpContent())
	hardwareFooter := stripANSI(NewReceiveModel(hardware).renderAnimatedHelp())

	assertFooterAndHelpShortcut(t, hardwareFooter, hardwareHelp, "v", "Verify address on hardware device")
}

func TestHelpMatchesAddressesAndUTXOsFooters(t *testing.T) {
	mock := NewMockWalletManager()
	helpContent := stripANSI(NewHelpModel(mock).renderHelpContent())

	addresses := NewAddressesModel(mock, nil)
	addresses.width = 100
	addresses.height = 30
	addressesView := stripANSI(addresses.View())
	assertFooterAndHelpShortcut(t, addressesView, helpContent, "PgUp/PgDn", "Scroll faster")
	assertFooterAndHelpShortcut(t, addressesView, helpContent, "tab", "Toggle receive / change")
	assertFooterAndHelpShortcut(t, addressesView, helpContent, "c", "Copy address")
	assertFooterAndHelpShortcut(t, addressesView, helpContent, "e", "Edit address label")

	utxos := NewUTXOsModel(mock, nil)
	utxos.width = 100
	utxos.height = 30
	utxosView := stripANSI(utxos.View())
	assertFooterAndHelpShortcut(t, utxosView, helpContent, "←/→", "Page navigation")
	assertFooterAndHelpShortcut(t, utxosView, helpContent, "e", "Edit UTXO label")
	assertFooterAndHelpShortcut(t, utxosView, helpContent, "c", "Copy outpoint (txid:vout)")
}

func TestHelpMatchesSettingsFooterByWalletMode(t *testing.T) {
	singleSig := NewMockWalletManager()
	singleSig.GetAccountsFn = func() []wallet.AccountData { return nil }
	singleHelp := stripANSI(NewHelpModel(singleSig).renderHelpContent())
	singleSettings := NewSettingsModel(singleSig, nil)
	singleSettings.width = 100
	singleSettings.height = 40
	singleView := stripANSI(singleSettings.View())
	assertFooterAndHelpShortcut(t, singleView, singleHelp, "m", "Reveal seed phrase after warning")
	assertFooterAndHelpShortcut(t, singleView, singleHelp, "c", "Copy xpub")
	assertFooterAndHelpShortcut(t, singleView, singleHelp, "d", "Open descriptor screen")

	watchOnly := NewMockWalletManager()
	watchOnly.IsHardwareWalletFn = func() bool { return true }
	watchOnly.IsWatchOnlyFn = func() bool { return true }
	watchOnly.GetAccountsFn = func() []wallet.AccountData { return nil }
	watchHelp := stripANSI(NewHelpModel(watchOnly).renderHelpContent())
	watchSettings := NewSettingsModel(watchOnly, nil)
	watchSettings.width = 100
	watchSettings.height = 40
	watchView := stripANSI(watchSettings.View())
	assertFooterAndHelpShortcut(t, watchView, watchHelp, "i", "Import matching seed for local signing")
	if helpHasShortcutLine(watchHelp, "m", "Reveal seed phrase after warning") {
		t.Error("watch-only help should not expose mnemonic reveal shortcut")
	}

	multisig := NewMockWalletManager()
	multisig.IsMultisigFn = func() bool { return true }
	multisig.GetAccountsFn = func() []wallet.AccountData { return nil }
	multiHelp := stripANSI(NewHelpModel(multisig).renderHelpContent())
	multiSettings := NewSettingsModel(multisig, nil)
	multiSettings.width = 100
	multiSettings.height = 40
	multiView := stripANSI(multiSettings.View())
	assertFooterAndHelpShortcut(t, multiView, multiHelp, "i", "Manage cosigners / import cosigner seeds")
	assertFooterAndHelpShortcut(t, multiView, multiHelp, "w", "Export Coldcard multisig config")
}

func stripANSI(s string) string {
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansi.ReplaceAllString(s, "")
}

func helpHasShortcutLine(helpContent, key, phrase string) bool {
	linePattern := `(?m)^\s*` + regexp.QuoteMeta(key) + `\s+.*` + regexp.QuoteMeta(phrase) + `.*$`
	return regexp.MustCompile(linePattern).FindStringIndex(helpContent) != nil
}

func assertFooterAndHelpShortcut(t *testing.T, footer, helpContent, key, helpPhrase string) {
	t.Helper()
	if !strings.Contains(footer, key) {
		t.Fatalf("expected live footer to contain key %q; footer=%q", key, footer)
	}
	if !helpHasShortcutLine(helpContent, key, helpPhrase) {
		t.Fatalf("expected help to document key %q with phrase %q; help=%q", key, helpPhrase, helpContent)
	}
}
