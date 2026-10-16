package models

import (
	"errors"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"

	tea "github.com/charmbracelet/bubbletea"
)

// TestViews_SurviveTinyAndOddTerminalSizes renders every top-level view at
// degenerate terminal sizes. A panic here (negative strings.Repeat, slice out
// of range, etc.) would crash the wallet when the user shrinks the window.
func TestViews_SurviveTinyAndOddTerminalSizes(t *testing.T) {
	sizes := [][2]int{{0, 0}, {1, 1}, {10, 5}, {20, 8}, {40, 12}, {80, 24}, {300, 100}}
	builders := map[string]func() tea.Model{
		"splash":     func() tea.Model { return NewSplashModel() },
		"unlock":     func() tea.Model { return NewUnlockModel(NewMockWalletManager()) },
		"dashboard":  func() tea.Model { return NewMainDashboardModel(NewMockWalletManager(), nil) },
		"receive":    func() tea.Model { return NewReceiveModel(NewMockWalletManager()) },
		"send":       func() tea.Model { return NewSendModel(NewMockWalletManager(), 0, nil) },
		"history":    func() tea.Model { return NewTransactionHistoryModel(NewMockWalletManager()) },
		"addresses":  func() tea.Model { return NewAddressesModel(NewMockWalletManager(), nil) },
		"utxos":      func() tea.Model { return NewUTXOsModel(NewMockWalletManager(), nil) },
		"settings":   func() tea.Model { return NewSettingsModel(NewMockWalletManager(), nil) },
		"help":       func() tea.Model { return NewHelpModel(NewMockWalletManager()) },
		"setup":      func() tea.Model { return NewWalletSetupModel(NewMockWalletManager(), nil) },
		"connection": func() tea.Model { return NewConnectionSetupModel(nil, false) },
	}
	for name, build := range builders {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%s/%dx%d", name, sz[0], sz[1]), func(t *testing.T) {
				m := build()
				m, _ = m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
				_ = m.View()
			})
		}
	}
}

// hostileMock returns a wallet populated with awkward data: short/malformed
// txids from a misbehaving backend, multi-byte labels and long error text.
func hostileMock() *MockWalletManager {
	mock := NewMockWalletManager()
	utxos := []api.UTXO{{TxID: "ab", Vout: 0, Value: 1000}, {TxID: "", Vout: 1, Value: 546}}
	txs := []api.Transaction{{TxID: "abc", Status: api.Status{Confirmed: true, BlockHeight: 1}}}
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:       wallet.SyncStatusError,
			LastError:    errors.New("连接失败: électrum server refused the connection ☕☕☕"),
			UTXOs:        utxos,
			Transactions: txs,
		}
	}
	mock.GetCachedUTXOsFn = func() ([]api.UTXO, bool) { return utxos, true }
	mock.GetCachedTransactionsFn = func() ([]api.Transaction, bool) { return txs, true }
	mock.GetTransactionLabelFn = func(string) string { return "☕☕☕☕☕☕☕☕☕☕☕☕☕☕☕☕☕☕" }
	return mock
}

func TestViews_SurviveHostileData(t *testing.T) {
	sizes := [][2]int{{20, 8}, {80, 24}, {200, 60}}
	builders := map[string]func() tea.Model{
		"dashboard": func() tea.Model { return NewMainDashboardModel(hostileMock(), nil) },
		"history":   func() tea.Model { return NewTransactionHistoryModel(hostileMock()) },
		"utxos":     func() tea.Model { return NewUTXOsModel(hostileMock(), nil) },
		"addresses": func() tea.Model { return NewAddressesModel(hostileMock(), nil) },
		"send":      func() tea.Model { return NewSendModel(hostileMock(), 1546, nil) },
	}
	for name, build := range builders {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%s/%dx%d", name, sz[0], sz[1]), func(t *testing.T) {
				m := build()
				m, _ = m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
				m, _ = m.Update(ScreenActiveMsg{})
				m, _ = m.Update(StateChangedMsg{State: hostileMock().GetState()})
				out := m.View()
				if !utf8.ValidString(out) {
					t.Fatalf("view produced invalid UTF-8 (string sliced mid-rune)")
				}
			})
		}
	}
}

func TestSendSteps_SurviveTinyTerminal(t *testing.T) {
	for step := SendStepAddress; step <= SendStepComplete; step++ {
		for _, sz := range [][2]int{{0, 0}, {10, 4}, {40, 12}} {
			t.Run(fmt.Sprintf("step%d/%dx%d", step, sz[0], sz[1]), func(t *testing.T) {
				m := NewSendModel(hostileMock(), 1546, nil)
				upd, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
				sm := upd.(SendModel)
				sm.step = step
				_ = sm.View()
			})
		}
	}
}
