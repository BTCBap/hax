package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBtcPerKBToSatPerVB(t *testing.T) {
	tests := []struct {
		name     string
		btcPerKB float64
		want     float64
		ok       bool
	}{
		{name: "negative means unavailable", btcPerKB: -1, ok: false},
		{name: "zero unavailable", btcPerKB: 0, ok: false},
		{name: "10 sat/vB", btcPerKB: 0.0001, want: 10, ok: true},
		{name: "1 sat/vB", btcPerKB: 0.00001, want: 1, ok: true},
		{name: "sub-min floors to 0.1", btcPerKB: 0.0000005, want: 0.1, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := btcPerKBToSatPerVB(tt.btcPerKB)
			if ok != tt.ok {
				t.Fatalf("ok=%v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssembleFeeEstimates_AllUnavailable(t *testing.T) {
	fees, err := assembleFeeEstimates(make([]float64, 5), make([]bool, 5))
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got fees=%v err=%v", fees, err)
	}
	if fees != nil {
		t.Fatalf("expected nil fees, got %+v", fees)
	}
}

func TestAssembleFeeEstimates_Partial(t *testing.T) {
	rates := []float64{20, 0, 5, 0, 0}
	ok := []bool{true, false, true, false, false}
	fees, err := assembleFeeEstimates(rates, ok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fees.FastestFee != 20 || fees.HourFee != 5 {
		t.Fatalf("unexpected fees: %+v", fees)
	}
	if fees.HalfHourFee != 0 || fees.EconomyFee != 0 || fees.MinimumFee != 0 {
		t.Fatalf("unavailable targets must stay 0: %+v", fees)
	}
}

func TestAssembleFeeEstimates_InvalidLength(t *testing.T) {
	_, err := assembleFeeEstimates([]float64{1}, []bool{true})
	if err == nil {
		t.Fatal("expected error for invalid length")
	}
}

func TestElectrumBackend_NoMempoolFeeAPI(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	srcPath := filepath.Join(filepath.Dir(file), "electrum_backend.go")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, needle := range []string{
		"mempool.space/api/v1/fees",
		"fetchMempoolPreciseFees",
	} {
		if strings.Contains(body, needle) {
			t.Fatalf("electrum_backend.go must not contain %q", needle)
		}
	}
	if !strings.Contains(body, `request("blockchain.estimatefee"`) {
		t.Fatal("electrum_backend.go must call blockchain.estimatefee")
	}
}
