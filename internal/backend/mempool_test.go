package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRPCMempoolEntryFeeAndIsUnspent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64        `json:"id"`
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]interface{}{"id": req.ID}
		switch req.Method {
		case "getmempoolentry":
			resp["result"] = map[string]interface{}{
				"vsize": 141, "fees": map[string]interface{}{"base": 0.00000282}}
		case "gettxout":
			// Output 0 unspent (mempool included), output 1 spent.
			if req.Params[1].(float64) == 0 && req.Params[2] == true {
				resp["result"] = map[string]interface{}{"value": 0.001}
			} else {
				resp["result"] = nil
			}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "Method not found"}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := newTestRPCBackend(t, srv)
	fee, err := b.MempoolEntryFee("aa")
	if err != nil || fee != 282 {
		t.Fatalf("fee = %d, %v; want 282", fee, err)
	}
	if ok, err := b.IsUnspent("aa", 0); err != nil || !ok {
		t.Fatalf("output 0: %v, %v; want unspent", ok, err)
	}
	if ok, err := b.IsUnspent("aa", 1); err != nil || ok {
		t.Fatalf("output 1: %v, %v; want spent", ok, err)
	}
}
