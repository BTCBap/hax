package models

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/wallet"
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
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+s produced no command")
	}
	if _, ok := cmd().(ChooseServerMsg); !ok {
		t.Fatal("ctrl+s should ask to choose a server")
	}

	m.err = fmt.Errorf("wrong password")
	if strings.Contains(m.View(), "choose server") {
		t.Fatal("server choice shown for an unrelated error")
	}
}
