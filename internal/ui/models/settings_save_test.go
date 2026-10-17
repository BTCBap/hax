package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSettings_SaveFailureIsShown(t *testing.T) {
	dir := t.TempDir()
	// A directory where config.json should be makes every load/save fail.
	if err := os.Mkdir(filepath.Join(dir, "config.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	cm, _ := config.NewConfigManager(dir)
	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	m := NewSettingsModel(mock, cm)
	m.width, m.height = 100, 40

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	sm := model.(SettingsModel)
	if !strings.HasPrefix(sm.copyMessage, "Not saved") {
		t.Fatalf("status = %q, want a not-saved message", sm.copyMessage)
	}
}

func TestConnectionSetup_FromSettingsAppliesServer(t *testing.T) {
	m := NewConnectionSetupModel(nil, true)
	m.step = StepConnectionComplete
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no command")
	}
	msg, ok := cmd().(BackendChosenMsg)
	if !ok || msg.Then != StateSettings {
		t.Fatalf("got %#v, want BackendChosenMsg back to settings", msg)
	}
}

func TestRoot_FailedBackendChangeKeepsListener(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	r := NewRootModel(mock, nil)
	gen, stop := r.listenGen, r.listenStop
	model, cmd := r.Update(BackendChangedMsg{Then: StateSettings, Err: os.ErrDeadlineExceeded})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected an error alert")
	}
	if r.listenGen != gen || r.listenStop != stop {
		t.Fatal("listener restarted although the server did not change")
	}
}
