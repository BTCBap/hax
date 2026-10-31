package models

import (
	"testing"

	"github.com/BTCBap/hax/internal/config"
)

func TestRoot_StealthToggleBroadcastsAndPersists(t *testing.T) {
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cm.Save(&config.AppConfig{Version: 1, FirstRunComplete: true}); err != nil {
		t.Fatal(err)
	}
	m := NewRootModel(NewMockWalletManager(), cm)
	updated, _ := m.Update(NavigateMsg{Target: StateMainDashboard})
	m = updated.(RootModel)

	updated, _ = m.Update(keyRunes("z"))
	m = updated.(RootModel)
	if !m.stealth {
		t.Fatal("z did not enable stealth mode")
	}
	dashboard, ok := m.mainDashboardModel.(MainDashboardModel)
	if !ok || !dashboard.stealth {
		t.Fatal("dashboard did not receive the stealth toggle")
	}

	// New screens inherit the current state.
	updated, _ = m.Update(NavigateMsg{Target: StateTransactionHistory})
	m = updated.(RootModel)
	history, ok := m.transactionHistoryModel.(TransactionHistoryModel)
	if !ok || !history.stealth {
		t.Fatal("new history screen did not inherit stealth mode")
	}

	if cmd := persistStealth(cm, true); cmd != nil {
		cmd()
	}
	cfg, err := cm.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || !cfg.StealthMode {
		t.Fatal("stealth preference was not saved")
	}
}

func TestRoot_StealthIgnoresFocusedTextInput(t *testing.T) {
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := NewRootModel(NewMockWalletManager(), cm)
	updated, _ := m.Update(NavigateMsg{Target: StateUnlock})
	m = updated.(RootModel)
	unlock, ok := m.unlockModel.(UnlockModel)
	if !ok {
		t.Fatal("unlock model missing")
	}
	unlock.passwordInput.Focus()
	m.unlockModel = unlock

	updated, _ = m.Update(keyRunes("z"))
	m = updated.(RootModel)
	if m.stealth {
		t.Fatal("z toggled stealth while a text input was focused")
	}
}

func TestRoot_LoadsStealthFromConfig(t *testing.T) {
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cm.Save(&config.AppConfig{Version: 1, FirstRunComplete: true, StealthMode: true}); err != nil {
		t.Fatal(err)
	}
	m := NewRootModel(nil, cm)
	if !m.stealth {
		t.Fatal("stealth preference was not loaded from config")
	}
}
