package wallet

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/BTCBap/hax/internal/ur"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
)

// DescriptorChecksum computes the checksum for an output descriptor
// Based on the descriptor checksum algorithm from Bitcoin Core / BIP380
func DescriptorChecksum(desc string) string {
	// Character set for checksum
	const inputCharset = "0123456789()[],'/*abcdefgh@:$%{}IJKLMNOPQRSTUVWXYZ&+-.;<=>?!^_|~ijklmnopqrstuvwxyzABCDEFGH`#\"\\ "
	const checksumCharset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

	// Polymod function for checksum calculation
	polymod := func(c uint64, val int) uint64 {
		c0 := byte(c >> 35)                        // #nosec G115 -- c is a 40-bit polymod state, so c>>35 fits in 5 bits
		c = ((c & 0x7ffffffff) << 5) ^ uint64(val) // #nosec G115 -- val is a non-negative charset index
		if c0&1 != 0 {
			c ^= 0xf5dee51989
		}
		if c0&2 != 0 {
			c ^= 0xa9fdca3312
		}
		if c0&4 != 0 {
			c ^= 0x1bab10e32d
		}
		if c0&8 != 0 {
			c ^= 0x3706b1677a
		}
		if c0&16 != 0 {
			c ^= 0x644d626ffd
		}
		return c
	}

	var c uint64 = 1
	var cls = 0
	var clscount = 0

	for _, ch := range desc {
		pos := strings.IndexRune(inputCharset, ch)
		if pos == -1 {
			return "" // Invalid character
		}
		c = polymod(c, pos&31)
		cls = cls*3 + (pos >> 5)
		clscount++
		if clscount == 3 {
			c = polymod(c, cls)
			cls = 0
			clscount = 0
		}
	}

	if clscount > 0 {
		c = polymod(c, cls)
	}
	for i := 0; i < 8; i++ {
		c = polymod(c, 0)
	}
	c ^= 1

	var result strings.Builder
	for i := 0; i < 8; i++ {
		result.WriteByte(checksumCharset[(c>>(5*(7-i)))&31])
	}

	return result.String()
}

// OutputDescriptor represents a wallet output descriptor
type OutputDescriptor struct {
	ScriptType  string // "wpkh", "sh(wpkh)", "pkh", "tr"
	Fingerprint string // Master key fingerprint (8 hex chars)
	DerivPath   string // Derivation path without m/ prefix (e.g., "84h/1h/0h")
	Xpub        string // Extended public key
	IsRange     bool   // If true, includes /<0;1>/* for both receive and change
}

// String returns the full descriptor with checksum
func (d *OutputDescriptor) String() string {
	keyExpr := fmt.Sprintf("[%s/%s]%s", d.Fingerprint, d.DerivPath, d.Xpub)
	if d.IsRange {
		keyExpr += "/<0;1>/*"
	}

	var desc string
	switch d.ScriptType {
	case "wpkh":
		desc = fmt.Sprintf("wpkh(%s)", keyExpr)
	case "sh(wpkh)":
		desc = fmt.Sprintf("sh(wpkh(%s))", keyExpr)
	case "pkh":
		desc = fmt.Sprintf("pkh(%s)", keyExpr)
	case "tr":
		desc = fmt.Sprintf("tr(%s)", keyExpr)
	default:
		desc = fmt.Sprintf("wpkh(%s)", keyExpr)
	}

	checksum := DescriptorChecksum(desc)
	if checksum != "" {
		desc = fmt.Sprintf("%s#%s", desc, checksum)
	}

	return desc
}

// GenerateDescriptor creates an output descriptor for a wallet account
// For multisig wallets, this returns a multisig descriptor
func (m *WalletManager) GenerateDescriptor(accountIndex uint32) (*OutputDescriptor, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	// For multisig wallets, return a special descriptor
	if m.IsMultisig() {
		// Return nil to signal that caller should use GetMultisigDescriptor directly
		return nil, fmt.Errorf("use GetMultisigDescriptor for multisig wallets")
	}

	if m.wallet == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return nil, fmt.Errorf("account not found")
	}
	account := m.data.Accounts[accountIndex]
	network := m.data.Network
	m.dataMu.RUnlock()

	// Get master fingerprint
	fingerprint, err := m.wallet.GetMasterFingerprintHex()
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint: %w", err)
	}

	// Determine script type and path based on account type
	var scriptType string
	var purpose uint32
	var addrType AddressType

	switch account.AddressType {
	case "legacy":
		scriptType = "pkh"
		purpose = 44
		addrType = AddressTypeLegacy
	case "segwit_nested":
		scriptType = "sh(wpkh)"
		purpose = 49
		addrType = AddressTypeSegWitNested
	case "segwit_native":
		scriptType = "wpkh"
		purpose = 84
		addrType = AddressTypeSegWitNative
	case "taproot":
		scriptType = "tr"
		purpose = 86
		addrType = AddressTypeTaproot
	default:
		scriptType = "wpkh"
		purpose = 84
		addrType = AddressTypeSegWitNative
	}

	var derivPath string
	var xpub string

	if account.CustomDerivationPath != "" {
		// Use custom path: parse it and convert to 'h' notation for Sparrow
		customPurpose, customCoinType, customAccount, parseErr := ParseAccountPath(account.CustomDerivationPath)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid custom derivation path: %w", parseErr)
		}
		derivPath = fmt.Sprintf("%dh/%dh/%dh", customPurpose, customCoinType, customAccount)
		xpub, err = m.wallet.GetExtendedPublicKeyAtPath(account.CustomDerivationPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get xpub at custom path: %w", err)
		}
	} else {
		// Standard path
		coinType := uint32(1) // Testnet
		if network == "mainnet" {
			coinType = 0
		}
		derivPath = fmt.Sprintf("%dh/%dh/%dh", purpose, coinType, accountIndex)
		xpub, err = m.wallet.GetExtendedPublicKey(accountIndex, addrType)
		if err != nil {
			return nil, fmt.Errorf("failed to get xpub: %w", err)
		}
	}

	return &OutputDescriptor{
		ScriptType:  scriptType,
		Fingerprint: fingerprint,
		DerivPath:   derivPath,
		Xpub:        xpub,
		IsRange:     true,
	}, nil
}

// ExportDescriptor returns the wallet descriptor as a string
func (m *WalletManager) ExportDescriptor(accountIndex uint32) (string, error) {
	// For multisig wallets, use the dedicated multisig descriptor function
	if m.IsMultisig() {
		return m.GetMultisigDescriptor()
	}

	// For hardware/descriptor-imported wallets, build descriptor from config
	if m.IsHardwareWallet() {
		hwConfig := m.GetHardwareWalletConfig()
		if hwConfig == nil {
			return "", fmt.Errorf("hardware wallet config not found")
		}

		// Determine script type
		var scriptType string
		switch hwConfig.AddressType {
		case "legacy":
			scriptType = "pkh"
		case "segwit_nested":
			scriptType = "sh(wpkh)"
		case "segwit_native":
			scriptType = "wpkh"
		case "taproot":
			scriptType = "tr"
		default:
			scriptType = "wpkh"
		}

		// Build key expression
		keyExpr := hwConfig.Xpub
		if hwConfig.KeyOrigin != "" {
			keyExpr = fmt.Sprintf("[%s]%s", hwConfig.KeyOrigin, hwConfig.Xpub)
		} else if hwConfig.Fingerprint != "" {
			keyExpr = fmt.Sprintf("[%s]%s", hwConfig.Fingerprint, hwConfig.Xpub)
		}
		// The xpub is at the account level; addresses are derived at
		// /0/i (receive) and /1/i (change).
		keyExpr += "/<0;1>/*"

		// Build descriptor
		var desc string
		switch scriptType {
		case "tr":
			desc = fmt.Sprintf("tr(%s)", keyExpr)
		case "wpkh":
			desc = fmt.Sprintf("wpkh(%s)", keyExpr)
		case "sh(wpkh)":
			desc = fmt.Sprintf("sh(wpkh(%s))", keyExpr)
		case "pkh":
			desc = fmt.Sprintf("pkh(%s)", keyExpr)
		default:
			desc = fmt.Sprintf("wpkh(%s)", keyExpr)
		}

		// Add checksum
		checksum := DescriptorChecksum(desc)
		if checksum != "" {
			desc = desc + "#" + checksum
		}

		return desc, nil
	}

	desc, err := m.GenerateDescriptor(accountIndex)
	if err != nil {
		return "", err
	}
	return desc.String(), nil
}

// ExportDescriptorUR returns the wallet descriptor encoded as a BC-UR string
// suitable for QR code scanning by Sparrow and other compatible wallets.
// Format: ur:crypto-output/<bytewords>
func (m *WalletManager) ExportDescriptorUR() (string, error) {
	if !m.unlocked || m.data == nil {
		return "", fmt.Errorf("wallet is locked")
	}

	network := 1 // testnet
	if m.data.Network == "mainnet" {
		network = 0
	}

	if m.IsMultisig() {
		return m.exportMultisigDescriptorUR(network)
	}
	return m.exportSingleSigDescriptorUR(network)
}

func (m *WalletManager) exportSingleSigDescriptorUR(network int) (string, error) {
	// Determine script type
	var scriptType string
	var addrType AddressType

	if m.IsHardwareWallet() {
		hwConfig := m.data.HardwareWallet
		addrType = ParseAddressTypeString(hwConfig.AddressType)

		keyInfo, err := m.xpubToKeyInfo(hwConfig.Xpub, hwConfig.KeyOrigin, hwConfig.Fingerprint, "")
		if err != nil {
			return "", fmt.Errorf("failed to extract key info: %w", err)
		}

		scriptType = addrTypeToURScript(addrType)
		return ur.EncodeDescriptorAsUR(ur.DescriptorInfo{
			ScriptType: scriptType,
			IsMultisig: false,
			Keys:       []ur.KeyInfo{keyInfo},
			Network:    network,
		})
	}

	// Software wallet
	if m.wallet == nil {
		return "", fmt.Errorf("wallet not initialized")
	}

	if len(m.data.Accounts) == 0 {
		return "", fmt.Errorf("no accounts")
	}
	account := m.data.Accounts[0]
	addrType = ParseAddressTypeString(account.AddressType)

	fp, err := m.wallet.GetMasterFingerprintHex()
	if err != nil {
		return "", fmt.Errorf("failed to get fingerprint: %w", err)
	}

	var xpub string
	var keyOrigin string

	if account.CustomDerivationPath != "" {
		xpub, err = m.wallet.GetExtendedPublicKeyAtPath(account.CustomDerivationPath)
		if err != nil {
			return "", fmt.Errorf("failed to get xpub at custom path: %w", err)
		}
		// Convert custom path to key origin: "m/84'/0'/3'" → "fp/84h/0h/3h"
		customPurpose, customCoinType, customAccount, parseErr := ParseAccountPath(account.CustomDerivationPath)
		if parseErr != nil {
			return "", fmt.Errorf("invalid custom derivation path: %w", parseErr)
		}
		keyOrigin = fmt.Sprintf("%s/%dh/%dh/%dh", fp, customPurpose, customCoinType, customAccount)
	} else {
		xpub, err = m.wallet.GetExtendedPublicKey(0, addrType)
		if err != nil {
			return "", fmt.Errorf("failed to get xpub: %w", err)
		}
		purpose := PurposeForAddressType(addrType)
		coinType := uint32(1)
		if m.data.Network == "mainnet" {
			coinType = 0
		}
		keyOrigin = fmt.Sprintf("%s/%dh/%dh/0h", fp, purpose, coinType)
	}

	keyInfo, err := m.xpubToKeyInfo(xpub, keyOrigin, fp, "")
	if err != nil {
		return "", fmt.Errorf("failed to extract key info: %w", err)
	}

	scriptType = addrTypeToURScript(addrType)
	return ur.EncodeDescriptorAsUR(ur.DescriptorInfo{
		ScriptType: scriptType,
		IsMultisig: false,
		Keys:       []ur.KeyInfo{keyInfo},
		Network:    network,
	})
}

func (m *WalletManager) exportMultisigDescriptorUR(network int) (string, error) {
	config := m.data.Multisig

	var scriptType string
	switch MultisigScriptType(config.ScriptType) {
	case MultisigLegacy:
		scriptType = "sh"
	case MultisigNestedSegwit:
		scriptType = "sh-wsh"
	case MultisigNativeSegwit:
		scriptType = "wsh"
	default:
		scriptType = "wsh"
	}

	keys := make([]ur.KeyInfo, len(config.Cosigners))
	for i, cs := range config.Cosigners {
		keyInfo, err := m.xpubToKeyInfo(cs.Xpub, cs.KeyOrigin, cs.Fingerprint, cs.Name)
		if err != nil {
			return "", fmt.Errorf("cosigner %d: %w", i+1, err)
		}
		keys[i] = keyInfo
	}

	return ur.EncodeDescriptorAsUR(ur.DescriptorInfo{
		ScriptType: scriptType,
		IsMultisig: true,
		Threshold:  config.M,
		Keys:       keys,
		Network:    network,
	})
}

// xpubToKeyInfo extracts raw key material from an xpub string and builds a KeyInfo.
func (m *WalletManager) xpubToKeyInfo(xpubStr, keyOrigin, fingerprint, name string) (ur.KeyInfo, error) {
	key, err := hdkeychain.NewKeyFromString(xpubStr)
	if err != nil {
		return ur.KeyInfo{}, fmt.Errorf("invalid xpub: %w", err)
	}

	pubKey, err := key.ECPubKey()
	if err != nil {
		return ur.KeyInfo{}, fmt.Errorf("failed to get public key: %w", err)
	}

	info := ur.KeyInfo{
		PublicKey: pubKey.SerializeCompressed(),
		ChainCode: key.ChainCode(),
		Name:      name,
	}

	// Parse fingerprint
	if fingerprint != "" {
		fpBytes, err := hex.DecodeString(fingerprint)
		if err == nil && len(fpBytes) == 4 {
			info.MasterFP = uint32(fpBytes[0])<<24 | uint32(fpBytes[1])<<16 | uint32(fpBytes[2])<<8 | uint32(fpBytes[3])
		}
	}

	// Parse key origin to derivation path
	if keyOrigin != "" {
		path, parentFP := parseKeyOriginToPath(keyOrigin)
		info.DerivationPath = path
		if parentFP != 0 {
			info.MasterFP = parentFP // Override with more specific fingerprint from origin
		}
	}

	// Compute parent fingerprint from the xpub's parent field
	info.ParentFP = key.ParentFingerprint()

	return info, nil
}

// parseKeyOriginToPath converts a key origin string like "0f056943/48h/1h/0h/2h"
// into a slice of uint32 path components (with hardened bit set) and the master fingerprint.
func parseKeyOriginToPath(origin string) ([]uint32, uint32) {
	parts := splitOrigin(origin)
	if len(parts) < 2 {
		return nil, 0
	}

	// First part is fingerprint
	var masterFP uint32
	fpBytes, err := hex.DecodeString(parts[0])
	if err == nil && len(fpBytes) == 4 {
		masterFP = uint32(fpBytes[0])<<24 | uint32(fpBytes[1])<<16 | uint32(fpBytes[2])<<8 | uint32(fpBytes[3])
	}

	// Remaining parts are path components
	var path []uint32
	for _, p := range parts[1:] {
		hardened := isHardened(p)
		numStr := trimHardened(p)
		val, err := strconv.ParseUint(numStr, 10, 31)
		if err != nil {
			continue
		}
		component := uint32(val)
		if hardened {
			component |= 0x80000000
		}
		path = append(path, component)
	}

	return path, masterFP
}

// addrTypeToURScript maps AddressType to the UR script type string.
func addrTypeToURScript(addrType AddressType) string {
	switch addrType {
	case AddressTypeLegacy:
		return "pkh"
	case AddressTypeSegWitNested:
		return "sh-wpkh"
	case AddressTypeSegWitNative:
		return "wpkh"
	case AddressTypeTaproot:
		return "tr"
	default:
		return "wpkh"
	}
}

// DescriptorParseResult holds the result of parsing any output descriptor
type DescriptorParseResult struct {
	IsMultisig bool
	// For multisig
	MultisigConfig *MultisigWalletConfig
	// For single-sig
	SingleSigConfig *SingleSigDescriptorConfig
	// Warnings (non-fatal issues)
	Warnings []string
}

// SingleSigDescriptorConfig holds parsed single-sig descriptor info
type SingleSigDescriptorConfig struct {
	Fingerprint string
	KeyOrigin   string
	Xpub        string
	AddressType AddressType
}

// ParseDescriptor parses any output descriptor (single-sig or multisig)
func ParseDescriptor(descriptor string) (*DescriptorParseResult, error) {
	var warnings []string

	// Trim whitespace
	descriptor = strings.TrimSpace(descriptor)
	if descriptor == "" {
		return nil, fmt.Errorf("empty descriptor")
	}

	// Strip and validate checksum if present
	if idx := strings.LastIndex(descriptor, "#"); idx != -1 {
		providedChecksum := descriptor[idx+1:]
		descriptorBody := descriptor[:idx]

		expectedChecksum := DescriptorChecksum(descriptorBody)
		if expectedChecksum != providedChecksum {
			warnings = append(warnings, fmt.Sprintf("checksum mismatch: expected %s, got %s", expectedChecksum, providedChecksum))
		}
		descriptor = descriptorBody
	} else {
		warnings = append(warnings, "no checksum provided")
	}

	// Try to detect descriptor type
	// Multisig patterns
	if strings.Contains(descriptor, "sortedmulti(") || strings.Contains(descriptor, "multi(") {
		config, msWarnings, err := parseMultisigDescriptor(descriptor)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, msWarnings...)
		return &DescriptorParseResult{
			IsMultisig:     true,
			MultisigConfig: config,
			Warnings:       warnings,
		}, nil
	}

	// Single-sig patterns: wpkh(...), sh(wpkh(...)), tr(...), pkh(...)
	config, ssWarnings, err := parseSingleSigDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, ssWarnings...)
	return &DescriptorParseResult{
		IsMultisig:      false,
		SingleSigConfig: config,
		Warnings:        warnings,
	}, nil
}

// parseSingleSigDescriptor parses single-sig descriptors
func parseSingleSigDescriptor(descriptor string) (*SingleSigDescriptorConfig, []string, error) {
	var warnings []string
	var addrType AddressType
	var keyExpr string

	// Detect type and extract key expression
	switch {
	case strings.HasPrefix(descriptor, "tr(") && strings.HasSuffix(descriptor, ")"):
		// Taproot: tr([fp/path]xpub...)
		addrType = AddressTypeTaproot
		keyExpr = descriptor[3 : len(descriptor)-1]

	case strings.HasPrefix(descriptor, "wpkh(") && strings.HasSuffix(descriptor, ")"):
		// Native SegWit: wpkh([fp/path]xpub...)
		addrType = AddressTypeSegWitNative
		keyExpr = descriptor[5 : len(descriptor)-1]

	case strings.HasPrefix(descriptor, "sh(wpkh(") && strings.HasSuffix(descriptor, "))"):
		// Nested SegWit: sh(wpkh([fp/path]xpub...))
		addrType = AddressTypeSegWitNested
		keyExpr = descriptor[8 : len(descriptor)-2]

	case strings.HasPrefix(descriptor, "pkh(") && strings.HasSuffix(descriptor, ")"):
		// Legacy: pkh([fp/path]xpub...)
		addrType = AddressTypeLegacy
		keyExpr = descriptor[4 : len(descriptor)-1]

	default:
		return nil, nil, fmt.Errorf("unrecognized descriptor format: expected wpkh(...), sh(wpkh(...)), tr(...), pkh(...), or multisig variant")
	}

	// Strip derivation suffix like /<0;1>/* or /0/*
	keyExpr = stripDerivationSuffix(keyExpr)

	// Parse key expression: [fingerprint/path]xpub
	var fingerprint, keyOrigin, xpub string

	if strings.HasPrefix(keyExpr, "[") {
		endBracket := strings.Index(keyExpr, "]")
		if endBracket == -1 {
			return nil, nil, fmt.Errorf("malformed key origin: missing closing bracket")
		}
		keyOrigin = keyExpr[1:endBracket]
		xpub = keyExpr[endBracket+1:]

		// Extract fingerprint (first component before /)
		if slashIdx := strings.Index(keyOrigin, "/"); slashIdx != -1 {
			fingerprint = keyOrigin[:slashIdx]
		} else {
			fingerprint = keyOrigin
		}
	} else {
		// Bare xpub
		xpub = keyExpr
		warnings = append(warnings, "bare xpub without key origin - some features may not work")
	}

	// Validate xpub
	if _, err := validateXpub(xpub); err != nil {
		return nil, nil, fmt.Errorf("invalid xpub: %w", err)
	}

	return &SingleSigDescriptorConfig{
		Fingerprint: fingerprint,
		KeyOrigin:   keyOrigin,
		Xpub:        xpub,
		AddressType: addrType,
	}, warnings, nil
}

// validateXpub checks if an xpub string is valid
func validateXpub(xpub string) (string, error) {
	// Use the existing hdkeychain validation
	_, err := hdkeychain.NewKeyFromString(xpub)
	return xpub, err
}

// parseMultisigDescriptor parses multisig output descriptors
func parseMultisigDescriptor(descriptor string) (*MultisigWalletConfig, []string, error) {
	var warnings []string
	var scriptType MultisigScriptType
	var inner string

	// Detect script type from wrapper
	switch {
	case strings.HasPrefix(descriptor, "wsh(sortedmulti(") && strings.HasSuffix(descriptor, "))"):
		scriptType = MultisigNativeSegwit
		inner = descriptor[len("wsh(sortedmulti(") : len(descriptor)-2]
	case strings.HasPrefix(descriptor, "wsh(multi(") && strings.HasSuffix(descriptor, "))"):
		scriptType = MultisigNativeSegwit
		inner = descriptor[len("wsh(multi(") : len(descriptor)-2]
	case strings.HasPrefix(descriptor, "sh(wsh(sortedmulti(") && strings.HasSuffix(descriptor, ")))"):
		scriptType = MultisigNestedSegwit
		inner = descriptor[len("sh(wsh(sortedmulti(") : len(descriptor)-3]
	case strings.HasPrefix(descriptor, "sh(wsh(multi(") && strings.HasSuffix(descriptor, ")))"):
		scriptType = MultisigNestedSegwit
		inner = descriptor[len("sh(wsh(multi(") : len(descriptor)-3]
	case strings.HasPrefix(descriptor, "sh(sortedmulti(") && strings.HasSuffix(descriptor, "))"):
		scriptType = MultisigLegacy
		inner = descriptor[len("sh(sortedmulti(") : len(descriptor)-2]
	case strings.HasPrefix(descriptor, "sh(multi(") && strings.HasSuffix(descriptor, "))"):
		scriptType = MultisigLegacy
		inner = descriptor[len("sh(multi(") : len(descriptor)-2]
	default:
		return nil, nil, fmt.Errorf("unrecognized multisig descriptor format")
	}

	// Parse M value and keys: M,key1,key2,...
	parts := splitDescriptorKeys(inner)
	if len(parts) < 2 {
		return nil, nil, fmt.Errorf("invalid multisig: expected at least M and one key")
	}

	var m int
	if _, err := fmt.Sscanf(parts[0], "%d", &m); err != nil {
		return nil, nil, fmt.Errorf("invalid M value '%s': %w", parts[0], err)
	}
	if m < 1 {
		return nil, nil, fmt.Errorf("m must be at least 1, got %d", m)
	}

	keys := parts[1:]
	n := len(keys)
	if m > n {
		return nil, nil, fmt.Errorf("m (%d) cannot be greater than n (%d)", m, n)
	}
	if n > 15 {
		return nil, nil, fmt.Errorf("maximum 15 cosigners supported, got %d", n)
	}

	cosigners := make([]MultisigCosignerConfig, 0, n)
	for i, keyStr := range keys {
		keyStr = stripDerivationSuffix(keyStr)

		result := ValidateCosignerInput(keyStr)
		if result.Error != nil {
			return nil, nil, fmt.Errorf("cosigner %d: %w", i+1, result.Error)
		}
		if result.Warning != "" {
			warnings = append(warnings, fmt.Sprintf("cosigner %d: %s", i+1, result.Warning))
		}

		cs := result.Config
		cs.Name = fmt.Sprintf("Cosigner %d", i+1)
		cosigners = append(cosigners, cs)
	}

	return &MultisigWalletConfig{
		M:          m,
		N:          n,
		ScriptType: int(scriptType),
		Cosigners:  cosigners,
	}, warnings, nil
}

// ParseMultisigDescriptor is kept for backwards compatibility
func ParseMultisigDescriptor(descriptor string) (*MultisigWalletConfig, []string, error) {
	result, err := ParseDescriptor(descriptor)
	if err != nil {
		return nil, nil, err
	}
	if !result.IsMultisig {
		return nil, nil, fmt.Errorf("descriptor is single-sig, not multisig")
	}
	return result.MultisigConfig, result.Warnings, nil
}

// splitDescriptorKeys splits the inner content of sortedmulti() by commas,
// properly handling nested brackets in key origins
func splitDescriptorKeys(inner string) []string {
	var parts []string
	var current strings.Builder
	bracketDepth := 0

	for _, ch := range inner {
		switch ch {
		case '[':
			bracketDepth++
			current.WriteRune(ch)
		case ']':
			bracketDepth--
			current.WriteRune(ch)
		case ',':
			if bracketDepth == 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Add final part
	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}

	return parts
}

// stripDerivationSuffix removes trailing derivation paths like /<0;1>/* or /0/* from a key string
func stripDerivationSuffix(key string) string {
	// Handle multipath format /<0;1>/*
	if idx := strings.Index(key, "/<"); idx != -1 {
		return key[:idx]
	}
	// Handle simple format /0/* or /1/*
	if idx := strings.Index(key, "/0/*"); idx != -1 {
		return key[:idx]
	}
	if idx := strings.Index(key, "/1/*"); idx != -1 {
		return key[:idx]
	}
	// Handle just /*
	if strings.HasSuffix(key, "/*") {
		// Find the last / before /*
		if idx := strings.LastIndex(key[:len(key)-2], "/"); idx != -1 {
			return key[:idx]
		}
	}
	return key
}
