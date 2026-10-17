package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// tipServer answers getblockcount with whatever height currently holds.
func tipServer(t *testing.T, height *atomic.Int64, fail *atomic.Bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if fail != nil && fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		res, _ := json.Marshal(height.Load())
		_ = json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: res})
	}))
}

func pollingBackend(server *httptest.Server) *RPCBackend {
	return &RPCBackend{
		config:       &BackendConfig{Type: BackendTypeRPC, Network: "testnet4", RPC: &RPCConfig{}},
		baseURL:      server.URL,
		client:       server.Client(),
		connected:    true,
		headerChan:   make(chan HeaderNotification, 10),
		pollInterval: 10 * time.Millisecond,
	}
}

func TestRPCBackend_PollsForNewBlocks(t *testing.T) {
	var height atomic.Int64
	height.Store(100)
	var fail atomic.Bool
	server := tipServer(t, &height, &fail)
	defer server.Close()

	b := pollingBackend(server)
	defer func() { _ = b.Disconnect() }()

	tip, err := b.SubscribeHeaders()
	if err != nil || tip != 100 {
		t.Fatalf("SubscribeHeaders = %d, %v; want 100, nil", tip, err)
	}
	// A second subscribe must not start a second poller.
	if _, err := b.SubscribeHeaders(); err != nil {
		t.Fatal(err)
	}

	// No new block: nothing should arrive.
	select {
	case n := <-b.HeaderNotifications():
		t.Fatalf("unexpected notification at unchanged tip: %+v", n)
	case <-time.After(50 * time.Millisecond):
	}

	// A failing node is skipped, not reported as a block.
	fail.Store(true)
	time.Sleep(30 * time.Millisecond)
	fail.Store(false)

	height.Store(101)
	select {
	case n := <-b.HeaderNotifications():
		if n.Height != 101 {
			t.Fatalf("got height %d, want 101", n.Height)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification for new block")
	}

	// Exactly one notification per new tip.
	select {
	case n := <-b.HeaderNotifications():
		t.Fatalf("duplicate notification: %+v", n)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRPCBackend_DisconnectStopsPolling(t *testing.T) {
	var height atomic.Int64
	height.Store(5)
	server := tipServer(t, &height, nil)
	defer server.Close()

	b := pollingBackend(server)
	if _, err := b.SubscribeHeaders(); err != nil {
		t.Fatal(err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.mu.RLock()
	stopped := b.pollStop == nil
	b.mu.RUnlock()
	if !stopped {
		t.Fatal("poller still registered after Disconnect")
	}
	time.Sleep(30 * time.Millisecond) // let the poller observe stop
	height.Store(6)
	select {
	case n := <-b.HeaderNotifications():
		t.Fatalf("notification after Disconnect: %+v", n)
	case <-time.After(60 * time.Millisecond):
	}
}

func TestRPCBackend_HeaderChannelNonNil(t *testing.T) {
	b, err := NewRPCBackend(&BackendConfig{Type: BackendTypeRPC, RPC: &RPCConfig{}})
	if err != nil {
		t.Fatal(err)
	}
	if b.HeaderNotifications() == nil {
		t.Fatal("RPC backend should expose a header channel for polled blocks")
	}
}
