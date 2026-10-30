package models

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

func historyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func historyKey(kind tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: kind}
}

func speedUpTransactions() []api.Transaction {
	now := time.Now().Unix()
	out := func(id string, confirmed bool) api.Transaction {
		status := api.Status{Confirmed: confirmed}
		if confirmed {
			status.BlockHeight = 100
			status.BlockTime = now - 3600
		}
		return api.Transaction{
			TxID: id, Size: 200, Weight: 800, Fee: 200,
			Vin: []api.Vin{{
				Prevout:  &api.Vout{ScriptpubkeyAddress: "our_addr", Value: 50_000},
				Sequence: 0,
			}},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "dest_addr", Value: 40_000},
				{ScriptpubkeyAddress: "our_addr", Value: 9_000},
			},
			Status: status,
		}
	}
	incoming := api.Transaction{
		TxID: "aa_pending_in", Size: 200, Weight: 800, Fee: 200,
		Vin:  []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: "external", Value: 50_000}, Sequence: 0}},
		Vout: []api.Vout{{ScriptpubkeyAddress: "our_addr", Value: 49_000}},
	}
	return []api.Transaction{
		out("zz_pending_out", false),
		out("zy_pending_out", false),
		out("confirmed_out", true),
		incoming,
	}
}

func configureSpeedUpMock(mock *MockWalletManager, estimates *api.FeeEstimates) {
	mock.GetAccountsFn = func() []wallet.AccountData {
		return []wallet.AccountData{{
			Addresses: []wallet.AddressData{{Address: "our_addr"}},
		}}
	}
	if estimates != nil {
		mock.GetCachedFeeEstimatesFn = func() (*api.FeeEstimates, bool) {
			return estimates, true
		}
	} else {
		mock.GetCachedFeeEstimatesFn = func() (*api.FeeEstimates, bool) {
			return nil, false
		}
	}
}

func speedUpTestModel(t *testing.T, estimates *api.FeeEstimates) (TransactionHistoryModel, *MockWalletManager) {
	t.Helper()
	mock := NewMockWalletManager()
	configureSpeedUpMock(mock, estimates)
	m := NewTransactionHistoryModel(mock)
	m.width, m.height = 120, 40
	m.loading = false
	m.fadeIn = 1.0
	m.transactions = speedUpTransactions()
	m.updateTableData()
	return m, mock
}

func textPositions(view, text string) []viewPosition {
	lines := strings.Split(stripANSI(view), "\n")
	var out []viewPosition
	for i, line := range lines {
		for offset := 0; ; {
			at := strings.Index(line[offset:], text)
			if at < 0 {
				break
			}
			at += offset
			out = append(out, viewPosition{line: i, col: lipgloss.Width(line[:at])})
			offset = at + len(text)
		}
	}
	return out
}

func TestHistory_SpeedUpColumnOnlyOnPendingOutgoingRows(t *testing.T) {
	m, _ := speedUpTestModel(t, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	view := m.View()

	positions := textPositions(view, "⇡ speed up")
	if len(positions) != 2 {
		t.Fatalf("speed-up marker appears %d times, want 2 (the pending outgoing rows):\n%s",
			len(positions), stripANSI(view))
	}
	if positions[0].col != positions[1].col {
		t.Fatalf("speed-up column moved between rows: %+v and %+v", positions[0], positions[1])
	}
}

func TestHistory_SpeedUpColumnShortensBelow100Columns(t *testing.T) {
	m, _ := speedUpTestModel(t, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	m.width = 80
	m.updateTableData()
	view := m.View()

	if strings.Contains(stripANSI(view), "⇡ speed up") {
		t.Fatalf("80-column view should only show the arrow:\n%s", stripANSI(view))
	}
	if got := strings.Count(stripANSI(view), "⇡"); got != 2 {
		t.Fatalf("arrow appears %d times, want 2", got)
	}
	assertFits(t, view, 80)
}

func TestHistory_SpeedUpTrayKeepsHeightAndHeader(t *testing.T) {
	m, _ := speedUpTestModel(t, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	base := m.View()
	wantLines := requireLineCount(t, base)
	wantHeader := requireTextPosition(t, base, "Transaction History")

	model, _ := m.Update(historyRune('b'))
	open := model.(TransactionHistoryModel)
	if open.speedUp == nil {
		t.Fatal("b did not open the speed-up tray")
	}

	choose := open.View()
	if got := requireLineCount(t, choose); got != wantLines {
		t.Fatalf("tray view has %d lines, want %d", got, wantLines)
	}
	if got := requireTextPosition(t, choose, "Transaction History"); got != wantHeader {
		t.Fatalf("header moved from %+v to %+v", wantHeader, got)
	}

	model, _ = open.Update(historyKey(tea.KeyEnter))
	confirm := model.(TransactionHistoryModel)
	if confirm.speedUp == nil || confirm.speedUp.tray.stage != TrayStageConfirm {
		t.Fatal("enter did not open the confirm stage")
	}
	confirmView := confirm.View()
	if got := requireLineCount(t, confirmView); got != wantLines {
		t.Fatalf("confirm view has %d lines, want %d", got, wantLines)
	}
	if got := requireTextPosition(t, confirmView, "Transaction History"); got != wantHeader {
		t.Fatalf("header moved on confirm from %+v to %+v", wantHeader, got)
	}
	if !strings.Contains(stripANSI(confirmView), "Current rate") ||
		!strings.Contains(stripANSI(confirmView), "New rate") ||
		!strings.Contains(stripANSI(confirmView), "Extra fee") {
		t.Fatalf("confirm summary is incomplete:\n%s", stripANSI(confirmView))
	}

	model, _ = confirm.Update(historyKey(tea.KeyEsc))
	back := model.(TransactionHistoryModel)
	if back.speedUp == nil || back.speedUp.tray.stage != TrayStageChoose {
		t.Fatal("esc did not return to the choose stage")
	}
	if got := requireLineCount(t, back.View()); got != wantLines {
		t.Fatalf("choose view after back has %d lines, want %d", got, wantLines)
	}
}

func TestHistory_SpeedUpConfirmEmitsPresetNavigation(t *testing.T) {
	m, _ := speedUpTestModel(t, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	model, _ := m.Update(historyRune('b'))
	m = model.(TransactionHistoryModel)

	// Move from Fastest to 30 minutes.
	model, _ = m.Update(historyKey(tea.KeyDown))
	m = model.(TransactionHistoryModel)
	model, _ = m.Update(historyKey(tea.KeyEnter))
	m = model.(TransactionHistoryModel)

	var cmd tea.Cmd
	model, cmd = m.Update(historyKey(tea.KeyEnter))
	m = model.(TransactionHistoryModel)
	if m.speedUp != nil {
		t.Fatal("confirm should close the tray")
	}

	nav, ok := findMsgOfType[NavigateMsg](collectMsgs(cmd))
	if !ok {
		t.Fatalf("confirm did not navigate, got %v", collectMsgs(cmd))
	}
	if nav.Target != StateSend || nav.FeeBumpTxID != "zz_pending_out" {
		t.Fatalf("unexpected navigation: %+v", nav)
	}
	if nav.FeeBumpRate != 8 {
		t.Fatalf("preset rate = %v, want 8", nav.FeeBumpRate)
	}
}

func TestHistory_SpeedUpCustomUsesOldNavigation(t *testing.T) {
	m, _ := speedUpTestModel(t, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	model, _ := m.Update(historyRune('b'))
	m = model.(TransactionHistoryModel)

	for i := 0; i < 3; i++ {
		model, _ = m.Update(historyKey(tea.KeyDown))
		m = model.(TransactionHistoryModel)
	}
	model, cmd := m.Update(historyKey(tea.KeyEnter))
	m = model.(TransactionHistoryModel)
	if m.speedUp != nil {
		t.Fatal("custom should close the tray")
	}

	nav, ok := findMsgOfType[NavigateMsg](collectMsgs(cmd))
	if !ok {
		t.Fatalf("custom did not navigate, got %v", collectMsgs(cmd))
	}
	if nav.FeeBumpTxID != "zz_pending_out" || nav.FeeBumpRate != 0 {
		t.Fatalf("custom navigation = %+v, want the old fee bump flow", nav)
	}
}

func TestHistory_SpeedUpLoadsFeeEstimatesAsync(t *testing.T) {
	m, mock := speedUpTestModel(t, nil)
	mock.GetFeeEstimatesFn = func() (*api.FeeEstimates, error) {
		return &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3}, nil
	}

	model, cmd := m.Update(historyRune('b'))
	m = model.(TransactionHistoryModel)
	if m.speedUp == nil || !m.speedUp.loading {
		t.Fatal("tray should wait for fee estimates")
	}
	if !strings.Contains(stripANSI(m.View()), "…") {
		t.Fatalf("loading tray should show placeholder helpers:\n%s", stripANSI(m.View()))
	}

	msg := extractMsg(cmd)
	if _, ok := msg.(SpeedUpFeeEstimatesMsg); !ok {
		t.Fatalf("expected SpeedUpFeeEstimatesMsg, got %T", msg)
	}
	model, _ = m.Update(msg)
	m = model.(TransactionHistoryModel)
	if m.speedUp.loading {
		t.Fatal("tray still loading after estimates arrived")
	}
	if !strings.Contains(stripANSI(m.View()), "12 sat/vB") {
		t.Fatalf("loaded tray is missing the fastest estimate:\n%s", stripANSI(m.View()))
	}
}

func TestHistory_SpeedUpConfirmedRowStillErrors(t *testing.T) {
	m, _ := speedUpTestModel(t, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	m.transactions = []api.Transaction{{
		TxID: "confirmed_only", Status: api.Status{Confirmed: true, BlockHeight: 10, BlockTime: time.Now().Unix() - 100},
	}}
	m.updateTableData()

	model, _ := m.Update(historyRune('b'))
	m = model.(TransactionHistoryModel)
	if m.speedUp != nil {
		t.Fatal("confirmed row should not open the tray")
	}
	if m.err == nil || !strings.Contains(m.err.Error(), "already confirmed") {
		t.Fatalf("expected already-confirmed error, got %v", m.err)
	}
}

// newSpeedUpHistoryLayout builds a history screen with the tray already
// open, for the 80x24 and 120x40 fit tests and screenshots.
func newSpeedUpHistoryLayout(mock *MockWalletManager, confirm bool) TransactionHistoryModel {
	configureSpeedUpMock(mock, &api.FeeEstimates{FastestFee: 12, HalfHourFee: 8, HourFee: 3})
	m := NewTransactionHistoryModel(mock)
	m.width, m.height = 120, 40
	m.loading = false
	m.fadeIn = 1.0
	m.blockHeight = 200
	m.transactions = speedUpTransactions()
	m.updateTableData()

	model, _ := m.Update(historyRune('b'))
	m = model.(TransactionHistoryModel)
	if confirm {
		model, _ = m.Update(historyKey(tea.KeyEnter))
		m = model.(TransactionHistoryModel)
	}
	return m
}
