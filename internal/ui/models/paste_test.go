package models

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSendPasteReachesFocusedInput verifies that the root forwards a bracketed
// paste to the send screen's focused text input instead of consuming it.
func TestSendPasteReachesFocusedInput(t *testing.T) {
	root := NewRootModel(NewMockWalletManager(), nil)
	model, _ := root.Update(NavigateMsg{Target: StateSend})
	root = model.(RootModel)

	const address = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	model, _ = root.Update(tea.PasteMsg{Content: address})
	root = model.(RootModel)

	send, ok := root.sendModel.(SendModel)
	if !ok {
		t.Fatalf("active send model has type %T", root.sendModel)
	}
	if got := send.addressInput.Value(); got != address {
		t.Fatalf("address input = %q, want %q", got, address)
	}
}
