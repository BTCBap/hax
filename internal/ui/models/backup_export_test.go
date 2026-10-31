package models

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const settingsTestBackup = `{"format":"haxwallet-backup","descriptor":"wpkh(tpub...)"}`

func TestSettings_BackupExportWritesFileOnlyAfterConfirm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	calls := 0
	mock := NewMockWalletManager()
	mock.GetWalletNameFn = func() string { return "test-wallet" }
	mock.ExportUnifiedBackupFn = func() ([]byte, error) {
		calls++
		return []byte(settingsTestBackup), nil
	}

	m := NewSettingsModel(mock, nil)
	m.width, m.height = 80, 24
	m = settingsUpdate(t, m, settingsKey("B"))
	if !m.showBackupExport {
		t.Fatal("B did not open the backup export screen")
	}
	if calls != 0 {
		t.Fatalf("backup exported %d times before confirming", calls)
	}

	choose := stripANSI(m.View())
	if !strings.Contains(choose, "Save to ~/.hax") || !strings.Contains(choose, "test-wallet_backup.json") {
		t.Fatalf("destination option or filename missing:\n%s", choose)
	}

	m = settingsUpdate(t, m, settingsKey("enter"))
	if m.backupExportStage != backupExportStageConfirm {
		t.Fatalf("stage = %v, want confirm", m.backupExportStage)
	}
	if calls != 0 {
		t.Fatalf("backup exported %d times on the confirm step", calls)
	}
	confirm := stripANSI(m.View())
	if !strings.Contains(confirm, "Writes public wallet data: descriptors, xpubs, labels.") {
		t.Fatalf("confirm view is missing the public-data statement:\n%s", confirm)
	}
	if strings.Contains(confirm, "Contains") {
		t.Fatalf("confirm view repeats the contents row:\n%s", confirm)
	}

	m = settingsUpdate(t, m, settingsKey("enter"))
	if calls != 1 {
		t.Fatalf("backup exported %d times, want 1", calls)
	}
	if m.backupExportStage != backupExportStageResult {
		t.Fatalf("stage = %v, want result", m.backupExportStage)
	}

	path := filepath.Join(home, ".hax", "test-wallet_backup.json")
	data, err := os.ReadFile(path) // #nosec G304 -- test reads a file it just wrote under t.TempDir()
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(data) != settingsTestBackup {
		t.Fatalf("backup file = %q, want %q", data, settingsTestBackup)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("backup permissions = %o, want 600", got)
		}
	}
	result := stripANSI(m.View())
	if !strings.Contains(result, "✓ Backup saved") || !strings.Contains(result, "~/.hax/test-wallet_backup.json") {
		t.Fatalf("result view is missing the saved path:\n%s", result)
	}
	if m.backupExportPath != path {
		t.Fatalf("saved path = %q, want %q", m.backupExportPath, path)
	}
}

func TestSettings_BackupExportEscDoesNotExport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	calls := 0
	mock := NewMockWalletManager()
	mock.ExportUnifiedBackupFn = func() ([]byte, error) {
		calls++
		return []byte(settingsTestBackup), nil
	}

	m := NewSettingsModel(mock, nil)
	m = settingsUpdate(t, m, settingsKey("B"))
	m = settingsUpdate(t, m, settingsKey("esc"))
	if m.showBackupExport {
		t.Fatal("esc from the choose stage should return to settings")
	}
	if calls != 0 {
		t.Fatalf("esc exported the backup %d times", calls)
	}
}

func TestSettings_BackupExportErrorShowsResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mock := NewMockWalletManager()
	mock.ExportUnifiedBackupFn = func() ([]byte, error) {
		return nil, os.ErrPermission
	}

	m := NewSettingsModel(mock, nil)
	m = settingsUpdate(t, m, settingsKey("B"))
	m = settingsUpdate(t, m, settingsKey("enter"))
	m = settingsUpdate(t, m, settingsKey("enter"))

	if m.backupExportStage != backupExportStageResult {
		t.Fatalf("stage = %v, want result", m.backupExportStage)
	}
	if !strings.Contains(stripANSI(m.View()), "Export failed") {
		t.Fatalf("error result missing:\n%s", stripANSI(m.View()))
	}
}

func TestSettings_BackupExportOptionsKeepLayout(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetWalletNameFn = func() string { return "test-wallet" }
	m := NewSettingsModel(mock, nil)
	m.width, m.height = 80, 24
	m.openBackupExport()

	choose := m.View()
	wantLines := requireLineCount(t, choose)
	firstHelper := requireTextPosition(t, choose, "test-wallet_backup.json")
	secondHelper := requireTextPosition(t, choose, "public data")
	if firstHelper.col != secondHelper.col {
		t.Fatalf("option helpers start at x=%d and x=%d", firstHelper.col, secondHelper.col)
	}

	for cursor := 0; cursor < 2; cursor++ {
		m.backupTray.cursor = cursor
		view := m.View()
		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("cursor %d has %d lines, want %d", cursor, got, wantLines)
		}
		if got := requireTextPosition(t, view, "test-wallet_backup.json"); got.col != firstHelper.col {
			t.Fatalf("cursor %d moved the helper column to x=%d", cursor, got.col)
		}
	}

	for cursor := 0; cursor < 2; cursor++ {
		confirm := NewSettingsModel(mock, nil)
		confirm.width, confirm.height = 80, 24
		confirm.openBackupExport()
		confirm.backupTray.cursor = cursor
		confirm.backupTray, _ = confirm.backupTray.Update(keyCode(tea.KeyEnter))
		view := confirm.View()
		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("confirm for cursor %d has %d lines, want %d", cursor, got, wantLines)
		}
		if cursor == 1 && !strings.Contains(stripANSI(view), "Destination  clipboard") {
			t.Fatalf("copy destination missing:\n%s", stripANSI(view))
		}
	}
}

func TestSettings_BackupExportStagesKeepLineCount(t *testing.T) {
	sizes := []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}}
	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			mock := NewMockWalletManager()
			mock.GetWalletNameFn = func() string { return "test-wallet" }
			m := NewSettingsModel(mock, nil)
			m.width, m.height = size.w, size.h

			m.openBackupExport()
			choose := m.View()
			want := requireLineCount(t, choose)
			assertFits(t, choose, size.w)

			m.backupExportStage = backupExportStageConfirm
			m.backupTray, _ = m.backupTray.Update(keyCode(tea.KeyEnter))
			confirm := m.View()
			if got := requireLineCount(t, confirm); got != want {
				t.Fatalf("confirm has %d lines, want %d", got, want)
			}
			assertFits(t, confirm, size.w)

			m.backupExportStage = backupExportStageResult
			m.backupExportResult = "✓ Backup saved"
			m.backupExportPath = filepath.Join("/home", "user", ".hax", "test-wallet_backup.json")
			success := m.View()
			if got := requireLineCount(t, success); got != want {
				t.Fatalf("result has %d lines, want %d", got, want)
			}
			assertFits(t, success, size.w)

			m.backupExportResult = ""
			m.backupExportPath = ""
			m.backupExportError = "Export failed: wallet is locked"
			errorView := m.View()
			if got := requireLineCount(t, errorView); got != want {
				t.Fatalf("error result has %d lines, want %d", got, want)
			}
			assertFits(t, errorView, size.w)
		})
	}
}
