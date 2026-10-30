package models

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/BTCBap/hax/internal/api"
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
	wantSummary := requireTextPosition(t, base, "coins · total")
	wantMarker := requireTextPosition(t, base, "●")

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
		if got := requireTextPosition(t, view, "coins · total"); got != wantSummary {
			t.Fatalf("row %d: summary moved from %+v to %+v", i, wantSummary, got)
		}
		if got := requireTextPosition(t, view, "●"); got.col != wantMarker.col {
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
