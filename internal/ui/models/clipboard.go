package models

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// clipboardClearTimeout is how long a copied address or transaction is left
// on the system clipboard. Overridden in tests.
var clipboardClearTimeout = 60 * time.Second

var (
	clipboardWrite = clipboard.WriteAll
	clipboardRead  = clipboard.ReadAll
	// clipboardSet is the OSC 52 writer. Tests replace it to inspect payloads.
	clipboardSet = tea.SetClipboard
)

var (
	clipboardMu  sync.Mutex
	clipboardVal string
)

// copyToClipboard writes text to the system clipboard and returns an OSC 52
// command so the copy also works over SSH and in terminals without a supported
// system clipboard. The clear timer only runs when the local clipboard write
// succeeded. The returned error is the local clipboard error; a non-nil
// command means the copy can still reach the terminal.
func copyToClipboard(text string) (tea.Cmd, error) {
	cmd := clipboardSet(text)
	err := clipboardWrite(text)
	if err != nil {
		return cmd, err
	}

	clipboardMu.Lock()
	clipboardVal = text
	clipboardMu.Unlock()

	timeout := clipboardClearTimeout
	go func() {
		time.Sleep(timeout)
		clearClipboard()
	}()
	return cmd, nil
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
