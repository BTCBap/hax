package models

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

var utxoCount = regexp.MustCompile(`\d+ UTXOs?\b`)

func TestDashboard_NetworkTagReplacesUTXOCount(t *testing.T) {
	for _, c := range []struct {
		network string
		wantTag bool
	}{{wallet.NetworkTestnet, true}, {wallet.NetworkMainnet, false}} {
		m, mock := newTestDashboard(t)
		mock.GetNetworkFn = func() string { return c.network }
		m.loading = false
		view := m.View()
		if got := strings.Contains(view, "TESTNET4"); got != c.wantTag {
			t.Errorf("%s: TESTNET4 shown=%v, want %v", c.network, got, c.wantTag)
		}
		if utxoCount.MatchString(view) {
			t.Errorf("%s: dashboard still shows a UTXO count", c.network)
		}
		if strings.Contains(strings.ToLower(view), "mainnet") {
			t.Errorf("%s: dashboard shows a mainnet label", c.network)
		}
	}
}

func TestUnlock_NetworkMismatchOffersServerChoice(t *testing.T) {
	m := NewUnlockModel(nil)
	m.passwordInput.Focus()
	m.err = fmt.Errorf("%w: wallet is Mainnet", wallet.ErrNetworkMismatch)
	if !strings.Contains(m.View(), "choose server") {
		t.Fatal("mismatch error should offer choosing a server")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s produced no command")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok || nav.Target != StateConnectionSetup {
		t.Fatalf("ctrl+s = %#v, want navigate to connection setup", nav)
	}

	m.err = fmt.Errorf("wrong password")
	if strings.Contains(m.View(), "choose server") {
		t.Fatal("server choice shown for an unrelated error")
	}
}
