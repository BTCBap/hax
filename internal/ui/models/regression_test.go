package models

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"hax/internal/api"
	"hax/internal/wallet"
)

// Regression tests for specific bugs that were fixed.
// Each test reproduces the original bug scenario and verifies the fix holds.

// --- Bug: Password field persisting characters after failed unlock ---
// The password input was not being cleared after a failed unlock attempt,
// so the old password characters remained visible on retry.
// Fix: unlock.go sets passwordInput.SetValue("") on UnlockResultMsg{Success: false}

func TestRegression_PasswordClearedAfterFailedUnlock(t *testing.T) {
	mock := NewMockWalletManager()
	mock.UnlockWalletFn = func(name, password string) error {
		return fmt.Errorf("incorrect password")
	}

	m := NewUnlockModel(mock)
	m.width = 80
	m.height = 24
	m.walletName = "test-wallet"
	m.walletList = []string{"test-wallet"}

	// Type a password
	m.passwordInput.SetValue("wrongpassword123")
	if m.passwordInput.Value() != "wrongpassword123" {
		t.Fatal("password should be set before unlock attempt")
	}

	// Simulate unlock failure
	model, _ := m.Update(UnlockResultMsg{Success: false, Err: fmt.Errorf("incorrect password")})
	updated := model.(UnlockModel)

	// Password field MUST be cleared after failed unlock
	if updated.passwordInput.Value() != "" {
		t.Errorf("password field should be empty after failed unlock, got %q", updated.passwordInput.Value())
	}

	// Error should be set
	if updated.err == nil {
		t.Error("expected error to be set after failed unlock")
	}

	// Should not be in unlocking state
	if updated.unlocking {
		t.Error("should not be unlocking after failure")
	}
}

func TestRegression_PasswordClearedOnSecondFailure(t *testing.T) {
	mock := NewMockWalletManager()
	failCount := 0
	mock.UnlockWalletFn = func(name, password string) error {
		failCount++
		return fmt.Errorf("incorrect password (attempt %d)", failCount)
	}

	m := NewUnlockModel(mock)
	m.width = 80
	m.height = 24
	m.walletName = "test-wallet"
	m.walletList = []string{"test-wallet"}

	// First failed attempt
	m.passwordInput.SetValue("attempt1")
	model, _ := m.Update(UnlockResultMsg{Success: false, Err: fmt.Errorf("incorrect password")})
	updated := model.(UnlockModel)
	if updated.passwordInput.Value() != "" {
		t.Errorf("password not cleared after first failure: %q", updated.passwordInput.Value())
	}

	// Second failed attempt (simulating user typing again)
	updated.passwordInput.SetValue("attempt2")
	model, _ = updated.Update(UnlockResultMsg{Success: false, Err: fmt.Errorf("incorrect password")})
	updated = model.(UnlockModel)
	if updated.passwordInput.Value() != "" {
		t.Errorf("password not cleared after second failure: %q", updated.passwordInput.Value())
	}
}

// --- Bug: History export labels crash ---
// Exporting labels when no labels exist or when transactions are nil caused a crash.
// Fix: ExportLabelsBIP329 returns an error ("no transaction labels to export")
// which the UI handles gracefully instead of panicking.

func TestRegression_ExportLabelsNoLabels(t *testing.T) {
	mock := NewMockWalletManager()
	mock.ExportLabelsBIP329Fn = func() (string, error) {
		return "", fmt.Errorf("no transaction labels to export")
	}
	mock.GetTransactionLabelFn = func(string) string { return "" }

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = false
	m.transactions = sampleTransactions()
	m.isActive = true
	m.updateTableData()

	// Press 'E' to export labels — should not panic
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	updated := model.(TransactionHistoryModel)

	// Should show error, not crash
	if updated.err == nil {
		t.Error("expected error message when no labels to export")
	}
}

func TestRegression_ExportLabelsWithNilTransactions(t *testing.T) {
	mock := NewMockWalletManager()
	mock.ExportLabelsBIP329Fn = func() (string, error) {
		return "", fmt.Errorf("no transaction labels to export")
	}

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = false
	m.transactions = nil // No transactions loaded
	m.isActive = true

	// Press 'E' — should not panic even with nil transactions
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	updated := model.(TransactionHistoryModel)

	if updated.err == nil {
		t.Error("expected error message when no labels to export")
	}
}

func TestRegression_KoinlyExportWithNilTransactions(t *testing.T) {
	mock := NewMockWalletManager()
	mock.ExportKoinlyFn = func(txs []api.Transaction) (string, error) {
		return "", fmt.Errorf("no transactions to export")
	}

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = false
	m.transactions = nil
	m.isActive = true

	// Press 'K' — should not panic with nil transactions
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	updated := model.(TransactionHistoryModel)

	if updated.err == nil {
		t.Error("expected error message when exporting with nil transactions")
	}
}

// --- Bug: ESC while loading breaks History screen ---
// Pressing ESC while transactions were still loading left the History screen
// in a broken state. The fix ensures ESC always navigates back to dashboard
// regardless of loading state.

func TestRegression_EscWhileLoadingHistory(t *testing.T) {
	mock := NewMockWalletManager()
	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = true // Actively loading
	m.isActive = true

	// Press ESC while loading
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Must navigate back to dashboard, not get stuck
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestRegression_QWhileLoadingHistory(t *testing.T) {
	mock := NewMockWalletManager()
	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = true
	m.isActive = true

	// Press 'q' while loading
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Must navigate back to dashboard
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestRegression_EscWhileLoadingHistory_NotInEditOrSearchMode(t *testing.T) {
	mock := NewMockWalletManager()
	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = true
	m.isActive = true

	// Ensure we're not in edit/search mode which intercepts ESC differently
	m.editMode = false
	m.searchMode = false

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestRegression_EscDuringHistorySearchMode(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetTransactionLabelFn = func(string) string { return "" }

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = false
	m.isActive = true
	m.transactions = sampleTransactions()
	m.updateTableData()

	// Enter search mode
	m.searchMode = true
	m.searchInput.SetValue("test query")

	// ESC in search mode should exit search, not navigate away
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(TransactionHistoryModel)

	if updated.searchMode {
		t.Error("ESC should exit search mode")
	}
	if updated.searchQuery != "" {
		t.Error("search query should be cleared")
	}
	// Should NOT navigate away — just exit search mode
	if cmd != nil {
		msgs := collectMsgs(cmd)
		if _, found := findMsgOfType[NavigateMsg](msgs); found {
			t.Error("ESC in search mode should not navigate away")
		}
	}
}

func TestRegression_HistoryViewDuringLoading_DoesNotPanic(t *testing.T) {
	mock := NewMockWalletManager()
	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = true
	m.isActive = true

	// View() during loading should not panic
	out := m.View()
	if out == "" {
		t.Error("View() should produce output even while loading")
	}
}

func TestRegression_HistoryTransitionFromLoadingToLoaded(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetTransactionLabelFn = func(string) string { return "" }

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = true
	m.isActive = true

	// Receive transactions while loading
	model, _ := m.Update(TransactionsMsg{
		Transactions: sampleTransactions(),
		BlockHeight:  100,
	})
	updated := model.(TransactionHistoryModel)

	if updated.loading {
		t.Error("loading should be false after TransactionsMsg")
	}
	if len(updated.transactions) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(updated.transactions))
	}

	// ESC should still work after transitioning from loading to loaded
	_, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

// --- Additional regression: State change during loading ---

func TestRegression_StateChangeWhileLoadingHistory(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetTransactionLabelFn = func(string) string { return "" }

	m := NewTransactionHistoryModel(mock)
	m.width = 120
	m.height = 40
	m.loading = true
	m.isActive = true

	// Receive state change while still loading
	model, _ := m.Update(StateChangedMsg{
		State: wallet.WalletStateSnapshot{
			Transactions: sampleTransactions(),
			BlockHeight:  300,
		},
	})
	updated := model.(TransactionHistoryModel)

	// Should handle gracefully
	if len(updated.transactions) != 2 {
		t.Errorf("expected 2 transactions from state change, got %d", len(updated.transactions))
	}
	if updated.blockHeight != 300 {
		t.Errorf("expected blockHeight=300, got %d", updated.blockHeight)
	}
}
