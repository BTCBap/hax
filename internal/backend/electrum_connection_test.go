package backend

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestConfig returns a minimal BackendConfig for unit tests (no real server).
func newTestConfig() *BackendConfig {
	return &BackendConfig{
		Type:    BackendTypeElectrum,
		Network: "testnet",
		Electrum: &ElectrumConfig{
			Host: "127.0.0.1",
			Port: 50001,
		},
	}
}

// newPipedBackend creates a backend wired to a net.Pipe connection.
// serverConn is the server-side endpoint the test controls.
// The backend is already marked as connected; readLoop is NOT started —
// call go b.readLoop() in tests that need it.
func newPipedBackend(t *testing.T) (*ElectrumBackend, net.Conn) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	b := &ElectrumBackend{
		config:           newTestConfig(),
		conn:             clientConn,
		reader:           bufio.NewReader(clientConn),
		connected:        true,
		scripthashChan:   make(chan ScripthashNotification, 100),
		headerChan:       make(chan HeaderNotification, 10),
		pendingResponses: make(map[uint64]chan electrumResponse),
		subscriptions:    make(map[string]bool),
		readerDone:       make(chan struct{}),
	}
	return b, serverConn
}

// writeJSONLine encodes v as JSON and writes it with a trailing newline to w.
func writeJSONLine(t *testing.T, w net.Conn, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("writeJSONLine: marshal error: %v", err)
	}
	w.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := w.Write(append(data, '\n')); err != nil {
		t.Fatalf("writeJSONLine: write error: %v", err)
	}
}

// readJSONLine reads one newline-terminated line from conn and unmarshals it.
func readJSONLine(t *testing.T, conn net.Conn, dst interface{}) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("readJSONLine: %v", err)
	}
	if err := json.Unmarshal(line, dst); err != nil {
		t.Fatalf("readJSONLine: unmarshal error: %v", err)
	}
}

// refusedAddr returns a tcp address guaranteed to refuse connections quickly.
func refusedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("refusedAddr: listen: %v", err)
	}
	addr := l.Addr().String()
	l.Close() // Close immediately so the port refuses connections
	return addr
}

// waitFor polls cond every tick until it returns true or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

// ─── Constructor Tests ────────────────────────────────────────────────────────

func TestNewElectrumBackend_WrongType(t *testing.T) {
	cfg := &BackendConfig{
		Type:     BackendTypeRPC,
		Electrum: &ElectrumConfig{Host: "localhost", Port: 50001},
	}
	_, err := NewElectrumBackend(cfg)
	if err == nil {
		t.Fatal("expected error for wrong backend type, got nil")
	}
}

func TestNewElectrumBackend_NilElectrumConfig(t *testing.T) {
	cfg := &BackendConfig{
		Type:     BackendTypeElectrum,
		Electrum: nil,
	}
	_, err := NewElectrumBackend(cfg)
	if err == nil {
		t.Fatal("expected error for nil Electrum config, got nil")
	}
}

func TestNewElectrumBackend_Valid(t *testing.T) {
	b, err := NewElectrumBackend(newTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	// Channels must be initialised
	if b.scripthashChan == nil {
		t.Error("scripthashChan is nil")
	}
	if b.headerChan == nil {
		t.Error("headerChan is nil")
	}
	if b.pendingResponses == nil {
		t.Error("pendingResponses map is nil")
	}
}

// ─── Connection State Tests ───────────────────────────────────────────────────

func TestIsConnected_InitiallyFalse(t *testing.T) {
	b, err := NewElectrumBackend(newTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.IsConnected() {
		t.Fatal("new backend should not be connected")
	}
}

func TestIsConnected_ThreadSafe(t *testing.T) {
	b := newTestBackend()
	// Toggle connected state under lock and read concurrently; must not race.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.IsConnected()
		}()
	}
	wg.Wait()
}

func TestDisconnect_WhenNotConnected(t *testing.T) {
	b, err := NewElectrumBackend(newTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Disconnect when already disconnected must be a no-op
	if err := b.Disconnect(); err != nil {
		t.Fatalf("expected nil error for no-op disconnect, got: %v", err)
	}
}

func TestDisconnect_Idempotent(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	defer serverConn.Close()

	if err := b.Disconnect(); err != nil {
		t.Fatalf("first disconnect error: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("second disconnect error: %v", err)
	}
	if b.IsConnected() {
		t.Fatal("should not be connected after double disconnect")
	}
}

func TestDisconnect_MarksDisconnected(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	defer serverConn.Close()

	if !b.IsConnected() {
		t.Fatal("expected connected before disconnect")
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect error: %v", err)
	}
	if b.IsConnected() {
		t.Fatal("expected disconnected after Disconnect()")
	}
}

func TestDisconnect_SignalsReaderDone(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	defer serverConn.Close()

	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect error: %v", err)
	}
	// readerDone channel must be closed after Disconnect()
	select {
	case <-b.readerDone:
		// expected: channel is closed
	default:
		t.Fatal("expected readerDone to be closed after Disconnect()")
	}
}

// ─── addressToScriptHash Tests ────────────────────────────────────────────────

func TestAddressToScriptHash_ValidTestnetNativeSegwit(t *testing.T) {
	b := &ElectrumBackend{config: newTestConfig()}
	// tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx is the standard testnet
	// native SegWit address for the all-zeros 20-byte hash.
	hash, err := b.addressToScriptHash("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hash) != 64 {
		t.Fatalf("expected 64-char scripthash, got %d: %q", len(hash), hash)
	}
}

func TestAddressToScriptHash_Deterministic(t *testing.T) {
	b := &ElectrumBackend{config: newTestConfig()}
	addr := "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	h1, err1 := b.addressToScriptHash(addr)
	h2, err2 := b.addressToScriptHash(addr)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic output, got %q and %q", h1, h2)
	}
}

func TestAddressToScriptHash_DifferentAddressesYieldDifferentHashes(t *testing.T) {
	b := &ElectrumBackend{config: newTestConfig()}
	h1, err := b.addressToScriptHash("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx")
	if err != nil {
		t.Fatalf("addr1 error: %v", err)
	}
	// A second distinct testnet address (standard testnet P2PKH faucet address)
	h2, err := b.addressToScriptHash("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn")
	if err != nil {
		t.Fatalf("addr2 error: %v", err)
	}
	if h1 == h2 {
		t.Fatal("different addresses must produce different scripthashes")
	}
}

func TestAddressToScriptHash_InvalidAddress(t *testing.T) {
	b := &ElectrumBackend{config: newTestConfig()}
	_, err := b.addressToScriptHash("not-a-valid-bitcoin-address")
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

// ─── readLoop Tests ───────────────────────────────────────────────────────────

func TestReadLoop_ExitsWhenConnectionClosed(t *testing.T) {
	b, serverConn := newPipedBackend(t)

	go b.readLoop()

	// Close server side → client ReadBytes returns EOF → loop exits
	serverConn.Close()

	waitFor(t, 2*time.Second, func() bool { return !b.IsConnected() })
}

func TestReadLoop_ExitsOnReaderDoneSignal(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	defer serverConn.Close()

	go b.readLoop()

	// Signal shutdown
	close(b.readerDone)

	// The loop should exit within a read deadline (60s is too long to wait;
	// also close the server conn so the blocking read unblocks immediately).
	serverConn.Close()

	waitFor(t, 2*time.Second, func() bool { return !b.IsConnected() })
}

func TestReadLoop_SkipsMalformedJSON(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	go b.readLoop()

	// Send malformed JSON line — readLoop should skip it and continue.
	serverConn.SetWriteDeadline(time.Now().Add(time.Second))
	serverConn.Write([]byte("{invalid json\n"))

	// Now send a valid header notification — proves the loop continued.
	writeJSONLine(t, serverConn, map[string]interface{}{
		"method": "blockchain.headers.subscribe",
		"params": []interface{}{map[string]interface{}{"height": 99999, "hex": "aabbcc"}},
	})

	select {
	case n := <-b.headerChan:
		if n.Height != 99999 {
			t.Fatalf("expected height 99999, got %d", n.Height)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected header notification after malformed JSON was skipped")
	}

	serverConn.Close()
}

func TestReadLoop_RoutesResponseByID(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	go b.readLoop()

	// Register a pending response channel for ID 42
	respChan := make(chan electrumResponse, 1)
	b.pendingMu.Lock()
	b.pendingResponses[42] = respChan
	b.pendingMu.Unlock()

	// Server sends a response for ID 42
	writeJSONLine(t, serverConn, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      42,
		"result":  "hello",
	})

	select {
	case resp := <-respChan:
		if resp.Error != nil {
			t.Fatalf("unexpected error in response: %+v", resp.Error)
		}
		var s string
		if err := json.Unmarshal(resp.Result, &s); err != nil || s != "hello" {
			t.Fatalf("expected result \"hello\", got %s", resp.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response not delivered to pending channel")
	}

	serverConn.Close()
}

func TestReadLoop_DropsResponseForUnknownID(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	go b.readLoop()

	// Server sends a response for ID 999 — no pending channel registered.
	// Should not panic or block.
	writeJSONLine(t, serverConn, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      999,
		"result":  "ignored",
	})

	// Give readLoop a moment to process then verify no crash.
	time.Sleep(50 * time.Millisecond)

	serverConn.Close()
	waitFor(t, 2*time.Second, func() bool { return !b.IsConnected() })
}

func TestReadLoop_RoutesNotificationToHandler(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	go b.readLoop()

	validHash := strings.Repeat("ab", 32) // 64 hex chars
	writeJSONLine(t, serverConn, map[string]interface{}{
		"jsonrpc": "2.0",
		// No "id" field → notification
		"method": "blockchain.scripthash.subscribe",
		"params": []interface{}{validHash, "status123"},
	})

	select {
	case n := <-b.scripthashChan:
		if n.Scripthash != validHash {
			t.Fatalf("expected scripthash %s, got %s", validHash, n.Scripthash)
		}
		if n.Status != "status123" {
			t.Fatalf("expected status123, got %s", n.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected scripthash notification from readLoop")
	}

	serverConn.Close()
}

// ─── request() Error Path Tests ───────────────────────────────────────────────

func TestRequest_SuccessfulRoundtrip(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	go b.readLoop()

	// Server goroutine: read request, send response
	go func() {
		var req struct {
			ID     uint64        `json:"id"`
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		readJSONLine(t, serverConn, &req)
		writeJSONLine(t, serverConn, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  []string{"ElectrumX 1.16.0", "1.4"},
		})
	}()

	result, err := b.request("server.version", []interface{}{"test", "1.4"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var versions []string
	if err := json.Unmarshal(result, &versions); err != nil || len(versions) < 2 {
		t.Fatalf("unexpected result: %s", result)
	}

	serverConn.Close()
}

func TestRequest_ServerReturnsError(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	go b.readLoop()

	go func() {
		var req struct {
			ID uint64 `json:"id"`
		}
		readJSONLine(t, serverConn, &req)
		writeJSONLine(t, serverConn, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]interface{}{
				"code":    2,
				"message": "unknown method",
			},
		})
	}()

	_, err := b.request("bad.method", []interface{}{})
	if err == nil {
		t.Fatal("expected error for server error response, got nil")
	}
	if !strings.Contains(err.Error(), "electrum error") {
		t.Fatalf("expected 'electrum error' in message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown method") {
		t.Fatalf("expected server message in error, got: %v", err)
	}

	serverConn.Close()
}

func TestRequest_WriteFailureDisconnects(t *testing.T) {
	b, serverConn := newPipedBackend(t)

	// Close the server side so the client write will fail.
	serverConn.Close()
	// Give OS a moment to propagate the close.
	time.Sleep(10 * time.Millisecond)

	_, err := b.request("server.ping", []interface{}{})
	if err == nil {
		t.Fatal("expected error after write to closed connection, got nil")
	}

	// Backend must mark itself as disconnected after a write failure.
	if b.IsConnected() {
		t.Fatal("expected connected=false after write failure")
	}
}

func TestRequest_NotConnectedReconnectFails(t *testing.T) {
	// Point config at a port that refuses connections
	host, portStr, err := net.SplitHostPort(refusedAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	var port int
	fmt.Sscan(portStr, &port)

	b := &ElectrumBackend{
		config: &BackendConfig{
			Type:    BackendTypeElectrum,
			Network: "testnet",
			Electrum: &ElectrumConfig{
				Host:   host,
				Port:   port,
				UseSSL: false,
			},
		},
		connected:        false,
		scripthashChan:   make(chan ScripthashNotification, 100),
		headerChan:       make(chan HeaderNotification, 10),
		pendingResponses: make(map[uint64]chan electrumResponse),
		subscriptions:    make(map[string]bool),
	}

	_, err = b.request("server.ping", []interface{}{})
	if err == nil {
		t.Fatal("expected error when not connected and reconnect fails")
	}
	if !strings.Contains(err.Error(), "not connected and reconnect failed") {
		t.Fatalf("expected 'not connected and reconnect failed' in error, got: %v", err)
	}
}

// ─── Notification Channel Full (Drop) Tests ───────────────────────────────────

func TestHandleNotification_FullScripthashChannel_DropsGracefully(t *testing.T) {
	b := newTestBackend()

	// Fill the channel to capacity
	validHash := strings.Repeat("cd", 32) // 64 hex chars
	for i := 0; i < cap(b.scripthashChan); i++ {
		b.scripthashChan <- ScripthashNotification{Scripthash: validHash, Status: "s"}
	}

	// This notification must be dropped without panic
	params, _ := json.Marshal([]interface{}{validHash, "overflow"})
	b.handleNotification("blockchain.scripthash.subscribe", params)

	// Channel should still be full (the dropped notification did not block)
	if len(b.scripthashChan) != cap(b.scripthashChan) {
		t.Fatalf("expected channel at capacity (%d), got %d", cap(b.scripthashChan), len(b.scripthashChan))
	}
}

func TestHandleNotification_FullHeaderChannel_DropsGracefully(t *testing.T) {
	b := newTestBackend()

	// Fill the header channel to capacity
	for i := 0; i < cap(b.headerChan); i++ {
		b.headerChan <- HeaderNotification{Height: int64(i), Hex: "aa"}
	}

	// This header notification must be dropped without panic
	params, _ := json.Marshal([]map[string]interface{}{
		{"height": 99999, "hex": "overflow"},
	})
	b.handleNotification("blockchain.headers.subscribe", params)

	if len(b.headerChan) != cap(b.headerChan) {
		t.Fatalf("expected header channel at capacity (%d), got %d", cap(b.headerChan), len(b.headerChan))
	}
}

// ─── ServerInfo / CertFingerprint Tests ──────────────────────────────────────

func TestServerInfo_DefaultsEmpty(t *testing.T) {
	b, err := NewElectrumBackend(newTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	software, protocol := b.ServerInfo()
	if software != "" || protocol != "" {
		t.Fatalf("expected empty ServerInfo before handshake, got %q %q", software, protocol)
	}
	if fp := b.CertFingerprint(); fp != "" {
		t.Fatalf("expected empty CertFingerprint before connection, got %q", fp)
	}
}

func TestServerInfo_ReflectsSetValues(t *testing.T) {
	b := newTestBackend()

	b.mu.Lock()
	b.serverSoftware = "ElectrumX 1.16.0"
	b.protocolVersion = "1.4"
	b.certFingerprint = "AA:BB:CC"
	b.mu.Unlock()

	software, protocol := b.ServerInfo()
	if software != "ElectrumX 1.16.0" {
		t.Fatalf("expected ElectrumX 1.16.0, got %q", software)
	}
	if protocol != "1.4" {
		t.Fatalf("expected 1.4, got %q", protocol)
	}
	if fp := b.CertFingerprint(); fp != "AA:BB:CC" {
		t.Fatalf("expected AA:BB:CC, got %q", fp)
	}
}

// ─── Connect() Idempotency Test ───────────────────────────────────────────────

func TestConnect_AlreadyConnectedIsNoop(t *testing.T) {
	b, serverConn := newPipedBackend(t)
	defer serverConn.Close()

	// Backend is already connected — calling Connect() must return nil immediately.
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect() on already-connected backend returned error: %v", err)
	}
	if !b.IsConnected() {
		t.Fatal("expected still connected after no-op Connect()")
	}
}
