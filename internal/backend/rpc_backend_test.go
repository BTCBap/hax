package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"hax/internal/api"
)

// mockRPCServer creates a test HTTP server that responds to JSON-RPC requests.
// The handler maps method names to response results.
func mockRPCServer(t *testing.T, handlers map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check Basic Auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		handler, exists := handlers[req.Method]
		if !exists {
			resp := rpcResponse{
				ID: req.ID,
				Error: &rpcError{
					Code:    -32601,
					Message: "Method not found",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Check if handler is an error
		if rpcErr, ok := handler.(*rpcError); ok {
			resp := rpcResponse{
				ID:    req.ID,
				Error: rpcErr,
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		resultBytes, err := json.Marshal(handler)
		if err != nil {
			t.Fatalf("failed to marshal handler result: %v", err)
		}

		resp := rpcResponse{
			ID:     req.ID,
			Result: resultBytes,
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

// newTestRPCBackend creates an RPCBackend connected to a test server
func newTestRPCBackend(t *testing.T, server *httptest.Server) *RPCBackend {
	t.Helper()
	b := &RPCBackend{
		config: &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC: &RPCConfig{
				Username: "testuser",
				Password: "testpass",
			},
		},
		baseURL:   server.URL,
		client:    server.Client(),
		connected: true,
	}
	return b
}

func TestRPCBackend_NewRPCBackend(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC:     &RPCConfig{Host: "localhost", Port: 18332},
		}
		b, err := NewRPCBackend(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b == nil {
			t.Fatal("expected non-nil backend")
		}
	})

	t.Run("wrong backend type", func(t *testing.T) {
		cfg := &BackendConfig{Type: BackendTypeElectrum, RPC: &RPCConfig{}}
		_, err := NewRPCBackend(cfg)
		if err == nil {
			t.Fatal("expected error for wrong backend type")
		}
	})

	t.Run("nil RPC config", func(t *testing.T) {
		cfg := &BackendConfig{Type: BackendTypeRPC}
		_, err := NewRPCBackend(cfg)
		if err == nil {
			t.Fatal("expected error for nil RPC config")
		}
	})
}

func TestRPCBackend_Type(t *testing.T) {
	b := &RPCBackend{}
	if b.Type() != BackendTypeRPC {
		t.Fatalf("expected %s, got %s", BackendTypeRPC, b.Type())
	}
}

func TestRPCBackend_Connect(t *testing.T) {
	handlers := map[string]interface{}{
		"getblockchaininfo": map[string]interface{}{
			"chain":                "testnet4",
			"blocks":               50000,
			"initialblockdownload": false,
		},
		"getnetworkinfo": map[string]interface{}{
			"subversion": "/Satoshi:27.0.0/",
			"version":    270000,
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := &RPCBackend{
		config: &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC: &RPCConfig{
				Username: "testuser",
				Password: "testpass",
			},
		},
	}

	// Override to use test server URL
	b.baseURL = server.URL
	b.client = server.Client()
	b.connected = true

	// Test that we can successfully connect by calling getblockchaininfo
	err := b.TestConnection()
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestRPCBackend_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	b := &RPCBackend{
		config: &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC: &RPCConfig{
				Username: "wrong",
				Password: "wrong",
			},
		},
		baseURL:   server.URL,
		client:    server.Client(),
		connected: true,
	}

	_, err := b.callRPC("getblockchaininfo", nil)
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestRPCBackend_GetBlockHeight(t *testing.T) {
	handlers := map[string]interface{}{
		"getblockcount": int64(123456),
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	height, err := b.GetBlockHeight()
	if err != nil {
		t.Fatalf("GetBlockHeight failed: %v", err)
	}
	if height != 123456 {
		t.Fatalf("expected height 123456, got %d", height)
	}
}

func TestRPCBackend_GetFeeEstimates(t *testing.T) {
	handlers := map[string]interface{}{
		"estimatesmartfee": map[string]interface{}{
			"feerate": 0.00010000, // 10 sat/vB in BTC/kvB
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	fees, err := b.GetFeeEstimates()
	if err != nil {
		t.Fatalf("GetFeeEstimates failed: %v", err)
	}

	// 0.00010000 BTC/kvB = 10 sat/vB
	if fees.FastestFee != 10.0 {
		t.Fatalf("expected FastestFee 10, got %f", fees.FastestFee)
	}
}

func TestRPCBackend_GetFeeEstimates_NoEstimate(t *testing.T) {
	handlers := map[string]interface{}{
		"estimatesmartfee": map[string]interface{}{
			"errors": []string{"Insufficient data or no feerate found"},
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	fees, err := b.GetFeeEstimates()
	if err != nil {
		t.Fatalf("GetFeeEstimates failed: %v", err)
	}

	// Should fall back to 1 sat/vB
	if fees.FastestFee != 1.0 {
		t.Fatalf("expected FastestFee 1 (fallback), got %f", fees.FastestFee)
	}
}

func TestRPCBackend_BroadcastTransaction(t *testing.T) {
	expectedTxID := "abc123def456"
	handlers := map[string]interface{}{
		"sendrawtransaction": expectedTxID,
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	txid, err := b.BroadcastTransaction("0200000001...")
	if err != nil {
		t.Fatalf("BroadcastTransaction failed: %v", err)
	}
	if txid != expectedTxID {
		t.Fatalf("expected txid %s, got %s", expectedTxID, txid)
	}
}

func TestRPCBackend_GetTransaction(t *testing.T) {
	handlers := map[string]interface{}{
		"getrawtransaction": map[string]interface{}{
			"txid":     "tx123",
			"version":  2,
			"size":     225,
			"vsize":    144,
			"weight":   573,
			"locktime": 0,
			"fee":      0.00001000,
			"vin": []map[string]interface{}{
				{
					"txid": "prevtx1",
					"vout": 0,
					"scriptSig": map[string]string{
						"hex": "abc123",
					},
					"txinwitness": []string{"witness1", "witness2"},
					"sequence":    4294967293,
				},
			},
			"vout": []map[string]interface{}{
				{
					"value": 0.00100000,
					"n":     0,
					"scriptPubKey": map[string]interface{}{
						"hex":     "0014abc123",
						"address": "tb1qtest",
						"type":    "witness_v0_keyhash",
					},
				},
			},
			"blockhash":     "blockhash123",
			"confirmations": 10,
			"blocktime":     1700000000,
		},
		"getblockheader": map[string]interface{}{
			"height": int64(50000),
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	tx, err := b.GetTransaction("tx123")
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}

	if tx.TxID != "tx123" {
		t.Fatalf("expected txid tx123, got %s", tx.TxID)
	}
	if tx.Version != 2 {
		t.Fatalf("expected version 2, got %d", tx.Version)
	}
	if tx.Fee != 1000 {
		t.Fatalf("expected fee 1000 sat, got %d", tx.Fee)
	}
	if !tx.Status.Confirmed {
		t.Fatal("expected confirmed=true")
	}
	if tx.Status.BlockHeight != 50000 {
		t.Fatalf("expected block height 50000, got %d", tx.Status.BlockHeight)
	}
	if len(tx.Vin) != 1 {
		t.Fatalf("expected 1 input, got %d", len(tx.Vin))
	}
	if len(tx.Vout) != 1 {
		t.Fatalf("expected 1 output, got %d", len(tx.Vout))
	}
	if tx.Vout[0].Value != 100000 {
		t.Fatalf("expected output value 100000 sat, got %d", tx.Vout[0].Value)
	}
	if tx.Vout[0].ScriptpubkeyAddress != "tb1qtest" {
		t.Fatalf("expected address tb1qtest, got %s", tx.Vout[0].ScriptpubkeyAddress)
	}
}

func TestRPCBackend_GetAddressUTXOs(t *testing.T) {
	handlers := map[string]interface{}{
		"scantxoutset": map[string]interface{}{
			"success": true,
			"unspents": []map[string]interface{}{
				{
					"txid":     "utxo_tx1",
					"vout":     0,
					"amount":   0.00100000,
					"coinbase": false,
					"height":   50000,
				},
				{
					"txid":     "utxo_tx2",
					"vout":     1,
					"amount":   0.00050000,
					"coinbase": false,
					"height":   50001,
				},
			},
			"total_amount": 0.00150000,
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	utxos, err := b.GetAddressUTXOs("tb1qtest")
	if err != nil {
		t.Fatalf("GetAddressUTXOs failed: %v", err)
	}

	if len(utxos) != 2 {
		t.Fatalf("expected 2 UTXOs, got %d", len(utxos))
	}

	if utxos[0].TxID != "utxo_tx1" {
		t.Fatalf("expected txid utxo_tx1, got %s", utxos[0].TxID)
	}
	if utxos[0].Value != 100000 {
		t.Fatalf("expected value 100000 sat, got %d", utxos[0].Value)
	}
	if !utxos[0].Status.Confirmed {
		t.Fatal("expected confirmed=true for utxo with height > 0")
	}
	if utxos[1].Value != 50000 {
		t.Fatalf("expected value 50000 sat, got %d", utxos[1].Value)
	}
}

func TestRPCBackend_GetAddressBalance(t *testing.T) {
	handlers := map[string]interface{}{
		"scantxoutset": map[string]interface{}{
			"success":      true,
			"total_amount": 0.00150000,
			"unspents":     []interface{}{},
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	balance, err := b.GetAddressBalance("tb1qtest")
	if err != nil {
		t.Fatalf("GetAddressBalance failed: %v", err)
	}

	if balance != 150000 {
		t.Fatalf("expected balance 150000 sat, got %d", balance)
	}
}

func TestRPCBackend_GetAddressInfo_WithUTXOs(t *testing.T) {
	handlers := map[string]interface{}{
		"scantxoutset": map[string]interface{}{
			"success": true,
			"unspents": []map[string]interface{}{
				{"txid": "tx1", "amount": 0.001},
			},
			"total_amount": 0.001,
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	info, err := b.GetAddressInfo("tb1qtest")
	if err != nil {
		t.Fatalf("GetAddressInfo failed: %v", err)
	}

	if info.ChainStats.TxCount == 0 {
		t.Fatal("expected TxCount > 0 for address with UTXOs")
	}
}

func TestRPCBackend_GetAddressInfo_NoUTXOs(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp rpcResponse
		resp.ID = req.ID

		switch req.Method {
		case "scantxoutset":
			result, _ := json.Marshal(map[string]interface{}{
				"success":      true,
				"unspents":     []interface{}{},
				"total_amount": 0.0,
			})
			resp.Result = result
		case "scanblocks":
			callCount++
			// Simulate no block filter index
			resp.Error = &rpcError{Code: -1, Message: "Index is not enabled for filtertype basic"}
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestRPCBackend(t, server)
	b.baseURL = server.URL
	b.client = server.Client()

	info, err := b.GetAddressInfo("tb1qtest")
	if err != nil {
		t.Fatalf("GetAddressInfo failed: %v", err)
	}

	// Without block filter index, address with no UTXOs should report unused
	if info.ChainStats.TxCount != 0 {
		t.Fatalf("expected TxCount 0 for address with no UTXOs and no block filter index, got %d", info.ChainStats.TxCount)
	}
}

func TestRPCBackend_RPCError(t *testing.T) {
	handlers := map[string]interface{}{
		"getrawtransaction": &rpcError{
			Code:    -5,
			Message: "No such mempool or blockchain transaction",
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	_, err := b.GetTransaction("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent transaction")
	}
}

func TestBtcToSatoshi(t *testing.T) {
	tests := []struct {
		btc      float64
		expected api.Satoshi
	}{
		{0.0, 0},
		{1.0, 100000000},
		{0.00000001, 1},
		{0.001, 100000},
		{21000000.0, 2100000000000000},
		{0.00100000, 100000},
		{0.00000546, 546}, // dust limit
	}

	for _, tt := range tests {
		result := btcToSatoshi(tt.btc)
		if result != tt.expected {
			t.Errorf("btcToSatoshi(%f) = %d, want %d", tt.btc, result, tt.expected)
		}
	}
}

func TestTxInvolvesAddress(t *testing.T) {
	tx := &api.Transaction{
		Vin: []api.Vin{
			{
				TxID: "prev1",
				Prevout: &api.Vout{
					ScriptpubkeyAddress: "tb1qinput",
					Value:               100000,
				},
			},
		},
		Vout: []api.Vout{
			{
				ScriptpubkeyAddress: "tb1qoutput",
				Value:               90000,
			},
		},
	}

	if !txInvolvesAddress(tx, "tb1qinput") {
		t.Error("expected txInvolvesAddress to return true for input address")
	}
	if !txInvolvesAddress(tx, "tb1qoutput") {
		t.Error("expected txInvolvesAddress to return true for output address")
	}
	if txInvolvesAddress(tx, "tb1qother") {
		t.Error("expected txInvolvesAddress to return false for unrelated address")
	}
}

func TestRPCBackend_Connect_Full(t *testing.T) {
	handlers := map[string]interface{}{
		"getblockchaininfo": map[string]interface{}{
			"chain":                "testnet4",
			"blocks":               50000,
			"initialblockdownload": false,
		},
		"getnetworkinfo": map[string]interface{}{
			"subversion": "/Satoshi:27.0.0/",
			"version":    270000,
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())

	b := &RPCBackend{
		config: &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC: &RPCConfig{
				Host:     u.Hostname(),
				Port:     port,
				Username: "testuser",
				Password: "testpass",
			},
		},
	}

	err := b.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	if !b.IsConnected() {
		t.Fatal("expected IsConnected() = true after Connect()")
	}
	if b.ServerVersion() != "/Satoshi:27.0.0/" {
		t.Fatalf("expected ServerVersion /Satoshi:27.0.0/, got %s", b.ServerVersion())
	}
	if b.Chain() != "testnet4" {
		t.Fatalf("expected Chain testnet4, got %s", b.Chain())
	}
}

func TestRPCBackend_Connect_AlreadyConnected(t *testing.T) {
	b := &RPCBackend{
		config: &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC:     &RPCConfig{},
		},
		connected: true,
	}

	err := b.Connect()
	if err != nil {
		t.Fatalf("Connect() should return nil for already connected: %v", err)
	}
}

func TestRPCBackend_Connect_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())

	b := &RPCBackend{
		config: &BackendConfig{
			Type:    BackendTypeRPC,
			Network: "testnet",
			RPC: &RPCConfig{
				Host:     u.Hostname(),
				Port:     port,
				Username: "testuser",
				Password: "testpass",
			},
		},
	}

	err := b.Connect()
	if err == nil {
		t.Fatal("expected Connect() to fail")
	}
	if b.IsConnected() {
		t.Fatal("expected IsConnected() = false after failed Connect()")
	}
}

func TestRPCBackend_Disconnect(t *testing.T) {
	handlers := map[string]interface{}{
		"getblockcount": int64(1),
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	if !b.IsConnected() {
		t.Fatal("expected connected before disconnect")
	}

	err := b.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect() failed: %v", err)
	}

	if b.IsConnected() {
		t.Fatal("expected not connected after disconnect")
	}

	_, err = b.callRPC("getblockcount", nil)
	if err == nil {
		t.Fatal("expected error calling RPC after disconnect")
	}
}

func TestRPCBackend_IsConnected(t *testing.T) {
	b := &RPCBackend{}

	if b.IsConnected() {
		t.Fatal("expected not connected for new backend")
	}

	b.connected = true
	if !b.IsConnected() {
		t.Fatal("expected connected after setting connected=true")
	}
}

func TestRPCBackend_GetAddressTransactions_Scanblocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp rpcResponse
		resp.ID = req.ID

		switch req.Method {
		case "scanblocks":
			result, _ := json.Marshal(map[string]interface{}{
				"relevant_blocks": []string{"blockhash1"},
			})
			resp.Result = result
		case "getblock":
			result, _ := json.Marshal(map[string]interface{}{
				"height": 50000,
				"time":   1700000000,
				"hash":   "blockhash1",
				"tx": []map[string]interface{}{
					{
						"txid":    "tx_match",
						"version": 2,
						"size":    225,
						"vsize":   144,
						"weight":  573,
						"fee":     0.00001,
						"vin": []map[string]interface{}{
							{
								"txid":      "prevtx",
								"vout":      0,
								"scriptSig": map[string]string{"hex": ""},
								"sequence":  4294967295,
							},
						},
						"vout": []map[string]interface{}{
							{
								"value": 0.001,
								"n":     0,
								"scriptPubKey": map[string]interface{}{
									"hex":     "0014abc",
									"address": "tb1qtarget",
									"type":    "witness_v0_keyhash",
								},
							},
						},
						"blockhash":     "blockhash1",
						"confirmations": 10,
						"blocktime":     1700000000,
					},
					{
						"txid":    "tx_other",
						"version": 2,
						"size":    225,
						"vsize":   144,
						"weight":  573,
						"fee":     0.00001,
						"vin": []map[string]interface{}{
							{
								"txid":      "prevtx2",
								"vout":      0,
								"scriptSig": map[string]string{"hex": ""},
								"sequence":  4294967295,
							},
						},
						"vout": []map[string]interface{}{
							{
								"value": 0.002,
								"n":     0,
								"scriptPubKey": map[string]interface{}{
									"hex":     "0014def",
									"address": "tb1qother",
									"type":    "witness_v0_keyhash",
								},
							},
						},
						"blockhash":     "blockhash1",
						"confirmations": 10,
						"blocktime":     1700000000,
					},
				},
			})
			resp.Result = result
		case "getblockheader":
			result, _ := json.Marshal(map[string]interface{}{
				"height": int64(50000),
			})
			resp.Result = result
		default:
			resp.Error = &rpcError{Code: -32601, Message: "Method not found"}
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestRPCBackend(t, server)
	b.baseURL = server.URL
	b.client = server.Client()

	txs, err := b.GetAddressTransactions("tb1qtarget")
	if err != nil {
		t.Fatalf("GetAddressTransactions failed: %v", err)
	}

	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction (filtered by address), got %d", len(txs))
	}
	if txs[0].TxID != "tx_match" {
		t.Fatalf("expected txid tx_match, got %s", txs[0].TxID)
	}
	if !txs[0].Status.Confirmed {
		t.Fatal("expected confirmed=true")
	}
	if txs[0].Status.BlockHeight != 50000 {
		t.Fatalf("expected block height 50000, got %d", txs[0].Status.BlockHeight)
	}
}

func TestRPCBackend_GetAddressTransactions_Fallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp rpcResponse
		resp.ID = req.ID

		switch req.Method {
		case "scanblocks":
			resp.Error = &rpcError{Code: -1, Message: "Index is not enabled for filtertype basic"}
		case "scantxoutset":
			result, _ := json.Marshal(map[string]interface{}{
				"success": true,
				"unspents": []map[string]interface{}{
					{"txid": "utxo_tx1", "vout": 0, "amount": 0.001, "height": 50000},
				},
				"total_amount": 0.001,
			})
			resp.Result = result
		case "getrawtransaction":
			result, _ := json.Marshal(map[string]interface{}{
				"txid":    "utxo_tx1",
				"version": 2,
				"size":    225,
				"vsize":   144,
				"weight":  573,
				"fee":     0.00001,
				"vin": []map[string]interface{}{
					{
						"txid":      "prevtx",
						"vout":      0,
						"scriptSig": map[string]string{"hex": ""},
						"sequence":  4294967295,
					},
				},
				"vout": []map[string]interface{}{
					{
						"value": 0.001,
						"n":     0,
						"scriptPubKey": map[string]interface{}{
							"hex":     "0014abc",
							"address": "tb1qtarget",
							"type":    "witness_v0_keyhash",
						},
					},
				},
				"blockhash":     "blockhash1",
				"confirmations": 10,
				"blocktime":     1700000000,
			})
			resp.Result = result
		case "getblockheader":
			result, _ := json.Marshal(map[string]interface{}{
				"height": int64(50000),
			})
			resp.Result = result
		default:
			resp.Error = &rpcError{Code: -32601, Message: "Method not found"}
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestRPCBackend(t, server)
	b.baseURL = server.URL
	b.client = server.Client()

	txs, err := b.GetAddressTransactions("tb1qtarget")
	if err != nil {
		t.Fatalf("GetAddressTransactions fallback failed: %v", err)
	}

	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction from UTXO fallback, got %d", len(txs))
	}
	if txs[0].TxID != "utxo_tx1" {
		t.Fatalf("expected txid utxo_tx1, got %s", txs[0].TxID)
	}
}

func TestRPCBackend_GetAddressTransactions_Empty(t *testing.T) {
	handlers := map[string]interface{}{
		"scanblocks": map[string]interface{}{
			"relevant_blocks": []string{},
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	txs, err := b.GetAddressTransactions("tb1qunused")
	if err != nil {
		t.Fatalf("GetAddressTransactions failed: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("expected 0 transactions, got %d", len(txs))
	}
}

func TestRPCBackend_GetAddressInfo_SpentViaScanblocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp rpcResponse
		resp.ID = req.ID

		switch req.Method {
		case "scantxoutset":
			result, _ := json.Marshal(map[string]interface{}{
				"success":      true,
				"unspents":     []interface{}{},
				"total_amount": 0.0,
			})
			resp.Result = result
		case "scanblocks":
			result, _ := json.Marshal(map[string]interface{}{
				"relevant_blocks": []string{"blockhash1", "blockhash2"},
			})
			resp.Result = result
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestRPCBackend(t, server)
	b.baseURL = server.URL
	b.client = server.Client()

	info, err := b.GetAddressInfo("tb1qspent")
	if err != nil {
		t.Fatalf("GetAddressInfo failed: %v", err)
	}

	if info.ChainStats.TxCount == 0 {
		t.Fatal("expected TxCount > 0 for address found via scanblocks")
	}
}

func TestRPCBackend_GetTransaction_NoPrevout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp rpcResponse
		resp.ID = req.ID

		switch req.Method {
		case "getrawtransaction":
			params, _ := json.Marshal(req.Params)
			var args []interface{}
			json.Unmarshal(params, &args)
			txid, _ := args[0].(string)

			if txid == "main_tx" {
				result, _ := json.Marshal(map[string]interface{}{
					"txid":    "main_tx",
					"version": 2,
					"size":    225,
					"vsize":   144,
					"weight":  573,
					// No "fee" field — triggers populateInputValuesAndFee
					"vin": []map[string]interface{}{
						{
							"txid":      "prev_tx",
							"vout":      0,
							"scriptSig": map[string]string{"hex": "abc"},
							"sequence":  4294967295,
						},
					},
					"vout": []map[string]interface{}{
						{
							"value": 0.00090000,
							"n":     0,
							"scriptPubKey": map[string]interface{}{
								"hex":     "0014abc",
								"address": "tb1qrecipient",
								"type":    "witness_v0_keyhash",
							},
						},
					},
					"blockhash":     "blockhash1",
					"confirmations": 5,
					"blocktime":     1700000000,
				})
				resp.Result = result
			} else if txid == "prev_tx" {
				result, _ := json.Marshal(map[string]interface{}{
					"txid":    "prev_tx",
					"version": 2,
					"size":    200,
					"vsize":   140,
					"weight":  560,
					"fee":     0.00001,
					"vin":     []map[string]interface{}{},
					"vout": []map[string]interface{}{
						{
							"value": 0.00100000,
							"n":     0,
							"scriptPubKey": map[string]interface{}{
								"hex":     "0014def",
								"address": "tb1qsender",
								"type":    "witness_v0_keyhash",
							},
						},
					},
					"blockhash":     "blockhash0",
					"confirmations": 10,
					"blocktime":     1699999000,
				})
				resp.Result = result
			}
		case "getblockheader":
			result, _ := json.Marshal(map[string]interface{}{
				"height": int64(50000),
			})
			resp.Result = result
		default:
			resp.Error = &rpcError{Code: -32601, Message: "Method not found"}
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestRPCBackend(t, server)
	b.baseURL = server.URL
	b.client = server.Client()

	tx, err := b.GetTransaction("main_tx")
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}

	// Fee should be calculated: 100000 (input) - 90000 (output) = 10000 sat
	if tx.Fee != 10000 {
		t.Fatalf("expected fee 10000 sat (calculated from inputs-outputs), got %d", tx.Fee)
	}
	if tx.Vin[0].Prevout == nil {
		t.Fatal("expected prevout to be populated after populateInputValuesAndFee")
	}
	if tx.Vin[0].Prevout.Value != 100000 {
		t.Fatalf("expected prevout value 100000, got %d", tx.Vin[0].Prevout.Value)
	}
}

func TestRPCBackend_GetTransaction_Coinbase(t *testing.T) {
	handlers := map[string]interface{}{
		"getrawtransaction": map[string]interface{}{
			"txid":    "coinbase_tx",
			"version": 2,
			"size":    200,
			"vsize":   170,
			"weight":  680,
			"vin": []map[string]interface{}{
				{
					"coinbase": "03a08601",
					"sequence": 4294967295,
				},
			},
			"vout": []map[string]interface{}{
				{
					"value": 6.25000000,
					"n":     0,
					"scriptPubKey": map[string]interface{}{
						"hex":     "0014miner",
						"address": "tb1qminer",
						"type":    "witness_v0_keyhash",
					},
				},
			},
			"blockhash":     "blockhash_cb",
			"confirmations": 100,
			"blocktime":     1700000000,
		},
		"getblockheader": map[string]interface{}{
			"height": int64(50000),
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	tx, err := b.GetTransaction("coinbase_tx")
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}

	if tx.TxID != "coinbase_tx" {
		t.Fatalf("expected txid coinbase_tx, got %s", tx.TxID)
	}
	// Coinbase tx should not trigger populateInputValuesAndFee
	if tx.Fee != 0 {
		t.Fatalf("expected fee 0 for coinbase tx, got %d", tx.Fee)
	}
	if tx.Vout[0].Value != 625000000 {
		t.Fatalf("expected output 625000000 sat, got %d", tx.Vout[0].Value)
	}
}

func TestRPCBackend_GetTransaction_Unconfirmed(t *testing.T) {
	handlers := map[string]interface{}{
		"getrawtransaction": map[string]interface{}{
			"txid":    "mempool_tx",
			"version": 2,
			"size":    225,
			"vsize":   144,
			"weight":  573,
			"fee":     0.00001000,
			"vin": []map[string]interface{}{
				{
					"txid":      "prev1",
					"vout":      0,
					"scriptSig": map[string]string{"hex": ""},
					"sequence":  4294967293,
				},
			},
			"vout": []map[string]interface{}{
				{
					"value": 0.001,
					"n":     0,
					"scriptPubKey": map[string]interface{}{
						"hex":     "0014abc",
						"address": "tb1qrecip",
						"type":    "witness_v0_keyhash",
					},
				},
			},
			// No blockhash, no confirmations — mempool tx
		},
	}
	server := mockRPCServer(t, handlers)
	defer server.Close()

	b := newTestRPCBackend(t, server)

	tx, err := b.GetTransaction("mempool_tx")
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}

	if tx.Status.Confirmed {
		t.Fatal("expected confirmed=false for mempool tx")
	}
	if tx.Status.BlockHeight != 0 {
		t.Fatalf("expected block height 0 for mempool tx, got %d", tx.Status.BlockHeight)
	}
	if tx.Status.BlockHash != "" {
		t.Fatalf("expected empty block hash for mempool tx, got %s", tx.Status.BlockHash)
	}
	if tx.Fee != 1000 {
		t.Fatalf("expected fee 1000 sat, got %d", tx.Fee)
	}
}

func TestRPCBackend_CallRPC_NotConnected(t *testing.T) {
	b := &RPCBackend{
		config: &BackendConfig{
			Type: BackendTypeRPC,
			RPC:  &RPCConfig{},
		},
	}

	_, err := b.callRPC("getblockcount", nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected 'not connected' error, got: %v", err)
	}
}

func TestRPCBackend_CallRPC_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("this is not json"))
	}))
	defer server.Close()

	b := newTestRPCBackend(t, server)

	_, err := b.callRPC("getblockcount", nil)
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestRPCBackend_ServerVersion_Chain(t *testing.T) {
	b := &RPCBackend{}

	if b.ServerVersion() != "" {
		t.Fatalf("expected empty ServerVersion before connect, got %s", b.ServerVersion())
	}
	if b.Chain() != "" {
		t.Fatalf("expected empty Chain before connect, got %s", b.Chain())
	}

	b.version = "/Satoshi:27.0.0/"
	b.chain = "testnet4"

	if b.ServerVersion() != "/Satoshi:27.0.0/" {
		t.Fatalf("expected /Satoshi:27.0.0/, got %s", b.ServerVersion())
	}
	if b.Chain() != "testnet4" {
		t.Fatalf("expected testnet4, got %s", b.Chain())
	}
}
