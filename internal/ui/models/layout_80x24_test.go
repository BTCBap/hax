package models

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
)

// TestViews_FitIn80Columns renders each top-level view, with empty and with
// awkward data, in an 80×24 terminal and fails if any line is wider than 80.
func TestViews_FitIn80Columns(t *testing.T) {
	builders := map[string]func(*MockWalletManager) tea.Model{
		"splash":     func(*MockWalletManager) tea.Model { return NewSplashModel() },
		"unlock":     func(w *MockWalletManager) tea.Model { return NewUnlockModel(w) },
		"dashboard":  func(w *MockWalletManager) tea.Model { return NewMainDashboardModel(w, nil) },
		"receive":    func(w *MockWalletManager) tea.Model { return NewReceiveModel(w) },
		"send":       func(w *MockWalletManager) tea.Model { return NewSendModel(w, 0, nil) },
		"history":    func(w *MockWalletManager) tea.Model { return NewTransactionHistoryModel(w) },
		"addresses":  func(w *MockWalletManager) tea.Model { return NewAddressesModel(w, nil) },
		"utxos":      func(w *MockWalletManager) tea.Model { return NewUTXOsModel(w, nil) },
		"settings":   func(w *MockWalletManager) tea.Model { return NewSettingsModel(w, nil) },
		"help":       func(w *MockWalletManager) tea.Model { return NewHelpModel(w) },
		"setup":      func(w *MockWalletManager) tea.Model { return NewWalletSetupModel(w, nil) },
		"connection": func(*MockWalletManager) tea.Model { return NewConnectionSetupModel(nil, false) },
	}
	mocks := map[string]func() *MockWalletManager{"empty": NewMockWalletManager, "hostile": hostileMock}
	for name, build := range builders {
		for mname, mk := range mocks {
			t.Run(name+"/"+mname, func(t *testing.T) {
				m := build(mk())
				m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
				assertFits(t, m.View(), 80)
			})
		}
	}
}

func assertFits(t *testing.T, view string, width int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("line %d is %d cells wide (max %d): %q", i, w, width, ansi.Strip(line))
		}
	}
}

func TestHistory_ColumnsFitTerminal(t *testing.T) {
	m := NewTransactionHistoryModel(NewMockWalletManager())
	m.width = 80
	w := m.effectiveColumnWidths()
	if w[4] != 0 {
		t.Fatal("TxID column should be hidden at 80 columns")
	}
	total := 4 + 2*len(w)
	for _, c := range w {
		total += c
	}
	if total > 80 {
		t.Fatalf("table is %d cells wide at 80 columns", total)
	}
	m.width = 120
	if m.effectiveColumnWidths() != m.columnWidths {
		t.Fatal("wide terminals should keep the configured widths")
	}
}
