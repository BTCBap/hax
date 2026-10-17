package models

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestReconnectStatus(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cases := []struct {
		attempt int
		in      time.Duration
		want    string
	}{
		{1, 2 * time.Second, "Reconnecting in 2s"},
		{3, 8 * time.Second, "Reconnecting in 8s (attempt 3)"},
		{2, 0, "Reconnecting... (attempt 2)"},
		{12, 60 * time.Second, "Reconnecting in 60s (attempt 12)"},
	}
	for _, c := range cases {
		got := reconnectStatus(now, wallet.WalletStateSnapshot{ReconnectAttempt: c.attempt, NextReconnectAt: now.Add(c.in)})
		if got != c.want {
			t.Errorf("attempt %d in %v: got %q, want %q", c.attempt, c.in, got, c.want)
		}
		if len(got) > 33 {
			t.Errorf("%q is wider than the status slot", got)
		}
	}
}

func TestDashboard_ShowsReconnectingAndKeepsBalance(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Status:           wallet.SyncStatusReconnecting,
			Balance:          123456,
			LastError:        errors.New("connection to server lost"),
			ReconnectAttempt: 2,
			NextReconnectAt:  time.Now().Add(4 * time.Second),
		}
	}
	m := tea.Model(NewMainDashboardModel(mock, nil))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Reconnecting in") || !strings.Contains(view, "(attempt 2)") {
		t.Fatalf("reconnecting status not shown:\n%s", view)
	}
	if !strings.Contains(view, "0.00123456") {
		t.Fatalf("last known balance hidden while reconnecting:\n%s", view)
	}
	assertFits(t, m.View(), 80)
}
