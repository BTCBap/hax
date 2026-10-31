package models

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

type viewPosition struct {
	line int
	col  int
}

func requireTextPosition(t *testing.T, view, text string) viewPosition {
	t.Helper()
	lines := strings.Split(stripANSI(view), "\n")
	line, col, ok := findChoiceLine(lines, text)
	if !ok {
		t.Fatalf("%q not found in view:\n%s", text, stripANSI(view))
	}
	return viewPosition{line: line, col: col}
}

func requireLineCount(t *testing.T, view string) int {
	t.Helper()
	return len(strings.Split(stripANSI(view), "\n"))
}

// requireHelperOnSelectedLine checks that helper sits on the same row as name.
func requireHelperOnSelectedLine(t *testing.T, view, name, helper string) {
	t.Helper()
	lines := strings.Split(stripANSI(view), "\n")
	line, _, ok := findChoiceLine(lines, name)
	if !ok {
		t.Fatalf("%q not found in view:\n%s", name, stripANSI(view))
	}
	if !strings.Contains(lines[line], helper) {
		t.Fatalf("helper %q is not on the row of %q:\n%s", helper, name, stripANSI(view))
	}
}

// The backup word check asks for words with single- and double-digit numbers.
// Its title, progress, prompt, input and help must stay on the same rows and
// columns no matter which word is asked, whether an answer was typed, or
// whether the wrong-word error is shown.
func TestSeedQuizLayoutStable(t *testing.T) {
	cases := []struct {
		name  string
		pos   int
		input string
		err   error
	}{
		{name: "word 3 empty", pos: 0},
		{name: "word 3 typed", pos: 0, input: "zoo"},
		{name: "word 3 error", pos: 0, err: fmt.Errorf("that is not word #3; check your backup or press esc to see the seed again")},
		{name: "word 12 empty", pos: 1},
		{name: "word 12 typed", pos: 1, input: "zoo"},
		{name: "word 12 error", pos: 1, err: fmt.Errorf("that is not word #12; check your backup or press esc to see the seed again")},
	}

	var wantTitle, wantPrompt, wantProgress, wantInput viewPosition
	var wantLines int
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newBackupModel(t)
			m.backupWords = []int{2, 11} // word #3 and word #12
			m.backupPos = tc.pos
			m.backupInput.SetValue(tc.input)
			m.err = tc.err

			view := m.View()
			title := requireTextPosition(t, view, "Check your backup")
			progress := requireTextPosition(t, view, " of 2")
			prompt := requireTextPosition(t, view, "Enter word #")
			input := requireTextPosition(t, view, "> ")
			lines := requireLineCount(t, view)

			if progress.line != title.line {
				t.Fatalf("progress at %+v is not on the title line %+v", progress, title)
			}

			if i == 0 {
				wantTitle, wantProgress = title, progress
				wantPrompt, wantInput, wantLines = prompt, input, lines
				return
			}
			if title != wantTitle {
				t.Fatalf("title moved from %+v to %+v", wantTitle, title)
			}
			if progress != wantProgress {
				t.Fatalf("progress moved from %+v to %+v", wantProgress, progress)
			}
			if prompt != wantPrompt {
				t.Fatalf("prompt moved from %+v to %+v", wantPrompt, prompt)
			}
			if input != wantInput {
				t.Fatalf("input moved from %+v to %+v", wantInput, input)
			}
			if lines != wantLines {
				t.Fatalf("view has %d lines, want %d", lines, wantLines)
			}
		})
	}
}

func TestUnlockPasswordLayoutStable(t *testing.T) {
	cases := []struct {
		name  string
		input string
		err   error
	}{
		{name: "empty", input: ""},
		{name: "typed", input: "secret"},
		{name: "error", input: "", err: fmt.Errorf("incorrect password")},
		{name: "typed with error", input: "secret", err: fmt.Errorf("incorrect password")},
	}

	var wantTitle, wantPrompt, wantInput viewPosition
	var wantLines int
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewUnlockModel(NewMockWalletManager())
			m.width = 80
			m.height = 24
			m.walletList = []string{"my-wallet"}
			m.walletName = "my-wallet"
			m.passwordInput.Focus()
			m.passwordInput.SetValue(tc.input)
			m.err = tc.err

			view := m.View()
			title := requireTextPosition(t, view, "Unlock Wallet")
			prompt := requireTextPosition(t, view, "Password:")
			input := requireTextPosition(t, view, "> ")
			lines := requireLineCount(t, view)

			if i == 0 {
				wantTitle, wantPrompt = title, prompt
				wantInput, wantLines = input, lines
				return
			}
			if title != wantTitle {
				t.Fatalf("title moved from %+v to %+v", wantTitle, title)
			}
			if prompt != wantPrompt {
				t.Fatalf("password prompt moved from %+v to %+v", wantPrompt, prompt)
			}
			if input != wantInput {
				t.Fatalf("password input moved from %+v to %+v", wantInput, input)
			}
			if lines != wantLines {
				t.Fatalf("view has %d lines, want %d", lines, wantLines)
			}
		})
	}
}

// Moving through every input and output of the visualizer must not change
// the line count or move either header.
func TestTxFlowLayoutStable(t *testing.T) {
	v := TxFlowView{Flow: txFlowFixture(9, 4), Width: 80, Height: 24}
	base := v.View()
	wantLines := requireLineCount(t, base)
	wantInputs := requireTextPosition(t, base, "Inputs")
	wantOutputs := requireTextPosition(t, base, "Outputs")

	for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
		v.SwitchSide(side)
		for i := 0; i < len(v.itemsFor(side)); i++ {
			view := v.View()
			if got := requireLineCount(t, view); got != wantLines {
				t.Fatalf("%s cursor %d: %d lines, want %d", sideName(side), i, got, wantLines)
			}
			if got := requireTextPosition(t, view, "Inputs"); got != wantInputs {
				t.Fatalf("%s cursor %d: Inputs header moved from %+v to %+v",
					sideName(side), i, wantInputs, got)
			}
			if got := requireTextPosition(t, view, "Outputs"); got != wantOutputs {
				t.Fatalf("%s cursor %d: Outputs header moved from %+v to %+v",
					sideName(side), i, wantOutputs, got)
			}

			v.MoveDown()
		}
	}
}

func TestSendAmountLayoutStable(t *testing.T) {
	cases := []struct {
		name  string
		input string
		err   error
	}{
		{name: "empty", input: ""},
		{name: "typed", input: "0.001"},
		{name: "error", input: "", err: fmt.Errorf("amount is too low")},
		{name: "typed with error", input: "0.001", err: fmt.Errorf("amount is too low")},
	}

	var wantHeader, wantPrompt viewPosition
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestSend(t)
			advanceToAmount(&m)
			m.width = 100
			m.height = 40
			m.amountInput.SetValue(tc.input)
			m.err = tc.err

			view := m.View()
			header := requireTextPosition(t, view, "Send Bitcoin")
			prompt := requireTextPosition(t, view, "Enter amount in BTC:")

			if i == 0 {
				wantHeader, wantPrompt = header, prompt
				return
			}
			if header != wantHeader {
				t.Fatalf("header moved from %+v to %+v", wantHeader, header)
			}
			if prompt != wantPrompt {
				t.Fatalf("amount prompt moved from %+v to %+v", wantPrompt, prompt)
			}
		})
	}
}

func TestConnectionRPCFormLayoutStable(t *testing.T) {
	cases := []string{"", "node.example.com", "a-very-long-host-name.example.onion"}
	var wantHost, wantUser viewPosition
	var wantLines int
	for i, host := range cases {
		m := NewConnectionSetupModel(nil, false)
		m.width = 80
		m.height = 40
		m.step = StepConfigureRPC
		m.hostInput.SetValue(host)

		view := m.View()
		hostLabel := requireTextPosition(t, view, "Host:")
		userLabel := requireTextPosition(t, view, "Username:")
		lines := requireLineCount(t, view)

		if i == 0 {
			wantHost, wantUser, wantLines = hostLabel, userLabel, lines
			continue
		}
		if hostLabel != wantHost {
			t.Fatalf("host field moved from %+v to %+v", wantHost, hostLabel)
		}
		if userLabel != wantUser {
			t.Fatalf("username field moved from %+v to %+v", wantUser, userLabel)
		}
		if lines != wantLines {
			t.Fatalf("view has %d lines, want %d", lines, wantLines)
		}
	}
}

// Every step the create flow can show must have a step number; a missing case
// renders "Step 0/0" and makes the progress bar jump backwards.
func TestCreateFlowProgressHasNoZeroStep(t *testing.T) {
	steps := []WalletSetupStep{
		StepChooseNetwork, StepChooseAction, StepChooseWalletType,
		StepEnterName, StepEnterPassword, StepConfirmPassword,
		StepChooseScriptType, StepChooseKeySource,
		StepEnterPassphrase, StepConfirmPassphrase, StepChooseWordCount,
		StepShowMnemonic, StepConfirmMnemonic,
		StepHWISetup, StepHardwareWalletConnect, StepHardwareWalletReview,
	}
	m := NewWalletSetupModel(NewMockWalletManager(), nil)
	m.width, m.height = 100, 40
	for _, step := range steps {
		m.step = step
		view := stripANSI(m.View())
		if strings.Contains(view, "Step 0/0") {
			t.Fatalf("step %d renders Step 0/0:\n%s", step, view)
		}
	}
}

// utxoLayoutModel builds a coins screen with three distinguishable rows:
// a confirmed change coin, an unconfirmed coin, and a dust coin. Addresses
// come from the cached transactions, exactly as the screen reads them.
func utxoLayoutModel(t *testing.T, width, height int) UTXOsModel {
	t.Helper()
	changeAddr := "tb1qchange000000000000000000000000000000000"
	receiveAddr := "tb1qreceive00000000000000000000000000000000"
	now := time.Now().Unix()
	txA := strings.Repeat("aa", 32)
	txB := strings.Repeat("bb", 32)
	txC := strings.Repeat("cc", 32)

	txs := []api.Transaction{
		{
			TxID:   txA,
			Vout:   []api.Vout{{ScriptpubkeyAddress: changeAddr, Value: 50_000}},
			Status: api.Status{Confirmed: true, BlockHeight: 90, BlockTime: now - 7200},
		},
		{
			TxID:   txB,
			Vout:   []api.Vout{{ScriptpubkeyAddress: changeAddr, Value: 300}},
			Status: api.Status{Confirmed: true, BlockHeight: 91, BlockTime: now - 10800},
		},
		{
			TxID:   txC,
			Vout:   []api.Vout{{ScriptpubkeyAddress: receiveAddr, Value: 30_000}},
			Status: api.Status{Confirmed: false},
		},
	}
	utxos := []api.UTXO{
		{TxID: txA, Vout: 0, Value: 50_000, Status: api.Status{Confirmed: true, BlockHeight: 90}},
		{TxID: txB, Vout: 0, Value: 300, Status: api.Status{Confirmed: true, BlockHeight: 91}},
		{TxID: txC, Vout: 0, Value: 30_000, Status: api.Status{Confirmed: false}},
	}
	labels := map[string]string{txA: "coin-alpha", txB: "coin-beta", txC: "coin-gamma"}
	mock := NewMockWalletManager()
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{BlockHeight: 100, Transactions: txs, UTXOs: utxos}
	}
	mock.GetAccountsFn = func() []wallet.AccountData {
		return []wallet.AccountData{{Addresses: []wallet.AddressData{
			{Address: receiveAddr},
			{Address: changeAddr, Change: true},
		}}}
	}
	mock.GetTransactionLabelFn = func(txid string) string { return labels[txid] }

	m := NewUTXOsModel(mock, nil)
	m.width, m.height = width, height
	m.itemsPerPage = m.rowsPerPage()
	m.utxos = utxos
	m.sortAndUpdateTable()
	return m
}

func TestUTXOsLayoutStableAcrossCursor(t *testing.T) {
	m := utxoLayoutModel(t, 80, 24)
	base := m.View()
	wantLines := requireLineCount(t, base)
	wantSummary := requireTextPosition(t, base, "coins")
	wantMempool := requireTextPosition(t, base, "mempool ")
	wantFrozen := requireTextPosition(t, base, "frozen ")
	wantMarker := requireTextPosition(t, base, "›")
	if !strings.Contains(stripANSI(base), "3 coins · 0.00080300 BTC · mempool 0.00030000 · frozen 0.00000000 (0)") {
		t.Fatalf("unexpected summary line:\n%s", stripANSI(base))
	}

	// Sorted by value: alpha, gamma, beta.
	rows := []struct {
		label  string
		helper string
	}{
		{"coin-alpha", "change"},
		{"coin-gamma", "unconfirmed"},
		{"coin-beta", "dust"},
	}
	wantLabels := make([]viewPosition, len(rows))
	for i, row := range rows {
		wantLabels[i] = requireTextPosition(t, base, row.label)
	}
	wantHelper := requireTextPosition(t, base, rows[0].helper)

	for i, row := range rows {
		m.selectedIdx = i
		view := m.View()
		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("row %d: %d lines, want %d", i, got, wantLines)
		}
		if got := requireTextPosition(t, view, "coins"); got != wantSummary {
			t.Fatalf("row %d: summary moved from %+v to %+v", i, wantSummary, got)
		}
		if got := requireTextPosition(t, view, "mempool "); got != wantMempool {
			t.Fatalf("row %d: mempool segment moved from %+v to %+v", i, wantMempool, got)
		}
		if got := requireTextPosition(t, view, "frozen "); got != wantFrozen {
			t.Fatalf("row %d: frozen segment moved from %+v to %+v", i, wantFrozen, got)
		}
		if got := requireTextPosition(t, view, "›"); got.col != wantMarker.col {
			t.Fatalf("row %d: marker moved from x=%d to x=%d", i, wantMarker.col, got.col)
		}
		for j, other := range rows {
			if got := requireTextPosition(t, view, other.label); got != wantLabels[j] {
				t.Fatalf("row %d: %q moved from %+v to %+v", i, other.label, wantLabels[j], got)
			}
		}
		if got := requireTextPosition(t, view, row.helper); got.col != wantHelper.col {
			t.Fatalf("row %d: helper moved from x=%d to x=%d", i, wantHelper.col, got.col)
		}
	}
}

func TestUTXOsSortKeysChangeHeader(t *testing.T) {
	m := utxoLayoutModel(t, 80, 24)
	view := m.View()
	valueHeader := requireTextPosition(t, view, "Value ↓")
	wantLines := requireLineCount(t, view)

	m.setSort(utxoSortAge)
	m.sortAndUpdateTable()
	view = m.View()
	if got := requireLineCount(t, view); got != wantLines {
		t.Fatalf("age sort changed line count from %d to %d", wantLines, got)
	}
	if got := requireTextPosition(t, view, "Age ↓"); got.line != valueHeader.line {
		t.Fatalf("age header on line %d, want the header row at line %d", got.line, valueHeader.line)
	}
	if strings.Contains(stripANSI(view), "Value ↓") {
		t.Fatalf("value header still marked after age sort:\n%s", stripANSI(view))
	}

	m.setSort(utxoSortAge)
	m.sortAndUpdateTable()
	if !strings.Contains(stripANSI(m.View()), "Age ↑") {
		t.Fatalf("second age sort did not flip the indicator:\n%s", stripANSI(m.View()))
	}
}

func TestUTXOsAndSendFeeFitAtSupportedSizes(t *testing.T) {
	sizes := []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}}
	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			um := utxoLayoutModel(t, size.w, size.h)
			uview := um.View()
			assertFits(t, uview, size.w)
			if got := requireLineCount(t, uview); got > size.h {
				t.Fatalf("utxos view has %d lines, terminal is %d high", got, size.h)
			}

			sm := newTestSend(t)
			advanceToFee(&sm)
			sm.width, sm.height = size.w, size.h
			sm.feeEstimates = testFeeEstimates()
			sm.estimatedSize = 141
			sview := sm.View()
			assertFits(t, sview, size.w)
			if got := requireLineCount(t, sview); got > size.h {
				t.Fatalf("send fee view has %d lines, terminal is %d high", got, size.h)
			}
		})
	}
}

func TestSendFeeHelperColumnStartsAtSameX(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}} {
		t.Run(size.name, func(t *testing.T) {
			m := newTestSend(t)
			advanceToFee(&m)
			m.width, m.height = size.w, size.h
			m.feeEstimates = testFeeEstimates()
			m.estimatedSize = 141

			helpers := []string{"~10 min", "~30 min", "~1 hour", "enter a rate"}
			view := m.View()
			want := requireTextPosition(t, view, helpers[0])
			wantLines := requireLineCount(t, view)
			for _, helper := range helpers[1:] {
				if got := requireTextPosition(t, view, helper); got.col != want.col {
					t.Fatalf("%q helper starts at x=%d, want %d:\n%s",
						helper, got.col, want.col, stripANSI(view))
				}
			}
			for cursor := 0; cursor < 4; cursor++ {
				m.cursor = cursor
				view = m.View()
				if got := requireLineCount(t, view); got != wantLines {
					t.Fatalf("cursor %d: %d lines, want %d", cursor, got, wantLines)
				}
				for _, helper := range helpers {
					if got := requireTextPosition(t, view, helper); got.col != want.col {
						t.Fatalf("cursor %d: %q helper moved to x=%d, want %d",
							cursor, helper, got.col, want.col)
					}
				}
			}
		})
	}
}

func TestConnectionChooseBackendLayoutStable(t *testing.T) {
	m := NewConnectionSetupModel(nil, false)
	m.width, m.height = 80, 24
	m.step = StepChooseBackend

	base := m.View()
	wantLines := requireLineCount(t, base)
	wantTitle := requireTextPosition(t, base, "Connection Setup")
	wantSubtitle := requireTextPosition(t, base, "Choose how to connect")
	wantHelp := requireTextPosition(t, base, "↑/↓ navigate")

	names := []string{"Electrum Server", "Bitcoin Core RPC"}
	descs := []string{
		"Direct protocol connection (Recommended)",
		"Connect to your own Bitcoin Core node",
	}
	wantNames := make([]viewPosition, len(names))
	for i, name := range names {
		wantNames[i] = requireTextPosition(t, base, name)
	}

	for cursor := range names {
		m.cursor = cursor
		view := m.View()

		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("cursor %d: %d lines, want %d", cursor, got, wantLines)
		}
		if got := requireTextPosition(t, view, "Connection Setup"); got != wantTitle {
			t.Fatalf("cursor %d: title moved from %+v to %+v", cursor, wantTitle, got)
		}
		if got := requireTextPosition(t, view, "Choose how to connect"); got != wantSubtitle {
			t.Fatalf("cursor %d: subtitle moved from %+v to %+v", cursor, wantSubtitle, got)
		}
		if got := requireTextPosition(t, view, "↑/↓ navigate"); got != wantHelp {
			t.Fatalf("cursor %d: help moved from %+v to %+v", cursor, wantHelp, got)
		}
		for i, name := range names {
			if got := requireTextPosition(t, view, name); got != wantNames[i] {
				t.Fatalf("cursor %d: %q moved from %+v to %+v",
					cursor, name, wantNames[i], got)
			}
		}
		requireHelperOnSelectedLine(t, view, names[cursor], descs[cursor])
		assertFits(t, view, 80)
	}
}

func TestConnectionElectrumLayoutStable(t *testing.T) {
	m := NewConnectionSetupModel(nil, false)
	m.width, m.height = 80, 24
	m.step = StepConfigureElectrum
	m.recentServers = []backend.SavedServer{
		{Name: "My Node", Host: "10.0.0.1", Port: 50001},
		{Name: "Tor Node", Host: "tor.onion", Port: 50002, UseTor: true},
	}

	publicServers := backend.PublicElectrumServers[m.network]
	total := len(publicServers) + len(m.recentServers) + 1

	base := m.View()
	wantLines := requireLineCount(t, base)
	fixed := []string{
		"Electrum Server", "Select a server or enter custom",
		"Recent Servers", "Custom Server", "Proxy Settings", "↑/↓ navigate",
	}
	wantFixed := make([]viewPosition, len(fixed))
	for i, text := range fixed {
		wantFixed[i] = requireTextPosition(t, base, text)
	}

	names := make([]string, 0, total)
	descs := make([]string, 0, total)
	for _, server := range publicServers {
		names = append(names, server.Name)
		descs = append(descs, fmt.Sprintf("%s:%d", server.Host, server.Port))
	}
	for _, rs := range m.recentServers {
		names = append(names, rs.Name)
		desc := fmt.Sprintf("%s:%d", rs.Host, rs.Port)
		if rs.UseTor {
			desc += ", Tor"
		}
		descs = append(descs, desc)
	}
	names = append(names, "Custom Server")
	descs = append(descs, "enter host and port")

	wantNames := make([]viewPosition, len(names))
	for i, name := range names {
		wantNames[i] = requireTextPosition(t, base, name)
	}

	for cursor := 0; cursor < total; cursor++ {
		m.cursor = cursor
		m.customServer = cursor == total-1
		view := m.View()

		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("cursor %d: %d lines, want %d", cursor, got, wantLines)
		}
		for i, text := range fixed {
			if got := requireTextPosition(t, view, text); got != wantFixed[i] {
				t.Fatalf("cursor %d: %q moved from %+v to %+v",
					cursor, text, wantFixed[i], got)
			}
		}
		for i, name := range names {
			if got := requireTextPosition(t, view, name); got != wantNames[i] {
				t.Fatalf("cursor %d: %q moved from %+v to %+v",
					cursor, name, wantNames[i], got)
			}
		}
		requireHelperOnSelectedLine(t, view, names[cursor], descs[cursor])
		assertFits(t, view, 80)
	}
}

func TestSendUTXOSelectionLayoutStable(t *testing.T) {
	m := newTestSend(t)
	advanceToUTXOSelection(&m)
	m.width, m.height = 80, 24

	base := m.View()
	wantLines := requireLineCount(t, base)
	wantHeader := requireTextPosition(t, base, "Send Bitcoin")
	wantStep := requireTextPosition(t, base, "Select UTXOs")
	wantSelection := requireTextPosition(t, base, "Selected: 3 UTXOs")
	wantHelp := requireTextPosition(t, base, "↑/↓ navigate")

	names := []string{
		"aaaa1111...bbbb2222:0",
		"bbbb2222...cccc3333:1",
		"cccc3333...dddd4444:0",
	}
	helpers := []string{"confirmed", "confirmed", "unconfirmed"}
	wantNames := make([]viewPosition, len(names))
	for i, name := range names {
		wantNames[i] = requireTextPosition(t, base, name)
	}

	var wantHelperCol int
	for cursor := range names {
		m.cursor = cursor
		view := m.View()

		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("cursor %d: %d lines, want %d", cursor, got, wantLines)
		}
		for i, name := range names {
			if got := requireTextPosition(t, view, name); got != wantNames[i] {
				t.Fatalf("cursor %d: %q moved from %+v to %+v",
					cursor, name, wantNames[i], got)
			}
		}
		if got := requireTextPosition(t, view, "Send Bitcoin"); got != wantHeader {
			t.Fatalf("cursor %d: header moved from %+v to %+v", cursor, wantHeader, got)
		}
		if got := requireTextPosition(t, view, "Select UTXOs"); got != wantStep {
			t.Fatalf("cursor %d: step moved from %+v to %+v", cursor, wantStep, got)
		}
		if got := requireTextPosition(t, view, "Selected: 3 UTXOs"); got != wantSelection {
			t.Fatalf("cursor %d: selection summary moved from %+v to %+v", cursor, wantSelection, got)
		}
		if got := requireTextPosition(t, view, "↑/↓ navigate"); got != wantHelp {
			t.Fatalf("cursor %d: help moved from %+v to %+v", cursor, wantHelp, got)
		}

		requireHelperOnSelectedLine(t, view, names[cursor], helpers[cursor])
		helper := requireTextPosition(t, view, helpers[cursor])
		if cursor == 0 {
			wantHelperCol = helper.col
		} else if helper.col != wantHelperCol {
			t.Fatalf("cursor %d: helper starts at x=%d, want %d", cursor, helper.col, wantHelperCol)
		}
		assertFits(t, view, 80)
	}
}

func TestHardwareDeviceMenusLayoutStable(t *testing.T) {
	devices := []wallet.HardwareDevice{
		{Type: "trezor", Model: "Trezor One", Fingerprint: "abcd1234", NeedsPin: true},
		{Type: "ledger", Model: "Ledger Nano X", Fingerprint: "deadbeef1234", NeedsPass: true},
		{
			Type:        "coldcard",
			Model:       "Coldcard Mk4",
			Fingerprint: "cafebabe",
			Error: "Could not connect to device: the user did not confirm the request " +
				"on the device screen before the transport closed unexpectedly",
		},
		{Type: "bitbox", Model: "BitBox02", Fingerprint: "12345678"},
	}

	// The picked device rows only fit unclipped inside the picker panel at the
	// wide layout; at 80 columns the panel truncates their right edge instead.
	t.Run("send", func(t *testing.T) {
		m := newTestSend(t)
		m.width, m.height = 110, 34
		m.showPSBTExport = true
		m.showHWIDevices = true
		m.hwiDevices = devices
		assertDeviceMenuStable(t, devices, "Connected devices:", func(selected int) string {
			m.selectedHWDevice = selected
			return m.View()
		})
	})

	t.Run("wallet setup", func(t *testing.T) {
		m := NewWalletSetupModel(NewMockWalletManager(), nil)
		m.width, m.height = 110, 34
		m.step = StepHardwareWalletConnect
		m.hwiDevices = devices
		assertDeviceMenuStable(t, devices, "Found 4 device(s):", func(selected int) string {
			m.selectedHWDevice = selected
			return m.View()
		})
	})

	t.Run("multisig import", func(t *testing.T) {
		m := NewWalletSetupModel(NewMockWalletManager(), nil)
		m.width, m.height = 110, 34
		m.isMultisig = true
		m.step = StepMultisigEnterCosigners
		m.showHWIDevices = true
		m.hwiDevices = devices
		assertDeviceMenuStable(t, devices, "Connected devices:", func(selected int) string {
			m.selectedHWDevice = selected
			return m.View()
		})
	})
}

func assertDeviceMenuStable(t *testing.T, devices []wallet.HardwareDevice, listHeader string, render func(int) string) {
	t.Helper()

	options := hardwareDeviceChoices(devices)
	base := render(0)
	wantLines := requireLineCount(t, base)
	wantHelp := requireTextPosition(t, base, "↑/↓")
	wantActions := requireTextPosition(t, base, listHeader)

	wantNames := make([]viewPosition, len(options))
	wantRows := make([]int, len(options))
	var wantStatusCol int
	statusColSet := false
	for i, opt := range options {
		wantNames[i] = requireTextPosition(t, base, opt.name)
		if opt.status != "" {
			status := requireTextPosition(t, base, stripANSI(opt.status))
			if status.line != wantNames[i].line {
				t.Fatalf("device %d: status is on line %d, name on line %d",
					i, status.line, wantNames[i].line)
			}
			if !statusColSet {
				wantStatusCol = status.col
				statusColSet = true
			} else if status.col != wantStatusCol {
				t.Fatalf("device %d: status starts at x=%d, want %d",
					i, status.col, wantStatusCol)
			}
		}
		lines := strings.Split(stripANSI(base), "\n")
		wantRows[i] = ansi.StringWidth(lines[wantNames[i].line])
	}

	var wantHelperCol int
	helperColSet := false
	for selected := range options {
		view := render(selected)

		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("device %d: %d lines, want %d", selected, got, wantLines)
		}
		if got := requireTextPosition(t, view, "↑/↓"); got != wantHelp {
			t.Fatalf("device %d: help moved from %+v to %+v", selected, wantHelp, got)
		}
		if got := requireTextPosition(t, view, listHeader); got != wantActions {
			t.Fatalf("device %d: list header moved from %+v to %+v", selected, wantActions, got)
		}

		lines := strings.Split(stripANSI(view), "\n")
		for i, opt := range options {
			namePos := requireTextPosition(t, view, opt.name)
			if namePos != wantNames[i] {
				t.Fatalf("device %d: %q moved from %+v to %+v",
					selected, opt.name, wantNames[i], namePos)
			}
			// Status is visible on every row, including non-selected ones.
			if opt.status != "" {
				status := requireTextPosition(t, view, stripANSI(opt.status))
				if status.line != namePos.line {
					t.Fatalf("device %d: status is on line %d, name on line %d",
						selected, status.line, namePos.line)
				}
				if status.col != wantStatusCol {
					t.Fatalf("device %d: status starts at x=%d, want %d",
						selected, status.col, wantStatusCol)
				}
			}
			if got := ansi.StringWidth(lines[namePos.line]); got != wantRows[i] {
				t.Fatalf("device %d: row %d is %d cells wide, want %d",
					selected, i, got, wantRows[i])
			}
		}

		opt := options[selected]
		nameLine := wantNames[selected].line
		if opt.needsPin {
			requireHelperOnSelectedLine(t, view, opt.name, devicePinHelper)
			helper := requireTextPosition(t, view, devicePinHelper)
			if !helperColSet {
				wantHelperCol = helper.col
				helperColSet = true
			} else if helper.col != wantHelperCol {
				t.Fatalf("device %d: helper starts at x=%d, want %d",
					selected, helper.col, wantHelperCol)
			}
		}
		for i, line := range lines {
			if i != nameLine && strings.Contains(line, devicePinHelper) {
				t.Fatalf("device %d: helper shown on non-selected line %d:\n%s",
					selected, i, strings.Join(lines, "\n"))
			}
		}
		assertFits(t, view, 110)
	}
}

func TestHardwareDeviceMenuStatusStyles(t *testing.T) {
	longError := "Could not connect to device: the user did not confirm the request " +
		"on the device screen before the transport closed unexpectedly"
	devices := []wallet.HardwareDevice{
		{Type: "trezor", Model: "Trezor One", Fingerprint: "abcd1234", NeedsPin: true},
		{Type: "ledger", Model: "Ledger Nano X", Fingerprint: "deadbeef", NeedsPass: true},
		{Type: "coldcard", Model: "Coldcard Mk4", Fingerprint: "cafebabe", Error: longError},
	}

	menu := renderDeviceMenu(hardwareDeviceChoices(devices), 0)
	for _, want := range []string{
		styles.WarningText.Render("⚠ needs PIN"),
		styles.DimText.Render("(needs passphrase)"),
		styles.ErrorText.Render(truncateDisplay(strings.Join(strings.Fields(longError), " "), deviceStatusWidth)),
	} {
		if !strings.Contains(menu, want) {
			t.Fatalf("device menu is missing status %q:\n%q", stripANSI(want), menu)
		}
	}

	plain := stripANSI(menu)
	if !strings.Contains(plain, "…") {
		t.Fatalf("long device error is not truncated:\n%s", plain)
	}
	if strings.Contains(plain, longError) {
		t.Fatalf("full device error should not be shown:\n%s", plain)
	}
}

func TestPSBTFileBrowserLayoutStable(t *testing.T) {
	m := newTestSend(t)
	m.width, m.height = 80, 24
	m.showFileBrowser = true
	m.psbtFiles = []string{"/tmp/a.psbt", "/home/me/Desktop/b.psbt"}

	base := m.View()
	wantLines := requireLineCount(t, base)
	wantHeader := requireTextPosition(t, base, "Import PSBT")
	wantList := requireTextPosition(t, base, "Found PSBT files")
	wantHelp := requireTextPosition(t, base, "↑/↓ navigate")

	names := []string{"a.psbt", "b.psbt"}
	helpers := []string{"tmp", "Desktop"}
	wantNames := make([]viewPosition, len(names))
	for i, name := range names {
		wantNames[i] = requireTextPosition(t, base, name)
	}

	var wantHelperCol int
	for cursor := range names {
		m.fileBrowserCursor = cursor
		view := m.View()

		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("cursor %d: %d lines, want %d", cursor, got, wantLines)
		}
		if got := requireTextPosition(t, view, "Import PSBT"); got != wantHeader {
			t.Fatalf("cursor %d: header moved from %+v to %+v", cursor, wantHeader, got)
		}
		if got := requireTextPosition(t, view, "Found PSBT files"); got != wantList {
			t.Fatalf("cursor %d: list header moved from %+v to %+v", cursor, wantList, got)
		}
		if got := requireTextPosition(t, view, "↑/↓ navigate"); got != wantHelp {
			t.Fatalf("cursor %d: help moved from %+v to %+v", cursor, wantHelp, got)
		}
		for i, name := range names {
			if got := requireTextPosition(t, view, name); got != wantNames[i] {
				t.Fatalf("cursor %d: %q moved from %+v to %+v",
					cursor, name, wantNames[i], got)
			}
		}

		requireHelperOnSelectedLine(t, view, names[cursor], helpers[cursor])
		helper := requireTextPosition(t, view, helpers[cursor])
		if cursor == 0 {
			wantHelperCol = helper.col
		} else if helper.col != wantHelperCol {
			t.Fatalf("cursor %d: helper starts at x=%d, want %d", cursor, helper.col, wantHelperCol)
		}
		assertFits(t, view, 80)
	}
}
