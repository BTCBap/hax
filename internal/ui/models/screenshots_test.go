//go:build screenshots

package models

// Renders key screens with true-color ANSI into $HAX_SCREENSHOT_DIR using mock
// wallet data (no network, no real keys). Consumed by docs/images/render-screens.sh.
//
//   HAX_SCREENSHOT_DIR=/tmp/hax-screens go test ./internal/ui/models -tags screenshots -run TestScreenshots
//
// Set HAX_SCREENSHOT_SIZES to render every screen at one or more terminal
// sizes, for example "80x24,120x40". Each screen is then written as
// <name>_<W>x<H>.ansi (true color) and <name>_<W>x<H>.txt (ANSI stripped).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

type screenshotSize struct {
	w, h int
}

type renderedScreen struct {
	name string
	view string
}

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

	sizes, multi := screenshotSizes(t)
	for _, size := range sizes {
		suffix := ""
		if multi {
			suffix = fmt.Sprintf("_%dx%d", size.w, size.h)
		}
		for _, screen := range renderScreens(size.w, size.h) {
			writeScreenshot(t, filepath.Join(dir, screen.name+suffix+".ansi"), screen.view)
			if multi {
				writeScreenshot(t, filepath.Join(dir, screen.name+suffix+".txt"), ansi.Strip(screen.view))
			}
		}
	}
}

// screenshotSizes returns the terminal sizes to render and whether the
// multi-size file naming applies. Without HAX_SCREENSHOT_SIZES it keeps the
// original 110x34 render and plain file names used by the docs images.
func screenshotSizes(t *testing.T) ([]screenshotSize, bool) {
	t.Helper()
	raw := os.Getenv("HAX_SCREENSHOT_SIZES")
	if raw == "" {
		return []screenshotSize{{110, 34}}, false
	}

	var sizes []screenshotSize
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		widthStr, heightStr, ok := strings.Cut(strings.ToLower(part), "x")
		if !ok {
			t.Fatalf("HAX_SCREENSHOT_SIZES: %q is not WxH", part)
		}
		w, err := strconv.Atoi(strings.TrimSpace(widthStr))
		if err != nil || w <= 0 {
			t.Fatalf("HAX_SCREENSHOT_SIZES: invalid width in %q", part)
		}
		h, err := strconv.Atoi(strings.TrimSpace(heightStr))
		if err != nil || h <= 0 {
			t.Fatalf("HAX_SCREENSHOT_SIZES: invalid height in %q", part)
		}
		sizes = append(sizes, screenshotSize{w: w, h: h})
	}
	if len(sizes) == 0 {
		t.Fatal("HAX_SCREENSHOT_SIZES has no sizes")
	}
	return sizes, true
}

func writeScreenshot(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// sized delivers a WindowSizeMsg so each model takes its width and height the
// same way it does in the running app.
func sized[M tea.Model](m M, w, h int) M {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(M)
}

// renderScreens builds the mock wallet and renders every screen at w×h.
func renderScreens(w, h int) []renderedScreen {
	sp := sized(NewSplashModel(), w, h)
	sp.progress = 1.0
	screens := []renderedScreen{{"splash", sp.View()}}

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
	d := sized(NewMainDashboardModel(mock, nil), w, h)
	d.balance, d.balanceLoaded, d.loading = balance, true, false
	screens = append(screens, renderedScreen{"dashboard", d.View()})

	// Send review: one recipient, 1 sat/vB (the medium tier), P2WPKH 1-in/2-out.
	sm := sized(NewSendModel(mock, balance, demoUTXOs), w, h)
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
	screens = append(screens, renderedScreen{"send", sm.View()})

	// One frozen coin, so the UTXO screen shows the marker.
	_ = mock.SetUTXOFrozen(txFaucet2, 0, true)
	um := sized(NewUTXOsModel(mock, nil), w, h)
	um.isActive, um.loading = true, false
	um.utxos = demoUTXOs
	um.sortAndUpdateTable()
	screens = append(screens, renderedScreen{"utxos", um.View()})

	rm := NewMockWalletManager()
	rm.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", "m/84'/1'/0'/0/0", 0, nil
	}
	rm.GetCachedAddrBalanceFn = func(string) (api.Satoshi, bool) { return 0, true }
	rm.IsHardwareWalletFn = func() bool { return false }
	r := sized(NewReceiveModel(rm), w, h)
	r.selectedAddress = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	r.derivationPath = "m/84'/1'/0'/0/0"
	r.revealProgress = 1.0
	screens = append(screens, renderedScreen{"receive", r.View()})

	hi := sized(NewTransactionHistoryModel(mock), w, h)
	hi.loading, hi.fadeIn, hi.isActive, hi.blockHeight = false, 1.0, true, 98_412
	hi.transactions = demoTxs
	hi.updateTableData()
	screens = append(screens, renderedScreen{"history", hi.View()})

	hp := sized(NewHelpModel(nil), w, h)
	hp.fadeIn = 1.0
	screens = append(screens, renderedScreen{"help", hp.View()})

	return screens
}
