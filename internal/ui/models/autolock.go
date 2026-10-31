package models

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/config"
)

// idleCheckInterval is how often root checks for an idle unlocked wallet.
const idleCheckInterval = 15 * time.Second

// idleCheckMsg drives the auto-lock timer.
type idleCheckMsg time.Time

func scheduleIdleCheck() tea.Cmd {
	return tea.Tick(idleCheckInterval, func(t time.Time) tea.Msg { return idleCheckMsg(t) })
}

// autoLockAfter returns the configured idle timeout (0 = disabled).
func (m RootModel) autoLockAfter() time.Duration {
	if m.autoLockOverride != 0 {
		if m.autoLockOverride < 0 {
			return 0
		}
		return m.autoLockOverride
	}
	if m.configManager == nil {
		return config.DefaultAutoLockMinutes * time.Minute
	}
	cfg, err := m.configManager.Load()
	if err != nil {
		// Fail safe: an unreadable config must not disable auto-lock.
		debugLog("[AUTOLOCK] config load failed, using default timeout: %v", err)
		return config.DefaultAutoLockMinutes * time.Minute
	}
	return cfg.AutoLockAfter()
}

// shouldAutoLock reports whether the wallet has been idle long enough to
// lock. Screens that run before unlock, and a broadcast in progress, are
// never interrupted.
func (m RootModel) shouldAutoLock(now time.Time) bool {
	after := m.autoLockAfter()
	if after <= 0 || m.lastActivity.IsZero() || now.Sub(m.lastActivity) < after {
		return false
	}
	switch m.state {
	case StateSplash, StateWalletSetup, StateUnlock:
		return false
	case StateSend:
		if sm, ok := m.sendModel.(SendModel); ok && sm.step == SendStepBroadcasting {
			return false
		}
	}
	return m.walletManager != nil && m.walletManager.IsUnlocked()
}

// autoLock locks the wallet, drops every screen that may hold wallet data and
// shows the unlock screen with the same wallet preselected.
func (m RootModel) autoLock() (RootModel, tea.Cmd) {
	name := m.walletManager.GetWalletName()
	minutes := int(m.autoLockAfter().Round(time.Minute) / time.Minute)
	m.walletManager.LockWallet()
	clearClipboard()

	m.mainDashboardModel = nil
	m.receiveModel = nil
	m.sendModel = nil
	m.transactionHistoryModel = nil
	m.addressesModel = nil
	m.utxosModel = nil
	m.feesModel = nil
	m.settingsModel = nil
	m.helpModel = nil
	// These screens can hold a seed phrase, BIP39 passphrase, or RPC
	// credentials, so drop them rather than keeping them resident.
	m.walletSetupModel = nil
	m.connectionSetupModel = nil

	um := NewUnlockModel(m.walletManager)
	um.preferredWallet = name
	if minutes >= 1 {
		um.notice = fmt.Sprintf("Locked after %d minute%s of inactivity", minutes, pluralize(minutes))
	} else {
		um.notice = "Locked after inactivity"
	}
	m.unlockModel = um
	m.state = StateUnlock
	m.lastActivity = time.Time{}
	if m.width > 0 {
		sized, _ := m.unlockModel.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.unlockModel = sized
	}
	return m, m.unlockModel.Init()
}
