package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/x/ansi"
)

const (
	bumpTestTxID = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	bumpTestDest = "tb1pqqqsyqcyq5rqwzqfpg9scrgwpugpzysnzs23v9ccrydpk8qarc0slpgr8c"
)

func bumpInfo() *wallet.FeeBumpInfo {
	return &wallet.FeeBumpInfo{
		TxID: bumpTestTxID, Fee: 282, VSize: 141, FeeRate: 2,
		CanRBF: true, MinRBFRate: 3, CanCPFP: true, CPFPValue: 59_000,
		Recipients: []wallet.TxRecipient{{Address: bumpTestDest, Amount: 40_000}},
	}
}

func bumpKey(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return keyCode(tea.KeyEnter)
	case "esc":
		return keyCode(tea.KeyEsc)
	}
	return keyRunes(s)
}

func newBumpTest(t *testing.T, info *wallet.FeeBumpInfo) (SendModel, *MockWalletManager, *[]string) {
	t.Helper()
	mock := NewMockWalletManager()
	var built []string
	mock.AnalyzeFeeBumpFn = func(string) (*wallet.FeeBumpInfo, error) { return info, nil }
	mock.BuildRBFFn = func(txid string, rate float64) (*wallet.PSBTResult, error) {
		built = append(built, "rbf")
		return &wallet.PSBTResult{Fee: 1410, Replaces: txid, VSize: 141}, nil
	}
	mock.BuildCPFPFn = func(txid string, rate float64) (*wallet.PSBTResult, error) {
		built = append(built, "cpfp")
		return &wallet.PSBTResult{Fee: 3000, ChildOf: txid, HasChange: true, VSize: 110}, nil
	}
	m := NewFeeBumpModel(mock, bumpTestTxID, 0, nil)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = asSendModel(t, model)
	m.feeEstimates = &api.FeeEstimates{FastestFee: 20, HalfHourFee: 10, HourFee: 2}
	return m, mock, &built
}

func update(t *testing.T, m SendModel, msg tea.Msg) (SendModel, tea.Cmd) {
	t.Helper()
	model, cmd := m.Update(msg)
	return asSendModel(t, model), cmd
}

// TestFeeBump_RBFFlow: the speed-up opens on the fee step, refuses a
// feerate below the BIP125 minimum, then builds a replacement and shows
// the kept payment in the review.
func TestFeeBump_RBFFlow(t *testing.T) {
	m, _, built := newBumpTest(t, bumpInfo())
	if m.step != SendStepFee || m.bump == nil {
		t.Fatalf("step = %v", m.step)
	}
	m, _ = update(t, m, m.analyzeFeeBump()())
	view := m.View()
	for _, want := range []string{"Speed Up Transaction", "a1b2c3d4…6d7e8f90",
		"282 sats · 2 sat/vB", "(RBF)", "m: use a child (CPFP) instead", "at least 3 sat/vB"} {
		if !strings.Contains(ansi.Strip(view), want) {
			t.Errorf("fee view missing %q:\n%s", want, ansi.Strip(view))
		}
	}
	assertFits(t, view, 80)

	// Low priority (2 sat/vB) is below the minimum.
	m.cursor, m.feeLevel = 2, FeeLevelLow
	m, _ = update(t, m, bumpKey("enter"))
	if m.err == nil || !strings.Contains(m.err.Error(), "at least 3") {
		t.Fatalf("err = %v", m.err)
	}
	if len(*built) != 0 {
		t.Fatal("built a bump below the minimum")
	}

	m.cursor, m.feeLevel = 1, FeeLevelMedium
	m, cmd := update(t, m, bumpKey("enter"))
	if m.step != SendStepPreviewLoading || cmd == nil {
		t.Fatalf("step = %v", m.step)
	}
	m, _ = update(t, m, m.previewTransaction()())
	if m.step != SendStepReview || (*built)[0] != "rbf" {
		t.Fatalf("step = %v, built %v", m.step, *built)
	}
	if len(m.recipients) != 1 || m.recipients[0].Address != bumpTestDest {
		t.Fatalf("recipients = %+v", m.recipients)
	}
	assertFits(t, m.View(), 80)
}

func TestFeeBumpWithRate_PrefillsAndAdvances(t *testing.T) {
	mock := NewMockWalletManager()
	var builtRate float64
	mock.AnalyzeFeeBumpFn = func(string) (*wallet.FeeBumpInfo, error) { return bumpInfo(), nil }
	mock.BuildRBFFn = func(_ string, rate float64) (*wallet.PSBTResult, error) {
		builtRate = rate
		return &wallet.PSBTResult{Fee: 1410, VSize: 141}, nil
	}

	m := NewFeeBumpModelWithRate(mock, bumpTestTxID, 10, 0, nil)
	if m.step != SendStepFee || m.feeLevel != FeeLevelCustom || m.customFeeRate != 10 {
		t.Fatalf("preset fee step = %v / %v / %v", m.step, m.feeLevel, m.customFeeRate)
	}

	m, cmd := update(t, m, FeeBumpInfoMsg{Info: bumpInfo()})
	if m.step != SendStepPreviewLoading || cmd == nil {
		t.Fatalf("preset did not start the preview: step=%v cmd=%v", m.step, cmd)
	}
	for _, msg := range collectMsgs(cmd) {
		m, _ = update(t, m, msg)
	}
	if m.step != SendStepReview || builtRate != 10 {
		t.Fatalf("step=%v builtRate=%v", m.step, builtRate)
	}
}

func TestFeeBumpWithRate_KeepsFeeStepOnError(t *testing.T) {
	mock := NewMockWalletManager()
	mock.AnalyzeFeeBumpFn = func(string) (*wallet.FeeBumpInfo, error) { return bumpInfo(), nil }
	mock.BuildRBFFn = func(string, float64) (*wallet.PSBTResult, error) {
		return nil, errTest
	}

	// Below the replacement minimum.
	m := NewFeeBumpModelWithRate(mock, bumpTestTxID, 1, 0, nil)
	m, _ = update(t, m, FeeBumpInfoMsg{Info: bumpInfo()})
	if m.step != SendStepFee || m.err == nil || m.customFeeRate != 1 {
		t.Fatalf("minimum error left step=%v err=%v rate=%v", m.step, m.err, m.customFeeRate)
	}

	// A valid rate whose build fails stays on the fee step.
	m = NewFeeBumpModelWithRate(mock, bumpTestTxID, 10, 0, nil)
	m, cmd := update(t, m, FeeBumpInfoMsg{Info: bumpInfo()})
	if m.step != SendStepPreviewLoading || cmd == nil {
		t.Fatalf("preview not started: step=%v", m.step)
	}
	for _, msg := range collectMsgs(cmd) {
		m, _ = update(t, m, msg)
	}
	if m.step != SendStepFee || m.err == nil || m.customFeeRate != 10 {
		t.Fatalf("build error left step=%v err=%v rate=%v", m.step, m.err, m.customFeeRate)
	}
}

func TestFeeBump_MethodToggleAndCPFP(t *testing.T) {
	m, _, built := newBumpTest(t, bumpInfo())
	m, _ = update(t, m, m.analyzeFeeBump()())
	m, _ = update(t, m, bumpKey("m"))
	if m.bump.method != bumpCPFP {
		t.Fatal("m didn't switch to CPFP")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "(CPFP)") ||
		!strings.Contains(v, "Package rate:") {
		t.Fatalf("view:\n%s", v)
	}
	m, _ = update(t, m, bumpKey("enter"))
	m, _ = update(t, m, m.previewTransaction()())
	if m.step != SendStepReview || (*built)[0] != "cpfp" || len(m.recipients) != 0 {
		t.Fatalf("step = %v, built %v, recipients %v", m.step, *built, m.recipients)
	}
}

func TestFeeBump_OnlyCPFP(t *testing.T) {
	info := bumpInfo()
	info.CanRBF, info.RBFReason = false, "not every input is from this wallet"
	m, _, _ := newBumpTest(t, info)
	m, _ = update(t, m, m.analyzeFeeBump()())
	if m.bump.method != bumpCPFP {
		t.Fatal("expected CPFP")
	}
	m, _ = update(t, m, bumpKey("m"))
	if m.bump.method != bumpCPFP {
		t.Fatal("switched to an impossible RBF")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "RBF not possible: not every input") {
		t.Fatalf("view:\n%s", v)
	}
}

func TestFeeBump_Impossible(t *testing.T) {
	info := bumpInfo()
	info.CanRBF, info.RBFReason = false, "it doesn't signal replaceability (BIP125)"
	info.CanCPFP, info.CPFPReason = false, "it has no unspent output paying this wallet"
	m, _, built := newBumpTest(t, info)
	m, _ = update(t, m, m.analyzeFeeBump()())
	if m.err == nil || !strings.Contains(m.err.Error(), "BIP125") {
		t.Fatalf("err = %v", m.err)
	}
	m.err = nil
	m, _ = update(t, m, bumpKey("enter"))
	if m.err == nil || len(*built) != 0 {
		t.Fatal("continued without a possible method")
	}
}

func TestFeeBump_EscReturnsToHistory(t *testing.T) {
	m, _, _ := newBumpTest(t, bumpInfo())
	_, cmd := update(t, m, bumpKey("esc"))
	if cmd == nil {
		t.Fatal("no command")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok || nav.Target != StateTransactionHistory {
		t.Fatalf("got %#v", nav)
	}
}

func TestHistory_BumpKey(t *testing.T) {
	m, _ := newTestHistory(t)
	m.transactions = sampleTransactions()
	m.updateTableData()

	pending := -1
	for i, tx := range m.filteredTransactions {
		if !tx.Status.Confirmed {
			pending = i
		}
	}
	if pending < 0 {
		t.Fatal("setup: no pending transaction")
	}
	m.selectedRow = pending
	m.table.SetCursor(pending)
	model, cmd := m.Update(bumpKey("b"))
	if cmd == nil {
		t.Fatal("no command for a pending transaction")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok || nav.Target != StateSend ||
		nav.FeeBumpTxID != m.filteredTransactions[pending].TxID {
		t.Fatalf("got %#v", nav)
	}

	h := model.(TransactionHistoryModel)
	h.selectedRow = 1 - pending
	h.table.SetCursor(1 - pending)
	model, _ = h.Update(bumpKey("b"))
	if model.(TransactionHistoryModel).err == nil {
		t.Fatal("no error for a confirmed transaction")
	}
}

func TestRoot_NavigateToFeeBump(t *testing.T) {
	mock := NewMockWalletManager()
	r := NewRootModel(mock, nil)
	model, _ := r.Update(NavigateMsg{Target: StateSend, FeeBumpTxID: bumpTestTxID})
	sm, ok := model.(RootModel).sendModel.(SendModel)
	if !ok || sm.bump == nil || sm.bump.txID != bumpTestTxID {
		t.Fatalf("send model not in fee bump mode: %#v", model.(RootModel).sendModel)
	}
}
