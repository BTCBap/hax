package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

const (
	stealthReceiveAddr = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	stealthChangeAddr  = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7"
	stealthPayeeAddr   = "tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l"
	stealthTxID        = "3b7f2e9c1a4d8f60b5e2c7d93a1f4e8b6c0d2a5f7e9b1c3d5f7a9b1c3e5d7f90"
)

func stealthTestState() wallet.WalletStateSnapshot {
	tx := api.Transaction{
		TxID: stealthTxID,
		Vin:  []api.Vin{{Prevout: &api.Vout{ScriptpubkeyAddress: stealthPayeeAddr, Value: 1_500_000}}},
		Vout: []api.Vout{
			{ScriptpubkeyAddress: stealthReceiveAddr, Value: 1_000_000},
			{ScriptpubkeyAddress: stealthChangeAddr, Value: 499_859},
		},
		Status: api.Status{Confirmed: true, BlockHeight: 90, BlockTime: 1_760_000_000},
	}
	utxo := api.UTXO{TxID: stealthTxID, Vout: 0, Value: 1_000_000, Status: api.Status{Confirmed: true, BlockHeight: 90}}
	return wallet.WalletStateSnapshot{
		Status:       wallet.SyncStatusSynced,
		Balance:      1_000_000,
		BlockHeight:  100,
		Transactions: []api.Transaction{tx},
		UTXOs:        []api.UTXO{utxo},
	}
}

func stealthTestMock() *MockWalletManager {
	state := stealthTestState()
	accounts := []wallet.AccountData{{
		Addresses: []wallet.AddressData{
			{Address: stealthReceiveAddr},
			{Address: stealthChangeAddr, Change: true},
		},
	}}
	mock := NewMockWalletManager()
	mock.GetStateFn = func() wallet.WalletStateSnapshot { return state }
	mock.GetCachedBalanceFn = func() (api.Satoshi, bool) { return state.Balance, true }
	mock.GetAccountsFn = func() []wallet.AccountData { return accounts }
	mock.GetTransactionLabelFn = func(string) string { return "Faucet" }
	mock.GetCachedTransactionsFn = func() ([]api.Transaction, bool) { return state.Transactions, true }
	mock.GetCurrentReceiveAddrFn = func(uint32) (string, string, uint32, error) {
		return stealthReceiveAddr, "m/84'/1'/0'/0/0", 0, nil
	}
	mock.GetCachedAddrBalanceFn = func(string) (api.Satoshi, bool) { return 1_000_000, true }
	return mock
}

func stealthDashboard() MainDashboardModel {
	m := NewMainDashboardModel(stealthTestMock(), nil)
	m.loading = false
	m.balance = 1_000_000
	m.balanceLoaded = true
	return m
}

func stealthHistory() TransactionHistoryModel {
	m := NewTransactionHistoryModel(stealthTestMock())
	m.loading = false
	m.fadeIn = 1.0
	m.isActive = true
	m.blockHeight = 100
	m.transactions = stealthTestState().Transactions
	m.updateTableData()
	return m
}

func stealthUTXOs() UTXOsModel {
	m := NewUTXOsModel(stealthTestMock(), nil)
	m.utxos = stealthTestState().UTXOs
	m.sortAndUpdateTable()
	return m
}

func stealthAddresses() AddressesModel {
	m := NewAddressesModel(stealthTestMock(), nil)
	m.receiveAddresses = []AddrDisplayInfo{{
		Address: stealthReceiveAddr,
		Path:    "m/84'/1'/0'/0/0",
		Index:   0,
		Used:    true,
		Balance: 1_000_000,
	}}
	m.updateTable()
	return m
}

func stealthReceive() ReceiveModel {
	m := NewReceiveModel(stealthTestMock())
	m.selectedAddress = stealthReceiveAddr
	m.derivationPath = "m/84'/1'/0'/0/0"
	m.addressBalance = 1_000_000
	m.revealProgress = 1.0
	return m
}

func TestMaskAmount(t *testing.T) {
	visible := "0.00123456 BTC"
	if got := maskAmount(visible, false); got != visible {
		t.Fatalf("visible amount changed: %q", got)
	}
	hidden := maskAmount(visible, true)
	if lipgloss.Width(hidden) != lipgloss.Width(visible) {
		t.Fatalf("mask width = %d, want %d", lipgloss.Width(hidden), lipgloss.Width(visible))
	}
	if !strings.HasSuffix(hidden, "••••••") {
		t.Fatalf("mask = %q, want bullets", hidden)
	}
	short := maskAmount("0 BTC", true)
	if lipgloss.Width(short) != lipgloss.Width("0 BTC") {
		t.Fatalf("short mask width = %d", lipgloss.Width(short))
	}
	if maskAmount("", true) != "" {
		t.Fatal("empty amount should stay empty")
	}
}

func TestStealthToggleKeepsLayoutStable(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		build  func() tea.Model
	}{
		{"dashboard", []string{"Balance:", "Recent Activity"}, func() tea.Model { return stealthDashboard() }},
		{"history", []string{"Amount"}, func() tea.Model { return stealthHistory() }},
		{"utxos", []string{"Balance:", "Value"}, func() tea.Model { return stealthUTXOs() }},
		{"addresses", []string{"Balance"}, func() tea.Model { return stealthAddresses() }},
		{"receive", []string{"Balance:"}, func() tea.Model { return stealthReceive() }},
	}
	sizes := []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}}
	for _, tc := range cases {
		for _, size := range sizes {
			t.Run(tc.name+"/"+size.name, func(t *testing.T) {
				m := tc.build()
				m, _ = m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
				before := m.View()

				m, _ = m.Update(StealthToggledMsg{On: true})
				after := m.View()
				if got, want := requireLineCount(t, after), requireLineCount(t, before); got != want {
					t.Fatalf("line count changed from %d to %d", want, got)
				}
				for _, label := range tc.labels {
					if got, want := requireTextPosition(t, after, label), requireTextPosition(t, before, label); got != want {
						t.Fatalf("%q moved from %+v to %+v", label, want, got)
					}
				}
				if strings.Contains(stripANSI(before), "••••••") {
					t.Fatal("amounts were masked before stealth was enabled")
				}
				if !strings.Contains(stripANSI(after), "••••••") {
					t.Fatalf("no masked amount after enabling stealth:\n%s", stripANSI(after))
				}
				if !strings.Contains(stripANSI(after), "◌ hidden") {
					t.Fatalf("stealth badge missing:\n%s", stripANSI(after))
				}
				assertFits(t, after, size.w)
			})
		}
	}
}

func TestPassphraseBadgeKeepsLayoutStable(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}} {
		t.Run("dashboard/"+size.name, func(t *testing.T) {
			mock := stealthTestMock()
			off, _ := tea.Model(NewMainDashboardModel(mock, nil)).Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			before := off.View()

			mock.HasPassphraseFn = func() bool { return true }
			on, _ := tea.Model(NewMainDashboardModel(mock, nil)).Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			after := on.View()

			assertBadgeStable(t, before, after, []string{"Balance:", "Recent Activity"}, "+ passphrase")
			assertFits(t, after, size.w)
		})
		t.Run("settings/"+size.name, func(t *testing.T) {
			mock := stealthTestMock()
			off, _ := tea.Model(NewSettingsModel(mock, nil)).Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			before := off.View()

			mock.HasPassphraseFn = func() bool { return true }
			on, _ := tea.Model(NewSettingsModel(mock, nil)).Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			after := on.View()

			assertBadgeStable(t, before, after, []string{"Wallet Name:", "Type:"}, "+ passphrase")
			assertFits(t, after, size.w)
		})
	}
}

func assertBadgeStable(t *testing.T, before, after string, labels []string, badge string) {
	t.Helper()
	if strings.Contains(stripANSI(before), badge) {
		t.Fatalf("badge %q shown before it was enabled", badge)
	}
	if !strings.Contains(stripANSI(after), badge) {
		t.Fatalf("badge %q missing:\n%s", badge, stripANSI(after))
	}
	if got, want := requireLineCount(t, after), requireLineCount(t, before); got != want {
		t.Fatalf("line count changed from %d to %d", want, got)
	}
	for _, label := range labels {
		if got, want := requireTextPosition(t, after, label), requireTextPosition(t, before, label); got != want {
			t.Fatalf("%q moved from %+v to %+v", label, want, got)
		}
	}
}

func TestStealthExceptionsShowRealAmounts(t *testing.T) {
	cases := []struct {
		name  string
		want  string
		build func(t *testing.T) tea.Model
	}{
		{"send-review", "BTC", func(t *testing.T) tea.Model {
			return newSendReviewFlowModel(t, 80, 40)
		}},
		{"psbt-inspector", "BTC", func(t *testing.T) tea.Model {
			return newPSBTInspectorFlowModel(t, 80, 40)
		}},
		{"fee-bump", "282 sats", func(t *testing.T) tea.Model {
			m, _, _ := newBumpTest(t, bumpInfo())
			m, _ = update(t, m, m.analyzeFeeBump()())
			return m
		}},
		{"visualizer", "0.00100000 BTC", func(t *testing.T) tea.Model {
			m := newHistoryFlowModel(t, 100, 30)
			return txFlowTestUpdate(t, m, txFlowTestKey("v"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.build(t)
			m, _ = m.Update(StealthToggledMsg{On: true})
			view := stripANSI(m.View())
			if strings.Contains(view, "••••") {
				t.Fatalf("exception screen masked an amount:\n%s", view)
			}
			if !strings.Contains(view, tc.want) {
				t.Fatalf("real amount %q missing:\n%s", tc.want, view)
			}
		})
	}
}
