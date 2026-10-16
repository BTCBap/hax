package models

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSettings_MnemonicCannotBeCopied(t *testing.T) {
	m := NewSettingsModel(nil, nil)
	m.showMnemonic = true
	m.mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	sm := updated.(SettingsModel)
	if sm.showCopied {
		t.Fatal("pressing 'c' on the seed view must not copy the mnemonic")
	}
	if !sm.showMnemonic {
		t.Fatal("seed view should remain open")
	}
	if strings.Contains(sm.viewMnemonic(), "copy") {
		t.Error("seed view footer still advertises a copy action")
	}
}

func TestWritePrivateFile_Mode0600(t *testing.T) {
	p := filepath.Join(t.TempDir(), "export.csv")
	// Pre-existing world-readable file must be tightened too.
	if err := os.WriteFile(p, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(p, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no POSIX permission bits; os.Stat reports 0666 there.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
}
