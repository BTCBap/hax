package models

import (
	"bytes"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/BTCBap/hax/internal/config"
)

// TestRootTeatest_SplashToWalletSetup runs a full tea.Program through the
// splash screen and verifies first run lands on wallet setup.
func TestRootTeatest_SplashToWalletSetup(t *testing.T) {
	// Fresh ConfigManager → IsFirstRun=true → splash navigates to WalletSetup
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	m := NewRootModel(&MockWalletManager{}, cm)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// The splash auto-navigates after 500ms via tea.Tick.
	// Wait for the wallet setup screen to appear.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("Wallet Setup"))
	}, teatest.WithDuration(3*time.Second))

	// Quit the program
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestRootTeatest_SplashKeySkip verifies that pressing a key during the splash
// screen immediately skips to wallet setup on first run.
func TestRootTeatest_SplashKeySkip(t *testing.T) {
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	m := NewRootModel(&MockWalletManager{}, cm)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Press a key to skip the splash immediately (don't wait for auto-navigate)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Should arrive at wallet setup
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("Wallet Setup"))
	}, teatest.WithDuration(2*time.Second))

	// Verify final model state
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	final := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
	root := final.(RootModel)
	if root.state != StateWalletSetup {
		t.Errorf("expected final state StateWalletSetup (%d), got %d",
			StateWalletSetup, root.state)
	}
}

// TestRootTeatest_NavigateToHelp runs the full program and navigates to the
// Help screen, verifying the content renders correctly.
func TestRootTeatest_NavigateToHelp(t *testing.T) {
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	m := NewRootModel(nil, cm)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 40))

	// Navigate directly to Help via a NavigateMsg
	tm.Send(NavigateMsg{Target: StateHelp})

	// Wait for help content to render
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("GETTING STARTED"))
	}, teatest.WithDuration(2*time.Second))

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	final := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
	root := final.(RootModel)
	if root.state != StateHelp {
		t.Errorf("expected final state StateHelp (%d), got %d", StateHelp, root.state)
	}
}
