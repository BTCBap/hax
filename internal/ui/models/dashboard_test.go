package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

func newTestDashboard(t *testing.T) (MainDashboardModel, *MockWalletManager) {
	t.Helper()
	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	mock.GetCachedBalanceFn = func() (api.Satoshi, bool) { return 100000, true }
	m := NewMainDashboardModel(mock, nil)
	m.width = 120
	m.height = 40
	return m, mock
}

func TestDashboard_InitialState(t *testing.T) {
	m, _ := newTestDashboard(t)
	if !m.loading {
		t.Error("expected loading=true initially")
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.cursor)
	}
}

func TestDashboard_Init_FetchesBalance(t *testing.T) {
	m, _ := newTestDashboard(t)
	cmd := m.Init()
	msgs := collectMsgs(cmd)

	// Should produce a BalanceMsg from fetchBalance
	if !containsMsgOfType[BalanceMsg](msgs) {
		t.Error("Init should produce a BalanceMsg")
	}
	bal, found := findMsgOfType[BalanceMsg](msgs)
	if found && bal.Err != nil {
		t.Errorf("unexpected error in BalanceMsg: %v", bal.Err)
	}
	if found && bal.Balance != 100000 {
		t.Errorf("expected balance 100000, got %d", bal.Balance)
	}
}

func TestDashboard_BalanceMsg_UpdatesBalance(t *testing.T) {
	m, _ := newTestDashboard(t)
	model, _ := m.Update(BalanceMsg{Balance: 250000, Err: nil})
	updated := model.(MainDashboardModel)
	if updated.balance != 250000 {
		t.Errorf("expected balance=250000, got %d", updated.balance)
	}
	if !updated.balanceLoaded {
		t.Error("expected balanceLoaded=true")
	}
	if updated.loading {
		t.Error("expected loading=false after BalanceMsg")
	}
}

func TestDashboard_StateChangedMsg_UpdatesBalance(t *testing.T) {
	m, mock := newTestDashboard(t)
	// Set mock state to return synced status
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:      wallet.SyncStatusSynced,
			Balance:     500000,
			BlockHeight: 12345,
		}
	}

	model, _ := m.Update(StateChangedMsg{
		State: wallet.WalletStateSnapshot{
			Status:      wallet.SyncStatusSynced,
			Balance:     500000,
			BlockHeight: 12345,
		},
	})
	updated := model.(MainDashboardModel)
	if updated.balance != 500000 {
		t.Errorf("expected balance=500000, got %d", updated.balance)
	}
	if updated.blockHeight != 12345 {
		t.Errorf("expected blockHeight=12345, got %d", updated.blockHeight)
	}
}

func TestDashboard_KeyNavigation(t *testing.T) {
	tests := []struct {
		key    string
		target AppState
	}{
		{"r", StateReceive},
		{"s", StateSend},
		{"h", StateTransactionHistory},
		{"a", StateAddresses},
		{"u", StateUTXOs},
		{"t", StateSettings},
		{"?", StateHelp},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m, _ := newTestDashboard(t)
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			assertNavigatesTo(t, cmd, tt.target)
		})
	}
}

func TestDashboard_ArrowNavigation(t *testing.T) {
	m, _ := newTestDashboard(t)

	// Start at 0, go down
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := model.(MainDashboardModel)
	if updated.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", updated.cursor)
	}

	// Go up back to 0
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated = model.(MainDashboardModel)
	if updated.cursor != 0 {
		t.Errorf("expected cursor=0 after up, got %d", updated.cursor)
	}

	// Up at 0 stays at 0
	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated = model.(MainDashboardModel)
	if updated.cursor != 0 {
		t.Errorf("expected cursor=0 (clamped), got %d", updated.cursor)
	}
}

func TestDashboard_EnterSelectsMenuItem(t *testing.T) {
	m, _ := newTestDashboard(t)

	// Move cursor to item 2 (History)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press enter
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assertNavigatesTo(t, cmd, StateTransactionHistory)
}

func TestDashboard_Quit(t *testing.T) {
	m, _ := newTestDashboard(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	msg := extractMsg(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestDashboard_View_ShowsWalletName(t *testing.T) {
	m, mock := newTestDashboard(t)
	mock.GetWalletNameFn = func() string { return "my-test-wallet" }
	m.balanceLoaded = true

	view := m.View()
	if !strings.Contains(view, "my-test-wallet") {
		t.Error("View should display the wallet name")
	}
}

func TestDashboard_View_ShowsMenu(t *testing.T) {
	m, _ := newTestDashboard(t)
	view := m.View()
	if !strings.Contains(view, "Receive") {
		t.Error("View should display Receive menu option")
	}
	if !strings.Contains(view, "Send") {
		t.Error("View should display Send menu option")
	}
	if !strings.Contains(view, "History") {
		t.Error("View should display History menu option")
	}
}

func TestDashboard_View_ShowsMultisigIndicator(t *testing.T) {
	m, mock := newTestDashboard(t)
	mock.IsMultisigFn = func() bool { return true }
	mock.GetMultisigConfigFn = func() *wallet.MultisigWalletConfig {
		return &wallet.MultisigWalletConfig{M: 2, N: 3}
	}
	m.balanceLoaded = true

	view := m.View()
	if !strings.Contains(view, "2-of-3") {
		t.Error("View should show multisig indicator")
	}
}

func TestDashboard_View_ShowsDisconnectedStatus(t *testing.T) {
	m, mock := newTestDashboard(t)
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{Status: wallet.SyncStatusDisconnected}
	}

	view := m.View()
	if !strings.Contains(view, "Disconnected") {
		t.Error("View should show disconnected status")
	}
}

func TestDashboard_View_ShowsSyncedStatus(t *testing.T) {
	m, mock := newTestDashboard(t)
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:      wallet.SyncStatusSynced,
			BlockHeight: 99999,
		}
	}

	view := m.View()
	if !strings.Contains(view, "Synced") {
		t.Error("View should show synced status")
	}
	if !strings.Contains(view, "99999") {
		t.Error("View should show block height")
	}
}

func TestDashboard_WindowSizeMsg(t *testing.T) {
	m, _ := newTestDashboard(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	updated := model.(MainDashboardModel)
	if updated.width != 200 {
		t.Errorf("expected width=200, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height=50, got %d", updated.height)
	}
}
