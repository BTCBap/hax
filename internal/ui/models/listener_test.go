package models

import (
	"testing"
	"time"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

func TestWaitForNotification_StopsWhenBackendChanges(t *testing.T) {
	b, err := backend.NewBackend(backend.DefaultElectrumConfig("testnet"))
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan tea.Msg, 1)
	go func() { done <- waitForNotification(b, 1, stop) }()
	close(stop)
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("stopped listener returned %#v, want nil", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit after stop was closed")
	}
}

func TestRoot_BackendChangeRestartsListener(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsUnlockedFn = func() bool { return true }
	r := NewRootModel(mock, nil)
	r.mainDashboardModel = NewMainDashboardModel(mock, nil)
	oldStop, oldGen := r.listenStop, r.listenGen

	model, cmd := r.Update(BackendChangedMsg{Then: StateMainDashboard})
	r = model.(RootModel)
	if cmd == nil {
		t.Fatal("expected listener restart and navigation commands")
	}
	select {
	case <-oldStop:
	default:
		t.Fatal("old listener was not stopped")
	}
	if r.listenGen != oldGen+1 || !r.listenerStarted {
		t.Fatalf("gen=%d started=%v, want gen %d and a running listener", r.listenGen, r.listenerStarted, oldGen+1)
	}

	// A notification from the previous backend is dropped and not re-armed.
	if _, cmd := r.Update(NewBlockNotification{Height: 5, Gen: oldGen}); cmd != nil {
		t.Fatal("stale block notification should be ignored")
	}
	if _, cmd := r.Update(NewBlockNotification{Height: 5, Gen: r.listenGen}); cmd == nil {
		t.Fatal("current block notification should alert and re-arm")
	}
}

func TestRoot_ChooseServerUsesWalletNetwork(t *testing.T) {
	r := NewRootModel(NewMockWalletManager(), nil)
	r.state = StateUnlock
	model, _ := r.Update(ChooseServerMsg{Network: wallet.NetworkMainnet})
	r = model.(RootModel)
	cs, ok := r.connectionSetupModel.(ConnectionSetupModel)
	if !ok {
		t.Fatalf("connection setup not opened: %T", r.connectionSetupModel)
	}
	if cs.network != "mainnet" || !cs.returnToUnlock {
		t.Fatalf("network=%q returnToUnlock=%v, want mainnet and true", cs.network, cs.returnToUnlock)
	}
	if r.setupNetwork != "" {
		t.Fatal("preselected network should be consumed")
	}
}

func TestUnlock_ChooseServerCarriesWalletNetwork(t *testing.T) {
	m := NewUnlockModel(nil)
	m.passwordInput.Focus()
	m.err = &wallet.NetworkMismatchError{WalletNetwork: wallet.NetworkMainnet, Detail: "x"}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	msg, ok := cmd().(ChooseServerMsg)
	if !ok || msg.Network != wallet.NetworkMainnet {
		t.Fatalf("got %#v, want ChooseServerMsg for mainnet", msg)
	}
}

func TestRoot_BlockBalanceCheckDoesNotDoubleListen(t *testing.T) {
	r := NewRootModel(NewMockWalletManager(), nil)
	if _, cmd := r.Update(BalanceChangeMsg{Gen: r.listenGen, FromBlock: true}); cmd != nil {
		t.Fatal("a block-triggered balance check must not re-arm the listener")
	}
	if _, cmd := r.Update(BalanceChangeMsg{Gen: r.listenGen}); cmd == nil {
		t.Fatal("a notification-triggered balance check should re-arm the listener")
	}
}
