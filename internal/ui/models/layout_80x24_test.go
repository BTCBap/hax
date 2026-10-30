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
		"splash":    func(*MockWalletManager) tea.Model { return NewSplashModel() },
		"unlock":    func(w *MockWalletManager) tea.Model { return NewUnlockModel(w) },
		"dashboard": func(w *MockWalletManager) tea.Model { return NewMainDashboardModel(w, nil) },
		"receive":   func(w *MockWalletManager) tea.Model { return NewReceiveModel(w) },
		"send":      func(w *MockWalletManager) tea.Model { return NewSendModel(w, 0, nil) },
		"feebump":   func(w *MockWalletManager) tea.Model { return NewFeeBumpModel(w, "ab12", 0, nil) },
		"history":   func(w *MockWalletManager) tea.Model { return NewTransactionHistoryModel(w) },
		"history_speedup": func(w *MockWalletManager) tea.Model {
			return newSpeedUpHistoryLayout(w, false)
		},
		"history_speedup_confirm": func(w *MockWalletManager) tea.Model {
			return newSpeedUpHistoryLayout(w, true)
		},
		"addresses": func(w *MockWalletManager) tea.Model { return NewAddressesModel(w, nil) },
		"receive_hwi": func(w *MockWalletManager) tea.Model {
			w.IsHardwareWalletFn = func() bool { return true }
			w.IsMultisigFn = func() bool { return false }
			r := NewReceiveModel(w)
			r.selectedAddress = verifyTestAddress
			r.derivationPath = verifyTestPath
			r.revealProgress = 1.0
			r.verify.Reset(verifyTestAddress, verifyTestPath)
			r.verify.State = AddressVerifyMismatch
			r.verify.DeviceAddress = verifyTestDevice
			return r
		},
		"addresses_hwi": func(w *MockWalletManager) tea.Model {
			w.IsHardwareWalletFn = func() bool { return true }
			w.IsMultisigFn = func() bool { return false }
			a := NewAddressesModel(w, nil)
			a.receiveAddresses = []AddrDisplayInfo{{
				Address: verifyTestAddress,
				Path:    verifyTestPath,
				Index:   0,
			}}
			a.updateTable()
			a.verify.Reset(verifyTestAddress, verifyTestPath)
			a.verify.State = AddressVerifyMismatch
			a.verify.DeviceAddress = verifyTestDevice
			return a
		},
		"utxos": func(w *MockWalletManager) tea.Model { return NewUTXOsModel(w, nil) },
		"utxos_stealth": func(*MockWalletManager) tea.Model {
			m := stealthUTXOs()
			m.stealth = true
			m.sortAndUpdateTable()
			return m
		},
		"addresses_stealth": func(*MockWalletManager) tea.Model {
			m := stealthAddresses()
			m.stealth = true
			m.updateTable()
			return m
		},
		"receive_stealth": func(*MockWalletManager) tea.Model {
			m := stealthReceive()
			m.stealth = true
			return m
		},
		"history_stealth": func(*MockWalletManager) tea.Model {
			m := stealthHistory()
			m.stealth = true
			m.updateTableData()
			return m
		},
		"dashboard_stealth": func(*MockWalletManager) tea.Model {
			m := stealthDashboard()
			m.stealth = true
			return m
		},
		"settings": func(w *MockWalletManager) tea.Model { return NewSettingsModel(w, nil) },
		"settings_labels_import": func(w *MockWalletManager) tea.Model {
			m := NewSettingsModel(w, nil)
			m.openLabelImport()
			m.labelImportPath.SetValue("")
			return m
		},
		"settings_labels_import_preview": func(w *MockWalletManager) tea.Model {
			m := NewSettingsModel(w, nil)
			m.showLabelImport = true
			m.labelImportStage = labelImportStagePreview
			m.labelImportPreview = bip329Preview{Tx: 2, Addr: 1, Output: 2, Frozen: 1}
			m.labelImportPath.SetValue("~/.hax/test-wallet_labels_bip329.jsonl")
			return m
		},
		"settings_labels_import_result": func(w *MockWalletManager) tea.Model {
			m := NewSettingsModel(w, nil)
			m.showLabelImport = true
			m.labelImportStage = labelImportStageResult
			m.labelImportResult = "✓ Imported 42 labels"
			m.labelImportPath.SetValue("~/.hax/test-wallet_labels_bip329.jsonl")
			return m
		},
		"settings_backup_export": func(w *MockWalletManager) tea.Model {
			m := NewSettingsModel(w, nil)
			m.openBackupExport()
			return m
		},
		"settings_backup_export_confirm": func(w *MockWalletManager) tea.Model {
			m := NewSettingsModel(w, nil)
			m.openBackupExport()
			m.backupTray, _ = m.backupTray.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m.backupExportStage = backupExportStageConfirm
			return m
		},
		"settings_backup_export_result": func(w *MockWalletManager) tea.Model {
			m := NewSettingsModel(w, nil)
			m.openBackupExport()
			m.backupExportStage = backupExportStageResult
			m.backupExportResult = "✓ Backup saved"
			m.backupExportPath = "~/.hax/test-wallet_backup.json"
			return m
		},
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

// TestTxFlowEntryViewsFit renders the visualizer as opened from the send
// review step, the PSBT inspector, and history at both supported sizes.
func TestTxFlowEntryViewsFit(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, width, height int) tea.Model
	}{
		{"send-review", func(t *testing.T, width, height int) tea.Model {
			return txFlowTestUpdate(t, newSendReviewFlowModel(t, width, height), txFlowTestKey("v"))
		}},
		{"psbt-inspector", func(t *testing.T, width, height int) tea.Model {
			return txFlowTestUpdate(t, newPSBTInspectorFlowModel(t, width, height), txFlowTestKey("v"))
		}},
		{"history", func(t *testing.T, width, height int) tea.Model {
			return txFlowTestUpdate(t, newHistoryFlowModel(t, width, height), txFlowTestKey("v"))
		}},
	}
	sizes := []struct {
		name string
		w, h int
	}{
		{"80x24", 80, 24},
		{"120x40", 120, 40},
	}
	for _, test := range tests {
		for _, size := range sizes {
			t.Run(test.name+"/"+size.name, func(t *testing.T) {
				m := test.build(t, size.w, size.h)
				view := m.View()
				if !strings.Contains(ansi.Strip(view), "Inputs") {
					t.Fatalf("visualizer did not open:\n%s", ansi.Strip(view))
				}
				assertFits(t, view, size.w)
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
