package models

import (
	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/ui/alert"
	"github.com/BTCBap/hax/internal/wallet"
)

// BalanceChangeMsg is sent after calculating total wallet balance change
type BalanceChangeMsg struct {
	Change api.Satoshi
	State  wallet.WalletStateSnapshot
	Gen    int // listener generation of the notification that caused it
	// FromBlock marks a check triggered by a polled block (Bitcoin Core);
	// the block handler already re-armed the listener.
	FromBlock bool
}

// PostBroadcastRefreshDoneMsg is sent after the post-broadcast lightweight refresh completes
// Carries the balance change so root.go can show a toast without waiting for Electrum
type PostBroadcastRefreshDoneMsg struct {
	BalanceChange api.Satoshi
	State         wallet.WalletStateSnapshot
}

// waitForNotification blocks until a notification arrives on either channel.
// Returns a NewTransactionNotification or NewBlockNotification message.
// It returns nil once stop is closed, which happens when the backend changes.
func waitForNotification(b backend.BitcoinBackend, gen int, stop <-chan struct{}) tea.Msg {
	select {
	case n := <-b.ScripthashNotifications():
		return NewTransactionNotification{Address: n.Scripthash, Gen: gen}
	case n := <-b.HeaderNotifications():
		return NewBlockNotification{Height: n.Height, Gen: gen}
	case <-stop:
		debugLog("[LISTENER] backend changed, stopping listener gen %d", gen)
		return nil
	}
}

// ContinueListening continues listening for more notifications
func ContinueListening(walletManager WalletService, gen int, stop <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		debugLog("[LISTENER] ContinueListening called")
		b := walletManager.GetBackend()
		if b == nil {
			return nil
		}

		debugLog("[LISTENER] Waiting for more notifications...")
		return waitForNotification(b, gen, stop)
	}
}

// StartListeningWithSubscriptions subscribes to block headers and then starts listening.
// This should be called once when the wallet is unlocked.
// Electrum pushes headers and address updates; Bitcoin Core RPC polls for
// new blocks. SubscribeHeaders is called inline — errors are logged, then we proceed to listen
// for notifications regardless (scripthash notifications still work without headers).
func StartListeningWithSubscriptions(walletManager WalletService, gen int, stop <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		debugLog("[LISTENER] StartListeningWithSubscriptions called")
		b := walletManager.GetBackend()
		if b == nil {
			return nil
		}

		debugLog("[LISTENER] Subscribing to headers...")
		if _, err := b.SubscribeHeaders(); err != nil {
			debugLog("[LISTENER] SubscribeHeaders failed: %v", err)
		}

		debugLog("[LISTENER] Waiting for notifications...")
		return waitForNotification(b, gen, stop)
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

// balanceCheckAfterBlock looks for a balance change after a new block on
// backends without address push notifications (Bitcoin Core RPC).
func balanceCheckAfterBlock(walletManager WalletService, gen int) tea.Cmd {
	return func() tea.Msg {
		change, err := walletManager.GetTotalBalanceChange()
		if err != nil {
			debugLog("[ROOT] Error calculating balance change after block: %v", err)
			change = 0
		}
		return BalanceChangeMsg{Change: change, State: walletManager.GetState(), Gen: gen, FromBlock: true}
	}
}

// calculateAndShowBalanceChange calculates total balance change and returns a message
func calculateAndShowBalanceChange(walletManager WalletService, alertModel *alert.AlertModel, gen int) tea.Cmd {
	return func() tea.Msg {
		balanceChange, err := walletManager.GetTotalBalanceChange()
		if err != nil {
			debugLog("[ROOT] Error calculating balance change: %v", err)
			balanceChange = 0
		}
		return BalanceChangeMsg{
			Change: balanceChange,
			State:  walletManager.GetState(),
			Gen:    gen,
		}
	}
}
