package backend

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeElectrum is a minimal TCP Electrum server. Each accepted connection
// answers server.version, server.ping, headers and scripthash subscribe.
type fakeElectrum struct {
	l       net.Listener
	tip     atomic.Int64
	status  atomic.Value // string
	pings   atomic.Int32
	mu      sync.Mutex
	conns   []net.Conn
	silence atomic.Bool // stop answering (but keep the socket open)
}

func newFakeElectrum(t *testing.T) *fakeElectrum {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeElectrum{l: l}
	f.tip.Store(100)
	f.status.Store("s1")
	go f.serve()
	t.Cleanup(func() { _ = l.Close(); f.dropAll() })
	return f
}

func (f *fakeElectrum) serve() {
	for {
		c, err := f.l.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns = append(f.conns, c)
		f.mu.Unlock()
		go f.handle(c)
	}
}

func (f *fakeElectrum) handle(c net.Conn) {
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     uint64        `json:"id"`
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		if json.Unmarshal(line, &req) != nil {
			continue
		}
		if f.silence.Load() {
			continue
		}
		var result interface{}
		switch req.Method {
		case "server.version":
			result = []string{"fake 1.0", "1.4"}
		case "server.ping":
			f.pings.Add(1)
			result = nil
		case "blockchain.headers.subscribe":
			result = map[string]interface{}{"height": f.tip.Load(), "hex": ""}
		case "blockchain.scripthash.subscribe":
			result = f.status.Load().(string)
		}
		out, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result})
		_, _ = c.Write(append(out, '\n'))
	}
}

func (f *fakeElectrum) dropAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		_ = c.Close()
	}
	f.conns = nil
}

func (f *fakeElectrum) backend(t *testing.T) *ElectrumBackend {
	t.Helper()
	host, port, _ := net.SplitHostPort(f.l.Addr().String())
	p, _ := strconv.Atoi(port)
	b, err := NewElectrumBackend(&BackendConfig{
		Type: BackendTypeElectrum, Network: "testnet4",
		Electrum: &ElectrumConfig{Host: host, Port: p},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	return b
}

func TestElectrum_DropIsReportedAndPendingFailsFast(t *testing.T) {
	f := newFakeElectrum(t)
	b := f.backend(t)
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}

	f.silence.Store(true) // the next request hangs until the drop
	errc := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := b.request("server.ping", []interface{}{})
		errc <- err
	}()
	time.Sleep(50 * time.Millisecond)
	f.dropAll()

	select {
	case err := <-errc:
		if !errors.Is(err, ErrConnectionLost) {
			t.Fatalf("pending request error = %v, want ErrConnectionLost", err)
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("pending request waited for its timeout instead of failing fast")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending request did not fail after the drop")
	}
	select {
	case err := <-b.Drops():
		if !errors.Is(err, ErrConnectionLost) {
			t.Fatalf("drop error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drop was not reported")
	}
	if b.IsConnected() {
		t.Fatal("backend still reports connected after the drop")
	}
}

func TestElectrum_DisconnectIsNotADrop(t *testing.T) {
	f := newFakeElectrum(t)
	b := f.backend(t)
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	_ = b.Disconnect()
	select {
	case err := <-b.Drops():
		t.Fatalf("Disconnect reported as a drop: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestElectrum_IdleConnectionIsPinged(t *testing.T) {
	f := newFakeElectrum(t)
	b := f.backend(t)
	b.idleTimeout = 30 * time.Millisecond
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return f.pings.Load() >= 2 })
	if !b.IsConnected() {
		t.Fatal("a quiet but healthy connection was dropped")
	}
}

func TestElectrum_ReconnectReplaysSubscriptions(t *testing.T) {
	f := newFakeElectrum(t)
	b := f.backend(t)
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	if h, err := b.SubscribeHeaders(); err != nil || h != 100 {
		t.Fatalf("SubscribeHeaders = %d, %v", h, err)
	}
	sh := "aa" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	if _, err := b.request("blockchain.scripthash.subscribe", []interface{}{sh}); err != nil {
		t.Fatal(err)
	}
	b.subMu.Lock()
	b.subscriptions[sh] = true
	b.subMu.Unlock()
	b.recordStatus(sh, "s1")

	// Server goes away; a block and a payment happen while we're offline.
	f.dropAll()
	<-b.Drops()
	f.tip.Store(102)
	f.status.Store("s2")

	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-b.HeaderNotifications():
		if n.Height != 102 {
			t.Fatalf("replayed tip %d, want 102", n.Height)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missed block not reported after reconnect")
	}
	select {
	case n := <-b.ScripthashNotifications():
		if n.Scripthash != sh || n.Status != "s2" {
			t.Fatalf("got %+v", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missed address change not reported after reconnect")
	}
}
