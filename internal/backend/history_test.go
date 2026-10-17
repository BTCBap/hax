package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const histAddr = "tb1qhistory"

// historyServer fakes Core for address history. Two blocks: the first
// pays histAddr, the second spends that coin to someone else. Decoded
// blocks carry no prevouts, as with getblock verbosity 2.
func historyServer(t *testing.T, filters bool) *httptest.Server {
	t.Helper()
	blocks := map[string]interface{}{
		"b1": map[string]interface{}{
			"height": 1, "time": 100, "hash": "b1",
			"tx": []interface{}{map[string]interface{}{
				"txid": "fund", "vin": []interface{}{},
				"vout": []interface{}{map[string]interface{}{
					"value": 0.001, "n": 0,
					"scriptPubKey": map[string]interface{}{"address": histAddr},
				}},
			}},
		},
		"b2": map[string]interface{}{
			"height": 2, "time": 200, "hash": "b2",
			"tx": []interface{}{map[string]interface{}{
				"txid": "spend",
				"vin":  []interface{}{map[string]interface{}{"txid": "fund", "vout": 0}},
				"vout": []interface{}{map[string]interface{}{
					"value": 0.0009, "n": 0,
					"scriptPubKey": map[string]interface{}{"address": "tb1qother"},
				}},
			}},
		},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64        `json:"id"`
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]interface{}{"id": req.ID}
		notFound := map[string]interface{}{"code": -1, "message": "not available"}
		switch {
		case req.Method == "scanblocks" && filters:
			resp["result"] = map[string]interface{}{
				"relevant_blocks": []string{"b1", "b2"}, "completed": true}
		case req.Method == "getblock" && req.Params[1].(float64) == 2:
			resp["result"] = blocks[req.Params[0].(string)]
		case req.Method == "scantxoutset":
			resp["result"] = map[string]interface{}{
				"success": true, "unspents": []interface{}{}}
		default:
			resp["error"] = notFound
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestRPCHistoryFindsSpendWithoutPrevouts(t *testing.T) {
	srv := historyServer(t, true)
	defer srv.Close()
	b := newTestRPCBackend(t, srv)

	txs, err := b.GetAddressTransactions(histAddr)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 2 || txs[0].TxID != "fund" || txs[1].TxID != "spend" {
		t.Fatalf("got %+v; want fund then spend", txs)
	}
	prev := txs[1].Vin[0].Prevout
	if prev == nil || prev.ScriptpubkeyAddress != histAddr || prev.Value != 100000 {
		t.Fatalf("spend prevout = %+v; want the funding output", prev)
	}
	if txs[1].Status.BlockHeight != 2 || !txs[1].Status.Confirmed {
		t.Fatalf("spend status = %+v", txs[1].Status)
	}
	if b.HistoryLimited() {
		t.Fatal("HistoryLimited with scanblocks available")
	}
}

func TestRPCHistoryLimitedWithoutBlockFilters(t *testing.T) {
	srv := historyServer(t, false)
	defer srv.Close()
	b := newTestRPCBackend(t, srv)

	txs, err := b.GetAddressTransactions(histAddr)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 0 {
		t.Fatalf("got %d transactions from an empty UTXO set", len(txs))
	}
	if !b.HistoryLimited() {
		t.Fatal("HistoryLimited = false without scanblocks")
	}
}
