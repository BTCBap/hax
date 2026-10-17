package models

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/version"
)

func TestNewSplashModel(t *testing.T) {
	m := NewSplashModel()
	if m.progress != 0.0 {
		t.Errorf("expected progress 0, got %f", m.progress)
	}
	if m.target != 1.0 {
		t.Errorf("expected target 1.0, got %f", m.target)
	}
	if m.ready {
		t.Error("expected ready to be false")
	}
	if m.width != 0 || m.height != 0 {
		t.Error("expected zero dimensions initially")
	}
}

func TestSplashInit_ReturnsCommands(t *testing.T) {
	m := NewSplashModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return commands")
	}
	// Init returns tea.Batch of tick + delayed navigate
	msgs := collectMsgs(cmd)
	if len(msgs) == 0 {
		t.Fatal("Init should produce messages")
	}
}

func TestSplashKeyPress_NavigatesToWalletSetup(t *testing.T) {
	m := NewSplashModel()
	// Press 'enter' key
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assertNavigatesTo(t, cmd, StateWalletSetup)
}

func TestSplashKeyPress_AnyKey_NavigatesToWalletSetup(t *testing.T) {
	m := NewSplashModel()
	// Press 'a' key
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assertNavigatesTo(t, cmd, StateWalletSetup)
}

func TestSplashCtrlC_DoesNotNavigate(t *testing.T) {
	m := NewSplashModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		msg := extractMsg(cmd)
		if _, ok := msg.(NavigateMsg); ok {
			t.Error("ctrl+c should not produce NavigateMsg")
		}
	}
}

func TestSplashTickMsg_AdvancesProgress(t *testing.T) {
	m := NewSplashModel()
	// Send several tick messages to advance animation
	var model tea.Model = m
	for i := 0; i < 10; i++ {
		model, _ = model.Update(TickMsg(time.Now()))
	}
	updated := model.(SplashModel)
	if updated.progress <= 0.0 {
		t.Error("expected progress to advance after ticks")
	}
	if updated.frame != 10 {
		t.Errorf("expected frame=10, got %d", updated.frame)
	}
}

func TestSplashWindowSizeMsg(t *testing.T) {
	m := NewSplashModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := model.(SplashModel)
	if updated.width != 120 {
		t.Errorf("expected width=120, got %d", updated.width)
	}
	if updated.height != 40 {
		t.Errorf("expected height=40, got %d", updated.height)
	}
}

func TestSplashView_ContainsExpectedText(t *testing.T) {
	m := NewSplashModel()
	view := m.View()
	// The logo is rendered as Unicode box-drawing characters (not literal "HAX").
	// Check for text that must be present in the splash view.
	if !strings.Contains(view, "BITCOIN WALLET") {
		t.Error("splash view should contain 'BITCOIN WALLET'")
	}
	if strings.Contains(view, "Testnet") {
		t.Error("splash view shouldn't call hax a testnet wallet")
	}
	if !strings.Contains(view, version.String()) {
		t.Error("splash view should show the version")
	}
}

func TestSplashView_DefaultDimensions(t *testing.T) {
	m := NewSplashModel()
	// Should not panic with zero dimensions
	view := m.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}
