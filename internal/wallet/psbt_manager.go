package wallet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/BTCBap/hax/internal/api"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
)

// fingerprintHexToUint32 converts an 8-char hex fingerprint to uint32.
// Uses little-endian byte order: first hex byte is least significant,
// matching btcd's binary.LittleEndian.PutUint32 PSBT serialization.
func fingerprintHexToUint32(fp string) uint32 {
	fpBytes, err := hex.DecodeString(fp)
	if err != nil || len(fpBytes) < 4 {
		return 0
	}
	return uint32(fpBytes[3])<<24 | uint32(fpBytes[2])<<16 |
		uint32(fpBytes[1])<<8 | uint32(fpBytes[0])
}

// addBip32DerivationToInputs adds BIP32 derivation paths for all cosigner keys to PSBT inputs.
func (m *WalletManager) addBip32DerivationToInputs(packet *psbt.Packet, utxos []UTXOWithPath) error {
	if m.data.Multisig == nil || len(m.data.Multisig.Cosigners) == 0 {
		return nil
	}

	coinType := uint32(1) // testnet
	if m.data.Network == "mainnet" {
		coinType = 0
	}

	for i, utxo := range utxos {
		if i >= len(packet.Inputs) {
			break
		}

		change, index, err := parseMultisigPath(utxo.Path)
		if err != nil {
			continue
		}

		seenPubkeys := make(map[string]bool)

		for _, cosigner := range m.data.Multisig.Cosigners {
			pubKey, err := m.deriveCosignerPubKey(cosigner, change, index)
			if err != nil {
				continue
			}

			pubKeyHex := hex.EncodeToString(pubKey)
			if seenPubkeys[pubKeyHex] {
				continue
			}
			seenPubkeys[pubKeyHex] = true

			derivPath := cosignerDerivationPath(cosigner, coinType, MultisigScriptType(m.data.Multisig.ScriptType))
			basePath, err := parseBip32Path(derivPath)
			if err != nil {
				continue
			}

			fullPath := make([]uint32, len(basePath)+2)
			copy(fullPath, basePath)
			if change {
				fullPath[len(basePath)] = 1
			} else {
				fullPath[len(basePath)] = 0
			}
			fullPath[len(basePath)+1] = index

			fingerprint := fingerprintHexToUint32(cosigner.Fingerprint)

			derivation := &psbt.Bip32Derivation{
				PubKey:               pubKey,
				MasterKeyFingerprint: fingerprint,
				Bip32Path:            fullPath,
			}

			packet.Inputs[i].Bip32Derivation = append(packet.Inputs[i].Bip32Derivation, derivation)
		}
	}

	return nil
}

// addGlobalXpubs populates PSBT_GLOBAL_XPUB entries with account-level xpubs
// for each multisig cosigner. This allows devices like Coldcard to reconstruct
// the M-of-N wallet configuration from the PSBT alone ("Offer Import").
//
// Coldcard trust policy: "Offer Import" only triggers when NO multisig wallets
// exist on the device (TRUST_OFFER). Once any wallet is stored, default policy
// is TRUST_VERIFY (reject unknown). User must set Settings > Multisig Wallets >
// "Allow from PSBT" to re-enable. See shared/multisig.py trust_policy in
// Coldcard firmware: https://github.com/Coldcard/firmware
func (m *WalletManager) addGlobalXpubs(packet *psbt.Packet) error {
	if m.data.Multisig == nil || len(m.data.Multisig.Cosigners) == 0 {
		return nil
	}

	coinType := uint32(1) // testnet
	if m.data.Network == "mainnet" {
		coinType = 0
	}
	scriptType := MultisigScriptType(m.data.Multisig.ScriptType)

	for _, cosigner := range m.data.Multisig.Cosigners {
		key, err := hdkeychain.NewKeyFromString(cosigner.Xpub)
		if err != nil {
			continue
		}

		derivPath := cosignerDerivationPath(cosigner, coinType, scriptType)
		bip32Path, err := parseBip32Path(derivPath)
		if err != nil {
			continue
		}

		packet.XPubs = append(packet.XPubs, psbt.XPub{
			ExtendedKey:          psbt.EncodeExtendedKey(key),
			MasterKeyFingerprint: fingerprintHexToUint32(cosigner.Fingerprint),
			Bip32Path:            bip32Path,
		})
	}

	return nil
}

// addMultisigChangeOutputMetadata adds BIP32 derivation paths and witness/redeem scripts
// to the PSBT change output. This allows hardware wallets and external signers to verify
// that the change address belongs to this multisig wallet (BIP174 PSBT_OUT_BIP32_DERIVATION,
// PSBT_OUT_WITNESS_SCRIPT, PSBT_OUT_REDEEM_SCRIPT).
func (m *WalletManager) addMultisigChangeOutputMetadata(packet *psbt.Packet, changeAddrIndex uint32, changeOutputIdx int) error {
	if m.data.Multisig == nil || len(m.data.Multisig.Cosigners) == 0 {
		return nil
	}

	if changeOutputIdx < 0 || changeOutputIdx >= len(packet.Outputs) {
		return fmt.Errorf("change output index %d out of range", changeOutputIdx)
	}

	coinType := uint32(1) // testnet
	if m.data.Network == "mainnet" {
		coinType = 0
	}

	scriptType := MultisigScriptType(m.data.Multisig.ScriptType)
	output := &packet.Outputs[changeOutputIdx]
	seenPubkeys := make(map[string]bool)

	for _, cosigner := range m.data.Multisig.Cosigners {
		pubKey, err := m.deriveCosignerPubKey(cosigner, true, changeAddrIndex)
		if err != nil {
			continue
		}

		pubKeyHex := hex.EncodeToString(pubKey)
		if seenPubkeys[pubKeyHex] {
			continue
		}
		seenPubkeys[pubKeyHex] = true

		derivPath := cosignerDerivationPath(cosigner, coinType, scriptType)
		basePath, err := parseBip32Path(derivPath)
		if err != nil {
			continue
		}

		// Full path: base path + 1 (change) + index
		fullPath := make([]uint32, len(basePath)+2)
		copy(fullPath, basePath)
		fullPath[len(basePath)] = 1 // change chain
		fullPath[len(basePath)+1] = changeAddrIndex

		fingerprint := fingerprintHexToUint32(cosigner.Fingerprint)

		output.Bip32Derivation = append(output.Bip32Derivation, &psbt.Bip32Derivation{
			PubKey:               pubKey,
			MasterKeyFingerprint: fingerprint,
			Bip32Path:            fullPath,
		})
	}

	// Derive the witness/redeem script for the change address
	_, witnessScript, err := m.GetMultisigAddress(true, changeAddrIndex)
	if err != nil {
		return fmt.Errorf("failed to derive change witness script: %w", err)
	}

	switch scriptType {
	case MultisigNativeSegwit:
		// P2WSH: only witness script needed
		output.WitnessScript = witnessScript
	case MultisigNestedSegwit:
		// P2SH-P2WSH: witness script + redeem script (the P2WSH witness program)
		output.WitnessScript = witnessScript
		witnessHash := sha256.Sum256(witnessScript)
		// RedeemScript = OP_0 <32-byte witness hash>
		output.RedeemScript, _ = txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(witnessHash[:]).
			Script()
	case MultisigLegacy:
		// P2SH: the multisig script is the redeem script
		output.RedeemScript = witnessScript
	}

	return nil
}

// deriveCosignerPubKey derives the public key for a cosigner at the given change/index
func (m *WalletManager) deriveCosignerPubKey(cosigner MultisigCosignerConfig, change bool, index uint32) ([]byte, error) {
	// Parse xpub
	xpub, err := hdkeychain.NewKeyFromString(cosigner.Xpub)
	if err != nil {
		return nil, err
	}

	// Derive change level
	changeIdx := uint32(0)
	if change {
		changeIdx = 1
	}
	changeKey, err := xpub.Derive(changeIdx)
	if err != nil {
		return nil, err
	}

	// Derive index
	indexKey, err := changeKey.Derive(index)
	if err != nil {
		return nil, err
	}

	// Get compressed public key
	pubKey, err := indexKey.ECPubKey()
	if err != nil {
		return nil, err
	}

	return pubKey.SerializeCompressed(), nil
}

// parseBip32Path parses a BIP32 path string into uint32 indices
// Handles both hardened (') and non-hardened indices
func parseBip32Path(path string) ([]uint32, error) {
	// Remove "m/" prefix if present
	path = strings.TrimPrefix(path, "m/")
	if path == "" {
		return nil, nil
	}

	parts := strings.Split(path, "/")
	result := make([]uint32, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Check for hardened marker
		hardened := isHardened(part)
		if hardened {
			part = trimHardened(part)
		}

		// Parse index
		idx, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid path component: %s", part)
		}

		// Apply hardened offset if needed
		if hardened {
			if idx >= 0x80000000 {
				return nil, fmt.Errorf("hardened path component out of range: %s'", part)
			}
			idx += 0x80000000 // Hardened offset
		}

		result = append(result, uint32(idx)) // #nosec G115 -- ParseUint(…, 32) and the check above bound idx to uint32
	}

	return result, nil
}

// parseMultisigPath extracts change (bool) and index from a BIP48 path
// Path format: m/48'/1'/0'/2'/change/index
func parseMultisigPath(path string) (change bool, index uint32, err error) {
	// Remove 'm/' prefix if present
	path = strings.TrimPrefix(path, "m/")

	parts := strings.Split(path, "/")
	if len(parts) < 6 {
		return false, 0, fmt.Errorf("invalid BIP48 path: %s", path)
	}

	// Last two parts are change and index (not hardened)
	changeStr := strings.TrimSuffix(parts[len(parts)-2], "'")
	indexStr := strings.TrimSuffix(parts[len(parts)-1], "'")

	changeVal, err := strconv.ParseUint(changeStr, 10, 32)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse change from path: %w", err)
	}
	// Non-hardened index: 0 .. 2^31-1. Rejects negatives that used to wrap.
	indexVal, err := strconv.ParseUint(indexStr, 10, 31)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse index from path: %w", err)
	}

	return changeVal == 1, uint32(indexVal), nil
}

// normalizeScriptType converts various script type names to a canonical form
func normalizeScriptType(scriptType string) string {
	switch strings.ToLower(scriptType) {
	case "taproot", "p2tr":
		return "taproot"
	case "native_segwit", "segwit_native", "native", "p2wpkh", "bech32":
		return "native"
	case "nested_segwit", "segwit_nested", "nested", "p2sh-p2wpkh":
		return "nested"
	case "legacy", "p2pkh":
		return "legacy"
	case "multisig", "multisig_native", "multisig_nested", "p2wsh", "p2sh":
		return "multisig"
	default:
		return "" // Unknown - don't block on this
	}
}

// SignPSBTTransaction signs all inputs in a PSBT that this wallet has keys for
// Returns the signed PSBT (may be partially signed if wallet doesn't have all keys)
func (m *WalletManager) SignPSBTTransaction(result *PSBTResult) error {
	if !m.unlocked {
		return newSigningError(ErrWalletLocked, -1, "wallet must be unlocked to sign")
	}

	network := m.getNetwork()

	// If we have SelectedUTXOs, use the traditional signing path
	// Otherwise, try to sign using derivation paths embedded in the PSBT
	if len(result.SelectedUTXOs) > 0 {
		return SignPSBT(result.Packet, m.wallet, result.SelectedUTXOs, network)
	}

	// Imported PSBT - use derivation paths from PSBT fields
	return SignPSBTFromDerivation(result.Packet, m.wallet, network)
}

// SignMultisigPSBTTransaction signs a multisig PSBT using local cosigner keys
// Returns the number of signatures added (0 if no local keys available)
func (m *WalletManager) SignMultisigPSBTTransaction(result *PSBTResult) (int, error) {
	if !m.unlocked {
		return 0, newSigningError(ErrWalletLocked, -1, "wallet must be unlocked to sign")
	}

	if !m.IsMultisig() {
		return 0, newSigningError(ErrWalletMismatch, -1, "attempted multisig signing on single-sig wallet")
	}

	config := m.data.Multisig

	network := m.getNetwork()

	// If we have SelectedUTXOs, use the traditional signing path
	// Otherwise, try to sign using derivation paths embedded in the PSBT
	if len(result.SelectedUTXOs) > 0 {
		return SignMultisigPSBT(result.Packet, config.Cosigners, result.SelectedUTXOs, network, MultisigScriptType(config.ScriptType))
	}

	// Imported PSBT - use derivation paths from PSBT fields
	return SignMultisigPSBTFromDerivation(result.Packet, config.Cosigners, MultisigScriptType(config.ScriptType), network)
}

// GetCosignerSigningStatus returns the signing status of each cosigner for a PSBT
// This shows which cosigners have signed and which can still sign
func (m *WalletManager) GetCosignerSigningStatus(result *PSBTResult) []CosignerSigningStatus {
	if !m.unlocked || !m.IsMultisig() || result == nil {
		return nil
	}

	config := m.data.Multisig

	network := m.getNetwork()

	return GetCosignerSigningStatus(result.Packet, config.Cosigners, result.SelectedUTXOs, network)
}

// CanSignMultisigLocally returns true if this multisig wallet has at least one
// cosigner with local signing keys (mnemonic stored)
func (m *WalletManager) CanSignMultisigLocally() bool {
	if !m.unlocked || !m.IsMultisig() {
		return false
	}

	for _, cs := range m.data.Multisig.Cosigners {
		if cs.IsMine && cs.Mnemonic != "" {
			return true
		}
	}
	return false
}

// GetMultisigSigningStatus returns info about which cosigners can sign
// Returns (localSigners, hardwareSigners, externalSigners count)
func (m *WalletManager) GetMultisigSigningStatus() (local, hardware, external int) {
	if !m.unlocked || !m.IsMultisig() {
		return 0, 0, 0
	}

	for _, cs := range m.data.Multisig.Cosigners {
		switch {
		case cs.IsMine && cs.Mnemonic != "":
			local++
		case cs.DeviceType == "coldcard" || cs.DeviceType == "trezor" || cs.DeviceType == "ledger":
			hardware++
		default:
			external++
		}
	}
	return
}

// ExportPSBTBase64 exports a PSBT to base64 string (BIP174 standard format)
// This can be shared with other wallets, hardware devices, or multisig coordinators
func (m *WalletManager) ExportPSBTBase64(result *PSBTResult) (string, error) {
	return ExportPSBT(result.Packet)
}

// ImportPSBTFromBase64 imports a PSBT from base64 string
// Returns the parsed PSBT packet for inspection or further signing
// Validates that the PSBT belongs to the current wallet
func (m *WalletManager) ImportPSBTFromBase64(base64Data string) (*PSBTResult, error) {
	packet, err := ImportPSBT(base64Data)
	if err != nil {
		return nil, err
	}

	// Validate that PSBT belongs to this wallet
	if err := m.validatePSBTOwnership(packet); err != nil {
		return nil, err
	}

	// Calculate fee from PSBT: fee = total_inputs - total_outputs
	// Input values come from WitnessUtxo (for SegWit/Taproot) or NonWitnessUtxo (for legacy)
	var totalInput int64
	for i, input := range packet.Inputs {
		if input.WitnessUtxo != nil {
			totalInput += input.WitnessUtxo.Value
		} else if input.NonWitnessUtxo != nil && i < len(packet.UnsignedTx.TxIn) {
			prevOutIndex := packet.UnsignedTx.TxIn[i].PreviousOutPoint.Index
			if int(prevOutIndex) < len(input.NonWitnessUtxo.TxOut) {
				totalInput += input.NonWitnessUtxo.TxOut[prevOutIndex].Value
			}
		}
	}

	var totalOutput int64
	for _, out := range packet.UnsignedTx.TxOut {
		totalOutput += out.Value
	}

	fee := totalInput - totalOutput
	if fee < 0 {
		fee = 0 // Sanity check - fee shouldn't be negative
	}

	return &PSBTResult{
		Packet: packet,
		Fee:    api.Satoshi(fee),
		VSize:  GetPSBTVSize(packet),
	}, nil
}

// validatePSBTOwnership checks if the PSBT belongs to the current wallet.
// Intentionally permissive — only rejects clear mismatches, lets signing handle ambiguity.
func (m *WalletManager) validatePSBTOwnership(packet *psbt.Packet) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}

	info := GetPSBTWalletInfo(packet)
	isMultisig := m.IsMultisig()

	if info.IsMultisig && !isMultisig {
		return newSigningError(ErrWalletMismatch, -1,
			fmt.Sprintf("PSBT is for a %d-of-%d multisig wallet, but current wallet is single-sig",
				info.RequiredSigs, info.TotalCosigners))
	}

	// For single-sig wallets, check script type mismatch
	// But only if both types are clearly known
	if !isMultisig && info.ScriptType != "unknown" {
		walletAddrType := ""
		if len(m.data.Accounts) > 0 {
			walletAddrType = m.data.Accounts[0].AddressType
		}

		// Normalize and compare script types
		psbtType := normalizeScriptType(info.ScriptType)
		walletType := normalizeScriptType(walletAddrType)

		// Only reject if both types are known AND clearly different
		if psbtType != "" && walletType != "" && psbtType != walletType {
			return newSigningError(ErrScriptTypeMismatch, -1,
				fmt.Sprintf("PSBT is for %s addresses, but wallet uses %s addresses",
					info.ScriptType, walletAddrType))
		}
	}

	// For all other cases, allow the import
	// The signing process will fail naturally if the PSBT doesn't match
	return nil
}

// FinalizePSBTTransaction finalizes a fully-signed PSBT and extracts the broadcast-ready transaction
// Returns the serialized transaction hex ready for broadcast
func (m *WalletManager) FinalizePSBTTransaction(result *PSBTResult) (string, error) {
	if !IsPSBTComplete(result.Packet) {
		return "", newSigningError(ErrInsufficientSigs, -1,
			fmt.Sprintf("PSBT has %d inputs but not all are signed", len(result.Packet.Inputs)))
	}

	finalTx, err := FinalizePSBT(result.Packet)
	if err != nil {
		return "", fmt.Errorf("failed to finalize PSBT: %w", err)
	}

	txHex, err := SerializeTx(finalTx)
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	return txHex, nil
}

// BroadcastPSBT finalizes a PSBT and broadcasts it to the network.
// On success, marks the change address as used to prevent reuse in subsequent
// transactions before Electrum subscriptions update.
// Returns the transaction ID.
func (m *WalletManager) BroadcastPSBT(result *PSBTResult) (string, error) {
	// Finalize to get transaction hex
	txHex, err := m.FinalizePSBTTransaction(result)
	if err != nil {
		return "", err
	}

	// Broadcast
	if err := m.ensureNetworkVerified(); err != nil {
		return "", err
	}
	txID, err := m.backend.BroadcastTransaction(txHex)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	// Mark change address as used immediately to prevent reuse race condition.
	// Without this, a second transaction created before the Electrum subscription
	// callback could reuse the same change address.
	if result.HasChange && result.ChangeAddress != "" {
		m.dataMu.Lock()
		if len(m.data.Accounts) > 0 {
			for i := range m.data.Accounts[0].Addresses {
				if m.data.Accounts[0].Addresses[i].Address == result.ChangeAddress {
					m.data.Accounts[0].Addresses[i].Used = true
					break
				}
			}
		}
		m.dataMu.Unlock()

		m.dataMu.RLock()
		if saveErr := m.saveWalletTracked(m.data.Name, m.data); saveErr != nil {
			log.Printf("[WALLET] BroadcastPSBT: failed to persist change address state: %v", saveErr)
		}
		m.dataMu.RUnlock()
	}

	return txID, nil
}

// GetHWIClient returns an HWI client for hardware wallet interaction.
// Returns nil if HWI is not installed.
func (m *WalletManager) GetHWIClient() *HWIClient {
	isTestnet := m.getNetwork().Name != "mainnet"
	return NewHWIClient(isTestnet)
}

// ListHardwareWallets enumerates connected hardware wallets using HWI.
// Returns a classified HWIError if HWI is not installed.
func (m *WalletManager) ListHardwareWallets() ([]HardwareDevice, error) {
	hwi := m.GetHWIClient()
	if hwi == nil || !hwi.IsAvailable() {
		return nil, ErrHWINotInstalled
	}
	return hwi.EnumerateDevices()
}

// SignPSBTWithHardwareWallet signs a PSBT using a connected hardware wallet.
// Returns the signed PSBT in base64 format.
func (m *WalletManager) SignPSBTWithHardwareWallet(psbtBase64 string, device HardwareDevice) (string, error) {
	hwi := m.GetHWIClient()
	if hwi == nil || !hwi.IsAvailable() {
		return "", ErrHWINotInstalled
	}
	return hwi.SignPSBT(device, psbtBase64)
}

// DisplayAddressOnDevice shows an address on a connected hardware wallet for verification.
// Enumerates devices, finds one matching the wallet's fingerprint, and calls DisplayAddress.
func (m *WalletManager) DisplayAddressOnDevice(addr, derivPath string) error {
	hwi := m.GetHWIClient()
	if hwi == nil || !hwi.IsAvailable() {
		return ErrHWINotInstalled
	}

	devices, err := hwi.EnumerateDevices()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return &HWIError{Code: HWIErrNoDevice, RawMsg: "no hardware wallets detected"}
	}

	// Find the device that holds this wallet's keys. Showing the address
	// on any other device would only produce a confusing mismatch.
	hwConfig := m.GetHardwareWalletConfig()
	target := &devices[0]
	if hwConfig != nil && hwConfig.Fingerprint != "" {
		target = nil
		for i := range devices {
			if strings.EqualFold(devices[i].Fingerprint, hwConfig.Fingerprint) {
				target = &devices[i]
				break
			}
		}
		if target == nil {
			return &HWIError{Code: HWIErrNoDevice, RawMsg: fmt.Sprintf(
				"no connected device has fingerprint %s", hwConfig.Fingerprint)}
		}
	}

	if target.NeedsPin {
		return &HWIError{Code: HWIErrDeviceLocked, RawMsg: "device needs PIN"}
	}

	return m.verifyAddressOnDevice(hwi, *target, addr, derivPath)
}

// hwiAddrType maps a wallet address type to HWI's --addr-type.
func hwiAddrType(addressType string) string {
	switch addressType {
	case "taproot":
		return "tap"
	case "segwit_native":
		return "wit"
	case "segwit_nested":
		return "sh_wit"
	case "legacy":
		return "legacy"
	}
	return ""
}

// verifyAddressOnDevice has the device derive and show the address at
// derivPath, then checks it against addr.
func (m *WalletManager) verifyAddressOnDevice(hwi *HWIClient, device HardwareDevice, addr, derivPath string) error {
	addrType := ""
	if hwConfig := m.GetHardwareWalletConfig(); hwConfig != nil {
		addrType = hwiAddrType(hwConfig.AddressType)
	}

	displayed, err := hwi.DisplayAddress(device, derivPath, addrType)
	if err != nil {
		return err
	}
	if displayed == "" {
		return fmt.Errorf("device did not report the address; compare it on the device screen")
	}

	// Coldcard and other devices in mainnet mode will show a mainnet address
	// (bc1...) even for testnet derivation paths. Detect this case and show
	// a helpful message instead of a generic mismatch error.
	if displayed != addr {
		isTestnetAddr := strings.HasPrefix(addr, "tb1") || strings.HasPrefix(addr, "2") || strings.HasPrefix(addr, "m") || strings.HasPrefix(addr, "n")
		isMainnetAddr := strings.HasPrefix(displayed, "bc1") || strings.HasPrefix(displayed, "1") || strings.HasPrefix(displayed, "3")
		if isTestnetAddr && isMainnetAddr {
			return fmt.Errorf("device is in mainnet mode but wallet uses testnet; " +
				"the address was shown on the device, so verify the derivation path matches")
		}
		return fmt.Errorf("address mismatch: device showed %s but expected %s", displayed, addr)
	}

	return nil
}
