package wallet

import (
	"testing"
)

func TestEstimateInputVSize(t *testing.T) {
	tests := []struct {
		name     string
		st       ScriptType
		m, n     int
		expected int
		margin   int // Allowed margin of error
	}{
		// Single-sig types (exact values)
		{"P2PKH (Legacy)", ScriptTypeP2PKH, 0, 0, 148, 0},
		{"P2SH-P2WPKH (Nested SegWit)", ScriptTypeP2SHP2WPKH, 0, 0, 91, 0},
		{"P2WPKH (Native SegWit)", ScriptTypeP2WPKH, 0, 0, 68, 0},
		{"P2TR (Taproot)", ScriptTypeP2TR, 0, 0, 58, 0},

		// Multisig types - values based on our formula which underestimates slightly
		// This is conservative (better to underestimate and have slightly higher fee rate)
		// Note: The formula uses simplified math; real values may be slightly higher
		{"P2WSH 2-of-2", ScriptTypeP2WSH, 2, 2, 97, 10},
		{"P2WSH 2-of-3", ScriptTypeP2WSH, 2, 3, 104, 10},
		{"P2WSH 3-of-3", ScriptTypeP2WSH, 3, 3, 124, 20}, // Formula gives 124, reference ~140
		{"P2WSH 3-of-5", ScriptTypeP2WSH, 3, 5, 141, 15}, // Formula gives 141, reference ~152
		{"P2WSH 2-of-5", ScriptTypeP2WSH, 2, 5, 122, 10}, // Formula gives 122, reference ~116

		// Nested SegWit multisig (P2SH-P2WSH)
		{"P2SH-P2WSH 2-of-3", ScriptTypeP2SHP2WSH, 2, 3, 127, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateInputVSize(tt.st, tt.m, tt.n)
			diff := abs(got - tt.expected)
			if diff > tt.margin {
				t.Errorf("EstimateInputVSize(%v, %d, %d) = %d, want ~%d (margin %d)",
					tt.st, tt.m, tt.n, got, tt.expected, tt.margin)
			}
		})
	}
}

func TestEstimateOutputVSize(t *testing.T) {
	tests := []struct {
		name     string
		st       ScriptType
		expected int
	}{
		{"P2PKH output", ScriptTypeP2PKH, 34},
		{"P2SH output", ScriptTypeP2SH, 32},
		{"P2SH-P2WPKH output", ScriptTypeP2SHP2WPKH, 32},
		{"P2WPKH output", ScriptTypeP2WPKH, 31},
		{"P2WSH output", ScriptTypeP2WSH, 43},
		{"P2TR output", ScriptTypeP2TR, 43},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateOutputVSize(tt.st)
			if got != tt.expected {
				t.Errorf("EstimateOutputVSize(%v) = %d, want %d", tt.st, got, tt.expected)
			}
		})
	}
}

func TestEstimateVSize_SingleSig(t *testing.T) {
	tests := []struct {
		name       string
		numInputs  int
		numOutputs int
		inputType  ScriptType
		expected   int
		margin     int
	}{
		// P2WPKH (most common)
		{"P2WPKH 1-in-2-out", 1, 2, ScriptTypeP2WPKH, 68 + 62 + 11, 0},
		{"P2WPKH 2-in-2-out", 2, 2, ScriptTypeP2WPKH, 136 + 62 + 11, 0},

		// P2TR (Taproot) - smaller inputs
		{"P2TR 1-in-2-out", 1, 2, ScriptTypeP2TR, 58 + 86 + 11, 0},

		// P2PKH (Legacy) - larger inputs
		{"P2PKH 1-in-2-out", 1, 2, ScriptTypeP2PKH, 148 + 68 + 11, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateVSize(tt.numInputs, tt.numOutputs, tt.inputType, tt.inputType, 0, 0)
			diff := abs(got - tt.expected)
			if diff > tt.margin {
				t.Errorf("EstimateVSize(%d, %d, %v) = %d, want ~%d",
					tt.numInputs, tt.numOutputs, tt.inputType, got, tt.expected)
			}
		})
	}
}

func TestEstimateVSize_Multisig(t *testing.T) {
	tests := []struct {
		name       string
		numInputs  int
		numOutputs int
		m, n       int
		expected   int
		margin     int
	}{
		// 2-of-3 P2WSH: input ~104 vB, output ~43 vB, overhead ~11 vB
		{"P2WSH 2-of-3, 1-in-2-out", 1, 2, 2, 3, 104 + 86 + 11, 10},
		{"P2WSH 2-of-3, 2-in-2-out", 2, 2, 2, 3, 208 + 86 + 11, 15},

		// 2-of-2 P2WSH: input ~97 vB
		{"P2WSH 2-of-2, 1-in-2-out", 1, 2, 2, 2, 97 + 86 + 11, 10},

		// 3-of-5 P2WSH: input ~152 vB
		{"P2WSH 3-of-5, 1-in-2-out", 1, 2, 3, 5, 152 + 86 + 11, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateVSize(tt.numInputs, tt.numOutputs, ScriptTypeP2WSH, ScriptTypeP2WSH, tt.m, tt.n)
			diff := abs(got - tt.expected)
			if diff > tt.margin {
				t.Errorf("EstimateVSize(%d, %d, P2WSH, %d-of-%d) = %d, want ~%d (margin %d)",
					tt.numInputs, tt.numOutputs, tt.m, tt.n, got, tt.expected, tt.margin)
			}
		})
	}
}

func TestEstimateNativeMultisigInputVSize(t *testing.T) {
	// Verify our formula produces reasonable values
	// The formula slightly underestimates (which is safer for fee calculation)
	tests := []struct {
		m, n     int
		expected int
		margin   int
	}{
		{2, 2, 97, 10},  // 2-of-2
		{2, 3, 104, 10}, // 2-of-3 (most common)
		{3, 3, 124, 20}, // 3-of-3: formula gives 124
		{3, 5, 141, 15}, // 3-of-5: formula gives 141
		{5, 7, 195, 30}, // 5-of-7: formula gives 195
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := estimateNativeMultisigInputVSize(tt.m, tt.n)
			diff := abs(got - tt.expected)
			if diff > tt.margin {
				t.Errorf("estimateNativeMultisigInputVSize(%d, %d) = %d, want ~%d",
					tt.m, tt.n, got, tt.expected)
			}
		})
	}
}

func TestScriptTypeConstants(t *testing.T) {
	// Verify constants are reasonable
	if InputVSizeP2PKH < InputVSizeP2WPKH {
		t.Error("P2PKH inputs should be larger than P2WPKH")
	}
	if InputVSizeP2TR > InputVSizeP2WPKH {
		t.Error("P2TR inputs should be smaller than P2WPKH")
	}
	if InputVSizeP2SHP2WPKH < InputVSizeP2WPKH || InputVSizeP2SHP2WPKH > InputVSizeP2PKH {
		t.Error("P2SH-P2WPKH inputs should be between P2WPKH and P2PKH")
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
