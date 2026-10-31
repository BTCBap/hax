package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const testMnemonic = "abandon ability able about above absent absorb abstract absurd abuse access accident"

func typeKeys(m WalletSetupModel, s string) WalletSetupModel {
	for _, r := range s {
		model, _ := m.Update(keyRune(r))
		m = model.(WalletSetupModel)
	}
	return m
}

func newBackupModel(t *testing.T) WalletSetupModel {
	t.Helper()
	m := NewWalletSetupModel(NewMockWalletManager(), nil)
	m.width, m.height = 100, 40
	m.generatedMnemonic = testMnemonic
	m.step = StepShowMnemonic
	model, _ := m.Update(keyCode(tea.KeyEnter))
	m = model.(WalletSetupModel)
	if m.step != StepConfirmMnemonic {
		t.Fatalf("step = %v, want backup check", m.step)
	}
	return m
}

func TestBackupCheck_CorrectWordsFinish(t *testing.T) {
	m := newBackupModel(t)
	words := strings.Fields(testMnemonic)
	if len(m.backupWords) != backupCheckWords {
		t.Fatalf("asked for %d words", len(m.backupWords))
	}
	var cmd tea.Cmd
	for i, pos := range m.backupWords {
		m = typeKeys(m, " "+strings.ToUpper(words[pos])+" ")
		var model screenModel
		model, cmd = m.Update(keyCode(tea.KeyEnter))
		m = model.(WalletSetupModel)
		if m.err != nil {
			t.Fatalf("word %d rejected: %v", i, m.err)
		}
	}
	if cmd == nil {
		t.Fatal("expected navigation after the last word")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.Target != StateMainDashboard {
		t.Fatalf("got %#v, want dashboard", nav)
	}
}

func TestBackupCheck_WrongWordStays(t *testing.T) {
	m := newBackupModel(t)
	m = typeKeys(m, "zoo")
	model, _ := m.Update(keyCode(tea.KeyEnter))
	m = model.(WalletSetupModel)
	if m.err == nil || m.backupPos != 0 || m.step != StepConfirmMnemonic {
		t.Fatal("a wrong word must be rejected without advancing")
	}
}

func TestBackupCheck_SkipNeedsConfirmation(t *testing.T) {
	m := newBackupModel(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = model.(WalletSetupModel)
	if !m.backupSkipConfirm || !strings.Contains(m.View(), "Skip the backup check?") {
		t.Fatal("ctrl+s should ask for confirmation")
	}
	model, _ = m.Update(keyCode(tea.KeyEsc))
	m = model.(WalletSetupModel)
	if m.backupSkipConfirm || m.step != StepConfirmMnemonic {
		t.Fatal("esc should cancel the skip prompt and stay on the check")
	}
	model, _ = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = model.(WalletSetupModel)
	_, cmd := m.Update(keyRune('y'))
	if cmd == nil {
		t.Fatal("confirmed skip should open the wallet")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.Target != StateMainDashboard {
		t.Fatalf("got %#v, want dashboard", nav)
	}
}

func TestPickWordPositions(t *testing.T) {
	for i := 0; i < 200; i++ {
		p := pickWordPositions(12, 3)
		if len(p) != 3 || p[0] >= p[1] || p[1] >= p[2] || p[0] < 0 || p[2] > 11 {
			t.Fatalf("bad positions %v", p)
		}
	}
}

func TestPasswordStrength(t *testing.T) {
	for _, c := range []struct {
		pw   string
		want int
	}{
		{"short", 0},
		{"aaaaaaaaaaaaaaaaaaaaaa", 1},
		{"password", 1},
		{"correcthorse", 2},
		{"Correct horse 9", 3},
		{"Correct horse battery 9!", 4},
	} {
		if got, _ := passwordStrength(c.pw); got != c.want {
			t.Errorf("passwordStrength(%q) = %d, want %d", c.pw, got, c.want)
		}
	}
	if renderPasswordStrength("") != "" {
		t.Error("empty password should render no meter")
	}
}
