package wallet

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/BTCBap/hax/internal/api"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

const bnbMaxTries = 100_000

var errBnBNoSolution = errors.New("no BnB solution found")

// TxOutput represents a transaction output
type TxOutput struct {
	Address string
	Amount  api.Satoshi // satoshis
}

// EstimateInputVSize returns the estimated vSize for a single input based on script type
// For multisig types, pass the M-of-N parameters; for single-sig, pass 0, 0
func EstimateInputVSize(st ScriptType, multisigM, multisigN int) int {
	switch st {
	case ScriptTypeP2PKH:
		return InputVSizeP2PKH
	case ScriptTypeP2SHP2WPKH:
		return InputVSizeP2SHP2WPKH
	case ScriptTypeP2WPKH:
		return InputVSizeP2WPKH
	case ScriptTypeP2TR:
		return InputVSizeP2TR
	case ScriptTypeP2SH: // Legacy multisig
		return estimateLegacyMultisigInputVSize(multisigM, multisigN)
	case ScriptTypeP2SHP2WSH: // Nested SegWit multisig
		return estimateNestedMultisigInputVSize(multisigM, multisigN)
	case ScriptTypeP2WSH: // Native SegWit multisig
		return estimateNativeMultisigInputVSize(multisigM, multisigN)
	default:
		return InputVSizeP2WPKH
	}
}

// EstimateOutputVSize returns the estimated vSize for a single output based on script type
func EstimateOutputVSize(st ScriptType) int {
	switch st {
	case ScriptTypeP2PKH:
		return OutputVSizeP2PKH
	case ScriptTypeP2SH, ScriptTypeP2SHP2WPKH, ScriptTypeP2SHP2WSH:
		return OutputVSizeP2SH
	case ScriptTypeP2WPKH:
		return OutputVSizeP2WPKH
	case ScriptTypeP2WSH:
		return OutputVSizeP2WSH
	case ScriptTypeP2TR:
		return OutputVSizeP2TR
	default:
		return OutputVSizeP2WPKH
	}
}

// EstimateVSize estimates total transaction vSize with script-type awareness
// inputType: the script type of inputs being spent
// outputType: the script type of outputs being created (often same as inputType for change)
// multisigM, multisigN: M-of-N parameters for multisig (0, 0 for single-sig)
func EstimateVSize(numInputs, numOutputs int, inputType, outputType ScriptType, multisigM, multisigN int) int {
	inputVSize := EstimateInputVSize(inputType, multisigM, multisigN) * numInputs
	outputVSize := EstimateOutputVSize(outputType) * numOutputs
	return inputVSize + outputVSize + TxOverheadVSize
}

// estimateNativeMultisigInputVSize estimates the vSize of a P2WSH multisig input
func estimateNativeMultisigInputVSize(m, n int) int {
	baseWeight := (36 + 1 + 4) * 4 // Non-witness part (in weight units)

	witnessStackItems := 1 + m + 1 // count + signatures + script
	emptyElement := 1
	signatures := m * 72 // 72 bytes per DER signature (industry standard, matches Sparrow)
	scriptSize := n*34 + 3

	witnessWeight := witnessStackItems + emptyElement + signatures + scriptSize

	totalWeight := baseWeight + witnessWeight
	vSize := (totalWeight + 3) / 4 // Round up

	return vSize
}

// estimateNestedMultisigInputVSize estimates the vSize of a P2SH-P2WSH multisig input
func estimateNestedMultisigInputVSize(m, n int) int {
	return estimateNativeMultisigInputVSize(m, n) + 23
}

// estimateLegacyMultisigInputVSize estimates the vSize of a P2SH legacy multisig input
func estimateLegacyMultisigInputVSize(m, n int) int {
	signatures := m * 72       // 72 bytes per DER signature (industry standard)
	scriptSize := n*34 + 3 + 1 // +1 for push opcode
	scriptSigLen := 1 + signatures + scriptSize

	varIntLen := 1
	if scriptSigLen > 252 {
		varIntLen = 3
	}

	return 36 + 4 + varIntLen + scriptSigLen
}

// SelectUTXOs selects UTXOs to cover the output amount plus fees.
// If forced is non-empty (coin control), uses exactly those UTXOs.
// Otherwise, uses a greedy algorithm: largest-first selection from available.
// Returns selected UTXOs, total input value, and error.
func SelectUTXOs(available, forced []UTXOWithPath, outputs []TxOutput, feeRate float64, inputType, outputType ScriptType, multisigM, multisigN int) ([]UTXOWithPath, api.Satoshi, error) {
	if len(forced) > 0 {
		return selectForcedUTXOs(forced, outputs, feeRate, inputType, outputType, multisigM, multisigN)
	}
	return selectAutoUTXOs(available, outputs, feeRate, inputType, outputType, multisigM, multisigN)
}

// selectForcedUTXOs validates and returns user-selected UTXOs (coin control)
func selectForcedUTXOs(forced []UTXOWithPath, outputs []TxOutput, feeRate float64, inputType, outputType ScriptType, multisigM, multisigN int) ([]UTXOWithPath, api.Satoshi, error) {
	if len(forced) == 0 {
		return nil, 0, fmt.Errorf("no UTXOs specified for coin control")
	}

	var totalInput api.Satoshi
	for _, utxo := range forced {
		totalInput += utxo.Value
	}

	var totalOutput api.Satoshi
	for _, output := range outputs {
		totalOutput += output.Amount
	}

	dustLimit := DustLimitSats

	numOutputs := len(outputs) + 1 // Assume change output
	vSize := EstimateVSize(len(forced), numOutputs, inputType, outputType, multisigM, multisigN)
	fee := api.Satoshi(math.Ceil(float64(vSize) * feeRate))

	changeAmount := totalInput - totalOutput - fee
	if changeAmount < dustLimit && changeAmount > 0 {
		numOutputs = len(outputs)
		vSize = EstimateVSize(len(forced), numOutputs, inputType, outputType, multisigM, multisigN)
		fee = api.Satoshi(math.Ceil(float64(vSize) * feeRate))
	}

	if totalInput < totalOutput+fee {
		return nil, 0, fmt.Errorf("insufficient funds in selected UTXOs: have %d sats, need %d sats (output: %d + fee: %d)",
			totalInput, totalOutput+fee, totalOutput, fee)
	}

	return forced, totalInput, nil
}

// selectAutoUTXOs performs automatic UTXO selection.
// Tries Branch-and-Bound first (changeless); falls back to largest-first.
func selectAutoUTXOs(available []UTXOWithPath, outputs []TxOutput, feeRate float64, inputType, outputType ScriptType, multisigM, multisigN int) ([]UTXOWithPath, api.Satoshi, error) {
	if len(available) == 0 {
		return nil, 0, fmt.Errorf("no UTXOs available")
	}

	selected, totalInput, err := selectBnB(available, outputs, feeRate, inputType, outputType, multisigM, multisigN)
	if err == nil {
		return selected, totalInput, nil
	}

	return selectLargestFirst(available, outputs, feeRate, inputType, outputType, multisigM, multisigN)
}

// effectiveUTXO pairs a UTXO with its effective value (face value minus spending cost)
type effectiveUTXO struct {
	utxo           UTXOWithPath
	effectiveValue api.Satoshi
}

// selectBnB implements Branch-and-Bound UTXO selection (same algorithm as Bitcoin Core).
// It searches for a UTXO combination that matches the target exactly within a tolerance,
// eliminating the need for a change output. Returns errBnBNoSolution if no match found.
func selectBnB(available []UTXOWithPath, outputs []TxOutput, feeRate float64, inputType, outputType ScriptType, multisigM, multisigN int) ([]UTXOWithPath, api.Satoshi, error) {
	inputFee := api.Satoshi(math.Ceil(float64(EstimateInputVSize(inputType, multisigM, multisigN)) * feeRate))

	// Build effective-value list, filtering out dust (non-positive effective value)
	candidates := make([]effectiveUTXO, 0, len(available))
	for _, u := range available {
		ev := u.Value - inputFee
		if ev > 0 {
			candidates = append(candidates, effectiveUTXO{utxo: u, effectiveValue: ev})
		}
	}
	if len(candidates) == 0 {
		return nil, 0, errBnBNoSolution
	}

	// Sort descending by effective value for better pruning
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].effectiveValue > candidates[j].effectiveValue
	})

	// Target = total output amount + base fee (overhead + recipient outputs, no inputs, no change)
	var totalOutput api.Satoshi
	for _, o := range outputs {
		totalOutput += o.Amount
	}
	baseFee := api.Satoshi(math.Ceil(float64(TxOverheadVSize+EstimateOutputVSize(outputType)*len(outputs)) * feeRate))
	target := totalOutput + baseFee

	// Cost of change = fee to create change output + fee to spend it later as an input
	changeOutputFee := api.Satoshi(math.Ceil(float64(EstimateOutputVSize(outputType)) * feeRate))
	changeSpendFee := inputFee
	costOfChange := changeOutputFee + changeSpendFee

	// Precompute suffix sums for pruning: suffixSum[i] = sum of effectiveValues[i:]
	suffixSum := make([]api.Satoshi, len(candidates)+1)
	for i := len(candidates) - 1; i >= 0; i-- {
		suffixSum[i] = suffixSum[i+1] + candidates[i].effectiveValue
	}

	// DFS with backtracking
	var bestSelection []bool
	bestWaste := costOfChange + 1 // worse than any valid solution
	currentSelection := make([]bool, len(candidates))
	var currentValue api.Satoshi
	tries := 0

	var search func(idx int)
	search = func(idx int) {
		if tries >= bnbMaxTries {
			return
		}
		tries++

		// Check if current selection is within [target, target + costOfChange]
		if currentValue >= target {
			waste := currentValue - target
			if waste <= costOfChange && waste < bestWaste {
				bestWaste = waste
				bestSelection = make([]bool, len(candidates))
				copy(bestSelection, currentSelection)
				if waste == 0 {
					tries = bnbMaxTries // perfect match, stop early
				}
			}
			return // adding more only increases waste
		}

		// Prune: can't reach target even with all remaining UTXOs
		if idx >= len(candidates) || currentValue+suffixSum[idx] < target {
			return
		}

		// Include candidate[idx]
		currentSelection[idx] = true
		currentValue += candidates[idx].effectiveValue
		search(idx + 1)
		currentValue -= candidates[idx].effectiveValue
		currentSelection[idx] = false

		// Exclude candidate[idx] (only if still possible to reach target)
		search(idx + 1)
	}

	search(0)

	if bestSelection == nil {
		return nil, 0, errBnBNoSolution
	}

	// Collect selected UTXOs
	var selected []UTXOWithPath
	var totalInput api.Satoshi
	for i, include := range bestSelection {
		if include {
			selected = append(selected, candidates[i].utxo)
			totalInput += candidates[i].utxo.Value
		}
	}

	return selected, totalInput, nil
}

// selectLargestFirst performs UTXO selection using largest-first greedy algorithm.
// Used as fallback when Branch-and-Bound finds no changeless solution.
func selectLargestFirst(available []UTXOWithPath, outputs []TxOutput, feeRate float64, inputType, outputType ScriptType, multisigM, multisigN int) ([]UTXOWithPath, api.Satoshi, error) {
	var totalOutput api.Satoshi
	for _, output := range outputs {
		totalOutput += output.Amount
	}

	sortedUTXOs := make([]UTXOWithPath, len(available))
	copy(sortedUTXOs, available)
	sort.Slice(sortedUTXOs, func(i, j int) bool {
		return sortedUTXOs[i].Value > sortedUTXOs[j].Value
	})

	dustLimit := DustLimitSats

	var selected []UTXOWithPath
	var totalInput api.Satoshi

	for _, utxo := range sortedUTXOs {
		selected = append(selected, utxo)
		totalInput += utxo.Value

		vSizeWithChange := EstimateVSize(len(selected), len(outputs)+1, inputType, outputType, multisigM, multisigN)
		feeWithChange := api.Satoshi(math.Ceil(float64(vSizeWithChange) * feeRate))
		changeAmount := totalInput - totalOutput - feeWithChange

		if changeAmount >= dustLimit {
			return selected, totalInput, nil
		}

		vSizeNoChange := EstimateVSize(len(selected), len(outputs), inputType, outputType, multisigM, multisigN)
		feeNoChange := api.Satoshi(math.Ceil(float64(vSizeNoChange) * feeRate))

		if totalInput >= totalOutput+feeNoChange {
			return selected, totalInput, nil
		}
	}

	numOutputs := len(outputs)
	if totalInput-totalOutput >= dustLimit {
		numOutputs++
	}
	finalVSize := EstimateVSize(len(selected), numOutputs, inputType, outputType, multisigM, multisigN)
	finalFee := api.Satoshi(math.Ceil(float64(finalVSize) * feeRate))
	needed := totalOutput + finalFee

	return nil, 0, fmt.Errorf("insufficient funds: have %d sats, need %d sats (output: %d + fee: %d)",
		totalInput, needed, totalOutput, finalFee)
}

// ValidateBitcoinAddress validates a Bitcoin address
func ValidateBitcoinAddress(address string, testnet bool) error {
	var network *chaincfg.Params
	if testnet {
		network = &TestNet4Params
	} else {
		network = &chaincfg.MainNetParams
	}

	addr, err := btcutil.DecodeAddress(address, network)
	if err != nil {
		return fmt.Errorf("invalid Bitcoin address: %w", err)
	}
	if !addr.IsForNet(network) {
		return fmt.Errorf("invalid Bitcoin address: not a %s address", network.Name)
	}
	return nil
}
