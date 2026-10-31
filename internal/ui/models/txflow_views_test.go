package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

const (
	txFlowTestOurInput = "tb1qourinput0000000000000000000000000000"
	txFlowTestPayee    = "tb1qpayee00000000000000000000000000000000"
	txFlowTestChange   = "tb1qchange0000000000000000000000000000000"
	txFlowTestReceive  = "tb1qreceive000000000000000000000000000000"
)

func txFlowTestKey(s string) tea.KeyPressMsg {
	return keyRunes(s)
}

func txFlowTestUpdate(t *testing.T, m screenModel, msg tea.Msg) screenModel {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated
}

func txFlowTestAccounts() []wallet.AccountData {
	return []wallet.AccountData{{
		Index: 0,
		Addresses: []wallet.AddressData{
			{Address: txFlowTestOurInput},
			{Address: txFlowTestReceive},
			{Address: txFlowTestChange, Change: true},
		},
		Labels: map[string]string{txFlowTestChange: "change label"},
	}}
}

func txFlowTestInspection() *wallet.PSBTInspection {
	return &wallet.PSBTInspection{
		Inputs: []wallet.InspectedInput{
			{OutPoint: strings.Repeat("aa", 32) + ":0", Address: txFlowTestOurInput,
				Value: 100_000, ValueKnown: true, Mine: true},
			{OutPoint: strings.Repeat("bb", 32) + ":1", Address: txFlowTestPayee},
		},
		Outputs: []wallet.InspectedOutput{
			{Address: txFlowTestPayee, Value: 40_000, Owner: wallet.OutputExternal},
			{Address: txFlowTestChange, Value: 59_000, Owner: wallet.OutputChange},
		},
		Fee: 1_000, FeeKnown: true, FeeRate: 7.1, VSize: 141,
	}
}

func txFlowTestTransaction() api.Transaction {
	return api.Transaction{
		TxID: strings.Repeat("cd", 32),
		Fee:  1_000,
		Size: 141,
		Vin: []api.Vin{{TxID: strings.Repeat("ee", 32), Vout: 0, Prevout: &api.Vout{
			ScriptpubkeyAddress: txFlowTestOurInput, Value: 100_000}}},
		Vout: []api.Vout{
			{ScriptpubkeyAddress: txFlowTestPayee, Value: 40_000},
			{ScriptpubkeyAddress: txFlowTestChange, Value: 59_000},
		},
		Status: api.Status{Confirmed: true, BlockHeight: 100},
	}
}

func txFlowTestMock() *MockWalletManager {
	mock := NewMockWalletManager()
	mock.GetAccountsFn = txFlowTestAccounts
	mock.GetAddressLabelFn = func(addr string) string {
		if addr == txFlowTestChange {
			return "change label"
		}
		return ""
	}
	return mock
}

func newSendReviewFlowModel(t *testing.T, width, height int) SendModel {
	t.Helper()
	m := NewSendModel(txFlowTestMock(), 0, nil)
	m = asSendModel(t, txFlowTestUpdate(t, m, tea.WindowSizeMsg{Width: width, Height: height}))
	m.step = SendStepReview
	m.psbtResult = &wallet.PSBTResult{
		Fee: 1_000, IntendedFee: 1_000, Change: 59_000, HasChange: true,
		ChangeAddress: txFlowTestChange, VSize: 141,
	}
	m.inspection = txFlowTestInspection()
	return m
}

func newPSBTInspectorFlowModel(t *testing.T, width, height int) SendModel {
	t.Helper()
	m := NewSendModel(txFlowTestMock(), 0, nil)
	m = asSendModel(t, txFlowTestUpdate(t, m, tea.WindowSizeMsg{Width: width, Height: height}))
	m.step = SendStepReview
	m.showPSBTExport = true
	m.psbtBase64 = "cHNidP8BAHECAAAAAXiq8Xx3"
	m.psbtSigsTotal = 1
	m.psbtResult = &wallet.PSBTResult{Fee: 1_000, VSize: 141}
	m.inspection = txFlowTestInspection()
	return m
}

func newHistoryFlowModel(t *testing.T, width, height int) TransactionHistoryModel {
	t.Helper()
	m := NewTransactionHistoryModel(txFlowTestMock())
	m = txFlowTestUpdate(t, m, tea.WindowSizeMsg{Width: width, Height: height}).(TransactionHistoryModel)
	m.loading = false
	m.fadeIn = 1.0
	m.isActive = true
	m.blockHeight = 101
	m.transactions = []api.Transaction{txFlowTestTransaction()}
	m.updateTableData()
	return m
}

func assertTxFlowVisible(t *testing.T, view string) {
	t.Helper()
	plain := stripANSI(view)
	for _, want := range []string{"Inputs", "Outputs", "●", "◆"} {
		if !strings.Contains(plain, want) {
			t.Errorf("visualizer is missing %q:\n%s", want, plain)
		}
	}
}

func TestSendReview_VisualizerRoundTrip(t *testing.T) {
	m := newSendReviewFlowModel(t, 80, 24)
	before := m.View()

	opened := asSendModel(t, txFlowTestUpdate(t, m, txFlowTestKey("v")))
	assertTxFlowVisible(t, opened.View())

	closed := asSendModel(t, txFlowTestUpdate(t, opened, keyCode(tea.KeyEsc)))
	if got, want := closed.View(), before; got != want {
		t.Fatalf("review view changed after closing the visualizer:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestPSBTInspector_VisualizerRoundTrip(t *testing.T) {
	m := newPSBTInspectorFlowModel(t, 80, 24)
	before := m.View()

	opened := asSendModel(t, txFlowTestUpdate(t, m, txFlowTestKey("v")))
	assertTxFlowVisible(t, opened.View())

	closed := asSendModel(t, txFlowTestUpdate(t, opened, keyCode(tea.KeyEsc)))
	if got, want := closed.View(), before; got != want {
		t.Fatalf("PSBT inspector view changed after closing the visualizer:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestHistory_VisualizerRoundTrip(t *testing.T) {
	m := newHistoryFlowModel(t, 80, 24)
	before := m.View()

	opened := txFlowTestUpdate(t, m, txFlowTestKey("v")).(TransactionHistoryModel)
	assertTxFlowVisible(t, opened.View())

	closed := txFlowTestUpdate(t, opened, keyCode(tea.KeyEsc)).(TransactionHistoryModel)
	if got, want := closed.View(), before; got != want {
		t.Fatalf("history view changed after closing the visualizer:\nwant:\n%s\ngot:\n%s", want, got)
	}
	if closed.currentPage != m.currentPage || closed.selectedRow != m.selectedRow {
		t.Fatalf("history position changed: page %d→%d row %d→%d",
			m.currentPage, closed.currentPage, m.selectedRow, closed.selectedRow)
	}
}

func TestTxFlowVisualizerKeys(t *testing.T) {
	v := TxFlowView{Flow: txFlowFixture(3, 3), Width: 80, Height: 24}
	handleTxFlowKey(&v, "down")
	if v.Cursor != 1 {
		t.Fatalf("down moved cursor to %d, want 1", v.Cursor)
	}
	handleTxFlowKey(&v, "k")
	if v.Cursor != 0 {
		t.Fatalf("k moved cursor to %d, want 0", v.Cursor)
	}
	handleTxFlowKey(&v, "tab")
	if v.Side != FlowOutput {
		t.Fatalf("tab selected %v, want outputs", v.Side)
	}
	handleTxFlowKey(&v, "left")
	if v.Side != FlowInput {
		t.Fatalf("left selected %v, want inputs", v.Side)
	}
	handleTxFlowKey(&v, "right")
	if v.Side != FlowOutput {
		t.Fatalf("right selected %v, want outputs", v.Side)
	}
	if !handleTxFlowKey(&v, "q") {
		t.Fatal("q did not close the visualizer")
	}
}
