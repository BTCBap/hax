package backend

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

func sampleTx(t *testing.T) (*wire.MsgTx, string) {
	t.Helper()
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: 1}, nil, nil))
	tx.AddTxOut(wire.NewTxOut(5000, []byte{0x00, 0x14, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}))
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	return tx, hex.EncodeToString(buf.Bytes())
}

func TestDecodeRawTx(t *testing.T) {
	tx, raw := sampleTx(t)
	got, err := decodeRawTx(tx.TxHash().String(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.TxHash() != tx.TxHash() {
		t.Fatal("decoded a different transaction")
	}
	other := strings.Repeat("ab", 32)
	if _, err := decodeRawTx(other, raw); err == nil {
		t.Fatal("a transaction that does not hash to the requested txid must be rejected")
	}
	if _, err := decodeRawTx(tx.TxHash().String(), "zz"); err == nil {
		t.Fatal("bad hex must fail")
	}
}

// Without -txindex Core cannot find a confirmed transaction by txid
// alone; the backend retries with the block hash from the height hint.
func TestRPCGetRawTransaction_BlockHashFallback(t *testing.T) {
	tx, raw := sampleTx(t)
	txid := tx.TxHash().String()
	const blockHash = "00000000000000000000000000000000000000000000000000000000000000aa"
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64        `json:"id"`
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Method)
		resp := map[string]interface{}{"id": req.ID}
		switch {
		case req.Method == "getblockhash":
			resp["result"] = blockHash
		case req.Method == "getrawtransaction" && len(req.Params) == 3 && req.Params[2] == blockHash:
			resp["result"] = raw
		case req.Method == "getrawtransaction":
			resp["error"] = map[string]interface{}{"code": -5, "message": "No such mempool transaction. Use -txindex"}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "Method not found"}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := newTestRPCBackend(t, srv)
	got, err := b.GetRawTransaction(txid, 812)
	if err != nil {
		t.Fatalf("GetRawTransaction: %v (calls %v)", err, calls)
	}
	if got.TxHash() != tx.TxHash() {
		t.Fatal("wrong transaction")
	}
	if strings.Join(calls, ",") != "getrawtransaction,getblockhash,getrawtransaction" {
		t.Fatalf("calls = %v", calls)
	}

	// Unconfirmed (no height): no block lookup, the error is returned.
	calls = nil
	if _, err := b.GetRawTransaction(txid, 0); err == nil {
		t.Fatal("expected an error without txindex or height")
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
}
