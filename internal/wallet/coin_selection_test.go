package wallet

import (
	"errors"
	"math"
	"testing"

	"github.com/BTCBap/hax/internal/api"
)

// makeUTXO is a test helper that creates a UTXOWithPath with the given value.
func makeUTXO(value api.Satoshi) UTXOWithPath {
	return UTXOWithPath{
		UTXO:    api.UTXO{TxID: "aabb", Vout: 0, Value: value},
		Address: "tb1qtest",
		Path:    "m/84'/1'/0'/0/0",
	}
}

// makeUTXOs creates a slice of UTXOWithPath from a list of values.
func makeUTXOs(values ...api.Satoshi) []UTXOWithPath {
	utxos := make([]UTXOWithPath, len(values))
	for i, v := range values {
		utxos[i] = makeUTXO(v)
	}
	return utxos
}

// singleOutput is a convenience for single-recipient tests.
func singleOutput(amount api.Satoshi) []TxOutput {
	return []TxOutput{{Address: "tb1qrecipient", Amount: amount}}
}

// bnbBaseFee returns the base fee (overhead + recipient outputs, no inputs/change)
// for a given number of outputs at the specified fee rate and output type.
func bnbBaseFee(numOutputs int, feeRate float64, outputType ScriptType) api.Satoshi {
	vsize := TxOverheadVSize + EstimateOutputVSize(outputType)*numOutputs
	return api.Satoshi(math.Ceil(float64(vsize) * feeRate))
}

// --- selectBnB tests ---

func TestBnB_ExactMatch(t *testing.T) {
	feeRate := 1.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH)

	// We need: target = outputAmount + baseFee
	// Each selected UTXO contributes effectiveValue = value - inputFee
	// For exact match: sum(effectiveValues) == target
	// So with 2 UTXOs of values a and b: (a - inputFee) + (b - inputFee) = outputAmount + baseFee
	// => a + b = outputAmount + baseFee + 2*inputFee
	outputAmount := api.Satoshi(50000)
	target := outputAmount + baseFee
	// Two UTXOs that together exactly hit target
	utxoA := target/2 + inputFee
	utxoB := target - target/2 + inputFee

	selected, totalInput, err := selectBnB(
		makeUTXOs(utxoA, utxoB),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("expected exact match, got error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 UTXOs selected, got %d", len(selected))
	}
	if totalInput != utxoA+utxoB {
		t.Errorf("totalInput = %d, want %d", totalInput, utxoA+utxoB)
	}
}

func TestBnB_WithinTolerance(t *testing.T) {
	feeRate := 1.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH)

	outputAmount := api.Satoshi(50000)
	target := outputAmount + baseFee

	changeOutputFee := api.Satoshi(math.Ceil(float64(EstimateOutputVSize(ScriptTypeP2WPKH)) * feeRate))
	costOfChange := changeOutputFee + inputFee

	// One UTXO that overshoots target by half of costOfChange (within tolerance)
	overshoot := costOfChange / 2
	utxoVal := target + overshoot + inputFee

	selected, _, err := selectBnB(
		makeUTXOs(utxoVal),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("expected within-tolerance match, got error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 UTXO, got %d", len(selected))
	}
}

func TestBnB_NoSolution_Overshoot(t *testing.T) {
	feeRate := 1.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH)

	outputAmount := api.Satoshi(50000)
	target := outputAmount + baseFee

	changeOutputFee := api.Satoshi(math.Ceil(float64(EstimateOutputVSize(ScriptTypeP2WPKH)) * feeRate))
	costOfChange := changeOutputFee + inputFee

	// Single UTXO that overshoots beyond costOfChange
	utxoVal := target + costOfChange + 100 + inputFee

	_, _, err := selectBnB(
		makeUTXOs(utxoVal),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if !errors.Is(err, errBnBNoSolution) {
		t.Fatalf("expected errBnBNoSolution, got: %v", err)
	}
}

func TestBnB_NoSolution_Insufficient(t *testing.T) {
	_, _, err := selectBnB(
		makeUTXOs(1000, 2000),
		singleOutput(1_000_000),
		1.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if !errors.Is(err, errBnBNoSolution) {
		t.Fatalf("expected errBnBNoSolution, got: %v", err)
	}
}

func TestBnB_SingleUTXOMatch(t *testing.T) {
	feeRate := 2.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH)

	outputAmount := api.Satoshi(100000)
	target := outputAmount + baseFee
	utxoVal := target + inputFee

	selected, totalInput, err := selectBnB(
		makeUTXOs(utxoVal, 500000, 200),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalInput != utxoVal {
		t.Errorf("expected single UTXO match (%d), got totalInput=%d with %d UTXOs", utxoVal, totalInput, len(selected))
	}
}

func TestBnB_AllDustUTXOs(t *testing.T) {
	// With fee rate 10, input cost = 68*10 = 680
	// UTXOs worth less than 680 have negative effective value
	_, _, err := selectBnB(
		makeUTXOs(100, 200, 300, 500),
		singleOutput(50000),
		10.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if !errors.Is(err, errBnBNoSolution) {
		t.Fatalf("expected errBnBNoSolution for all-dust UTXOs, got: %v", err)
	}
}

func TestBnB_PrefersLowerWaste(t *testing.T) {
	feeRate := 1.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH)

	outputAmount := api.Satoshi(10000)
	target := outputAmount + baseFee

	// UTXO A: exact match (waste=0)
	exactUTXO := target + inputFee
	// UTXO B: slightly over (waste > 0)
	overUTXO := target + inputFee + 10

	selected, totalInput, err := selectBnB(
		makeUTXOs(overUTXO, exactUTXO),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 || totalInput != exactUTXO {
		t.Errorf("expected exact UTXO (%d) selected, got totalInput=%d with %d UTXOs", exactUTXO, totalInput, len(selected))
	}
}

func TestBnB_MaxIterationsCap(t *testing.T) {
	// Create many UTXOs with similar values to force lots of branching
	utxos := make([]UTXOWithPath, 50)
	for i := range utxos {
		utxos[i] = makeUTXO(api.Satoshi(10000 + i))
	}

	// Request something hard to match exactly — will hit iteration cap
	_, _, err := selectBnB(
		utxos,
		singleOutput(999999),
		1.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	// Should either find a solution or return errBnBNoSolution (not panic/hang)
	if err != nil && !errors.Is(err, errBnBNoSolution) {
		t.Fatalf("unexpected error type: %v", err)
	}
}

func TestBnB_ZeroFeeRate(t *testing.T) {
	feeRate := 0.0
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH) // should be 0

	outputAmount := api.Satoshi(50000)
	// With zero fee, effective value = face value, target = outputAmount
	target := outputAmount + baseFee

	selected, totalInput, err := selectBnB(
		makeUTXOs(target, 100000),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalInput != target {
		t.Errorf("expected exact match at %d, got %d with %d UTXOs", target, totalInput, len(selected))
	}
}

func TestBnB_HighFeeRate(t *testing.T) {
	// High fee rate makes small UTXOs unprofitable (negative effective value)
	feeRate := 100.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))

	// Two small UTXOs below input cost, one large one
	smallA := inputFee - 100
	smallB := inputFee - 50
	large := api.Satoshi(500000)

	selected, _, err := selectBnB(
		makeUTXOs(smallA, smallB, large),
		singleOutput(100000),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil && !errors.Is(err, errBnBNoSolution) {
		t.Fatalf("unexpected error: %v", err)
	}
	// If a solution was found, it should not include the dust UTXOs
	if err == nil {
		for _, s := range selected {
			if s.Value == smallA || s.Value == smallB {
				t.Error("solution should not include dust UTXOs")
			}
		}
	}
}

func TestBnB_MultipleOutputs(t *testing.T) {
	feeRate := 1.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))

	outputs := []TxOutput{
		{Address: "tb1qa", Amount: 30000},
		{Address: "tb1qb", Amount: 20000},
	}
	baseFee := bnbBaseFee(2, feeRate, ScriptTypeP2WPKH)
	target := api.Satoshi(50000) + baseFee

	utxoVal := target + inputFee

	selected, _, err := selectBnB(
		makeUTXOs(utxoVal, 1000000),
		outputs,
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Errorf("expected 1 UTXO, got %d", len(selected))
	}
}

func TestBnB_AllScriptTypes(t *testing.T) {
	types := []struct {
		name string
		st   ScriptType
	}{
		{"P2PKH", ScriptTypeP2PKH},
		{"P2SHP2WPKH", ScriptTypeP2SHP2WPKH},
		{"P2WPKH", ScriptTypeP2WPKH},
		{"P2TR", ScriptTypeP2TR},
	}

	for _, tt := range types {
		t.Run(tt.name, func(t *testing.T) {
			feeRate := 1.0
			inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(tt.st, 0, 0)) * feeRate))
			baseFee := bnbBaseFee(1, feeRate, tt.st)

			outputAmount := api.Satoshi(50000)
			target := outputAmount + baseFee
			utxoVal := target + inputFee

			_, _, err := selectBnB(
				makeUTXOs(utxoVal),
				singleOutput(outputAmount),
				feeRate, tt.st, tt.st, 0, 0,
			)
			if err != nil {
				t.Errorf("script type %s: unexpected error: %v", tt.name, err)
			}
		})
	}
}

// --- selectAutoUTXOs integration tests ---

func TestAutoSelection_BnBSuccess(t *testing.T) {
	feeRate := 1.0
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(ScriptTypeP2WPKH, 0, 0)) * feeRate))
	baseFee := bnbBaseFee(1, feeRate, ScriptTypeP2WPKH)

	outputAmount := api.Satoshi(50000)
	target := outputAmount + baseFee
	utxoVal := target + inputFee

	selected, totalInput, err := selectAutoUTXOs(
		makeUTXOs(utxoVal),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// BnB should have found a changeless match
	if len(selected) != 1 || totalInput != utxoVal {
		t.Errorf("expected BnB exact match, got %d UTXOs totalInput=%d", len(selected), totalInput)
	}
}

func TestAutoSelection_FallbackToLargestFirst(t *testing.T) {
	// Use UTXOs that can't match BnB but can cover via largest-first + change
	selected, _, err := selectAutoUTXOs(
		makeUTXOs(100000, 200000),
		singleOutput(50000),
		1.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) == 0 {
		t.Fatal("expected at least one UTXO selected")
	}
}

func TestAutoSelection_EmptyUTXOs(t *testing.T) {
	_, _, err := selectAutoUTXOs(
		nil,
		singleOutput(50000),
		1.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err == nil {
		t.Fatal("expected error for empty UTXOs")
	}
}

func TestAutoSelection_InsufficientFunds(t *testing.T) {
	_, _, err := selectAutoUTXOs(
		makeUTXOs(1000),
		singleOutput(1_000_000),
		1.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err == nil {
		t.Fatal("expected insufficient funds error")
	}
}

// --- selectLargestFirst regression tests ---

func TestLargestFirst_BasicSelection(t *testing.T) {
	selected, totalInput, err := selectLargestFirst(
		makeUTXOs(10000, 50000, 30000),
		singleOutput(40000),
		1.0, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) == 0 {
		t.Fatal("expected UTXOs selected")
	}
	// Largest-first should pick 50000 first
	if selected[0].Value != 50000 {
		t.Errorf("expected largest UTXO first (50000), got %d", selected[0].Value)
	}
	_ = totalInput
}

func TestLargestFirst_DustChangeAbsorbed(t *testing.T) {
	// Create a scenario where change would be dust and gets absorbed into fee
	feeRate := 1.0
	// With one P2WPKH input and one output: vSize = 68 + 31 + 11 = 110, fee = 110
	// UTXO = outputAmount + fee + small_dust
	outputAmount := api.Satoshi(50000)
	utxoVal := outputAmount + 115 // just slightly over needed (dust change)

	selected, _, err := selectLargestFirst(
		makeUTXOs(utxoVal),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Errorf("expected 1 UTXO, got %d", len(selected))
	}
}

func TestLargestFirst_ExactMatchNoChange(t *testing.T) {
	feeRate := 1.0
	// Exact match: UTXO = outputAmount + fee (no change output)
	// vSize with 1 input, 1 output (no change) = 68 + 31 + 11 = 110
	fee := api.Satoshi(math.Ceil(float64(EstimateVSize(1, 1, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0)) * feeRate))
	outputAmount := api.Satoshi(50000)
	utxoVal := outputAmount + fee

	selected, _, err := selectLargestFirst(
		makeUTXOs(utxoVal),
		singleOutput(outputAmount),
		feeRate, ScriptTypeP2WPKH, ScriptTypeP2WPKH, 0, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Errorf("expected 1 UTXO, got %d", len(selected))
	}
}
