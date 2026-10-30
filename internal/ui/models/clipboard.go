package models

import (
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

// clipboardClearTimeout is how long a copied address or transaction is left
// on the system clipboard. Overridden in tests.
var clipboardClearTimeout = 60 * time.Second

var (
	clipboardWrite = clipboard.WriteAll
	clipboardRead  = clipboard.ReadAll
)

var (
	clipboardMu  sync.Mutex
	clipboardVal string
)

// copyToClipboard writes text and schedules a best-effort clear after
// clipboardClearTimeout. The clear only happens while the clipboard still
// contains the value hax wrote, so later user copies are left alone.
func copyToClipboard(text string) error {
	if err := clipboardWrite(text); err != nil {
		return err
	}
	clipboardMu.Lock()
	clipboardVal = text
	clipboardMu.Unlock()

	go func() {
		time.Sleep(clipboardClearTimeout)
		clearClipboard()
	}()
	return nil
}

// clearClipboard clears the clipboard if it still holds a value hax copied.
func clearClipboard() {
	clipboardMu.Lock()
	want := clipboardVal
	clipboardMu.Unlock()
	if want == "" {
		return
	}
	current, err := clipboardRead()
	if err != nil || current != want {
		return
	}
	if err := clipboardWrite(""); err != nil {
		return
	}
	clipboardMu.Lock()
	if clipboardVal == want {
		clipboardVal = ""
	}
	clipboardMu.Unlock()
}
