package wallet

import (
	"testing"
	"time"
)

// droppingBackend can also report a connection drop, like Electrum.
type droppingBackend struct {
	flakyBackend
	drops chan error
}

func (b *droppingBackend) Drops() <-chan error { return b.drops }

func fastReconnect(t *testing.T) {
	t.Helper()
	base, max, interval := reconnectBaseDelay, reconnectMaxDelay, syncInterval
	reconnectBaseDelay, reconnectMaxDelay, syncInterval = 10*time.Millisecond, 40*time.Millisecond, time.Hour
	t.Cleanup(func() { reconnectBaseDelay, reconnectMaxDelay, syncInterval = base, max, interval })
}

func waitState(t *testing.T, m *WalletManager, what string, cond func(WalletStateSnapshot) bool) WalletStateSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.GetState(); cond(s) {
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; state %+v", what, m.GetState())
	return WalletStateSnapshot{}
}

func TestBackgroundSync_ReconnectsWithBackoff(t *testing.T) {
	fastReconnect(t)
	b := &flakyBackend{}
	m := unlockedWithBackend(t, b)
	good := m.GetState()

	b.down.Store(true)
	m.StartBackgroundSync()
	defer m.StopBackgroundSync()

	s := waitState(t, m, "a second reconnect attempt", func(s WalletStateSnapshot) bool {
		return s.Status == SyncStatusReconnecting && s.ReconnectAttempt >= 2
	})
	if s.Balance != good.Balance || s.LastError == nil || s.NextReconnectAt.IsZero() {
		t.Fatalf("reconnecting state lost data or details: %+v", s)
	}

	b.down.Store(false)
	s = waitState(t, m, "synced after recovery", func(s WalletStateSnapshot) bool { return s.Status == SyncStatusSynced })
	if s.ReconnectAttempt != 0 || s.Balance != good.Balance {
		t.Fatalf("after recovery: %+v", s)
	}
}

func TestBackgroundSync_DropTriggersImmediateReconnect(t *testing.T) {
	fastReconnect(t)
	b := &droppingBackend{drops: make(chan error, 1)}
	m := unlockedWithBackend(t, &b.flakyBackend)
	// Swap in the dropping wrapper (same data source).
	m.backend = b
	m.StartBackgroundSync()
	defer m.StopBackgroundSync()
	waitState(t, m, "initial sync", func(s WalletStateSnapshot) bool { return s.Status == SyncStatusSynced })

	b.down.Store(true)
	b.drops <- errDropped
	waitState(t, m, "reconnecting after drop", func(s WalletStateSnapshot) bool { return s.Status == SyncStatusReconnecting })
	b.down.Store(false)
	waitState(t, m, "synced after drop", func(s WalletStateSnapshot) bool { return s.Status == SyncStatusSynced })
}

func TestBackgroundSync_StopDuringReconnect(t *testing.T) {
	fastReconnect(t)
	reconnectBaseDelay = time.Hour // stuck waiting for the next attempt
	b := &flakyBackend{}
	m := unlockedWithBackend(t, b)
	b.down.Store(true)
	m.StartBackgroundSync()
	waitState(t, m, "reconnecting", func(s WalletStateSnapshot) bool { return s.Status == SyncStatusReconnecting })
	done := make(chan struct{})
	go func() { m.StopBackgroundSync(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StopBackgroundSync hung while waiting to reconnect")
	}
}

func TestRetryable(t *testing.T) {
	if retryable(nil) || retryable(errRefreshBusy) {
		t.Fatal("nil and busy are not retryable")
	}
	if !retryable(errDropped) {
		t.Fatal("a dropped connection is retryable")
	}
	if retryable(&NetworkMismatchError{}) {
		t.Fatal("a wrong-network server needs the user, not a retry")
	}
}
