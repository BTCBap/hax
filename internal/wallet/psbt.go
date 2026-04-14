package wallet

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"hax/internal/api"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// psbtDebugEnabled is set once at startup based on DEBUG env var
var psbtDebugEnabled = len(os.Getenv("DEBUG")) > 0

// psbtDebugLog logs only when DEBUG env var is set
func psbtDebugLog(format string, args ...interface{}) {
	if psbtDebugEnabled {
		log.Printf(format, args...)
	}
}

// Signing error types for user-friendly error messages
var (
	// ErrKeyMismatch indicates the wallet key doesn't match the PSBT input
	ErrKeyMismatch = errors.New("key mismatch")

	// ErrNoLocalKey indicates no local signing key is available for this input
	ErrNoLocalKey = errors.New("no local signing key")

	// ErrAlreadySigned indicates this input is already signed by this key
	ErrAlreadySigned = errors.New("already signed")

	// ErrInvalidDerivationPath indicates the derivation path in PSBT is invalid
	ErrInvalidDerivationPath = errors.New("invalid derivation path")

	// ErrMissingWitnessUtxo indicates the PSBT is missing required UTXO data
	ErrMissingWitnessUtxo = errors.New("missing witness UTXO")

	// ErrEmptyPSBT indicates the PSBT has no inputs
	ErrEmptyPSBT = errors.New("empty PSBT")

	// ErrWalletMismatch indicates the PSBT is for a different wallet
	ErrWalletMismatch = errors.New("wallet mismatch")

	// ErrScriptTypeMismatch indicates the PSBT uses a different script type than the wallet
	ErrScriptTypeMismatch = errors.New("script type mismatch")

	// ErrWalletLocked indicates the wallet must be unlocked before signing
	ErrWalletLocked = errors.New("wallet locked")

	// ErrMissingWitnessScript indicates a multisig input is missing its witness script
	ErrMissingWitnessScript = errors.New("missing witness script")

	// ErrInsufficientSigs indicates not enough signatures to finalize the PSBT
	ErrInsufficientSigs = errors.New("insufficient signatures")

	// ErrSigningFailed indicates the cryptographic signing operation failed
	ErrSigningFailed = errors.New("signing failed")

	// ErrDerivationFailed indicates key derivation failed
	ErrDerivationFailed = errors.New("key derivation failed")
)

// SigningError wraps a signing error with additional context for user-friendly display
type SigningError struct {
	Err      error  // Underlying error type (ErrKeyMismatch, etc.)
	InputIdx int    // Which input failed (-1 if N/A)
	Details  string // Technical details for debugging
	Hint     string // User-friendly suggestion
}

func (e *SigningError) Error() string {
	if e.Hint != "" {
		return e.Hint
	}
	if e.InputIdx >= 0 {
		return fmt.Sprintf("input %d: %s", e.InputIdx, e.Err.Error())
	}
	return e.Err.Error()
}

func (e *SigningError) Unwrap() error {
	return e.Err
}

// newSigningError creates a new signing error with user-friendly messaging
func newSigningError(err error, inputIdx int, details string) *SigningError {
	se := &SigningError{
		Err:      err,
		InputIdx: inputIdx,
		Details:  details,
	}

	// Set user-friendly hint based on error type
	switch {
	case errors.Is(err, ErrKeyMismatch):
		se.Hint = "Key mismatch: Your wallet key doesn't match this PSBT. This transaction may be for a different wallet."
	case errors.Is(err, ErrNoLocalKey):
		se.Hint = "No local signing key available. Import the cosigner seed phrase or use a hardware wallet to sign."
	case errors.Is(err, ErrAlreadySigned):
		se.Hint = "Already signed: This input is already signed by your key. No additional signature needed."
	case errors.Is(err, ErrInvalidDerivationPath):
		se.Hint = "Invalid derivation path in PSBT. The transaction may have been created with incompatible software."
	case errors.Is(err, ErrMissingWitnessUtxo):
		se.Hint = "Missing UTXO data in PSBT. The PSBT may be incomplete or corrupted."
	case errors.Is(err, ErrEmptyPSBT):
		se.Hint = "Empty PSBT: No inputs to sign."
	case errors.Is(err, ErrWalletMismatch):
		se.Hint = "Wrong wallet: This PSBT was created for a different wallet. The master fingerprint doesn't match."
	case errors.Is(err, ErrScriptTypeMismatch):
		se.Hint = "Script type mismatch: This PSBT uses a different address type than your wallet (e.g., Taproot vs Native SegWit)."
	case errors.Is(err, ErrWalletLocked):
		se.Hint = "Wallet is locked. Enter your password to unlock before signing."
	case errors.Is(err, ErrMissingWitnessScript):
		se.Hint = "Missing witness script for multisig input. The PSBT may be incomplete — re-export from the coordinator."
	case errors.Is(err, ErrInsufficientSigs):
		se.Hint = "Not enough signatures to broadcast. Collect more cosigner signatures before finalizing."
	case errors.Is(err, ErrSigningFailed):
		se.Hint = "Signing failed due to a cryptographic error. The PSBT data may be corrupted."
	case errors.Is(err, ErrDerivationFailed):
		se.Hint = "Failed to derive signing key. The wallet seed may not match this PSBT."
	}

	return se
}

// PSBTResult contains the result of creating a PSBT
type PSBTResult struct {
	Packet        *psbt.Packet
	Fee           api.Satoshi
	Change        api.Satoshi
	HasChange     bool
	SelectedUTXOs []UTXOWithPath
	ChangeAddress string
	VSize         int

	// Dust tracking - for user warnings
	DustAbsorbed api.Satoshi // Amount of dust absorbed into fee (0 if change was created)
	IntendedFee  api.Satoshi // Fee before dust absorption (same as Fee if no dust)
}

// TotalInput returns the sum of all selected UTXO values.
func (r *PSBTResult) TotalInput() api.Satoshi {
	var total api.Satoshi
	for _, u := range r.SelectedUTXOs {
		total += u.Value
	}
	return total
}

// CosignerSigningStatus represents the signing status of a single cosigner
type CosignerSigningStatus struct {
	Name        string
	Fingerprint string
	HasSigned   bool
	CanSign     bool // Has local mnemonic
}

// CountPSBTSigners returns the number of unique signers in a PSBT
// Counts PartialSigs for SegWit/Legacy, TaprootKeySpendSig for Taproot
// Also checks FinalScriptWitness for already-finalized inputs
func CountPSBTSigners(packet *psbt.Packet) int {
	if packet == nil || len(packet.Inputs) == 0 {
		return 0
	}

	// Check first input to determine signature type and count
	firstInput := packet.Inputs[0]

	// For multisig (SegWit/Legacy), return count of partial sigs
	if len(firstInput.PartialSigs) >= 1 {
		return len(firstInput.PartialSigs)
	}

	// For Taproot key-path spend, check TaprootKeySpendSig
	if len(firstInput.TaprootKeySpendSig) > 0 {
		return 1
	}

	// Check if already finalized (signature moved to FinalScriptWitness)
	if len(firstInput.FinalScriptWitness) > 0 {
		return 1
	}

	// Check all inputs for single-sig scenarios
	signedInputs := 0
	for _, input := range packet.Inputs {
		if len(input.TaprootKeySpendSig) > 0 ||
			len(input.PartialSigs) > 0 ||
			len(input.FinalScriptWitness) > 0 {
			signedInputs++
		}
	}

	// Single-sig: all inputs signed = 1 signer
	if signedInputs == len(packet.Inputs) {
		return 1
	}
	return 0
}

// GetCosignerSigningStatus analyzes a PSBT and returns the signing status of each cosigner
// This checks the partial signatures in the PSBT inputs against each cosigner's derived pubkey
func GetCosignerSigningStatus(
	packet *psbt.Packet,
	cosigners []MultisigCosignerConfig,
	utxos []UTXOWithPath,
	network *chaincfg.Params,
) []CosignerSigningStatus {
	status := make([]CosignerSigningStatus, len(cosigners))

	for i, cs := range cosigners {
		status[i] = CosignerSigningStatus{
			Name:        cs.Name,
			Fingerprint: cs.Fingerprint,
			CanSign:     cs.IsMine && cs.Mnemonic != "",
		}

		if len(packet.Inputs) == 0 {
			continue
		}

		// Check if this cosigner has signed any input
		// We only need to check the first input since all inputs in a multisig
		// transaction are typically signed together
		firstInput := packet.Inputs[0]

		if len(utxos) > 0 {
			// Original path: derive pubkey from xpub using UTXO path info
			change, index, err := parsePathChangeIndex(utxos[0].Path)
			if err != nil {
				continue
			}

			// Derive pubkey from xpub
			pubKey, err := DeriveChildPubKey(cs.Xpub, change, index, network)
			if err != nil {
				continue
			}

			// Check if this pubkey has a signature in the PSBT
			for _, partialSig := range firstInput.PartialSigs {
				if bytes.Equal(partialSig.PubKey, pubKey) {
					status[i].HasSigned = true
					break
				}
			}
		} else {
			// Imported PSBT path: use Bip32Derivation from PSBT to match cosigner by fingerprint
			// Parse cosigner fingerprint (hex string like "c6f69ea5")
			csFingerprint := strings.ToLower(cs.Fingerprint)

			// Look through Bip32Derivation entries to find this cosigner's pubkey
			for _, deriv := range firstInput.Bip32Derivation {
				// MasterKeyFingerprint is uint32 - convert to hex string
				// Try both byte orderings since different tools may store differently
				fp := deriv.MasterKeyFingerprint

				// Big-endian style (what you'd expect reading hex left to right)
				derivFPBE := fmt.Sprintf("%02x%02x%02x%02x",
					byte(fp>>24), byte(fp>>16), byte(fp>>8), byte(fp))

				// Little-endian style (how Go stores uint32 in memory)
				derivFPLE := fmt.Sprintf("%02x%02x%02x%02x",
					byte(fp), byte(fp>>8), byte(fp>>16), byte(fp>>24))

				// Also standard %08x format
				derivFPHex := fmt.Sprintf("%08x", fp)

				if derivFPBE == csFingerprint || derivFPLE == csFingerprint || derivFPHex == csFingerprint {
					// Found this cosigner's pubkey in the PSBT
					// Now check if this pubkey has a signature
					for _, partialSig := range firstInput.PartialSigs {
						if bytes.Equal(partialSig.PubKey, deriv.PubKey) {
							status[i].HasSigned = true
							break
						}
					}
					break
				}
			}
		}
	}

	return status
}

// MultisigPSBTInput contains additional info needed for multisig PSBT inputs
type MultisigPSBTInput struct {
	WitnessScript []byte            // The actual multisig redeem script
	Bip32Derivs   []Bip32Derivation // BIP32 derivation info for each cosigner key
}

// Bip32Derivation contains BIP32 derivation info for a public key
type Bip32Derivation struct {
	PubKey      []byte   // 33-byte compressed public key
	Fingerprint [4]byte  // Master key fingerprint
	Path        []uint32 // Derivation path indices
}

// CreatePSBT creates a PSBT from the given inputs and outputs
// This is the proper BIP174 way to construct transactions
// For multisig, pass witnessScripts (one per input) to include in PSBT
// inputScriptType: the script type of inputs being spent (for accurate fee estimation)
// multisigM, multisigN: M-of-N parameters for multisig (0, 0 for single-sig)
func CreatePSBT(
	utxos []UTXOWithPath,
	outputs []TxOutput,
	changeAddr string,
	feeRate float64,
	network *chaincfg.Params,
	witnessScripts [][]byte, // Optional: witness scripts for multisig inputs (nil for single-sig)
	inputScriptType ScriptType, // Script type for fee estimation
	multisigM, multisigN int, // M-of-N for multisig (0, 0 for single-sig)
) (*PSBTResult, error) {
	if len(utxos) == 0 {
		return nil, fmt.Errorf("no UTXOs provided")
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("no outputs provided")
	}

	// Calculate totals
	var totalInput api.Satoshi
	for _, utxo := range utxos {
		totalInput += utxo.Value
	}

	var totalOutput api.Satoshi
	for _, out := range outputs {
		totalOutput += out.Amount
	}

	// Estimate vSize for fee calculation using script-type-aware estimation
	// Use math.Ceil to support fractional sat/vB rates
	numOutputs := len(outputs) + 1 // +1 for potential change
	vSize := EstimateVSize(len(utxos), numOutputs, inputScriptType, inputScriptType, multisigM, multisigN)
	fee := api.Satoshi(math.Ceil(float64(vSize) * feeRate))
	intendedFee := fee // Track the original calculated fee

	// Get dust limit based on change address type
	dustLimit := GetDustLimit(changeAddr)

	// Check if change would be dust
	changeAmount := totalInput - totalOutput - fee
	hasChange := changeAmount >= dustLimit
	var dustAbsorbed api.Satoshi

	if !hasChange && changeAmount > 0 {
		// Change is below dust limit - absorb into fee
		dustAbsorbed = changeAmount

		// Recalculate without change output (smaller tx = lower base fee)
		numOutputs = len(outputs)
		vSize = EstimateVSize(len(utxos), numOutputs, inputScriptType, inputScriptType, multisigM, multisigN)
		fee = api.Satoshi(math.Ceil(float64(vSize) * feeRate))

		// Add the dust to the fee (this is what actually happens)
		fee = totalInput - totalOutput
		changeAmount = 0
	}

	// Validate sufficient funds
	if totalInput < totalOutput+fee {
		return nil, fmt.Errorf("insufficient funds: have %d sats, need %d sats",
			totalInput, totalOutput+fee)
	}

	// Create the unsigned transaction (version 2 for modern transactions)
	unsignedTx := wire.NewMsgTx(2)

	// Add inputs
	for _, utxo := range utxos {
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, fmt.Errorf("invalid txid %s: %w", utxo.TxID, err)
		}
		outPoint := wire.NewOutPoint(txHash, utxo.Vout)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = 0xfffffffd // Enable RBF
		unsignedTx.AddTxIn(txIn)
	}

	// Add recipient outputs
	for _, out := range outputs {
		addr, err := btcutil.DecodeAddress(out.Address, network)
		if err != nil {
			return nil, fmt.Errorf("invalid address %s: %w", out.Address, err)
		}
		pkScript, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create script: %w", err)
		}
		unsignedTx.AddTxOut(wire.NewTxOut(out.Amount.Int64(), pkScript))
	}

	// Add change output if needed
	if hasChange {
		changeAddrDecoded, err := btcutil.DecodeAddress(changeAddr, network)
		if err != nil {
			return nil, fmt.Errorf("invalid change address: %w", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddrDecoded)
		if err != nil {
			return nil, fmt.Errorf("failed to create change script: %w", err)
		}
		unsignedTx.AddTxOut(wire.NewTxOut(changeAmount.Int64(), changePkScript))
	}

	// Create PSBT from unsigned transaction
	packet, err := psbt.NewFromUnsignedTx(unsignedTx)
	if err != nil {
		return nil, fmt.Errorf("failed to create PSBT: %w", err)
	}

	// Add input metadata (witness UTXO for SegWit inputs)
	for i, utxo := range utxos {
		// Decode the address to get the script
		addr, err := btcutil.DecodeAddress(utxo.Address, network)
		if err != nil {
			return nil, fmt.Errorf("invalid UTXO address: %w", err)
		}
		pkScript, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create UTXO script: %w", err)
		}

		psbtDebugLog("[DEBUG CreatePSBT] Input %d:", i)
		psbtDebugLog("  UTXO TxID: %s:%d", utxo.TxID, utxo.Vout)
		psbtDebugLog("  UTXO Address: %s", utxo.Address)
		psbtDebugLog("  UTXO Path: %s", utxo.Path)
		psbtDebugLog("  PkScript: %x", pkScript)

		packet.Inputs[i].WitnessUtxo = &wire.TxOut{
			Value:    utxo.Value.Int64(),
			PkScript: pkScript,
		}

		// P2SH-P2WPKH (BIP49): redeemScript is set during signing, not here
		if _, ok := addr.(*btcutil.AddressScriptHash); ok {
			if strings.Contains(utxo.Path, "49'") || strings.Contains(utxo.Path, "49h") {
			}
		}

		// BIP174: Legacy P2SH uses RedeemScript, SegWit P2WSH/P2SH-P2WSH uses WitnessScript
		if witnessScripts != nil && i < len(witnessScripts) && len(witnessScripts[i]) > 0 {
			if inputScriptType == ScriptTypeP2SH {
				packet.Inputs[i].RedeemScript = witnessScripts[i]
			} else {
				packet.Inputs[i].WitnessScript = witnessScripts[i]
				// P2SH-P2WSH also needs RedeemScript for finalization
				if inputScriptType == ScriptTypeP2SHP2WSH {
					witnessHash := sha256.Sum256(witnessScripts[i])
					redeemScript, _ := txscript.NewScriptBuilder().
						AddOp(txscript.OP_0).
						AddData(witnessHash[:]).
						Script()
					packet.Inputs[i].RedeemScript = redeemScript
				}
			}
		}

		// Set sighash type to SIGHASH_ALL (standard)
		sighashAll := txscript.SigHashAll
		packet.Inputs[i].SighashType = sighashAll
	}

	return &PSBTResult{
		Packet:        packet,
		Fee:           fee,
		Change:        changeAmount,
		HasChange:     hasChange,
		SelectedUTXOs: utxos,
		ChangeAddress: changeAddr,
		VSize:         vSize,
		DustAbsorbed:  dustAbsorbed,
		IntendedFee:   intendedFee,
	}, nil
}

// SignPSBT signs all inputs in the PSBT that we have keys for
func SignPSBT(packet *psbt.Packet, wallet *HDWallet, utxos []UTXOWithPath, network *chaincfg.Params) error {
	psbtDebugLog("[DEBUG SignPSBT] Starting to sign %d inputs", len(utxos))

	updater, err := psbt.NewUpdater(packet)
	if err != nil {
		return fmt.Errorf("failed to create PSBT updater: %w", err)
	}

	for i, utxo := range utxos {
		path, err := ParseBIP44Path(utxo.Path)
		if err != nil {
			return newSigningError(ErrInvalidDerivationPath, i,
				fmt.Sprintf("path %q: %v", utxo.Path, err))
		}

		extKey, err := wallet.DeriveKey(path)
		if err != nil {
			return newSigningError(ErrDerivationFailed, i,
				fmt.Sprintf("path %s: %v", utxo.Path, err))
		}

		privKey, err := extKey.ECPrivKey()
		if err != nil {
			return newSigningError(ErrDerivationFailed, i,
				fmt.Sprintf("failed to extract private key: %v", err))
		}

		pubKey := privKey.PubKey().SerializeCompressed()
		witnessUtxo := packet.Inputs[i].WitnessUtxo
		if witnessUtxo == nil {
			return newSigningError(ErrMissingWitnessUtxo, i, "WitnessUtxo is nil")
		}

		isBIP44 := path.Purpose == 44 // P2PKH
		isBIP49 := path.Purpose == 49 // P2SH-P2WPKH
		isBIP86 := path.Purpose == 86 // P2TR

		psbtDebugLog("[DEBUG SignPSBT] Input %d: path=%s, purpose=%d, isBIP86=%v",
			i, utxo.Path, path.Purpose, isBIP86)
		psbtDebugLog("  UTXO address: %s", utxo.Address)
		psbtDebugLog("  WitnessUtxo.PkScript: %x", witnessUtxo.PkScript)

		if isBIP86 {
			if err := signTaprootInput(packet, i, privKey, witnessUtxo); err != nil {
				return err
			}
		} else if isBIP44 {
			if err := signLegacyInput(packet, i, privKey, pubKey, witnessUtxo, network); err != nil {
				return err
			}
		} else if isBIP49 {
			// P2SH-P2WPKH: redeemScript must be set before signing
			pubKeyHash := btcutil.Hash160(pubKey)

			redeemScript, err := txscript.NewScriptBuilder().
				AddOp(txscript.OP_0).
				AddData(pubKeyHash).
				Script()
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("failed to create redeemScript: %v", err))
			}

			// Verify derived key matches the UTXO being spent
			p2shAddr, err := btcutil.NewAddressScriptHash(redeemScript, network)
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("failed to create P2SH address for verification: %v", err))
			}
			expectedScript, err := txscript.PayToAddrScript(p2shAddr)
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("failed to create expected script: %v", err))
			}

			if !bytes.Equal(expectedScript, witnessUtxo.PkScript) {
				return newSigningError(ErrKeyMismatch, i,
					fmt.Sprintf("derived key (path %s) produces address %s, but UTXO has different address", utxo.Path, p2shAddr.EncodeAddress()))
			}

			packet.Inputs[i].RedeemScript = redeemScript

			// BIP143: scriptCode for P2SH-P2WPKH uses P2PKH form, not the redeemScript
			scriptCode, err := txscript.NewScriptBuilder().
				AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).
				AddData(pubKeyHash).
				AddOp(txscript.OP_EQUALVERIFY).
				AddOp(txscript.OP_CHECKSIG).
				Script()
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("failed to create BIP143 scriptCode: %v", err))
			}

			prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
				witnessUtxo.PkScript,
				witnessUtxo.Value,
			)
			sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutputFetcher)

			sig, err := txscript.RawTxInWitnessSignature(
				packet.UnsignedTx,
				sigHashes,
				i,
				witnessUtxo.Value,
				scriptCode,
				txscript.SigHashAll,
				privKey,
			)
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("BIP49 witness signature failed: %v", err))
			}

			packet.Inputs[i].PartialSigs = append(packet.Inputs[i].PartialSigs, &psbt.PartialSig{
				PubKey:    pubKey,
				Signature: sig,
			})
		} else {
			// Native SegWit (P2WPKH)
			prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
				witnessUtxo.PkScript,
				witnessUtxo.Value,
			)
			sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutputFetcher)

			sig, err := txscript.RawTxInWitnessSignature(
				packet.UnsignedTx,
				sigHashes,
				i,
				witnessUtxo.Value,
				witnessUtxo.PkScript,
				txscript.SigHashAll,
				privKey,
			)
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("SegWit witness signature failed: %v", err))
			}

			_, err = updater.Sign(i, sig, pubKey, nil, nil)
			if err != nil {
				return newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("failed to add signature to PSBT: %v", err))
			}
		}
	}

	return nil
}

// SignPSBTFromDerivation signs a PSBT using the derivation paths embedded in the PSBT itself.
// Used for imported PSBTs where we don't have SelectedUTXOs.
func SignPSBTFromDerivation(packet *psbt.Packet, wallet *HDWallet, network *chaincfg.Params) error {
	if packet == nil || len(packet.Inputs) == 0 {
		return newSigningError(ErrEmptyPSBT, -1, "packet is nil or has no inputs")
	}

	psbtDebugLog("[DEBUG SignPSBTFromDerivation] Signing %d inputs from PSBT derivation data", len(packet.Inputs))

	for i := range packet.Inputs {
		input := &packet.Inputs[i]

		if len(input.TaprootKeySpendSig) > 0 || len(input.FinalScriptWitness) > 0 {
			psbtDebugLog("[DEBUG] Input %d already signed, skipping", i)
			continue
		}
		if len(input.PartialSigs) > 0 {
			psbtDebugLog("[DEBUG] Input %d already has partial sigs, skipping", i)
			continue
		}

		// WitnessUtxo first, fall back to NonWitnessUtxo for Legacy inputs
		witnessUtxo := input.WitnessUtxo
		if witnessUtxo == nil && input.NonWitnessUtxo != nil && i < len(packet.UnsignedTx.TxIn) {
			prevOutIdx := packet.UnsignedTx.TxIn[i].PreviousOutPoint.Index
			if int(prevOutIdx) < len(input.NonWitnessUtxo.TxOut) {
				witnessUtxo = input.NonWitnessUtxo.TxOut[prevOutIdx]
			}
		}
		if witnessUtxo == nil {
			return newSigningError(ErrMissingWitnessUtxo, i, "no UTXO data available (neither WitnessUtxo nor NonWitnessUtxo)")
		}

		// Try Taproot first (TaprootBip32Derivation)
		if len(input.TaprootBip32Derivation) > 0 {
			psbtDebugLog("[DEBUG] Input %d: Found %d TaprootBip32Derivation entries",
				i, len(input.TaprootBip32Derivation))

			for _, deriv := range input.TaprootBip32Derivation {
				// deriv.XOnlyPubKey is the 32-byte x-only pubkey
				// deriv.MasterKeyFingerprint is 4 bytes
				// deriv.Bip32Path is []uint32 derivation path

				psbtDebugLog("[DEBUG]   XOnlyPubKey: %x", deriv.XOnlyPubKey)
				psbtDebugLog("[DEBUG]   Fingerprint: %x", deriv.MasterKeyFingerprint)
				psbtDebugLog("[DEBUG]   Path: %v", deriv.Bip32Path)

				// Convert BIP32 path to our format
				// Path is like [2147483734, 2147483649, 2147483648, 0, 11]
				// which represents m/86'/1'/0'/0/11
				if len(deriv.Bip32Path) < 5 {
					psbtDebugLog("[DEBUG]   Path too short, skipping")
					continue
				}

				purpose := deriv.Bip32Path[0] & 0x7FFFFFFF
				coinType := deriv.Bip32Path[1] & 0x7FFFFFFF
				account := deriv.Bip32Path[2] & 0x7FFFFFFF
				change := deriv.Bip32Path[3]
				index := deriv.Bip32Path[4]

				psbtDebugLog("[DEBUG]   Parsed: purpose=%d, coinType=%d, account=%d, change=%d, index=%d",
					purpose, coinType, account, change, index)

				path := BIP44Path{
					Purpose:      purpose,
					CoinType:     coinType,
					Account:      account,
					Change:       change,
					AddressIndex: index,
				}

				extKey, err := wallet.DeriveKey(path)
				if err != nil {
					psbtDebugLog("[DEBUG]   Failed to derive key: %v", err)
					continue
				}

				privKey, err := extKey.ECPrivKey()
				if err != nil {
					psbtDebugLog("[DEBUG]   Failed to get private key: %v", err)
					continue
				}

				ourXOnlyPubKey := schnorr.SerializePubKey(privKey.PubKey())
				if !bytes.Equal(ourXOnlyPubKey, deriv.XOnlyPubKey) {
					psbtDebugLog("[DEBUG]   Pubkey mismatch: ours=%x, theirs=%x", ourXOnlyPubKey, deriv.XOnlyPubKey)
					continue
				}

				psbtDebugLog("[DEBUG]   Pubkey matches! Signing input %d", i)

				err = signTaprootInput(packet, i, privKey, witnessUtxo)
				if err != nil {
					return err
				}

				psbtDebugLog("[DEBUG]   Successfully signed input %d", i)
				break // Signed this input, move to next
			}
			continue
		}

		if len(input.Bip32Derivation) > 0 {
			psbtDebugLog("[DEBUG] Input %d: Found %d Bip32Derivation entries", i, len(input.Bip32Derivation))

			for _, deriv := range input.Bip32Derivation {
				psbtDebugLog("[DEBUG]   PubKey: %x", deriv.PubKey)
				psbtDebugLog("[DEBUG]   Fingerprint: %x", deriv.MasterKeyFingerprint)
				psbtDebugLog("[DEBUG]   Path: %v", deriv.Bip32Path)

				if len(deriv.Bip32Path) < 5 {
					psbtDebugLog("[DEBUG]   Path too short, skipping")
					continue
				}

				purpose := deriv.Bip32Path[0] & 0x7FFFFFFF
				coinType := deriv.Bip32Path[1] & 0x7FFFFFFF
				account := deriv.Bip32Path[2] & 0x7FFFFFFF
				change := deriv.Bip32Path[3]
				index := deriv.Bip32Path[4]

				path := BIP44Path{
					Purpose:      purpose,
					CoinType:     coinType,
					Account:      account,
					Change:       change,
					AddressIndex: index,
				}

				extKey, err := wallet.DeriveKey(path)
				if err != nil {
					psbtDebugLog("[DEBUG]   Failed to derive key: %v", err)
					continue
				}

				privKey, err := extKey.ECPrivKey()
				if err != nil {
					psbtDebugLog("[DEBUG]   Failed to get private key: %v", err)
					continue
				}

				ourPubKey := privKey.PubKey().SerializeCompressed()
				if !bytes.Equal(ourPubKey, deriv.PubKey) {
					psbtDebugLog("[DEBUG]   Pubkey mismatch")
					continue
				}

				psbtDebugLog("[DEBUG]   Pubkey matches! Signing input %d", i)

				if purpose == 86 {
					err = signTaprootInput(packet, i, privKey, witnessUtxo)
				} else if purpose == 44 {
					err = signLegacyInput(packet, i, privKey, ourPubKey, witnessUtxo, network)
				} else {
					err = signSegWitInput(packet, i, privKey, ourPubKey, witnessUtxo, purpose == 49)
				}

				if err != nil {
					return err
				}

				psbtDebugLog("[DEBUG]   Successfully signed input %d", i)
				break
			}
		}
	}

	return nil
}

// signSegWitInput signs a SegWit (P2WPKH or P2SH-P2WPKH) input
func signSegWitInput(packet *psbt.Packet, inputIndex int, privKey *btcec.PrivateKey, pubKey []byte, witnessUtxo *wire.TxOut, isNested bool) error {
	updater, err := psbt.NewUpdater(packet)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("failed to create PSBT updater: %v", err))
	}

	if isNested {
		// P2SH-P2WPKH: Need redeemScript
		pubKeyHash := btcutil.Hash160(pubKey)
		redeemScript, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(pubKeyHash).
			Script()
		if err != nil {
			return newSigningError(ErrSigningFailed, inputIndex,
				fmt.Sprintf("failed to create redeemScript: %v", err))
		}
		packet.Inputs[inputIndex].RedeemScript = redeemScript
	}

	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
		witnessUtxo.PkScript,
		witnessUtxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutputFetcher)

	sig, err := txscript.RawTxInWitnessSignature(
		packet.UnsignedTx,
		sigHashes,
		inputIndex,
		witnessUtxo.Value,
		witnessUtxo.PkScript,
		txscript.SigHashAll,
		privKey,
	)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("SegWit witness signature failed: %v", err))
	}

	_, err = updater.Sign(inputIndex, sig, pubKey, nil, nil)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("failed to add signature to PSBT: %v", err))
	}
	return nil
}

// SignMultisigPSBTFromDerivation signs a multisig PSBT using derivation paths from PSBT fields.
// Used for imported PSBTs where we don't have SelectedUTXOs.
func SignMultisigPSBTFromDerivation(
	packet *psbt.Packet,
	cosigners []MultisigCosignerConfig,
	scriptType MultisigScriptType,
	network *chaincfg.Params,
) (int, error) {
	if packet == nil || len(packet.Inputs) == 0 {
		return 0, newSigningError(ErrEmptyPSBT, -1, "packet is nil or has no inputs")
	}

	updater, err := psbt.NewUpdater(packet)
	if err != nil {
		return 0, fmt.Errorf("failed to create PSBT updater: %w", err)
	}

	sigsAdded := 0
	isTestnet := network.Net != chaincfg.MainNetParams.Net
	isLegacyP2SH := scriptType == MultisigLegacy

	// Determine M from multisig script (RedeemScript for Legacy, WitnessScript for SegWit)
	requiredSigs := 0
	if len(packet.Inputs) > 0 {
		multisigScript := packet.Inputs[0].WitnessScript
		if len(multisigScript) == 0 {
			multisigScript = packet.Inputs[0].RedeemScript
		}
		if len(multisigScript) > 0 {
			firstByte := multisigScript[0]
			if firstByte >= OP1 && firstByte <= OP16 {
				requiredSigs = int(firstByte - OPSmallIntBase)
			}
		}
	}

	for _, cosigner := range cosigners {
		// Stop once we have enough signatures
		if requiredSigs > 0 && len(packet.Inputs) > 0 {
			currentSigs := len(packet.Inputs[0].PartialSigs)
			if currentSigs >= requiredSigs {
				break
			}
		}

		if !cosigner.IsMine || cosigner.Mnemonic == "" {
			continue
		}

		psbtDebugLog("[DEBUG] Trying cosigner %s", cosigner.Fingerprint)

		seed, err := MnemonicToSeed(cosigner.Mnemonic, "")
		if err != nil {
			return sigsAdded, fmt.Errorf("failed to derive seed: %w", err)
		}

		hdWallet, err := NewHDWallet(seed, isTestnet)
		if err != nil {
			return sigsAdded, fmt.Errorf("failed to create HD wallet: %w", err)
		}

		for i := range packet.Inputs {
			input := &packet.Inputs[i]

			multisigScript := input.WitnessScript
			if len(multisigScript) == 0 {
				multisigScript = input.RedeemScript
			}
			if len(multisigScript) == 0 {
				psbtDebugLog("[DEBUG] Input %d: no multisig script", i)
				continue
			}

			witnessUtxo := input.WitnessUtxo
			if witnessUtxo == nil {
				psbtDebugLog("[DEBUG] Input %d: no witness UTXO", i)
				continue
			}

			// Match by fingerprint, try multiple byte orderings for tool compatibility
			for _, deriv := range input.Bip32Derivation {
				csFingerprint := strings.ToLower(cosigner.Fingerprint)
				fp := deriv.MasterKeyFingerprint
				derivFPBE := fmt.Sprintf("%02x%02x%02x%02x", byte(fp>>24), byte(fp>>16), byte(fp>>8), byte(fp))
				derivFPLE := fmt.Sprintf("%02x%02x%02x%02x", byte(fp), byte(fp>>8), byte(fp>>16), byte(fp>>24))
				derivFPHex := fmt.Sprintf("%08x", fp)

				if derivFPBE != csFingerprint && derivFPLE != csFingerprint && derivFPHex != csFingerprint {
					continue
				}

				// Full path: m/48'/coin'/account'/script'/change/index; Relative: change/index
				var change, index uint32

				if len(deriv.Bip32Path) >= 6 {
					change = deriv.Bip32Path[4]
					index = deriv.Bip32Path[5]
				} else if len(deriv.Bip32Path) >= 2 {
					change = deriv.Bip32Path[0]
					index = deriv.Bip32Path[1]
				} else if len(deriv.Bip32Path) == 1 {
					change = 0
					index = deriv.Bip32Path[0]
				} else {
					continue
				}

				coinType := uint32(1)
				if !isTestnet {
					coinType = 0
				}

				bip48Path := BIP48Path{
					CoinType:   coinType,
					Account:    0,
					ScriptType: scriptType,
				}

				extKey, err := hdWallet.DeriveBIP48Key(bip48Path, change, index)
				if err != nil {
					continue
				}

				privKey, err := extKey.ECPrivKey()
				if err != nil {
					continue
				}

				ourPubKey := privKey.PubKey().SerializeCompressed()
				if !bytes.Equal(ourPubKey, deriv.PubKey) {
					continue
				}

				alreadySigned := false
				for _, sig := range input.PartialSigs {
					if bytes.Equal(sig.PubKey, ourPubKey) {
						alreadySigned = true
						break
					}
				}
				if alreadySigned {
					continue
				}

				psbtDebugLog("[DEBUG]   Pubkey matches! Signing input %d", i)

				var signedSig []byte
				if isLegacyP2SH {
					// Legacy P2SH: use non-witness signature
					signedSig, err = txscript.RawTxInSignature(
						packet.UnsignedTx,
						i,
						multisigScript,
						txscript.SigHashAll,
						privKey,
					)
				} else {
					// SegWit: use witness signature
					prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
						witnessUtxo.PkScript,
						witnessUtxo.Value,
					)
					sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutputFetcher)

					signedSig, err = txscript.RawTxInWitnessSignature(
						packet.UnsignedTx,
						sigHashes,
						i,
						witnessUtxo.Value,
						multisigScript,
						txscript.SigHashAll,
						privKey,
					)
				}
				if err != nil {
					psbtDebugLog("[DEBUG]   Signing failed: %v", err)
					continue
				}

				if isLegacyP2SH {
					// For Legacy P2SH, add PartialSig directly — btcd's updater.Sign
					// validates against WitnessUtxo which fails for non-witness signatures
					packet.Inputs[i].PartialSigs = append(packet.Inputs[i].PartialSigs, &psbt.PartialSig{
						PubKey:    ourPubKey,
						Signature: signedSig,
					})
				} else {
					_, err = updater.Sign(i, signedSig, ourPubKey, nil, nil)
					if err != nil {
						psbtDebugLog("[DEBUG]   Adding sig failed: %v", err)
						continue
					}
				}

				sigsAdded++
				psbtDebugLog("[DEBUG]   Successfully signed input %d", i)
				break // Move to next input
			}
		}
	}

	return sigsAdded, nil
}

// signTaprootInput signs a Taproot (P2TR) input using BIP340 Schnorr key-path spend.
func signTaprootInput(packet *psbt.Packet, inputIndex int, privKey *btcec.PrivateKey, witnessUtxo *wire.TxOut) error {
	pubKey := privKey.PubKey()

	// Normalize internal key to even Y per BIP340
	xOnlyBytes := schnorr.SerializePubKey(pubKey)
	internalKey, err := schnorr.ParsePubKey(xOnlyBytes)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("failed to parse internal key: %v", err))
	}

	taprootKey := txscript.ComputeTaprootKeyNoScript(internalKey)
	taprootPkScript, err := txscript.PayToTaprootScript(taprootKey)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("failed to create Taproot pkScript: %v", err))
	}

	if !bytes.Equal(taprootPkScript, witnessUtxo.PkScript) {
		return newSigningError(ErrKeyMismatch, inputIndex,
			fmt.Sprintf("taproot pkScript %x doesn't match UTXO %x", taprootPkScript, witnessUtxo.PkScript))
	}

	// Taproot sighash requires prevouts for all inputs
	prevOuts := make(map[wire.OutPoint]*wire.TxOut)
	for i, input := range packet.UnsignedTx.TxIn {
		if packet.Inputs[i].WitnessUtxo != nil {
			prevOuts[input.PreviousOutPoint] = packet.Inputs[i].WitnessUtxo
		}
	}
	prevOutputFetcher := txscript.NewMultiPrevOutFetcher(prevOuts)

	sig, err := txscript.RawTxInTaprootSignature(
		packet.UnsignedTx,
		txscript.NewTxSigHashes(packet.UnsignedTx, prevOutputFetcher),
		inputIndex,
		witnessUtxo.Value,
		taprootPkScript,
		nil, // nil TapLeaf = key-path spend (BIP86)
		txscript.SigHashDefault,
		privKey,
	)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("Taproot Schnorr signature failed: %v", err))
	}

	packet.Inputs[inputIndex].TaprootKeySpendSig = sig
	packet.Inputs[inputIndex].TaprootInternalKey = xOnlyBytes

	// CRITICAL: For Taproot with SIGHASH_DEFAULT, clear the SighashType field.
	// Per BIP-371: "The PSBT_IN_SIGHASH_TYPE field MUST be filled if using any
	// sighash type other than SIGHASH_DEFAULT (0x00). Otherwise, it may be omitted."
	// Setting it to 0 or leaving it unset indicates SIGHASH_DEFAULT was used.
	// This is required for Sparrow to correctly verify the 64-byte signature.
	packet.Inputs[inputIndex].SighashType = 0

	return nil
}

// signLegacyInput signs a Legacy (P2PKH) input using ECDSA.
func signLegacyInput(packet *psbt.Packet, inputIndex int, privKey *btcec.PrivateKey, pubKey []byte, witnessUtxo *wire.TxOut, network *chaincfg.Params) error {
	pubKeyHash := btcutil.Hash160(pubKey)
	p2pkhAddr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, network)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("failed to create P2PKH address: %v", err))
	}

	expectedScript, err := txscript.PayToAddrScript(p2pkhAddr)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("failed to create expected P2PKH script: %v", err))
	}

	if !bytes.Equal(expectedScript, witnessUtxo.PkScript) {
		return newSigningError(ErrKeyMismatch, inputIndex,
			fmt.Sprintf("derived P2PKH key produces address %s but UTXO has different address", p2pkhAddr.EncodeAddress()))
	}

	sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, txscript.NewCannedPrevOutputFetcher(
		witnessUtxo.PkScript,
		witnessUtxo.Value,
	))

	sig, err := txscript.RawTxInSignature(
		packet.UnsignedTx,
		inputIndex,
		witnessUtxo.PkScript,
		txscript.SigHashAll,
		privKey,
	)
	if err != nil {
		return newSigningError(ErrSigningFailed, inputIndex,
			fmt.Sprintf("Legacy ECDSA signature failed: %v", err))
	}

	_ = sigHashes

	packet.Inputs[inputIndex].PartialSigs = append(packet.Inputs[inputIndex].PartialSigs, &psbt.PartialSig{
		PubKey:    pubKey,
		Signature: sig,
	})

	return nil
}

// SignMultisigPSBT signs a PSBT for multisig inputs using cosigners that have local keys.
// Stops after M signatures are collected.
func SignMultisigPSBT(
	packet *psbt.Packet,
	cosigners []MultisigCosignerConfig,
	utxos []UTXOWithPath,
	network *chaincfg.Params,
	scriptType MultisigScriptType,
) (int, error) {
	updater, err := psbt.NewUpdater(packet)
	if err != nil {
		return 0, newSigningError(ErrSigningFailed, -1,
			fmt.Sprintf("failed to create PSBT updater: %v", err))
	}

	sigsAdded := 0
	isTestnet := network.Net != chaincfg.MainNetParams.Net
	isLegacyP2SH := scriptType == MultisigLegacy

	// Determine M from multisig script (RedeemScript for Legacy, WitnessScript for SegWit)
	requiredSigs := 0
	if len(packet.Inputs) > 0 {
		multisigScript := packet.Inputs[0].WitnessScript
		if len(multisigScript) == 0 {
			multisigScript = packet.Inputs[0].RedeemScript
		}
		if len(multisigScript) > 0 {
			firstByte := multisigScript[0]
			if firstByte >= OP1 && firstByte <= OP16 {
				requiredSigs = int(firstByte - OPSmallIntBase)
			}
		}
	}

	for _, cosigner := range cosigners {
		if requiredSigs > 0 && len(packet.Inputs) > 0 {
			if len(packet.Inputs[0].PartialSigs) >= requiredSigs {
				break
			}
		}

		if !cosigner.IsMine || cosigner.Mnemonic == "" {
			continue
		}

		seed, err := MnemonicToSeed(cosigner.Mnemonic, "")
		if err != nil {
			return sigsAdded, newSigningError(ErrDerivationFailed, -1,
				fmt.Sprintf("cosigner %s: failed to derive seed: %v", cosigner.Fingerprint, err))
		}

		hdWallet, err := NewHDWallet(seed, isTestnet)
		if err != nil {
			return sigsAdded, newSigningError(ErrDerivationFailed, -1,
				fmt.Sprintf("cosigner %s: failed to create HD wallet: %v", cosigner.Fingerprint, err))
		}

		for i, utxo := range utxos {
			change, index, err := parsePathChangeIndex(utxo.Path)
			if err != nil {
				return sigsAdded, newSigningError(ErrInvalidDerivationPath, i,
					fmt.Sprintf("path %q: %v", utxo.Path, err))
			}

			// Resolve actual derivation path (handles BIP84 xpubs pasted into BIP48 multisig)
			coinType := uint32(0)
			if isTestnet {
				coinType = 1
			}

			var extKey *hdkeychain.ExtendedKey
			actualPath := cosignerDerivationPath(cosigner, coinType, scriptType)

			if strings.Contains(actualPath, "/48'/") {
				bip48Path := BIP48Path{
					CoinType:   coinType,
					Account:    0,
					ScriptType: scriptType,
				}
				extKey, err = hdWallet.DeriveBIP48Key(bip48Path, change, index)
			} else if strings.Contains(actualPath, "/84'/") {
				path := SegWitNativePath(isTestnet, 0, change, index)
				extKey, err = hdWallet.DeriveKey(path)
			} else if strings.Contains(actualPath, "/49'/") {
				path := SegWitNestedPath(isTestnet, 0, change, index)
				extKey, err = hdWallet.DeriveKey(path)
			} else if strings.Contains(actualPath, "/44'/") {
				path := StandardBIP44Path(isTestnet, 0, change, index)
				extKey, err = hdWallet.DeriveKey(path)
			} else {
				bip48Path := BIP48Path{
					CoinType:   coinType,
					Account:    0,
					ScriptType: scriptType,
				}
				extKey, err = hdWallet.DeriveBIP48Key(bip48Path, change, index)
			}

			if err != nil {
				return sigsAdded, newSigningError(ErrDerivationFailed, i,
					fmt.Sprintf("path %s, cosigner %s: %v", actualPath, cosigner.Fingerprint, err))
			}

			privKey, err := extKey.ECPrivKey()
			if err != nil {
				return sigsAdded, newSigningError(ErrDerivationFailed, i,
					fmt.Sprintf("failed to extract private key: %v", err))
			}

			multisigScript := packet.Inputs[i].WitnessScript
			if len(multisigScript) == 0 {
				multisigScript = packet.Inputs[i].RedeemScript
			}
			if len(multisigScript) == 0 {
				return sigsAdded, newSigningError(ErrMissingWitnessScript, i, "no multisig script (neither WitnessScript nor RedeemScript)")
			}

			witnessUtxo := packet.Inputs[i].WitnessUtxo
			if witnessUtxo == nil {
				return sigsAdded, newSigningError(ErrMissingWitnessUtxo, i, "WitnessUtxo is nil")
			}

			pubKey := privKey.PubKey().SerializeCompressed()

			alreadySigned := false
			for _, existingSig := range packet.Inputs[i].PartialSigs {
				if bytes.Equal(existingSig.PubKey, pubKey) {
					alreadySigned = true
					break
				}
			}

			if alreadySigned {
				continue
			}

			var sig []byte
			if isLegacyP2SH {
				sig, err = txscript.RawTxInSignature(
					packet.UnsignedTx,
					i,
					multisigScript,
					txscript.SigHashAll,
					privKey,
				)
			} else {
				prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
					witnessUtxo.PkScript,
					witnessUtxo.Value,
				)
				sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutputFetcher)

				sig, err = txscript.RawTxInWitnessSignature(
					packet.UnsignedTx,
					sigHashes,
					i,
					witnessUtxo.Value,
					multisigScript,
					txscript.SigHashAll,
					privKey,
				)
			}
			if err != nil {
				return sigsAdded, newSigningError(ErrSigningFailed, i,
					fmt.Sprintf("multisig signature failed: %v", err))
			}

			if isLegacyP2SH {
				// For Legacy P2SH, add PartialSig directly — btcd's updater.Sign
				// validates against WitnessUtxo which fails for non-witness signatures
				packet.Inputs[i].PartialSigs = append(packet.Inputs[i].PartialSigs, &psbt.PartialSig{
					PubKey:    pubKey,
					Signature: sig,
				})
			} else {
				_, err = updater.Sign(i, sig, pubKey, nil, nil)
				if err != nil {
					return sigsAdded, newSigningError(ErrSigningFailed, i,
						fmt.Sprintf("failed to add multisig signature to PSBT: %v", err))
				}
			}

			sigsAdded++
		}
	}

	return sigsAdded, nil
}

// parseBIP48PathComponents extracts change and index from a BIP48 path
// Path format: m/48'/coin'/account'/script'/change/index
func parseBIP48PathComponents(path string) (change uint32, index uint32, err error) {
	path = strings.TrimPrefix(path, "m/")

	rawParts := strings.Split(path, "/")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		if p != "" {
			parts = append(parts, p)
		}
	}

	if len(parts) < 6 {
		return 0, 0, fmt.Errorf("invalid BIP48 path: expected 6 parts, got %d", len(parts))
	}

	changeStr := trimHardened(parts[len(parts)-2])
	var changeVal uint64
	_, err = fmt.Sscanf(changeStr, "%d", &changeVal)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid change value: %s", parts[len(parts)-2])
	}

	indexStr := parts[len(parts)-1]
	indexStr = trimHardened(indexStr)
	var indexVal uint64
	_, err = fmt.Sscanf(indexStr, "%d", &indexVal)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid index value: %s", parts[len(parts)-1])
	}

	return uint32(changeVal), uint32(indexVal), nil
}

// parsePathChangeIndex extracts change and index from any derivation path format.
// Takes the last two components as change and index.
func parsePathChangeIndex(path string) (change uint32, index uint32, err error) {
	path = strings.TrimPrefix(path, "m/")
	parts := strings.Split(path, "/")

	var cleanParts []string
	for _, p := range parts {
		if p != "" {
			cleanParts = append(cleanParts, p)
		}
	}

	if len(cleanParts) < 2 {
		return 0, 0, fmt.Errorf("path too short: need at least change/index, got %d parts", len(cleanParts))
	}

	changeStr := trimHardened(cleanParts[len(cleanParts)-2])
	var changeVal uint64
	_, err = fmt.Sscanf(changeStr, "%d", &changeVal)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid change value: %s", cleanParts[len(cleanParts)-2])
	}

	indexStr := trimHardened(cleanParts[len(cleanParts)-1])
	var indexVal uint64
	_, err = fmt.Sscanf(indexStr, "%d", &indexVal)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid index value: %s", cleanParts[len(cleanParts)-1])
	}

	return uint32(changeVal), uint32(indexVal), nil
}

// FinalizePSBT finalizes a fully-signed PSBT and extracts the transaction.
func FinalizePSBT(packet *psbt.Packet) (*wire.MsgTx, error) {
	for i := range packet.Inputs {
		input := &packet.Inputs[i]

		if len(input.FinalScriptWitness) > 0 || len(input.FinalScriptSig) > 0 {
			continue
		}

		if len(input.TaprootKeySpendSig) > 0 {
			if err := finalizeTaprootKeyPath(packet, i); err != nil {
				return nil, fmt.Errorf("failed to finalize Taproot input %d: %w", i, err)
			}
		} else if len(input.WitnessScript) > 0 && len(input.PartialSigs) > 0 {
			if err := finalizeP2WSHMultisig(packet, i); err != nil {
				return nil, fmt.Errorf("failed to finalize P2WSH multisig input %d: %w", i, err)
			}
		} else if len(input.RedeemScript) > 0 && len(input.PartialSigs) > 0 && isMultisigScript(input.RedeemScript) {
			if err := finalizeLegacyP2SHMultisig(packet, i); err != nil {
				return nil, fmt.Errorf("failed to finalize Legacy P2SH multisig input %d: %w", i, err)
			}
		} else if len(input.PartialSigs) > 0 && input.WitnessUtxo != nil && txscript.IsPayToPubKeyHash(input.WitnessUtxo.PkScript) {
			// btcd's psbt.Finalize mishandles P2PKH when WitnessUtxo is set
			if err := finalizeLegacyP2PKH(packet, i); err != nil {
				return nil, fmt.Errorf("failed to finalize Legacy P2PKH input %d: %w", i, err)
			}
		} else {
			if err := psbt.Finalize(packet, i); err != nil {
				return nil, fmt.Errorf("failed to finalize input %d: %w", i, err)
			}
		}
	}

	finalTx, err := psbt.Extract(packet)
	if err != nil {
		return nil, fmt.Errorf("failed to extract transaction: %w", err)
	}

	return finalTx, nil
}

// finalizeTaprootKeyPath finalizes a Taproot key-path spend input
// The witness for key-path spend is just: [signature]
// Signature is 64 bytes for SIGHASH_DEFAULT, or 65 bytes with explicit sighash type
func finalizeTaprootKeyPath(packet *psbt.Packet, inputIndex int) error {
	input := &packet.Inputs[inputIndex]

	if len(input.TaprootKeySpendSig) == 0 {
		return fmt.Errorf("missing Taproot key spend signature")
	}

	witness := wire.TxWitness{input.TaprootKeySpendSig}

	var witnessBytes []byte
	witnessBytes = append(witnessBytes, byte(len(witness))) // Number of stack items
	for _, item := range witness {
		witnessBytes = append(witnessBytes, byte(len(item))) // Length of item
		witnessBytes = append(witnessBytes, item...)         // Item data
	}
	input.FinalScriptWitness = witnessBytes

	input.TaprootKeySpendSig = nil
	input.TaprootInternalKey = nil
	input.SighashType = 0

	return nil
}

// finalizeP2WSHMultisig finalizes a P2WSH multisig input
// The witness stack for P2WSH multisig is: [empty] [sig1] [sig2] ... [sigM] [witnessScript]
// Reference: BIP141 (SegWit), BIP143 (SegWit sighash)
func finalizeP2WSHMultisig(packet *psbt.Packet, inputIndex int) error {
	input := &packet.Inputs[inputIndex]

	if len(input.WitnessScript) == 0 {
		return fmt.Errorf("missing witness script")
	}
	if len(input.PartialSigs) == 0 {
		return fmt.Errorf("no signatures")
	}

	// OP_M <pubkey1> ... <pubkeyN> OP_N OP_CHECKMULTISIG
	witnessScript := input.WitnessScript
	if len(witnessScript) < 37 {
		return fmt.Errorf("witness script too short: %d bytes", len(witnessScript))
	}

	if witnessScript[len(witnessScript)-1] != OPCheckMultiSig {
		return fmt.Errorf("witness script doesn't end with OP_CHECKMULTISIG")
	}

	firstByte := witnessScript[0]
	if firstByte < OP1 || firstByte > OP16 {
		return fmt.Errorf("invalid OP_M in witness script: 0x%02x", firstByte)
	}
	m := int(firstByte - OPSmallIntBase)

	nByte := witnessScript[len(witnessScript)-2]
	if nByte < OP1 || nByte > OP16 {
		return fmt.Errorf("invalid OP_N in witness script: 0x%02x", nByte)
	}
	n := int(nByte - OPSmallIntBase)

	if m > n {
		return fmt.Errorf("invalid multisig: M(%d) > N(%d)", m, n)
	}

	var pubkeysInScript [][]byte
	pos := 1

	for len(pubkeysInScript) < n && pos < len(witnessScript)-2 {
		if witnessScript[pos] == OPPushBytes33 { // Push 33 bytes (compressed pubkey)
			if pos+34 > len(witnessScript)-2 { // Ensure room for pubkey + OP_N + OP_CHECKMULTISIG
				return fmt.Errorf("witness script truncated at pubkey %d", len(pubkeysInScript)+1)
			}
			pubkey := make([]byte, 33)
			copy(pubkey, witnessScript[pos+1:pos+34])
			pubkeysInScript = append(pubkeysInScript, pubkey)
			pos += 34 // 1 (length byte) + 33 (pubkey)
		} else {
			return fmt.Errorf("unexpected byte 0x%02x at position %d (expected OPPushBytes33 for pubkey)", witnessScript[pos], pos)
		}
	}

	if len(pubkeysInScript) != n {
		return fmt.Errorf("found %d pubkeys but N=%d", len(pubkeysInScript), n)
	}

	if len(input.PartialSigs) < m {
		return newSigningError(ErrInsufficientSigs, inputIndex,
			fmt.Sprintf("have %d of %d required signatures", len(input.PartialSigs), m))
	}

	// Signatures must match pubkey order in the script
	var witness wire.TxWitness
	witness = append(witness, []byte{}) // OP_CHECKMULTISIG off-by-one dummy element

	sigsAdded := 0
	for _, scriptPubkey := range pubkeysInScript {
		if sigsAdded >= m {
			break
		}
		for _, partialSig := range input.PartialSigs {
			if bytes.Equal(partialSig.PubKey, scriptPubkey) {
				witness = append(witness, partialSig.Signature)
				sigsAdded++
				break
			}
		}
	}

	if sigsAdded < m {
		return newSigningError(ErrInsufficientSigs, inputIndex,
			fmt.Sprintf("found %d matching signatures but need %d", sigsAdded, m))
	}

	// Add witness script at the end
	witness = append(witness, witnessScript)

	// Serialize witness for PSBT FinalScriptWitness field
	// Format: [varint count] [varint len1] [elem1] [varint len2] [elem2] ...
	input.FinalScriptWitness = serializeWitness(witness)

	// P2SH-P2WSH: also set FinalScriptSig to push the P2WSH redeemScript
	// For P2SH-wrapped SegWit, the scriptSig contains a single push of the witness program
	if len(input.RedeemScript) > 0 {
		builder := txscript.NewScriptBuilder()
		builder.AddData(input.RedeemScript)
		scriptSig, err := builder.Script()
		if err != nil {
			return fmt.Errorf("failed to build P2SH-P2WSH scriptSig: %w", err)
		}
		input.FinalScriptSig = scriptSig
	}

	// Clear partial data (standard PSBT practice after finalization)
	input.PartialSigs = nil
	input.SighashType = 0
	input.WitnessScript = nil
	input.RedeemScript = nil
	input.Bip32Derivation = nil

	return nil
}

// finalizeLegacyP2PKH finalizes a Legacy P2PKH single-sig input
// The scriptSig for P2PKH is: <sig> <pubkey>
// No witness data is needed for Legacy inputs
func finalizeLegacyP2PKH(packet *psbt.Packet, inputIndex int) error {
	input := &packet.Inputs[inputIndex]

	if len(input.PartialSigs) == 0 {
		return fmt.Errorf("no signatures")
	}

	// For single-sig P2PKH, there should be exactly one PartialSig
	partialSig := input.PartialSigs[0]

	// Build scriptSig: <sig> <pubkey>
	builder := txscript.NewScriptBuilder()
	builder.AddData(partialSig.Signature)
	builder.AddData(partialSig.PubKey)

	scriptSig, err := builder.Script()
	if err != nil {
		return fmt.Errorf("failed to build scriptSig: %w", err)
	}

	input.FinalScriptSig = scriptSig

	// Clear partial data
	input.PartialSigs = nil
	input.SighashType = 0
	input.Bip32Derivation = nil

	return nil
}

// isMultisigScript checks if a script is a multisig script (OP_M ... OP_N OP_CHECKMULTISIG)
func isMultisigScript(script []byte) bool {
	if len(script) < 37 { // Minimum: OP_1 [33-byte key] OP_1 OP_CHECKMULTISIG
		return false
	}
	// Last byte must be OP_CHECKMULTISIG
	if script[len(script)-1] != OPCheckMultiSig {
		return false
	}
	// First byte must be OP_1..OP_16
	if script[0] < OP1 || script[0] > OP16 {
		return false
	}
	return true
}

// finalizeLegacyP2SHMultisig finalizes a Legacy P2SH multisig input
// The scriptSig for P2SH multisig is: OP_0 <sig1> <sig2> ... <sigM> <redeemScript>
// Unlike P2WSH, there is no witness data — everything goes in scriptSig
func finalizeLegacyP2SHMultisig(packet *psbt.Packet, inputIndex int) error {
	input := &packet.Inputs[inputIndex]

	redeemScript := input.RedeemScript
	if len(redeemScript) == 0 {
		return fmt.Errorf("missing redeem script")
	}
	if len(input.PartialSigs) == 0 {
		return fmt.Errorf("no signatures")
	}

	// Parse the redeemScript to get M (required sigs) and pubkeys
	// Format: OP_M <pubkey1> ... <pubkeyN> OP_N OP_CHECKMULTISIG
	if len(redeemScript) < 37 {
		return fmt.Errorf("redeem script too short: %d bytes", len(redeemScript))
	}

	if redeemScript[len(redeemScript)-1] != OPCheckMultiSig {
		return fmt.Errorf("redeem script doesn't end with OP_CHECKMULTISIG")
	}

	firstByte := redeemScript[0]
	if firstByte < OP1 || firstByte > OP16 {
		return fmt.Errorf("invalid OP_M in redeem script: 0x%02x", firstByte)
	}
	m := int(firstByte - OPSmallIntBase)

	nByte := redeemScript[len(redeemScript)-2]
	if nByte < OP1 || nByte > OP16 {
		return fmt.Errorf("invalid OP_N in redeem script: 0x%02x", nByte)
	}
	n := int(nByte - OPSmallIntBase)

	if m > n {
		return fmt.Errorf("invalid multisig: M(%d) > N(%d)", m, n)
	}

	// Extract pubkeys from redeem script to determine signature order
	var pubkeysInScript [][]byte
	pos := 1 // Start after OP_M

	for len(pubkeysInScript) < n && pos < len(redeemScript)-2 {
		if redeemScript[pos] == OPPushBytes33 {
			if pos+34 > len(redeemScript)-2 {
				return fmt.Errorf("redeem script truncated at pubkey %d", len(pubkeysInScript)+1)
			}
			pubkey := make([]byte, 33)
			copy(pubkey, redeemScript[pos+1:pos+34])
			pubkeysInScript = append(pubkeysInScript, pubkey)
			pos += 34
		} else {
			return fmt.Errorf("unexpected byte 0x%02x at position %d", redeemScript[pos], pos)
		}
	}

	if len(pubkeysInScript) != n {
		return fmt.Errorf("found %d pubkeys but N=%d", len(pubkeysInScript), n)
	}

	if len(input.PartialSigs) < m {
		return fmt.Errorf("not enough partial signatures: have %d, need %d", len(input.PartialSigs), m)
	}

	// Build scriptSig: OP_0 <sig1> <sig2> ... <sigM> <redeemScript>
	// Signatures must be in the same order as their pubkeys appear in the script
	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_FALSE) // OP_0 for CHECKMULTISIG off-by-one bug

	sigsAdded := 0
	for _, scriptPubkey := range pubkeysInScript {
		if sigsAdded >= m {
			break
		}

		for _, partialSig := range input.PartialSigs {
			if bytes.Equal(partialSig.PubKey, scriptPubkey) {
				builder.AddData(partialSig.Signature)
				sigsAdded++
				break
			}
		}
	}

	if sigsAdded < m {
		return fmt.Errorf("could not find %d valid signatures (found %d)", m, sigsAdded)
	}

	// Push the redeemScript as the last element
	builder.AddData(redeemScript)

	scriptSig, err := builder.Script()
	if err != nil {
		return fmt.Errorf("failed to build scriptSig: %w", err)
	}

	input.FinalScriptSig = scriptSig

	// Clear partial data (standard PSBT practice after finalization)
	input.PartialSigs = nil
	input.SighashType = 0
	input.RedeemScript = nil
	input.Bip32Derivation = nil

	return nil
}

// serializeWitness serializes a witness stack for PSBT FinalScriptWitness
func serializeWitness(witness wire.TxWitness) []byte {
	var buf bytes.Buffer

	// Write number of witness elements as varint
	wire.WriteVarInt(&buf, 0, uint64(len(witness)))

	// Write each element with length prefix
	for _, elem := range witness {
		wire.WriteVarBytes(&buf, 0, elem)
	}

	return buf.Bytes()
}

// ExportPSBT exports a PSBT to base64 string (standard format)
func ExportPSBT(packet *psbt.Packet) (string, error) {
	var buf bytes.Buffer
	if err := packet.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize PSBT: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// ImportPSBT imports a PSBT from base64 string
func ImportPSBT(data string) (*psbt.Packet, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}

	packet, err := psbt.NewFromRawBytes(bytes.NewReader(decoded), false)
	if err != nil {
		return nil, fmt.Errorf("invalid PSBT: %w", err)
	}

	return packet, nil
}

// SerializeTx serializes a transaction to hex string for broadcasting
func SerializeTx(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}
	return fmt.Sprintf("%x", buf.Bytes()), nil
}

// GetPSBTVSize returns the virtual size of the transaction in the PSBT
// It detects the script type from PSBT data for accurate estimation
func GetPSBTVSize(packet *psbt.Packet) int {
	numInputs := len(packet.UnsignedTx.TxIn)
	numOutputs := len(packet.UnsignedTx.TxOut)

	// Detect script type from PSBT wallet info
	info := GetPSBTWalletInfo(packet)
	st, m, n := psbtInfoToScriptParams(info)

	return EstimateVSize(numInputs, numOutputs, st, st, m, n)
}

// psbtInfoToScriptParams converts PSBTWalletInfo to script type parameters
func psbtInfoToScriptParams(info PSBTWalletInfo) (ScriptType, int, int) {
	if info.IsMultisig {
		m := info.RequiredSigs
		n := info.TotalCosigners
		if m == 0 {
			m = 2 // Default to 2-of-3 if not detected
		}
		if n == 0 {
			n = 3
		}

		switch info.ScriptType {
		case "multisig_nested":
			return ScriptTypeP2SHP2WSH, m, n
		case "multisig", "multisig_native":
			return ScriptTypeP2WSH, m, n
		default:
			return ScriptTypeP2WSH, m, n
		}
	}

	// Single-sig
	switch info.ScriptType {
	case "taproot":
		return ScriptTypeP2TR, 0, 0
	case "legacy":
		return ScriptTypeP2PKH, 0, 0
	case "nested_segwit":
		return ScriptTypeP2SHP2WPKH, 0, 0
	case "native_segwit":
		return ScriptTypeP2WPKH, 0, 0
	default:
		return ScriptTypeP2WPKH, 0, 0
	}
}

// ValidatePSBT checks if a PSBT is valid and ready for signing/finalizing
func ValidatePSBT(packet *psbt.Packet) error {
	if err := packet.SanityCheck(); err != nil {
		return fmt.Errorf("PSBT sanity check failed: %w", err)
	}
	return nil
}

// PSBTWalletInfo contains information extracted from a PSBT about which wallet it belongs to
type PSBTWalletInfo struct {
	Fingerprints   []string // Unique master fingerprints found in PSBT
	ScriptType     string   // Detected script type: "taproot", "native_segwit", "nested_segwit", "legacy", "multisig", "unknown"
	IsMultisig     bool     // True if PSBT appears to be for a multisig wallet
	RequiredSigs   int      // For multisig: required signatures (M in M-of-N)
	TotalCosigners int      // For multisig: total cosigners (N in M-of-N)
}

// GetPSBTWalletInfo extracts wallet information from a PSBT
// This helps determine which wallet the PSBT belongs to
func GetPSBTWalletInfo(packet *psbt.Packet) PSBTWalletInfo {
	info := PSBTWalletInfo{
		Fingerprints: []string{},
		ScriptType:   "unknown",
	}

	if packet == nil || len(packet.Inputs) == 0 {
		return info
	}

	fingerprintSet := make(map[string]bool)
	var detectedPurpose uint32

	for _, input := range packet.Inputs {
		// Check Taproot derivation paths
		for _, deriv := range input.TaprootBip32Derivation {
			fp := fmt.Sprintf("%08x", deriv.MasterKeyFingerprint)
			fingerprintSet[fp] = true
			if len(deriv.Bip32Path) >= 1 {
				purpose := deriv.Bip32Path[0] & 0x7FFFFFFF
				if purpose == 86 {
					info.ScriptType = "taproot"
					detectedPurpose = purpose
				}
			}
		}

		// Check standard BIP32 derivation paths
		for _, deriv := range input.Bip32Derivation {
			fp := fmt.Sprintf("%08x", deriv.MasterKeyFingerprint)
			fingerprintSet[fp] = true
			if len(deriv.Bip32Path) >= 1 {
				purpose := deriv.Bip32Path[0] & 0x7FFFFFFF
				switch purpose {
				case 44:
					if info.ScriptType == "unknown" {
						info.ScriptType = "legacy"
						detectedPurpose = purpose
					}
				case 49:
					if info.ScriptType == "unknown" {
						info.ScriptType = "nested_segwit"
						detectedPurpose = purpose
					}
				case 84:
					if info.ScriptType == "unknown" {
						info.ScriptType = "native_segwit"
						detectedPurpose = purpose
					}
				case 48:
					// BIP48 multisig
					info.IsMultisig = true
					if len(deriv.Bip32Path) >= 4 {
						scriptType := deriv.Bip32Path[3] & 0x7FFFFFFF
						switch scriptType {
						case 1:
							info.ScriptType = "multisig_nested"
						case 2:
							info.ScriptType = "multisig_native"
						default:
							info.ScriptType = "multisig"
						}
					}
				}
			}
		}

		// Check witness script for multisig info
		if len(input.WitnessScript) > 0 {
			firstByte := input.WitnessScript[0]
			if firstByte >= 0x51 && firstByte <= 0x60 { // OP_1 to OP_16
				info.IsMultisig = true
				info.RequiredSigs = int(firstByte - 0x50)
				// Count pubkeys in witness script
				lastByte := input.WitnessScript[len(input.WitnessScript)-2]
				if lastByte >= 0x51 && lastByte <= 0x60 {
					info.TotalCosigners = int(lastByte - 0x50)
				}
			}
		}
	}

	// Convert fingerprint set to slice
	for fp := range fingerprintSet {
		info.Fingerprints = append(info.Fingerprints, fp)
	}

	_ = detectedPurpose // Suppress unused warning

	return info
}

// ValidatePSBTForWallet checks if a PSBT belongs to the specified wallet
// Returns nil if valid, or an error explaining the mismatch
func ValidatePSBTForWallet(packet *psbt.Packet, walletFingerprint string, walletScriptType string, isMultisig bool) error {
	if packet == nil || len(packet.Inputs) == 0 {
		return newSigningError(ErrEmptyPSBT, -1, "no inputs in PSBT")
	}

	info := GetPSBTWalletInfo(packet)

	// Check if PSBT is multisig vs single-sig
	if info.IsMultisig && !isMultisig {
		return newSigningError(ErrWalletMismatch, -1,
			fmt.Sprintf("PSBT is for a %d-of-%d multisig wallet, but current wallet is single-sig",
				info.RequiredSigs, info.TotalCosigners))
	}
	if !info.IsMultisig && isMultisig {
		return newSigningError(ErrWalletMismatch, -1,
			"PSBT is for a single-sig wallet, but current wallet is multisig")
	}

	// Check fingerprint match (if we have fingerprints in the PSBT)
	if len(info.Fingerprints) > 0 && walletFingerprint != "" {
		walletFP := strings.ToLower(walletFingerprint)
		foundMatch := false
		for _, fp := range info.Fingerprints {
			if strings.ToLower(fp) == walletFP {
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return newSigningError(ErrWalletMismatch, -1,
				fmt.Sprintf("PSBT fingerprints %v don't match wallet fingerprint %s",
					info.Fingerprints, walletFingerprint))
		}
	}

	// Check script type match for single-sig wallets
	if !isMultisig && info.ScriptType != "unknown" && walletScriptType != "" {
		psbtType := info.ScriptType
		walletType := strings.ToLower(walletScriptType)

		// Normalize types for comparison
		typeMap := map[string]string{
			"taproot":       "taproot",
			"native_segwit": "native",
			"segwit_native": "native",
			"native":        "native",
			"nested_segwit": "nested",
			"segwit_nested": "nested",
			"nested":        "nested",
			"legacy":        "legacy",
		}

		normalizedPSBT := typeMap[psbtType]
		normalizedWallet := typeMap[walletType]

		if normalizedPSBT != "" && normalizedWallet != "" && normalizedPSBT != normalizedWallet {
			return newSigningError(ErrScriptTypeMismatch, -1,
				fmt.Sprintf("PSBT is for %s addresses, but wallet uses %s addresses",
					psbtType, walletScriptType))
		}
	}

	return nil
}

// IsPSBTComplete checks if all inputs are signed
func IsPSBTComplete(packet *psbt.Packet) bool {
	for _, input := range packet.Inputs {
		// Check for any valid signature type:
		// - PartialSigs: SegWit/Legacy signatures
		// - TaprootKeySpendSig: Taproot key-path spend
		// - FinalScriptWitness: Already finalized
		hasSig := len(input.PartialSigs) > 0 ||
			len(input.TaprootKeySpendSig) > 0 ||
			len(input.FinalScriptWitness) > 0
		if !hasSig {
			return false
		}
	}
	return true
}
