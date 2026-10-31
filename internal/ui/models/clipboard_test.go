package models

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClipboard struct {
	mu    sync.Mutex
	value string
}

func (f *fakeClipboard) get() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value
}

func (f *fakeClipboard) set(s string) {
	f.mu.Lock()
	f.value = s
	f.mu.Unlock()
}

func withFakeClipboard(t *testing.T) (*fakeClipboard, func()) {
	t.Helper()
	fc := &fakeClipboard{}
	origWrite, origRead := clipboardWrite, clipboardRead
	origTimeout := clipboardClearTimeout
	clipboardWrite = func(s string) error { fc.set(s); return nil }
	clipboardRead = func() (string, error) { return fc.get(), nil }
	clipboardClearTimeout = 10 * time.Millisecond
	return fc, func() {
		clipboardWrite, clipboardRead = origWrite, origRead
		clipboardClearTimeout = origTimeout
		clipboardMu.Lock()
		clipboardVal = ""
		clipboardMu.Unlock()
	}
}

func TestCopyToClipboard_AutoClearsAfterTimeout(t *testing.T) {
	fc, cleanup := withFakeClipboard(t)
	defer cleanup()

	cmd, err := copyToClipboard("bc1qexample")
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if cmd == nil || cmd() == nil {
		t.Fatal("expected an OSC 52 clipboard command")
	}
	if got := fc.get(); got != "bc1qexample" {
		t.Fatalf("clipboard = %q, want copied value", got)
	}

	deadline := time.Now().Add(time.Second)
	for fc.get() != "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := fc.get(); got != "" {
		t.Error("expected clipboard to be cleared after timeout")
	}
}

func TestCopyToClipboard_OSC52WhenLocalClipboardFails(t *testing.T) {
	fc, cleanup := withFakeClipboard(t)
	defer cleanup()

	calls := 0
	clipboardWrite = func(s string) error {
		calls++
		return errors.New("no system clipboard")
	}

	cmd, err := copyToClipboard("bc1qexample")
	if err == nil {
		t.Fatal("expected the local clipboard error")
	}
	if calls != 1 {
		t.Fatalf("clipboardWrite called %d times, want 1", calls)
	}
	if cmd == nil {
		t.Fatal("expected an OSC 52 clipboard command even when the local clipboard fails")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("OSC 52 command produced no message")
	}
	if fc.get() != "" {
		t.Fatal("clipboard should not have been written")
	}
}

func TestClearClipboard_LeavesLaterUserCopy(t *testing.T) {
	fc, cleanup := withFakeClipboard(t)
	defer cleanup()

	fc.set("hax-value")
	clipboardMu.Lock()
	clipboardVal = "hax-value"
	clipboardMu.Unlock()

	// The user copies something else before the timer fires.
	fc.set("user-value")
	clearClipboard()

	if got := fc.get(); got != "user-value" {
		t.Errorf("clipboard = %q, want the user's later copy preserved", got)
	}
}
