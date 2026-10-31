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

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

type screenshotSize struct {
	w, h int
}

type renderedScreen struct {
	name string
	view string
}

// docsScreens are the extra renders used by the README images. The multi-size
// harness skips them: their sizes are fixed for the docs layout.
var docsScreens = map[string]bool{"txflow": true, "verify": true, "psbt_inspect": true}

// docsPSBTInspectHeight is tall enough to show the whole sign view: the
// inspection box, the PSBT box, the signature progress panel, and the usage
// hints above the help line. The default 34-row terminal clips the progress
// panel in the README image.
const docsPSBTInspectHeight = 48

// docsTxFlowHeight gives the README image enough diagram rows for the
// sub-cell ribbons to curve gently between four outputs and two inputs. At 20
// rows the curves are too steep and read as staircases.
const docsTxFlowHeight = 28

func TestScreenshots(t *testing.T) {
	dir := os.Getenv("HAX_SCREENSHOT_DIR")
	if dir == "" {
		t.Skip("HAX_SCREENSHOT_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	sizes, multi := screenshotSizes(t)
	for _, size := range sizes {
		suffix := ""
		if multi {
			suffix = fmt.Sprintf("_%dx%d", size.w, size.h)
		}
		for _, screen := range renderScreens(t, size.w, size.h) {
			if multi && docsScreens[screen.name] {
				continue
			}
			writeScreenshot(t, filepath.Join(dir, screen.name+suffix+".ansi"), screen.view)
			if multi {
				writeScreenshot(t, filepath.Join(dir, screen.name+suffix+".txt"), ansi.Strip(screen.view))
			}
		}
	}
}

// addressScreenLayout pins the visible width of every line in the screens that
// render bitcoin addresses. The widths are the ones the layout had before the
// shared address format was applied, so a column shift or an added row fails
// here even when the terminal size is unchanged.
type addressExpectation struct {
	address string
	width   int
}

var addressScreenLayout = map[string]struct {
	widths []int
	addrs  []addressExpectation
}{
	"txflow": {
		widths: []int{19, 19, 19, 63, 22, 110, 0, 110, 68, 110, 68, 110, 68, 84, 0, 71, 38, 85, 32},
		addrs: []addressExpectation{
			{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", 27},
			{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", 62},
			{"tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l", 29},
		},
	},
	"txflow_many": {
		widths: []int{110, 0, 0, 0, 0, 0, 68, 68, 68, 68, 68, 110, 68, 110, 68, 110, 68, 110, 68, 84, 68, 68, 68, 68, 62, 0, 0, 0, 0, 0, 69, 37, 84, 32},
		addrs: []addressExpectation{
			{txFlowAddr(0), 27},
			{txFlowAddr(0), ansi.StringWidth(txFlowAddr(0))},
			{txFlowAddr(1), 27},
		},
	},
	"txflow_send": {
		widths: []int{19, 19, 19, 63, 22, 110, 0, 0, 0, 0, 68, 68, 68, 68, 68, 110, 68, 110, 68, 84, 68, 68, 68, 68, 62, 0, 0, 0, 0, 71, 38, 85, 32},
		addrs: []addressExpectation{
			{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", 27},
			{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", 62},
			{"tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l", 29},
		},
	},
	"txflow_history": {
		widths: []int{22, 110, 0, 0, 0, 0, 0, 68, 68, 68, 68, 68, 68, 110, 68, 110, 68, 84, 68, 68, 68, 68, 68, 68, 62, 0, 0, 0, 0, 51, 40, 26, 32},
		addrs: []addressExpectation{
			{"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", 27},
			{"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", 42},
			{"tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l", 29},
		},
	},
	"psbt_inspect": {
		widths: []int{23, 23, 23, 63, 32, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 110, 77},
		addrs: []addressExpectation{
			{"tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l", 0},
			{"tb1qqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesrxh6hy", 0},
		},
	},
	"utxos": {
		widths: []int{27, 0, 69, 0, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 76, 0, 80},
		addrs: []addressExpectation{
			{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", 12},
			{"tb1qqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesrxh6hy", 12},
		},
	},
	"addresses": {
		widths: []int{17, 17, 29, 0, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 104, 79},
		addrs: []addressExpectation{
			{"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", 43},
			{"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7", 43},
		},
	},
}

func TestScreenshotAddressScreensKeepLayout(t *testing.T) {
	rendered := make(map[string]renderedScreen)
	for _, screen := range renderScreens(t, 110, 34) {
		rendered[screen.name] = screen
	}

	for name, want := range addressScreenLayout {
		screen, ok := rendered[name]
		if !ok {
			t.Fatalf("screen %q was not rendered", name)
		}
		lines := strings.Split(screen.view, "\n")
		if len(lines) != len(want.widths) {
			t.Fatalf("%s rendered %d lines, want %d", name, len(lines), len(want.widths))
		}
		for i, line := range lines {
			if got := ansi.StringWidth(line); got != want.widths[i] {
				t.Errorf("%s line %d is %d cells wide, want %d: %q",
					name, i, got, want.widths[i], ansi.Strip(line))
			}
		}
		plain := ansi.Strip(screen.view)
		for _, addr := range want.addrs {
			if got := ansi.Strip(ui.FormatAddress(addr.address, addr.width)); !strings.Contains(plain, got) {
				t.Errorf("%s does not show formatted address %q:\n%s", name, got, plain)
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
func sized[M screenModel](m M, w, h int) M {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(M)
}

// screenshotConfigManager writes a throwaway config so the settings and
// connection screens show the server details a real wallet would.
func screenshotConfigManager(t *testing.T) *config.ConfigManager {
	t.Helper()
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.AppConfig{
		Version:          1,
		FirstRunComplete: true,
		Backend:          backend.DefaultElectrumConfig(wallet.NetworkTestnet),
	}
	if err := cm.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return cm
}

// renderScreens builds the mock wallet and renders every screen at w×h.
func renderScreens(t *testing.T, w, h int) []renderedScreen {
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
	// Keep the spend transaction by identity: the history table sorts its
	// slice in place, so its index changes.
	var spendTx api.Transaction
	for _, tx := range demoTxs {
		if tx.TxID == txSpend {
			spendTx = tx
		}
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

	// Onboarding screens. The seed is the public BIP39 test vector, never a
	// generated phrase.
	const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	setup := sized(NewWalletSetupModel(mock, nil), w, h)
	setup.network = wallet.NetworkTestnet
	setup.cursor = 1
	screens = append(screens, renderedScreen{"setup_network", setup.View()})
	setup.step = StepChooseAction
	setup.cursor = 0
	screens = append(screens, renderedScreen{"setup_choice", setup.View()})
	setup.step = StepShowMnemonic
	setup.wordCount = 12
	setup.generatedMnemonic = testMnemonic
	screens = append(screens, renderedScreen{"setup_seed", setup.View()})
	setup.step = StepConfirmMnemonic
	setup.startBackupCheck()
	setup.backupWords = []int{3, 7, 11}
	setup.backupPos = 0
	setup.backupInput.Blur()
	screens = append(screens, renderedScreen{"setup_quiz", setup.View()})

	unlock := sized(NewUnlockModel(mock), w, h)
	unlock.walletName = "demo-testnet4"
	screens = append(screens, renderedScreen{"unlock", unlock.View()})

	d := sized(NewMainDashboardModel(mock, nil), w, h)
	d.balance, d.balanceLoaded, d.loading = balance, true, false
	screens = append(screens, renderedScreen{"dashboard", d.View()})

	lightView := func() string {
		styles.SetDarkBackground(false)
		defer styles.SetDarkBackground(true)
		light := sized(NewMainDashboardModel(mock, nil), w, h)
		light.balance, light.balanceLoaded, light.loading = balance, true, false
		return light.View()
	}()
	screens = append(screens, renderedScreen{"dashboard_light", lightView})

	dStealth := d
	dStealth.stealth = true
	screens = append(screens, renderedScreen{"dashboard_stealth", dStealth.View()})

	fm := sized(NewFeesModel(mock, nil), w, h)
	fm.serverName = "electrum.example.com"
	fm.estimates = fees
	fm.refreshing = false
	fm.err = nil
	fm.updatedAt = time.Now().Add(-12 * time.Second)
	screens = append(screens, renderedScreen{"fees", fm.View()})

	mock.HasPassphraseFn = func() bool { return true }
	dPassphrase := d
	screens = append(screens, renderedScreen{"dashboard_passphrase", dPassphrase.View()})
	mock.HasPassphraseFn = nil

	// Send address and amount steps.
	sendAddr := sized(NewSendModel(mock, balance, demoUTXOs), w, h)
	sendAddr.addressInput.SetValue(payee)
	sendAddr.addressInput.Blur()
	screens = append(screens, renderedScreen{"send_address", sendAddr.View()})

	sendAmt := sized(NewSendModel(mock, balance, demoUTXOs), w, h)
	sendAmt.step = SendStepAmount
	sendAmt.recipientAddress = payee
	sendAmt.amountInput.SetValue("0.00100000")
	sendAmt.amountInput.Blur()
	screens = append(screens, renderedScreen{"send_amount", sendAmt.View()})

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
	screens = append(screens, renderedScreen{"send_review", sm.View()})

	// Fee priority step: helpers line up in the column to the right of each
	// target, like the inline speed-up tray.
	fee := sized(NewSendModel(mock, balance, demoUTXOs), w, h)
	fee.feeEstimates = fees
	fee.step = SendStepFee
	fee.cursor = cursorFromFeeLevel(FeeLevelMedium)
	fee.estimatedSize = 141
	fee.estimatedFee = 141
	screens = append(screens, renderedScreen{"send_fee", fee.View()})

	// Hardware wallet picker: an unlocked Coldcard, a selected Trezor that
	// needs a PIN, and a Ledger whose multi-line error is truncated to the
	// status column.
	hw := sized(NewSendModel(mock, balance, demoUTXOs), w, h)
	hw.showPSBTExport = true
	hw.showHWIDevices = true
	hw.loading = false
	hw.hwiDevices = []wallet.HardwareDevice{
		{Type: wallet.HWTypeColdcard, Model: "Coldcard Mk4", Fingerprint: "cafebabe", Path: "usb:1-1"},
		{Type: wallet.HWTypeTrezor, Model: "Trezor One", Fingerprint: "abcd1234", Path: "udp:127.0.0.1:21324", NeedsPin: true},
		{
			Type:        wallet.HWTypeLedger,
			Model:       "Ledger Nano X",
			Fingerprint: "deadbeef",
			Path:        "usb:1-2",
			Error: "Could not connect to device:\n" +
				"the user did not confirm the request on the device screen\n" +
				"before the transport closed unexpectedly",
		},
	}
	hw.selectedHWDevice = 1
	screens = append(screens, renderedScreen{"send_hw_devices", hw.View()})

	// The same review PSBT, shown as a flow diagram.
	sm.inspection = &wallet.PSBTInspection{
		Inputs: []wallet.InspectedInput{{
			OutPoint: txSpend + ":0", Address: ourRecv2,
			Value: 700_000, ValueKnown: true, Mine: true,
		}},
		Outputs: []wallet.InspectedOutput{
			{Address: payee, Value: 100_000, Owner: wallet.OutputExternal},
			{Address: ourChange, Value: 599_859, Owner: wallet.OutputChange},
		},
		Fee: 141, FeeKnown: true, FeeRate: 1, VSize: 141,
	}
	sm.openInspectionFlow()
	screens = append(screens, renderedScreen{"txflow_send", sm.View()})

	// The same PSBT in the inspector without the flow diagram: the PSBT text,
	// signature progress, and "What you're signing" rows.
	inspect := sized(NewSendModel(mock, balance, demoUTXOs), w, docsPSBTInspectHeight)
	inspect.step = SendStepReview
	inspect.showPSBTExport = true
	inspect.psbtBase64 = "cHNidP8BAHECAAAAAXiq8Xx3"
	inspect.psbtSigsTotal = 1
	inspect.psbtResult = &wallet.PSBTResult{
		Fee: 141, IntendedFee: 141, Change: 599_859, HasChange: true,
		ChangeAddress: ourChange, VSize: 141,
	}
	inspect.inspection = sm.inspection
	// The viewport only syncs when the model sees a window size while the
	// export view is open; size it again now that the view is active.
	inspect.syncScrollViewport()
	screens = append(screens, renderedScreen{"psbt_inspect", inspect.View()})

	// Compact copy for the README: a batch send spending two of our coins to
	// two external payees with change back. Both sides have four rows (two
	// inputs, two payees + change + fee), so docsTxFlowHeight leaves the
	// visualizer exactly enough rows for the spacer lines between them.
	batch := sized(NewSendModel(mock, balance, demoUTXOs), w, docsTxFlowHeight)
	batch.feeEstimates = fees
	batch.recipients = []Recipient{
		{Address: payee, Amount: 150_000},
		{Address: faucet, Amount: 75_000},
	}
	batch.feeLevel = FeeLevelMedium
	batch.estimatedSize = 302
	batch.estimatedFee = 302
	batch.step = SendStepReview
	batch.psbtResult = &wallet.PSBTResult{
		Fee: 302, IntendedFee: 302, Change: 1_724_557, HasChange: true,
		ChangeAddress: ourChange, VSize: 302,
		SelectedUTXOs: []wallet.UTXOWithPath{
			{UTXO: demoUTXOs[0], Address: ourRecv2, Path: "m/84'/1'/0'/0/1"},
			{UTXO: demoUTXOs[1], Address: ourChange, Path: "m/84'/1'/0'/1/0"},
		},
	}
	batch.inspection = &wallet.PSBTInspection{
		Inputs: []wallet.InspectedInput{
			{
				OutPoint: txFaucet2 + ":0", Address: ourRecv2,
				Value: 700_000, ValueKnown: true, Mine: true,
			},
			{
				OutPoint: txSpend + ":1", Address: ourChange,
				Value: 1_249_859, ValueKnown: true, Mine: true,
			},
		},
		Outputs: []wallet.InspectedOutput{
			{Address: payee, Value: 150_000, Owner: wallet.OutputExternal},
			{Address: faucet, Value: 75_000, Owner: wallet.OutputExternal},
			{Address: ourChange, Value: 1_724_557, Owner: wallet.OutputChange},
		},
		Fee: 302, FeeKnown: true, FeeRate: 1, VSize: 302,
	}
	batch.openInspectionFlow()
	screens = append(screens, renderedScreen{"txflow", batch.View()})

	// One frozen coin, so the UTXO screen shows the marker.
	_ = mock.SetUTXOFrozen(txFaucet2, 0, true)
	um := sized(NewUTXOsModel(mock, nil), w, h)
	um.isActive, um.loading = true, false
	um.utxos = demoUTXOs
	um.sortAndUpdateTable()
	screens = append(screens, renderedScreen{"utxos", um.View()})

	// Address list: two used receive addresses and the change address.
	am := sized(NewAddressesModel(mock, nil), w, h)
	am.receiveAddresses = []AddrDisplayInfo{
		{Address: ourRecv, Path: "m/84'/1'/0'/0/0", Index: 0, Used: true, Label: "Faucet"},
		{Address: ourRecv2, Path: "m/84'/1'/0'/0/1", Index: 1, Used: true, Balance: 700_000, Label: "Savings"},
	}
	am.changeAddresses = []AddrDisplayInfo{
		{Address: ourChange, Path: "m/84'/1'/0'/1/0", Index: 0, Used: true, Balance: 1_249_859, Label: "Change"},
	}
	am.loading = false
	am.updateTable()
	screens = append(screens, renderedScreen{"addresses", am.View()})

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

	// Receive screen with the on-device verification panel in each result state.
	hwm := NewMockWalletManager()
	hwm.IsHardwareWalletFn = func() bool { return true }
	hwm.IsMultisigFn = func() bool { return false }
	for _, verify := range []struct {
		name  string
		state AddressVerifyState
	}{
		{name: "verify_waiting", state: AddressVerifyWaiting},
		{name: "verify_match", state: AddressVerifyMatch},
		{name: "verify_mismatch", state: AddressVerifyMismatch},
	} {
		hm := sized(NewReceiveModel(hwm), w, h)
		hm.selectedAddress = verifyTestAddress
		hm.derivationPath = verifyTestPath
		hm.revealProgress = 1.0
		hm.verify.Reset(verifyTestAddress, verifyTestPath)
		hm.verify.Begin()
		switch verify.state {
		case AddressVerifyWaiting:
			hm.verify.Frame = 3
			hm.verify.StartedAt = time.Now().Add(-2 * time.Second)
		case AddressVerifyMatch:
			hm.verify.Complete(nil)
		case AddressVerifyMismatch:
			hm.verify.Complete(&wallet.AddressMismatchError{
				Displayed: verifyTestDevice,
				Expected:  verifyTestAddress,
			})
		}
		screens = append(screens, renderedScreen{verify.name, hm.View()})
		if verify.state == AddressVerifyMatch {
			screens = append(screens, renderedScreen{"verify", hm.View()})
		}
	}

	hi := sized(NewTransactionHistoryModel(mock), w, h)
	hi.loading, hi.fadeIn, hi.isActive, hi.blockHeight = false, 1.0, true, 98_412
	hi.transactions = demoTxs
	hi.updateTableData()
	screens = append(screens, renderedScreen{"history", hi.View()})

	// The demo spend transaction opened as a flow diagram: a payee output and
	// a change output back to the wallet.
	hi.openTransactionFlow(spendTx)
	screens = append(screens, renderedScreen{"txflow_history", hi.View()})

	// Inline speed-up tray on a pending outgoing transaction.
	speedMock := NewMockWalletManager()
	speedChoose := newSpeedUpHistoryLayout(speedMock, false)
	screens = append(screens, renderedScreen{"history_speedup_tray", sized(speedChoose, w, h).View()})
	speedConfirm := newSpeedUpHistoryLayout(NewMockWalletManager(), true)
	screens = append(screens, renderedScreen{"history_speedup_confirm", sized(speedConfirm, w, h).View()})

	// A busy transaction so the diagram shows how many inputs and outputs
	// are laid out together.
	screens = append(screens, renderedScreen{"txflow_many", TxFlowView{
		Flow: txFlowFixture(9, 4), Width: w, Height: h,
	}.View()})

	// The visualizer at the two other supported terminal sizes.
	screens = append(screens, renderedScreen{"txflow_80", TxFlowView{
		Flow: txFlowExample(), Width: 80, Height: 24,
	}.View()})
	screens = append(screens, renderedScreen{"txflow_120", TxFlowView{
		Flow: txFlowExample(), Width: 120, Height: 40,
	}.View()})

	hp := sized(NewHelpModel(nil), w, h)
	hp.fadeIn = 1.0
	screens = append(screens, renderedScreen{"help", hp.View()})

	// Settings and connection setup share a throwaway config so the server
	// rows read like a real testnet4 install.
	settingsMock := NewMockWalletManager()
	settingsMock.IsUnlockedFn = func() bool { return true }
	settingsMock.GetWalletNameFn = func() string { return "demo-testnet4" }
	settingsMock.GetWalletTypeFn = func() string { return "Software" }
	settingsMock.GetNetworkFn = func() string { return wallet.NetworkTestnet }
	settingsMock.GetCreatedAtFn = func() int64 { return 1759830000 }
	settingsMock.GetScriptTypeFn = func() (wallet.ScriptType, int, int) {
		return wallet.ScriptTypeP2WPKH, 0, 0
	}
	settingsMock.GetAccountsFn = func() []wallet.AccountData {
		return []wallet.AccountData{{
			Index:       0,
			Name:        "Main Account",
			AddressType: "segwit_native",
			Addresses: []wallet.AddressData{
				{Index: 0, Address: ourRecv, Path: "m/84'/1'/0'/0/0", Used: true},
				{Index: 1, Address: ourRecv2, Path: "m/84'/1'/0'/0/1", Used: true},
				{Address: ourChange, Path: "m/84'/1'/0'/1/0", Change: true, Used: true},
			},
		}}
	}
	settingsMock.GetExtendedPublicKeyFn = func(uint32) (string, error) {
		return "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M", nil
	}
	settingsCM := screenshotConfigManager(t)
	settingsScreen := sized(NewSettingsModel(settingsMock, settingsCM), w, h)
	screens = append(screens, renderedScreen{"settings", settingsScreen.View()})

	serverScreen := sized(NewConnectionSetupModel(settingsCM, true), w, h)
	serverScreen.step = StepConfigureElectrum
	serverScreen.cursor = 0
	screens = append(screens, renderedScreen{"settings_server", serverScreen.View()})

	// Multisig cosigner list with signing status.
	msConfig := &wallet.MultisigWalletConfig{
		M: 2, N: 3, ScriptType: int(wallet.MultisigNativeSegwit),
		Cosigners: []wallet.MultisigCosignerConfig{
			{
				Name: "Coldcard", Fingerprint: "0f056943",
				KeyOrigin: "0f056943/48h/1h/0h/2h", DeviceType: "coldcard",
				Xpub: "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M",
			},
			{
				Name: "Trezor", Fingerprint: "abcd1234",
				KeyOrigin: "abcd1234/48h/1h/0h/2h", DeviceType: "trezor",
				Xpub: "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1N",
			},
			{
				Name: "This device", Fingerprint: "cafebabe",
				KeyOrigin: "cafebabe/48h/1h/0h/2h", IsMine: true, DeviceType: "local",
				Mnemonic: testMnemonic,
				Xpub:     "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1P",
			},
		},
	}
	msMock := NewMockWalletManager()
	msMock.GetWalletNameFn = func() string { return "demo-multisig" }
	msMock.GetNetworkFn = func() string { return wallet.NetworkTestnet }
	msMock.IsMultisigFn = func() bool { return true }
	msMock.GetMultisigConfigFn = func() *wallet.MultisigWalletConfig { return msConfig }
	msMock.GetCosignersFn = func() []wallet.MultisigCosignerConfig { return msConfig.Cosigners }
	msMock.GetMultisigSigningStatFn = func() (int, int, int) { return 1, 1, 1 }
	msScreen := sized(NewSettingsModel(msMock, nil), w, h)
	msScreen.showCosignerList = true
	screens = append(screens, renderedScreen{"multisig", msScreen.View()})

	labelScreen := sized(NewSettingsModel(mock, nil), w, h)
	labelScreen.showLabelImport = true
	labelScreen.labelImportStage = labelImportStagePreview
	labelScreen.labelImportPreview = bip329Preview{Tx: 3, Addr: 2, Output: 1, Frozen: 1}
	labelScreen.labelImportPath.SetValue("~/.hax/demo-testnet4_labels_bip329.jsonl")
	screens = append(screens, renderedScreen{"labels_import_preview", labelScreen.View()})

	backupScreen := sized(NewSettingsModel(mock, nil), w, h)
	backupScreen.openBackupExport()
	backupScreen.backupTray, _ = backupScreen.backupTray.Update(keyCode(tea.KeyEnter))
	backupScreen.backupExportStage = backupExportStageConfirm
	screens = append(screens, renderedScreen{"backup_export_confirm", backupScreen.View()})

	return screens
}
