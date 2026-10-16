//go:build screenshots

package models

// Renders key screens with true-color ANSI into $HAX_SCREENSHOT_DIR using mock
// wallet data (no network, no real keys). Consumed by docs/images/screens.tape.
//
//   HAX_SCREENSHOT_DIR=/tmp/hax-screens go test ./internal/ui/models -tags screenshots -run TestScreenshots

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

func TestScreenshots(t *testing.T) {
	dir := os.Getenv("HAX_SCREENSHOT_DIR")
	if dir == "" {
		t.Skip("HAX_SCREENSHOT_DIR not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const w, h = 110, 34

	sp := NewSplashModel()
	sp.width, sp.height, sp.progress = w, h, 1.0
	write("splash", sp.View())

	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	mock.GetWalletNameFn = func() string { return "demo-testnet4" }
	mock.GetWalletTypeFn = func() string { return "Software" }
	mock.GetCachedBalanceFn = func() (api.Satoshi, bool) { return 2_150_000, true }
	demoTxs := []api.Transaction{
		{
			TxID:   "aabbccdd1234567890",
			Vin:    []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "external_addr", Value: 2_200_000}}},
			Vout:   []api.Vout{{ScriptpubkeyAddress: "tb1q_our_addr", Value: 2_200_000}},
			Status: api.Status{Confirmed: true, BlockHeight: 98_300, BlockTime: 1760000000},
		},
		{
			TxID: "eeff00112233445566",
			Vin:  []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "tb1q_our_addr", Value: 2_200_000}}},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "external_dest", Value: 49_000},
				{ScriptpubkeyAddress: "tb1q_our_change", Value: 2_150_000},
			},
			Status: api.Status{Confirmed: true, BlockHeight: 98_410, BlockTime: 1760090000},
		},
	}
	mock.GetAccountsFn = func() []wallet.AccountData {
		return []wallet.AccountData{{Addresses: []wallet.AddressData{
			{Address: "tb1q_our_addr"}, {Address: "tb1q_our_change", Change: true},
		}}}
	}
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:       wallet.SyncStatusSynced,
			Balance:      2_150_000,
			BlockHeight:  98_412,
			Transactions: demoTxs,
			UTXOs:        []api.UTXO{{TxID: "eeff00112233445566", Vout: 1, Value: 2_150_000, Status: api.Status{Confirmed: true}}},
			FeeEstimates: &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 5, EconomyFee: 2, MinimumFee: 1},
		}
	}
	d := NewMainDashboardModel(mock, nil)
	d.width, d.height = w, h
	d.balance, d.balanceLoaded, d.loading = 2_150_000, true, false
	write("dashboard", d.View())

	rm := NewMockWalletManager()
	rm.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", "m/84'/1'/0'/0/0", 0, nil
	}
	rm.GetCachedAddrBalanceFn = func(string) (api.Satoshi, bool) { return 0, true }
	rm.IsHardwareWalletFn = func() bool { return false }
	r := NewReceiveModel(rm)
	r.width, r.height = w, 44
	r.selectedAddress = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	r.derivationPath = "m/84'/1'/0'/0/0"
	r.revealProgress = 1.0
	write("receive", r.View())

	hm := NewMockWalletManager()
	hm.GetTransactionLabelFn = func(txid string) string {
		if txid == "aabbccdd1234567890" {
			return "Faucet"
		}
		return ""
	}
	hm.GetAccountsFn = func() []wallet.AccountData {
		return []wallet.AccountData{{Addresses: []wallet.AddressData{
			{Address: "tb1q_our_addr"}, {Address: "tb1q_our_change", Change: true},
		}}}
	}
	hi := NewTransactionHistoryModel(hm)
	hi.width, hi.height = w, h
	hi.loading, hi.fadeIn, hi.isActive, hi.blockHeight = false, 1.0, true, 200
	hi.transactions = []api.Transaction{
		{
			TxID:   "aabbccdd1234567890",
			Vin:    []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "external_addr", Value: 2_200_000}}},
			Vout:   []api.Vout{{ScriptpubkeyAddress: "tb1q_our_addr", Value: 2_200_000}},
			Status: api.Status{Confirmed: true, BlockHeight: 100, BlockTime: 1760000000},
		},
		{
			TxID: "eeff00112233445566",
			Vin:  []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "tb1q_our_addr", Value: 2_200_000}}},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "external_dest", Value: 49_000},
				{ScriptpubkeyAddress: "tb1q_our_change", Value: 2_150_000},
			},
			Status: api.Status{Confirmed: false},
		},
	}
	hi.updateTableData()
	write("history", hi.View())

	hp := NewHelpModel(nil)
	hp.width, hp.height, hp.fadeIn = w, h, 1.0
	hp.viewport.Width, hp.viewport.Height = w-4, h-8
	hp.viewport.SetContent(hp.renderHelpContent())
	write("help", hp.View())
}
