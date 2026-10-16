package models

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/ui/alert"
	"github.com/BTCBap/hax/internal/wallet"
)

// BalanceChangeMsg is sent after calculating total wallet balance change
type BalanceChangeMsg struct {
	Change api.Satoshi
	State  wallet.WalletStateSnapshot
}

// PostBroadcastRefreshDoneMsg is sent after the post-broadcast lightweight refresh completes
// Carries the balance change so root.go can show a toast without waiting for Electrum
type PostBroadcastRefreshDoneMsg struct {
	BalanceChange api.Satoshi
	State         wallet.WalletStateSnapshot
}

// waitForNotification blocks until a notification arrives on either channel.
// Returns a NewTransactionNotification or NewBlockNotification message.
func waitForNotification(b backend.BitcoinBackend) tea.Msg {
	select {
	case n := <-b.ScripthashNotifications():
		return NewTransactionNotification{Address: n.Scripthash}
	case n := <-b.HeaderNotifications():
		return NewBlockNotification{Height: n.Height}
	}
}

// ContinueListening continues listening for more notifications
func ContinueListening(walletManager WalletService) tea.Cmd {
	return func() tea.Msg {
		debugLog("[LISTENER] ContinueListening called")
		b := walletManager.GetBackend()
		if b == nil || b.Type() != backend.BackendTypeElectrum {
			debugLog("[LISTENER] Backend is not Electrum in ContinueListening")
			return nil
		}

		debugLog("[LISTENER] Waiting for more notifications...")
		return waitForNotification(b)
	}
}

// StartListeningWithSubscriptions subscribes to block headers and then starts listening.
// This should be called once when the wallet is unlocked.
// SubscribeHeaders is called inline — errors are logged, then we proceed to listen
// for notifications regardless (scripthash notifications still work without headers).
func StartListeningWithSubscriptions(walletManager WalletService) tea.Cmd {
	return func() tea.Msg {
		debugLog("[LISTENER] StartListeningWithSubscriptions called")
		b := walletManager.GetBackend()
		if b == nil || b.Type() != backend.BackendTypeElectrum {
			debugLog("[LISTENER] Backend is not Electrum, returning nil")
			return nil
		}

		debugLog("[LISTENER] Subscribing to headers...")
		if _, err := b.SubscribeHeaders(); err != nil {
			debugLog("[LISTENER] SubscribeHeaders failed: %v", err)
		}

		debugLog("[LISTENER] Waiting for notifications...")
		return waitForNotification(b)
	}
}

// emitStateChanged returns a Cmd that emits StateChangedMsg with current state.
// This broadcasts the unified state to all screens for synchronization.
func emitStateChanged(walletManager WalletService) tea.Cmd {
	return func() tea.Msg {
		return StateChangedMsg{State: walletManager.GetState()}
	}
}

// asyncRefreshAndEmitState performs cache refresh and emits the updated state.
// Runs inside a tea.Cmd goroutine — no additional goroutine needed.
func asyncRefreshAndEmitState(walletManager WalletService) tea.Cmd {
	return func() tea.Msg {
		walletManager.RefreshCache()
		return StateChangedMsg{State: walletManager.GetState()}
	}
}

// calculateAndShowBalanceChange calculates total balance change and returns a message
func calculateAndShowBalanceChange(walletManager WalletService, alertModel *alert.AlertModel) tea.Cmd {
	return func() tea.Msg {
		balanceChange, err := walletManager.GetTotalBalanceChange()
		if err != nil {
			debugLog("[ROOT] Error calculating balance change: %v", err)
			balanceChange = 0
		}
		return BalanceChangeMsg{
			Change: balanceChange,
			State:  walletManager.GetState(),
		}
	}
}
