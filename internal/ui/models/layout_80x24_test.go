package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/wallet"
)

// hwiDevicesForLayout mixes every device status, including an error long
// enough to need truncation, so the 80-column checks exercise the widest rows.
func hwiDevicesForLayout() []wallet.HardwareDevice {
	return []wallet.HardwareDevice{
		{Type: "trezor", Model: "Trezor One", Fingerprint: "abcd1234", NeedsPin: true},
		{Type: "ledger", Model: "Ledger Nano X", Fingerprint: "deadbeef1234", NeedsPass: true},
		{
			Type:        "coldcard",
			Model:       "Coldcard Mk4",
			Fingerprint: "cafebabe",
			Error: "Could not connect to device: the user did not confirm the request " +
				"on the device screen before the transport closed unexpectedly",
		},
		{Type: "bitbox", Model: "BitBox02", Fingerprint: "12345678"},
	}
}

// TestViews_FitIn80Columns renders each top-level view, with empty and with
// awkward data, in an 80×24 terminal and fails if any line is wider than 80.
func TestViews_FitIn80Columns(t *testing.T) {
	builders := map[string]func(*MockWalletManager) screenModel{
		"splash":    func(*MockWalletManager) screenModel { return NewSplashModel() },
		"unlock":    func(w *MockWalletManager) screenModel { return NewUnlockModel(w) },
		"dashboard": func(w *MockWalletManager) screenModel { return NewMainDashboardModel(w, nil) },
		"receive":   func(w *MockWalletManager) screenModel { return NewReceiveModel(w) },
		"send":      func(w *MockWalletManager) screenModel { return NewSendModel(w, 0, nil) },
		"send_hwi_devices": func(w *MockWalletManager) screenModel {
			m := NewSendModel(w, 0, nil)
			m.showPSBTExport = true
			m.showHWIDevices = true
			m.hwiDevices = hwiDevicesForLayout()
			return m
		},
		"feebump": func(w *MockWalletManager) screenModel { return NewFeeBumpModel(w, "ab12", 0, nil) },
		"history": func(w *MockWalletManager) screenModel { return NewTransactionHistoryModel(w) },
		"history_speedup": func(w *MockWalletManager) screenModel {
			return newSpeedUpHistoryLayout(w, false)
		},
		"history_speedup_confirm": func(w *MockWalletManager) screenModel {
			return newSpeedUpHistoryLayout(w, true)
		},
		"addresses": func(w *MockWalletManager) screenModel { return NewAddressesModel(w, nil) },
		"receive_hwi": func(w *MockWalletManager) screenModel {
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
		"addresses_hwi": func(w *MockWalletManager) screenModel {
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
		"utxos": func(w *MockWalletManager) screenModel { return NewUTXOsModel(w, nil) },
		"fees":  func(w *MockWalletManager) screenModel { return NewFeesModel(w, nil) },
		"utxos_stealth": func(*MockWalletManager) screenModel {
			m := stealthUTXOs()
			m.stealth = true
			m.sortAndUpdateTable()
			return m
		},
		"addresses_stealth": func(*MockWalletManager) screenModel {
			m := stealthAddresses()
			m.stealth = true
			m.updateTable()
			return m
		},
		"receive_stealth": func(*MockWalletManager) screenModel {
			m := stealthReceive()
			m.stealth = true
			return m
		},
		"history_stealth": func(*MockWalletManager) screenModel {
			m := stealthHistory()
			m.stealth = true
			m.updateTableData()
			return m
		},
		"dashboard_stealth": func(*MockWalletManager) screenModel {
			m := stealthDashboard()
			m.stealth = true
			return m
		},
		"settings": func(w *MockWalletManager) screenModel { return NewSettingsModel(w, nil) },
		"settings_labels_import": func(w *MockWalletManager) screenModel {
			m := NewSettingsModel(w, nil)
			m.openLabelImport()
			m.labelImportPath.SetValue("")
			return m
		},
		"settings_labels_import_preview": func(w *MockWalletManager) screenModel {
			m := NewSettingsModel(w, nil)
			m.showLabelImport = true
			m.labelImportStage = labelImportStagePreview
			m.labelImportPreview = bip329Preview{Tx: 2, Addr: 1, Output: 2, Frozen: 1}
			m.labelImportPath.SetValue("~/.hax/test-wallet_labels_bip329.jsonl")
			return m
		},
		"settings_labels_import_result": func(w *MockWalletManager) screenModel {
			m := NewSettingsModel(w, nil)
			m.showLabelImport = true
			m.labelImportStage = labelImportStageResult
			m.labelImportResult = "✓ Imported 42 labels"
			m.labelImportPath.SetValue("~/.hax/test-wallet_labels_bip329.jsonl")
			return m
		},
		"settings_backup_export": func(w *MockWalletManager) screenModel {
			m := NewSettingsModel(w, nil)
			m.openBackupExport()
			return m
		},
		"settings_backup_export_confirm": func(w *MockWalletManager) screenModel {
			m := NewSettingsModel(w, nil)
			m.openBackupExport()
			m.backupTray, _ = m.backupTray.Update(keyCode(tea.KeyEnter))
			m.backupExportStage = backupExportStageConfirm
			return m
		},
		"settings_backup_export_result": func(w *MockWalletManager) screenModel {
			m := NewSettingsModel(w, nil)
			m.openBackupExport()
			m.backupExportStage = backupExportStageResult
			m.backupExportResult = "✓ Backup saved"
			m.backupExportPath = "~/.hax/test-wallet_backup.json"
			return m
		},
		"help":  func(w *MockWalletManager) screenModel { return NewHelpModel(w) },
		"setup": func(w *MockWalletManager) screenModel { return NewWalletSetupModel(w, nil) },
		"setup_hwi_devices": func(w *MockWalletManager) screenModel {
			m := NewWalletSetupModel(w, nil)
			m.step = StepHardwareWalletConnect
			m.hwiDevices = hwiDevicesForLayout()
			return m
		},
		"setup_multisig_hwi_devices": func(w *MockWalletManager) screenModel {
			m := NewWalletSetupModel(w, nil)
			m.isMultisig = true
			m.step = StepMultisigEnterCosigners
			m.showHWIDevices = true
			m.hwiDevices = hwiDevicesForLayout()
			return m
		},
		"connection": func(*MockWalletManager) screenModel { return NewConnectionSetupModel(nil, false) },
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
		build func(t *testing.T, width, height int) screenModel
	}{
		{"send-review", func(t *testing.T, width, height int) screenModel {
			return txFlowTestUpdate(t, newSendReviewFlowModel(t, width, height), txFlowTestKey("v"))
		}},
		{"psbt-inspector", func(t *testing.T, width, height int) screenModel {
			return txFlowTestUpdate(t, newPSBTInspectorFlowModel(t, width, height), txFlowTestKey("v"))
		}},
		{"history", func(t *testing.T, width, height int) screenModel {
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
