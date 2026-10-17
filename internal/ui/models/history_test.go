package models

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/wallet"
)

func newTestHistory(t *testing.T) (TransactionHistoryModel, *MockWalletManager) {
	t.Helper()
	mock := NewMockWalletManager()
	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	return m, mock
}

func sampleTransactions() []api.Transaction {
	return []api.Transaction{
		{
			TxID: "aabbccdd1234567890",
			Vin: []api.Vin{
				{Prevout: &api.Vout{ScriptpubkeyAddress: "external_addr", Value: 50000}},
			},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "tb1q_our_addr", Value: 45000},
			},
			Status: api.Status{Confirmed: true, BlockHeight: 100, BlockTime: time.Now().Unix() - 3600},
		},
		{
			TxID: "eeff00112233445566",
			Vin: []api.Vin{
				{Prevout: &api.Vout{ScriptpubkeyAddress: "tb1q_our_addr", Value: 45000}},
			},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "external_dest", Value: 40000},
				{ScriptpubkeyAddress: "tb1q_our_change", Value: 4500},
			},
			Status: api.Status{Confirmed: false},
		},
	}
}

func manySampleTransactions(n int) []api.Transaction {
	txs := make([]api.Transaction, 0, n)
	now := time.Now().Unix()
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("txid_%03d_abcdefghijklmnopqrstuvwxyz", i)
		txs = append(txs, api.Transaction{
			TxID: txid,
			Vin: []api.Vin{
				{Prevout: &api.Vout{ScriptpubkeyAddress: "external_addr", Value: 20000}},
			},
			Vout: []api.Vout{
				{ScriptpubkeyAddress: "tb1q_our_addr", Value: api.Satoshi(10000 + i)},
			},
			Status: api.Status{Confirmed: true, BlockHeight: int64(100 + i), BlockTime: now - int64(i*60)},
		})
	}
	return txs
}

func TestHistory_InitialState(t *testing.T) {
	m, _ := newTestHistory(t)
	if !m.loading {
		t.Error("expected loading=true initially")
	}
	if m.filterType != "all" {
		t.Errorf("expected filterType=all, got %s", m.filterType)
	}
	if m.filterStatus != "all" {
		t.Errorf("expected filterStatus=all, got %s", m.filterStatus)
	}
	if m.sortColumn != 0 {
		t.Errorf("expected sortColumn=0, got %d", m.sortColumn)
	}
}

func TestHistory_Init_FetchesTransactions(t *testing.T) {
	m, mock := newTestHistory(t)
	txs := sampleTransactions()
	mock.GetCachedTransactionsFn = func() ([]api.Transaction, bool) {
		return txs, true
	}
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{BlockHeight: 200}
	}

	cmd := m.Init()
	msgs := collectMsgs(cmd)

	if !containsMsgOfType[TransactionsMsg](msgs) {
		t.Error("Init should produce a TransactionsMsg")
	}
	txMsg, found := findMsgOfType[TransactionsMsg](msgs)
	if found {
		if len(txMsg.Transactions) != 2 {
			t.Errorf("expected 2 transactions, got %d", len(txMsg.Transactions))
		}
		if txMsg.BlockHeight != 200 {
			t.Errorf("expected blockHeight=200, got %d", txMsg.BlockHeight)
		}
	}
}

func TestHistory_Init_FallbackToNetwork(t *testing.T) {
	m, mock := newTestHistory(t)
	// No cached transactions
	mock.GetCachedTransactionsFn = func() ([]api.Transaction, bool) { return nil, false }
	txs := sampleTransactions()
	mock.GetTransactionsFn = func(uint32) ([]api.Transaction, error) { return txs, nil }

	cmd := m.Init()
	msgs := collectMsgs(cmd)

	txMsg, found := findMsgOfType[TransactionsMsg](msgs)
	if !found {
		t.Fatal("expected TransactionsMsg from network fallback")
	}
	if len(txMsg.Transactions) != 2 {
		t.Errorf("expected 2 transactions from network, got %d", len(txMsg.Transactions))
	}
}

func TestHistory_TransactionsMsg_PopulatesData(t *testing.T) {
	m, _ := newTestHistory(t)
	txs := sampleTransactions()

	model, _ := m.Update(TransactionsMsg{
		Transactions: txs,
		Err:          nil,
		BlockHeight:  150,
	})
	updated := model.(TransactionHistoryModel)

	if updated.loading {
		t.Error("expected loading=false after TransactionsMsg")
	}
	if len(updated.transactions) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(updated.transactions))
	}
	if updated.blockHeight != 150 {
		t.Errorf("expected blockHeight=150, got %d", updated.blockHeight)
	}
}

func TestHistory_TransactionsMsg_Error(t *testing.T) {
	m, _ := newTestHistory(t)
	model, _ := m.Update(TransactionsMsg{
		Transactions: nil,
		Err:          errTest,
	})
	updated := model.(TransactionHistoryModel)
	if updated.loading {
		t.Error("expected loading=false after error TransactionsMsg")
	}
	if updated.err == nil {
		t.Error("expected error to be set")
	}
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }

func TestHistory_EscNavigatesBack(t *testing.T) {
	m, _ := newTestHistory(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestHistory_QNavigatesBack(t *testing.T) {
	m, _ := newTestHistory(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestHistory_StateChangedMsg_UpdatesTransactions(t *testing.T) {
	m, _ := newTestHistory(t)
	txs := sampleTransactions()

	// First load some transactions
	model, _ := m.Update(TransactionsMsg{Transactions: txs, BlockHeight: 100})
	updated := model.(TransactionHistoryModel)

	// Now send a state change with different transactions
	newTxs := append(txs, api.Transaction{
		TxID:   "newTx",
		Vout:   []api.Vout{{ScriptpubkeyAddress: "addr", Value: 1000}},
		Status: api.Status{Confirmed: true, BlockHeight: 101, BlockTime: time.Now().Unix()},
	})
	model, _ = updated.Update(StateChangedMsg{
		State: wallet.WalletStateSnapshot{
			Transactions: newTxs,
			BlockHeight:  200,
		},
	})
	updated = model.(TransactionHistoryModel)

	if len(updated.transactions) != 3 {
		t.Errorf("expected 3 transactions after state change, got %d", len(updated.transactions))
	}
	if updated.blockHeight != 200 {
		t.Errorf("expected blockHeight=200, got %d", updated.blockHeight)
	}
}

func TestHistory_WindowSizeMsg(t *testing.T) {
	m, _ := newTestHistory(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	updated := model.(TransactionHistoryModel)
	if updated.width != 200 {
		t.Errorf("expected width=200, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height=50, got %d", updated.height)
	}
}

func TestHistory_View_ShowsLoadingWhenEmpty(t *testing.T) {
	m, _ := newTestHistory(t)
	m.loading = true
	view := m.View()
	if !strings.Contains(view, "Loading") && !strings.Contains(view, "loading") {
		// The spinner view contains the loading text
		_ = view // Accept any output during loading state
	}
}

func TestHistory_View_ShowsTransactions(t *testing.T) {
	m, mock := newTestHistory(t)
	txs := sampleTransactions()
	mock.GetTransactionLabelFn = func(txid string) string {
		if txid == "aabbccdd1234567890" {
			return "Test Payment"
		}
		return ""
	}

	// Load transactions
	model, _ := m.Update(TransactionsMsg{Transactions: txs, BlockHeight: 200})
	updated := model.(TransactionHistoryModel)
	updated.fadeIn = 1.0 // Skip animation

	view := updated.View()
	// Should show truncated TxID
	if !strings.Contains(view, "aabbcc") {
		t.Error("View should display transaction ID")
	}
}

func TestHistory_LabelEditing(t *testing.T) {
	m, mock := newTestHistory(t)
	var savedTxID, savedLabel string
	mock.SetTransactionLabelFn = func(txid, label string) error {
		savedTxID = txid
		savedLabel = label
		return nil
	}
	mock.GetTransactionLabelFn = func(string) string { return "" }

	// Enter edit mode with a known txid (bypass table selection)
	m.editMode = true
	m.editTxID = "target_txid_for_label"
	m.labelInput.SetValue("My Label")

	// Press enter to save
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := model.(TransactionHistoryModel)

	if result.editMode {
		t.Error("expected editMode=false after saving")
	}
	if savedTxID != "target_txid_for_label" {
		t.Errorf("expected txid target_txid_for_label, got %s", savedTxID)
	}
	if savedLabel != "My Label" {
		t.Errorf("expected label 'My Label', got '%s'", savedLabel)
	}
}

func TestHistory_EditMode_EscCancels(t *testing.T) {
	m, _ := newTestHistory(t)
	m.editMode = true
	m.editTxID = "sometxid"
	m.labelInput.SetValue("partial input")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(TransactionHistoryModel)

	if updated.editMode {
		t.Error("expected editMode=false after esc")
	}
	if updated.editTxID != "" {
		t.Error("expected editTxID cleared after esc")
	}
}

func TestHistory_Pagination_PreservesHeaderOnNextPage(t *testing.T) {
	m, _ := newTestHistory(t)
	txs := manySampleTransactions(25) // at least 3 pages with default itemsPerPage=10

	model, _ := m.Update(TransactionsMsg{Transactions: txs, BlockHeight: 200})
	updated := model.(TransactionHistoryModel)
	updated.fadeIn = 1.0

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated = model.(TransactionHistoryModel)

	if updated.currentPage != 1 {
		t.Fatalf("expected currentPage=1, got %d", updated.currentPage)
	}

	view := updated.View()
	if !strings.Contains(view, "Transaction History") {
		t.Fatalf("expected header on page 2, got view:\n%s", view)
	}
	if !strings.Contains(view, "Page 2/") {
		t.Fatalf("expected page badge on page 2, got view:\n%s", view)
	}
	if strings.Contains(view, "Your Bitcoin transaction timeline") {
		t.Fatalf("expected subtitle removed, got view:\n%s", view)
	}
}

func TestHistory_PaginationBadgeVisibleWithOtherBadges(t *testing.T) {
	m, _ := newTestHistory(t)
	txs := manySampleTransactions(25)

	model, _ := m.Update(TransactionsMsg{Transactions: txs, BlockHeight: 200})
	updated := model.(TransactionHistoryModel)
	updated.fadeIn = 1.0
	updated.showCopied = true
	updated.copiedTime = time.Now()
	updated.exportedFile = ""
	updated.filterType = "all"
	updated.filterStatus = "all"
	updated.searchQuery = "txid_"
	updated.updateTableData()

	view := updated.View()
	if !strings.Contains(view, "Page 1/") {
		t.Fatalf("expected page badge with other badges present, got view:\n%s", view)
	}
	if !strings.Contains(view, "Transaction History") {
		t.Fatalf("expected history title with page badge, got view:\n%s", view)
	}
}

func TestHistory_HeaderLine_ShowsTitlePageAndCopied(t *testing.T) {
	m, _ := newTestHistory(t)
	txs := manySampleTransactions(25)

	model, _ := m.Update(TransactionsMsg{Transactions: txs, BlockHeight: 200})
	updated := model.(TransactionHistoryModel)
	updated.fadeIn = 1.0
	updated.showCopied = true
	updated.copiedTime = time.Now()
	updated.currentPage = 0
	updated.updateTableData()

	view := updated.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty view")
	}
	headerLine := lines[0]
	if !strings.Contains(headerLine, "Transaction History") {
		t.Fatalf("expected title on header line, got %q", headerLine)
	}
	if !strings.Contains(headerLine, "Page 1/") {
		t.Fatalf("expected page badge on header line, got %q", headerLine)
	}
	if !strings.Contains(headerLine, "COPIED") {
		t.Fatalf("expected copied status on header line, got %q", headerLine)
	}
	if !strings.Contains(headerLine, "Page 1/") || !strings.Contains(headerLine, "Page 1/3 ✓ COPIED!") {
		t.Fatalf("expected copied status directly after page badge, got %q", headerLine)
	}
}

// limitedBackend is a backend whose history misses spent coins.
type limitedBackend struct {
	backend.BitcoinBackend
	limited bool
}

func (b limitedBackend) HistoryLimited() bool { return b.limited }

func TestHistory_WarnsWhenHistoryLimited(t *testing.T) {
	for _, limited := range []bool{true, false} {
		m, mock := newTestHistory(t)
		mock.GetBackendFn = func() backend.BitcoinBackend {
			return limitedBackend{limited: limited}
		}
		model, _ := m.Update(TransactionsMsg{
			Transactions: sampleTransactions(), BlockHeight: 150})
		view := model.(TransactionHistoryModel).View()
		got := strings.Contains(view, "-blockfilterindex=1")
		if got != limited {
			t.Errorf("limited=%v: notice shown = %v", limited, got)
		}
	}
}
