package models

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUnlockBackoff_GrowsAndCaps(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 0},
		{1, 0},
		{2, time.Second},
		{3, 2 * time.Second},
		{4, 4 * time.Second},
		{5, 8 * time.Second},
		{6, 8 * time.Second},
		{100, 8 * time.Second},
	}
	for _, tc := range cases {
		if got := unlockBackoff(tc.failures); got != tc.want {
			t.Errorf("unlockBackoff(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestUnlock_ThrottleBlocksAttemptDuringWindow(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"mywallet"}
	m.walletName = "mywallet"
	m.passwordInput.Focus()
	m.passwordInput.SetValue("guess")
	m.throttleUntil = time.Now().Add(time.Minute)

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(UnlockModel)

	if updated.unlocking {
		t.Error("expected unlock to be postponed during throttle window")
	}
	if cmd != nil {
		// A nil command proves no unlock work was dispatched.
		msg := extractMsg(cmd)
		if _, ok := msg.(UnlockResultMsg); ok {
			t.Error("expected no unlock attempt during throttle window")
		}
	}
	if updated.err == nil || !strings.Contains(updated.err.Error(), "wait") {
		t.Errorf("expected throttle notice in existing error row, got %v", updated.err)
	}
}

func TestUnlock_ThrottleExpires(t *testing.T) {
	m := NewUnlockModel(nil)
	m.walletList = []string{"mywallet"}
	m.walletName = "mywallet"
	m.passwordInput.Focus()
	m.passwordInput.SetValue("guess")
	m.throttleUntil = time.Now().Add(-time.Second)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(UnlockModel)

	if !updated.unlocking {
		t.Error("expected unlock attempt once throttle has expired")
	}
}

func TestUnlock_SuccessClearsThrottle(t *testing.T) {
	m := NewUnlockModel(nil)
	m.failedAttempts = 4
	m.throttleUntil = time.Now().Add(time.Minute)

	model, _ := m.Update(UnlockResultMsg{Success: true})
	updated := model.(UnlockModel)

	if updated.failedAttempts != 0 {
		t.Errorf("expected failedAttempts reset, got %d", updated.failedAttempts)
	}
	if !updated.throttleUntil.IsZero() {
		t.Errorf("expected throttle cleared, got %v", updated.throttleUntil)
	}
}

func TestUnlock_FailureStartsThrottle(t *testing.T) {
	m := NewUnlockModel(nil)
	m.unlocking = true
	m.passwordInput.SetValue("wrong")

	model, _ := m.Update(UnlockResultMsg{Success: false, Err: errFake("bad password")})
	updated := model.(UnlockModel)

	if updated.failedAttempts != 1 {
		t.Errorf("expected failedAttempts=1, got %d", updated.failedAttempts)
	}
	if !updated.throttleUntil.IsZero() {
		t.Error("expected no delay after the first failure")
	}

	updated.unlocking = true
	model, _ = updated.Update(UnlockResultMsg{Success: false, Err: errFake("bad password")})
	updated = model.(UnlockModel)

	if updated.failedAttempts != 2 {
		t.Errorf("expected failedAttempts=2, got %d", updated.failedAttempts)
	}
	if updated.throttleUntil.IsZero() {
		t.Error("expected throttle to be scheduled after consecutive failures")
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
