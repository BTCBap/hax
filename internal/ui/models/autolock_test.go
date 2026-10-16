package models

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/wallet"
)

func idleRoot(state AppState) (RootModel, *bool) {
	locked := false
	mock := NewMockWalletManager()
	mock.LockWalletFn = func() { locked = true }
	mock.IsUnlockedFn = func() bool { return !locked }
	mock.GetWalletNameFn = func() string { return "savings" }
	r := NewRootModel(mock, nil)
	r.state = state
	r.autoLockOverride = time.Minute
	r.lastActivity = time.Now().Add(-2 * time.Minute)
	r.mainDashboardModel = NewMainDashboardModel(mock, nil)
	return r, &locked
}

func TestAutoLock_LocksIdleWalletAndShowsUnlock(t *testing.T) {
	r, locked := idleRoot(StateMainDashboard)
	out, _ := r.Update(idleCheckMsg(time.Now()))
	r = out.(RootModel)
	if !*locked || r.state != StateUnlock {
		t.Fatalf("locked=%v state=%v", *locked, r.state)
	}
	if r.mainDashboardModel != nil {
		t.Fatal("dashboard model should be dropped on lock")
	}
	um := r.unlockModel.(UnlockModel)
	if um.preferredWallet != "savings" || !strings.Contains(um.notice, "1 minute") {
		t.Fatalf("preferred=%q notice=%q", um.preferredWallet, um.notice)
	}
	// The preferred wallet is preselected with the password focused.
	upd, _ := um.Update(WalletListMsg{Wallets: []string{"a", "savings", "c"}})
	um = upd.(UnlockModel)
	if um.walletName != "savings" || !um.passwordInput.Focused() {
		t.Fatalf("walletName=%q focused=%v", um.walletName, um.passwordInput.Focused())
	}
}

func TestAutoLock_DoesNotFire(t *testing.T) {
	cases := map[string]func(*RootModel){
		"recent activity": func(r *RootModel) { r.lastActivity = time.Now() },
		"disabled":        func(r *RootModel) { r.autoLockOverride = -1 },
		"setup screen":    func(r *RootModel) { r.state = StateWalletSetup },
		"broadcasting": func(r *RootModel) {
			r.state = StateSend
			sm := NewSendModel(r.walletManager, 0, nil)
			sm.step = SendStepBroadcasting
			r.sendModel = sm
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r, locked := idleRoot(StateMainDashboard)
			mutate(&r)
			out, _ := r.Update(idleCheckMsg(time.Now()))
			if *locked || out.(RootModel).state == StateUnlock {
				t.Fatal("auto-lock fired")
			}
		})
	}
}

func TestAutoLock_InputResetsTimer(t *testing.T) {
	r, locked := idleRoot(StateMainDashboard)
	out, _ := r.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = out.(RootModel).Update(idleCheckMsg(time.Now()))
	if *locked {
		t.Fatal("key press should reset the idle timer")
	}
}

func pinErrState() wallet.WalletStateSnapshot {
	pe := &backend.PinMismatchError{Server: "electrum.example:50002", Pinned: strings.Repeat("a", 64), Presented: strings.Repeat("b", 64)}
	return wallet.WalletStateSnapshot{Status: wallet.SyncStatusError, LastError: fmt.Errorf("connection failed: %w", pe)}
}

func TestRetrust_PromptCancelAndAccept(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetStateFn = pinErrState
	var trusted *backend.PinMismatchError
	orig := trustPresentedKey
	trustPresentedKey = func(pe *backend.PinMismatchError) error { trusted = pe; return nil }
	defer func() { trustPresentedKey = orig }()

	m := NewMainDashboardModel(mock, nil)
	m.width, m.height = 120, 40
	if v := m.View(); !strings.Contains(v, "Server key changed") || !strings.Contains(v, "re-trust") {
		t.Fatalf("banner missing:\n%s", v)
	}
	shiftT := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")}
	out, _ := m.Update(shiftT)
	m = out.(MainDashboardModel)
	if m.retrustPrompt == nil || !strings.Contains(m.View(), "Trust the new key") {
		t.Fatal("prompt not shown")
	}
	out, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = out.(MainDashboardModel)
	if m.retrustPrompt != nil || trusted != nil {
		t.Fatal("non-y key must cancel without trusting")
	}
	out, _ = m.Update(shiftT)
	out, cmd := out.(MainDashboardModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = out.(MainDashboardModel)
	if trusted == nil || trusted.Server != "electrum.example:50002" || cmd == nil {
		t.Fatalf("trust not applied: %+v", trusted)
	}

	// A failure is surfaced, not swallowed.
	trustPresentedKey = func(*backend.PinMismatchError) error { return errors.New("disk full") }
	out, _ = m.Update(shiftT)
	out, _ = out.(MainDashboardModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if !strings.Contains(out.(MainDashboardModel).View(), "disk full") {
		t.Fatal("trust error not shown")
	}
}
