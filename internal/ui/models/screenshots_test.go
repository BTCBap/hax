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
	"time"

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

	// Demo data: plausible testnet4 txids/addresses (BIP173 test vectors and
	// fixed hex), fees as Electrum/Core report on a quiet testnet (2/1/1 sat/vB).
	const (
		txFaucet  = "3b7f2e9c1a4d8f60b5e2c7d93a1f4e8b6c0d2a5f7e9b1c3d5f7a9b1c3e5d7f90"
		txFaucet2 = "9d1c5a7e3f2b8d40c6a1e9f75b3d2c8a0e4f6b1d9c7a5e3f1b8d6c4a2e0f9b71"
		txSpend   = "c4e8a2f61d9b3c7e05a8f2d4b6c1e9a73f5d8b2c0e6a4f9d1b7c3e5a8f2d6b04"
		ourRecv   = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
		ourRecv2  = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7"
		ourChange = "tb1qqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesrxh6hy"
		faucet    = "tb1qm90ugl4d48jv8n6e5t9ln6t9zlpm5th68x4f8g"
		payee     = "tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l"
	)
	fees := &api.FeeEstimates{FastestFee: 2, HalfHourFee: 1, HourFee: 1, EconomyFee: 1, MinimumFee: 1}
	demoTxs := []api.Transaction{
		{
			TxID: txFaucet,
			Size: 222, Weight: 561, Fee: 141,
			Vin:    []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: faucet, Value: 1_500_000}}},
			Vout:   []api.Vout{{ScriptpubkeyAddress: ourRecv, Value: 1_500_000}},
			Status: api.Status{Confirmed: true, BlockHeight: 98_120, BlockTime: 1759830000},
		},
		{
			TxID: txFaucet2,
			Size: 222, Weight: 561, Fee: 141,
			Vin:    []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: faucet, Value: 700_000}}},
			Vout:   []api.Vout{{ScriptpubkeyAddress: ourRecv2, Value: 700_000}},
			Status: api.Status{Confirmed: true, BlockHeight: 98_300, BlockTime: 1760000000},
		},
		{
			TxID: txSpend,
			Size: 222, Weight: 561, Fee: 141,
			Vin: []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: ourRecv, Value: 1_500_000}}},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: payee, Value: 250_000},
				{ScriptpubkeyAddress: ourChange, Value: 1_249_859},
			},
			Status: api.Status{Confirmed: true, BlockHeight: 98_410, BlockTime: 1760090000},
		},
	}
	demoUTXOs := []api.UTXO{
		{TxID: txFaucet2, Vout: 0, Value: 700_000, Status: api.Status{Confirmed: true, BlockHeight: 98_300}},
		{TxID: txSpend, Vout: 1, Value: 1_249_859, Status: api.Status{Confirmed: true, BlockHeight: 98_410}},
	}
	var balance api.Satoshi
	for _, u := range demoUTXOs {
		balance += u.Value
	}
	accounts := func() []wallet.AccountData {
		return []wallet.AccountData{{Addresses: []wallet.AddressData{
			{Address: ourRecv}, {Address: ourRecv2}, {Address: ourChange, Change: true},
		}}}
	}
	labels := func(txid string) string {
		switch txid {
		case txFaucet, txFaucet2:
			return "Faucet"
		case txSpend:
			return "Test payment"
		}
		return ""
	}
	state := wallet.WalletStateSnapshot{
		Status:       wallet.SyncStatusSynced,
		Balance:      balance,
		BlockHeight:  98_412,
		LastSyncAt:   time.Now().Add(-20 * time.Second),
		Transactions: demoTxs,
		UTXOs:        demoUTXOs,
		FeeEstimates: fees,
	}

	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	mock.GetWalletNameFn = func() string { return "demo-testnet4" }
	mock.GetWalletTypeFn = func() string { return "Software" }
	mock.GetScriptTypeFn = func() (wallet.ScriptType, int, int) { return wallet.ScriptTypeP2WPKH, 0, 0 }
	mock.GetCachedBalanceFn = func() (api.Satoshi, bool) { return balance, true }
	mock.GetAccountsFn = accounts
	mock.GetTransactionLabelFn = labels
	mock.GetUTXOsFn = func(uint32) ([]api.UTXO, error) { return demoUTXOs, nil }
	mock.GetStateFn = func() wallet.WalletStateSnapshot { return state }
	d := NewMainDashboardModel(mock, nil)
	d.width, d.height = w, h
	d.balance, d.balanceLoaded, d.loading = balance, true, false
	write("dashboard", d.View())

	// Send review: one recipient, 1 sat/vB (the medium tier), P2WPKH 1-in/2-out.
	sm := NewSendModel(mock, balance, demoUTXOs)
	sm.width, sm.height = w, h
	sm.feeEstimates = fees
	sm.recipients = []Recipient{{Address: payee, Amount: 100_000}}
	sm.feeLevel = FeeLevelMedium
	sm.estimatedSize = 141
	sm.estimatedFee = 141
	sm.step = SendStepReview
	sm.psbtResult = &wallet.PSBTResult{
		Fee: 141, IntendedFee: 141, Change: 599_859, HasChange: true,
		ChangeAddress: ourChange, VSize: 141,
		SelectedUTXOs: []wallet.UTXOWithPath{{UTXO: demoUTXOs[0], Address: ourRecv2, Path: "m/84'/1'/0'/0/1"}},
	}
	write("send", sm.View())

	um := NewUTXOsModel(mock, nil)
	um.width, um.height, um.isActive, um.loading = w, h, true, false
	um.utxos = demoUTXOs
	um.sortAndUpdateTable()
	write("utxos", um.View())

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

	hi := NewTransactionHistoryModel(mock)
	hi.width, hi.height = w, h
	hi.loading, hi.fadeIn, hi.isActive, hi.blockHeight = false, 1.0, true, 98_412
	hi.transactions = demoTxs
	hi.updateTableData()
	write("history", hi.View())

	hp := NewHelpModel(nil)
	hp.width, hp.height, hp.fadeIn = w, h, 1.0
	hp.viewport.Width, hp.viewport.Height = w-4, h-8
	hp.viewport.SetContent(hp.renderHelpContent())
	write("help", hp.View())
}
