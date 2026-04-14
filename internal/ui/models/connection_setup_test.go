package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"hax/internal/config"
)

func newTestConnectionSetupModel(t *testing.T, returnToSettings bool) ConnectionSetupModel {
	t.Helper()
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	return NewConnectionSetupModel(cm, returnToSettings)
}

func TestNewConnectionSetupModel_InitialState(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	if m.step != StepChooseBackend {
		t.Errorf("expected initial step StepChooseBackend, got %d", m.step)
	}
	if m.cursor != 0 {
		t.Error("expected cursor=0")
	}
	if m.buttonState != ButtonConnect {
		t.Errorf("expected buttonState ButtonConnect, got %d", m.buttonState)
	}
	if !m.useSSL {
		t.Error("expected useSSL=true by default")
	}
	if m.network != "testnet" {
		t.Errorf("expected network=testnet, got %q", m.network)
	}
	if m.returnToSettings {
		t.Error("expected returnToSettings=false")
	}
}

func TestConnectionSetup_ReturnToSettingsFlag(t *testing.T) {
	m := newTestConnectionSetupModel(t, true)
	if !m.returnToSettings {
		t.Error("expected returnToSettings=true when created with flag")
	}
}

func TestConnectionSetup_EscFromChooseBackend_Quits(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command from esc")
	}
	msg := extractMsg(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg from esc at StepChooseBackend, got %T", msg)
	}
}

func TestConnectionSetup_EscFromChooseBackend_ReturnsToSettings(t *testing.T) {
	m := newTestConnectionSetupModel(t, true)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertNavigatesTo(t, cmd, StateSettings)
}

func TestConnectionSetup_SelectElectrum(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	// Cursor starts at 0 (Electrum). Press enter to select.
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(ConnectionSetupModel)
	if updated.step != StepConfigureElectrum {
		t.Errorf("expected step StepConfigureElectrum, got %d", updated.step)
	}
}

func TestConnectionSetup_CursorNavigation(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)

	// Move down
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := model.(ConnectionSetupModel)
	if updated.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", updated.cursor)
	}

	// Move down again (should not exceed bounds)
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated = model.(ConnectionSetupModel)
	if updated.cursor != 1 {
		t.Errorf("expected cursor=1 (clamped), got %d", updated.cursor)
	}

	// Move up
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated = model.(ConnectionSetupModel)
	if updated.cursor != 0 {
		t.Errorf("expected cursor=0 after up, got %d", updated.cursor)
	}
}

func TestConnectionSetup_EscFromConfigureElectrum_GoesBack(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	// Enter Electrum config
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(ConnectionSetupModel)
	if updated.step != StepConfigureElectrum {
		t.Fatalf("precondition: expected StepConfigureElectrum, got %d", updated.step)
	}
	// Press esc to go back
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated = model.(ConnectionSetupModel)
	if updated.step != StepChooseBackend {
		t.Errorf("expected StepChooseBackend after esc, got %d", updated.step)
	}
}

func TestConnectionSetup_WindowSizeMsg(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	updated := model.(ConnectionSetupModel)
	if updated.width != 100 {
		t.Errorf("expected width=100, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height=50, got %d", updated.height)
	}
}

func TestConnectionSetup_View_ShowsBackendOptions(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	view := m.View()
	expected := []string{
		"Connection Setup",
		"Electrum Server",
		"Bitcoin Core RPC",
	}
	for _, s := range expected {
		if !strings.Contains(view, s) {
			t.Errorf("connection setup view should contain %q", s)
		}
	}
}

func TestConnectionSetup_CtrlC_Quits(t *testing.T) {
	m := newTestConnectionSetupModel(t, false)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg := extractMsg(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg from ctrl+c, got %T", msg)
	}
}
