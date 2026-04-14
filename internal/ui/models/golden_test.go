//go:build golden

package models

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"hax/internal/api"
	"hax/internal/wallet"
)

func TestMain(m *testing.M) {
	// Force ASCII color profile for deterministic golden file output.
	// Without this, ANSI escape codes vary between local and CI environments.
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// Golden file tests snapshot key screens to catch visual regressions.
// Run with -tags golden and optional -update flag to regenerate golden files:
//   go test ./internal/ui/models/ -tags golden -run TestGolden -update

func TestGolden_Splash(t *testing.T) {
	m := NewSplashModel()
	m.width = 80
	m.height = 24
	// Set animation to complete state for deterministic output
	m.progress = 1.0
	m.frame = 0

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_Help(t *testing.T) {
	m := NewHelpModel(nil)
	m.width = 100
	m.height = 40
	// Settle animation for deterministic output
	m.fadeIn = 1.0
	// Update viewport dimensions to match
	m.viewport.Width = 96
	m.viewport.Height = 32
	m.viewport.SetContent(m.renderHelpContent())

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_Dashboard(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	mock.GetWalletNameFn = func() string { return "test-wallet" }
	mock.GetWalletTypeFn = func() string { return "Software" }
	mock.GetCachedBalanceFn = func() (api.Satoshi, bool) { return 123456, true }
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:      wallet.SyncStatusSynced,
			Balance:     123456,
			BlockHeight: 50000,
		}
	}

	m := NewMainDashboardModel(mock, nil)
	m.width = 100
	m.height = 30
	m.balance = 123456
	m.balanceLoaded = true
	m.loading = false

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_Dashboard_Multisig(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	mock.GetWalletNameFn = func() string { return "multisig-vault" }
	mock.GetWalletTypeFn = func() string { return "Multisig" }
	mock.IsMultisigFn = func() bool { return true }
	mock.GetMultisigConfigFn = func() *wallet.MultisigWalletConfig {
		return &wallet.MultisigWalletConfig{M: 2, N: 3}
	}
	mock.GetCachedBalanceFn = func() (api.Satoshi, bool) { return 500000, true }
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:      wallet.SyncStatusSynced,
			Balance:     500000,
			BlockHeight: 99999,
		}
	}

	m := NewMainDashboardModel(mock, nil)
	m.width = 100
	m.height = 30
	m.balance = 500000
	m.balanceLoaded = true
	m.loading = false

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_Unlock_PasswordEntry(t *testing.T) {
	mock := NewMockWalletManager()
	mock.ListWalletsFn = func() ([]string, error) {
		return []string{"my-wallet"}, nil
	}
	m := NewUnlockModel(mock)
	m.width = 80
	m.height = 24
	m.walletName = "my-wallet"
	m.walletList = []string{"my-wallet"}

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_Unlock_WalletSelection(t *testing.T) {
	mock := NewMockWalletManager()
	mock.ListWalletsFn = func() ([]string, error) {
		return []string{"wallet-alpha", "wallet-beta", "wallet-gamma"}, nil
	}
	m := NewUnlockModel(mock)
	m.width = 80
	m.height = 24
	m.walletList = []string{"wallet-alpha", "wallet-beta", "wallet-gamma"}
	m.walletName = "wallet-alpha"
	m.selectedWallet = 0
	m.cursor = 0
	// Blur password input to show wallet selection list
	m.passwordInput.Blur()

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_History_WithTransactions(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetTransactionLabelFn = func(txid string) string {
		if txid == "aabbccdd1234567890" {
			return "Coffee payment"
		}
		return ""
	}

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = false
	m.fadeIn = 1.0
	m.isActive = true
	m.blockHeight = 200

	// Use fixed timestamps (not time.Now()) for deterministic golden output.
	txs := []api.Transaction{
		{
			TxID:   "aabbccdd1234567890",
			Vin:    []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "external_addr", Value: 50000}}},
			Vout:   []api.Vout{{ScriptpubkeyAddress: "tb1q_our_addr", Value: 45000}},
			Status: api.Status{Confirmed: true, BlockHeight: 100, BlockTime: 1700000000},
		},
		{
			TxID: "eeff00112233445566",
			Vin:  []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "tb1q_our_addr", Value: 45000}}},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "external_dest", Value: 40000},
				{ScriptpubkeyAddress: "tb1q_our_change", Value: 4500},
			},
			Status: api.Status{Confirmed: false},
		},
	}
	m.transactions = txs
	m.updateTableData()

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}

func TestGolden_Receive_WithAddress(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", "m/84'/1'/0'/0/0", 0, nil
	}
	mock.GetCachedAddrBalanceFn = func(string) (api.Satoshi, bool) {
		return 0, true
	}
	mock.IsHardwareWalletFn = func() bool { return false }

	m := NewReceiveModel(mock)
	m.width = 80
	m.height = 30
	m.selectedAddress = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	m.derivationPath = "m/84'/1'/0'/0/0"
	m.currentIndex = 0
	m.revealProgress = 1.0 // Skip animation

	out := m.View()
	teatest.RequireEqualOutput(t, []byte(out))
}
