package models

import (
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

	if err := copyToClipboard("bc1qexample"); err != nil {
		t.Fatalf("copy failed: %v", err)
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
