package models

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		return 0
	}
	cfg, err := m.configManager.Load()
	if err != nil {
		return 0
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
	case StateSplash, StateConnectionSetup, StateWalletSetup, StateUnlock:
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

	m.mainDashboardModel = nil
	m.receiveModel = nil
	m.sendModel = nil
	m.transactionHistoryModel = nil
	m.addressesModel = nil
	m.utxosModel = nil
	m.settingsModel = nil
	m.helpModel = nil

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
