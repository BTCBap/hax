package models

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// keyRune builds a key press for a single printable rune.
func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// keyRunes builds a key press carrying a string of printable characters.
// A single rune maps to its own code; longer strings use KeyExtended, which
// matches how Bubble Tea v2 delivers pasted or multi-rune input.
func keyRunes(s string) tea.KeyPressMsg {
	runes := []rune(s)
	if len(runes) == 0 {
		return tea.KeyPressMsg{}
	}
	if len(runes) > 1 {
		return tea.KeyPressMsg{Code: tea.KeyExtended, Text: s}
	}
	return keyRune(runes[0])
}

// keyCode builds a key press for a special key code (enter, esc, arrows, ...).
func keyCode(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// extractMsg runs a tea.Cmd and returns the resulting message.
// Returns nil if cmd is nil.
func extractMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// collectMsgs recursively flattens a tea.Cmd (including tea.Batch) into individual messages.
// tea.Batch returns a BatchMsg ([]Cmd) which we recursively unwrap.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	// tea.BatchMsg is []Cmd - recursively collect from each
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, collectMsgs(tea.Cmd(c))...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// containsMsgOfType checks if any message in the slice matches type T.
func containsMsgOfType[T any](msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if _, ok := msg.(T); ok {
			return true
		}
	}
	return false
}

// findMsgOfType finds the first message of type T in the slice.
func findMsgOfType[T any](msgs []tea.Msg) (T, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(T); ok {
			return m, true
		}
	}
	var zero T
	return zero, false
}

// assertNavigatesTo asserts that a tea.Cmd produces a NavigateMsg targeting the given state.
func assertNavigatesTo(t *testing.T, cmd tea.Cmd, target AppState) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msgs := collectMsgs(cmd)
	nav, found := findMsgOfType[NavigateMsg](msgs)
	if !found {
		t.Fatalf("expected NavigateMsg in commands, got messages: %v", msgs)
	}
	if nav.Target != target {
		t.Errorf("expected navigation to %d, got %d", target, nav.Target)
	}
}
