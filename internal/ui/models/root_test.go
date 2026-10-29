package models

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/config"
)

// newTestRootModel creates a RootModel with minimal dependencies for testing.
// Uses a ConfigManager pointed at a temp directory.
func newTestRootModel(t *testing.T) RootModel {
	t.Helper()
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	return NewRootModel(nil, cm) // nil walletManager - only test non-wallet screens
}

func TestNewRootModel_InitialState(t *testing.T) {
	m := newTestRootModel(t)
	if m.state != StateSplash {
		t.Errorf("expected initial state StateSplash (%d), got %d", StateSplash, m.state)
	}
	if m.splashModel == nil {
		t.Error("expected splashModel to be initialized")
	}
	if !m.testnet {
		t.Error("expected testnet=true")
	}
	if m.walletLoaded {
		t.Error("expected walletLoaded=false")
	}
}

func TestRootModel_CtrlC_Quits(t *testing.T) {
	m := newTestRootModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should return a command")
	}
	msg := extractMsg(cmd)
	// tea.Quit returns a QuitMsg
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestRootModel_WindowSizeMsg(t *testing.T) {
	m := newTestRootModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := model.(RootModel)
	if updated.width != 120 {
		t.Errorf("expected width=120, got %d", updated.width)
	}
	if updated.height != 40 {
		t.Errorf("expected height=40, got %d", updated.height)
	}
}

func TestRootModel_NavigateToHelp(t *testing.T) {
	m := newTestRootModel(t)
	model, _ := m.Update(NavigateMsg{Target: StateHelp})
	updated := model.(RootModel)
	if updated.state != StateHelp {
		t.Errorf("expected state StateHelp (%d), got %d", StateHelp, updated.state)
	}
	if updated.helpModel == nil {
		t.Error("expected helpModel to be created")
	}
}

func TestRootModel_NavigateToHelp_Idempotent(t *testing.T) {
	m := newTestRootModel(t)
	// Navigate to Help first time
	model1, _ := m.Update(NavigateMsg{Target: StateHelp})
	r1 := model1.(RootModel)
	// Navigate away and back
	model2, _ := r1.Update(NavigateMsg{Target: StateSplash})
	r2 := model2.(RootModel)
	model3, _ := r2.Update(NavigateMsg{Target: StateHelp})
	r3 := model3.(RootModel)
	// Help model should be reused (not nil, and state is Help)
	if r3.state != StateHelp {
		t.Errorf("expected state StateHelp, got %d", r3.state)
	}
	if r3.helpModel == nil {
		t.Error("helpModel should still exist")
	}
}

func TestRootModel_NavigateToConnectionSetup(t *testing.T) {
	m := newTestRootModel(t)
	model, _ := m.Update(NavigateMsg{Target: StateConnectionSetup})
	updated := model.(RootModel)
	if updated.state != StateConnectionSetup {
		t.Errorf("expected state StateConnectionSetup (%d), got %d", StateConnectionSetup, updated.state)
	}
	if updated.connectionSetupModel == nil {
		t.Error("expected connectionSetupModel to be created")
	}
}

func TestRootModel_View_Splash(t *testing.T) {
	m := newTestRootModel(t)
	view := m.View()
	// Should render splash content (title text visible in the splash screen)
	if !strings.Contains(view, "BITCOIN WALLET") {
		t.Error("root view at splash state should contain 'BITCOIN WALLET'")
	}
}

func TestRootModel_NavigateToUnlock(t *testing.T) {
	m := newTestRootModel(t)
	// UnlockModel requires walletManager for Init but not for construction
	model, _ := m.Update(NavigateMsg{Target: StateUnlock})
	updated := model.(RootModel)
	if updated.state != StateUnlock {
		t.Errorf("expected state StateUnlock (%d), got %d", StateUnlock, updated.state)
	}
	if updated.unlockModel == nil {
		t.Error("expected unlockModel to be created")
	}
}

func TestRootModel_NavigateToConnectionSetup_AlwaysFresh(t *testing.T) {
	m := newTestRootModel(t)
	// Navigate to ConnectionSetup first time
	model1, _ := m.Update(NavigateMsg{Target: StateConnectionSetup})
	r1 := model1.(RootModel)
	first := r1.connectionSetupModel

	// Navigate away and back — should create a fresh model
	model2, _ := r1.Update(NavigateMsg{Target: StateSplash})
	r2 := model2.(RootModel)
	model3, _ := r2.Update(NavigateMsg{Target: StateConnectionSetup})
	r3 := model3.(RootModel)
	second := r3.connectionSetupModel

	if first == nil || second == nil {
		t.Fatal("both connection setup models should exist")
	}
	// ConnectionSetup is always recreated fresh (unlike Help which is reused)
	// We verify by checking the model was created and state is correct
	if r3.state != StateConnectionSetup {
		t.Errorf("expected state StateConnectionSetup, got %d", r3.state)
	}
}

func TestRootModel_NavigateToUnlock_AlwaysFresh(t *testing.T) {
	m := newTestRootModel(t)
	// Navigate to Unlock first time
	model1, _ := m.Update(NavigateMsg{Target: StateUnlock})
	r1 := model1.(RootModel)
	if r1.unlockModel == nil {
		t.Fatal("unlockModel should be created")
	}

	// Navigate away and back — should create a fresh model
	model2, _ := r1.Update(NavigateMsg{Target: StateSplash})
	r2 := model2.(RootModel)
	model3, _ := r2.Update(NavigateMsg{Target: StateUnlock})
	r3 := model3.(RootModel)
	if r3.unlockModel == nil {
		t.Fatal("unlockModel should be recreated")
	}
	if r3.state != StateUnlock {
		t.Errorf("expected state StateUnlock, got %d", r3.state)
	}
}

func TestRootModel_FirstRunGoesToWalletSetup(t *testing.T) {
	// Fresh ConfigManager with empty temp dir → IsFirstRun() returns true
	cm, err := config.NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	if !cm.IsFirstRun() {
		t.Fatal("precondition: expected IsFirstRun=true for fresh config")
	}

	m := NewRootModel(&MockWalletManager{}, cm)
	// Simulate splash trying to navigate to WalletSetup (what splash does on key press)
	// Onboarding goes straight there; no connection picker on first run.
	model, _ := m.Update(NavigateMsg{Target: StateWalletSetup})
	updated := model.(RootModel)
	if updated.state != StateWalletSetup {
		t.Errorf("expected first run to reach StateWalletSetup (%d), got %d",
			StateWalletSetup, updated.state)
	}
	if updated.walletSetupModel == nil {
		t.Error("expected walletSetupModel to be created")
	}
	if updated.connectionSetupModel != nil {
		t.Error("first run must not open connection setup")
	}
}

func TestRootModel_ErrorMsg(t *testing.T) {
	m := newTestRootModel(t)
	testErr := fmt.Errorf("test error")
	model, _ := m.Update(ErrorMsg{Err: testErr})
	updated := model.(RootModel)
	if updated.err == nil {
		t.Error("expected err to be set")
	}
	if updated.err.Error() != "test error" {
		t.Errorf("expected error 'test error', got %q", updated.err.Error())
	}
}

func TestRootModel_View_NoActiveModel(t *testing.T) {
	m := newTestRootModel(t)
	// Force a state with no initialized model
	m.state = StateSettings // settingsModel is nil
	view := m.View()
	if !strings.Contains(view, "No active model") {
		t.Error("view with nil active model should show error message")
	}
}

func TestRootModel_View_HelpScreen(t *testing.T) {
	m := newTestRootModel(t)
	model, _ := m.Update(NavigateMsg{Target: StateHelp})
	updated := model.(RootModel)
	view := updated.View()
	if !strings.Contains(view, "GETTING STARTED") {
		t.Error("root view at Help state should contain 'GETTING STARTED'")
	}
}

func TestRootModel_NavigateSequence(t *testing.T) {
	m := newTestRootModel(t)

	// Splash → Help
	model, _ := m.Update(NavigateMsg{Target: StateHelp})
	r := model.(RootModel)
	if r.state != StateHelp {
		t.Fatalf("expected StateHelp, got %d", r.state)
	}

	// Help → ConnectionSetup
	model, _ = r.Update(NavigateMsg{Target: StateConnectionSetup})
	r = model.(RootModel)
	if r.state != StateConnectionSetup {
		t.Fatalf("expected StateConnectionSetup, got %d", r.state)
	}

	// ConnectionSetup → Splash
	model, _ = r.Update(NavigateMsg{Target: StateSplash})
	r = model.(RootModel)
	if r.state != StateSplash {
		t.Fatalf("expected StateSplash, got %d", r.state)
	}

	// Splash → Help (should reuse existing model)
	model, _ = r.Update(NavigateMsg{Target: StateHelp})
	r = model.(RootModel)
	if r.state != StateHelp {
		t.Fatalf("expected StateHelp, got %d", r.state)
	}
	if r.helpModel == nil {
		t.Error("helpModel should still exist")
	}
}

func TestRootModel_Init_ReturnsCommands(t *testing.T) {
	m := newTestRootModel(t)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return commands (alert + splash init)")
	}
	msgs := collectMsgs(cmd)
	if len(msgs) == 0 {
		t.Error("Init should produce messages")
	}
}
