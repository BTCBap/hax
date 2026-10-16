package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

// =============================================================================
// Multisig Wallet Operations
// =============================================================================

// CreateMultisigWallet creates a watch-only multisig wallet from cosigner xpubs
func (m *WalletManager) CreateMultisigWallet(
	name string,
	password string,
	requiredSigs int,
	cosigners []MultisigCosignerConfig,
	scriptType MultisigScriptType,
) error {
	return m.CreateMultisigWalletOnNetwork(name, password, requiredSigs, cosigners, scriptType, NetworkTestnet)
}

// CreateMultisigWalletOnNetwork creates a watch-only multisig wallet for
// network. Every cosigner xpub must belong to that network.
func (m *WalletManager) CreateMultisigWalletOnNetwork(
	name string,
	password string,
	requiredSigs int,
	cosigners []MultisigCosignerConfig,
	scriptType MultisigScriptType,
	network string,
) error {
	params, err := ParamsForNetwork(network)
	if err != nil {
		return err
	}
	// Validate inputs
	if name == "" {
		return fmt.Errorf("wallet name cannot be empty")
	}
	if m.WalletExists(name) {
		return fmt.Errorf("wallet '%s' already exists", name)
	}
	if requiredSigs < 1 {
		return fmt.Errorf("required signatures must be at least 1")
	}
	if len(cosigners) < requiredSigs {
		return fmt.Errorf("need at least %d cosigners for %d-of-%d multisig",
			requiredSigs, requiredSigs, len(cosigners))
	}
	if len(cosigners) > 15 {
		return fmt.Errorf("maximum 15 cosigners supported")
	}

	// Validate all xpubs
	for i, cs := range cosigners {
		if cs.Xpub == "" {
			return fmt.Errorf("cosigner %d: xpub cannot be empty", i+1)
		}
		// Try to parse the xpub to validate it
		key, err := hdkeychain.NewKeyFromString(cs.Xpub)
		if err != nil {
			return fmt.Errorf("cosigner %d: invalid xpub: %w", i+1, err)
		}
		if !key.IsForNet(params) {
			return fmt.Errorf("cosigner %d: xpub is for a different network (%s)", i+1, expectedXpubHint(network))
		}
	}

	// Create wallet data (no mnemonic for watch-only)
	walletData := &WalletData{
		Version:   1,
		Name:      name,
		Mnemonic:  "", // Empty for watch-only multisig
		CreatedAt: time.Now().Unix(),
		Network:   network,
		Accounts:  make([]AccountData, 0),
		Multisig: &MultisigWalletConfig{
			M:          requiredSigs,
			N:          len(cosigners),
			ScriptType: int(scriptType),
			Cosigners:  cosigners,
		},
	}

	// Create default account for multisig
	accountData := AccountData{
		Index:     0,
		Name:      "Multisig",
		Addresses: make([]AddressData, 0),
		Labels:    make(map[string]string),
	}

	// Set address type based on script type
	switch scriptType {
	case MultisigLegacy:
		accountData.AddressType = "multisig_legacy"
	case MultisigNestedSegwit:
		accountData.AddressType = "multisig_nested"
	case MultisigNativeSegwit:
		accountData.AddressType = "multisig_native"
	}

	walletData.Accounts = append(walletData.Accounts, accountData)

	// Save to disk
	if err := m.storage.SaveWallet(name, walletData, password); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	return nil
}

// IsMultisig returns true if the current wallet is a multisig wallet
func (m *WalletManager) IsMultisig() bool {
	if !m.unlocked || m.data == nil {
		return false
	}
	return m.data.Multisig != nil
}

// GetMultisigConfig returns the multisig configuration for the current wallet
func (m *WalletManager) GetMultisigConfig() *MultisigWalletConfig {
	if !m.unlocked || m.data == nil {
		return nil
	}
	return m.data.Multisig
}

// GetMultisigAddress derives a multisig address at the given index
func (m *WalletManager) GetMultisigAddress(change bool, index uint32) (string, []byte, error) {
	if !m.IsMultisig() {
		return "", nil, fmt.Errorf("not a multisig wallet")
	}

	config := m.data.Multisig
	network := m.getNetwork()

	// Derive child public keys from all cosigner xpubs
	var changeVal uint32 = 0
	if change {
		changeVal = 1
	}

	pubKeys := make([][]byte, len(config.Cosigners))
	for i, cs := range config.Cosigners {
		pubKey, err := DeriveChildPubKey(cs.Xpub, changeVal, index, network)
		if err != nil {
			return "", nil, fmt.Errorf("failed to derive key from cosigner %d: %w", i+1, err)
		}
		pubKeys[i] = pubKey
	}

	// Generate multisig address based on script type
	msConfig := &MultisigConfig{
		M:          config.M,
		N:          config.N,
		ScriptType: MultisigScriptType(config.ScriptType),
	}

	address, witnessScript, err := GenerateMultisigAddress(msConfig, pubKeys, network)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate multisig address: %w", err)
	}

	return address, witnessScript, nil
}

// GetMultisigReceiveAddress returns the next unused receive address
func (m *WalletManager) GetMultisigReceiveAddress() (string, error) {
	if !m.IsMultisig() {
		return "", fmt.Errorf("not a multisig wallet")
	}

	// Find first unused address
	// For now, use a simple counter approach
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()

	if len(m.data.Accounts) == 0 {
		return "", fmt.Errorf("no accounts in wallet")
	}

	account := &m.data.Accounts[0]
	nextIndex := uint32(0)

	// Find the highest used receive address index
	for _, addr := range account.Addresses {
		if !addr.Change && addr.Used && addr.Index >= nextIndex {
			nextIndex = addr.Index + 1
		}
	}

	// Generate address at next index
	address, _, err := m.GetMultisigAddress(false, nextIndex)
	if err != nil {
		return "", err
	}

	return address, nil
}

// GetMultisigDescriptor returns the output descriptor for the multisig wallet
func (m *WalletManager) GetMultisigDescriptor() (string, error) {
	if !m.IsMultisig() {
		return "", fmt.Errorf("not a multisig wallet")
	}

	config := m.data.Multisig

	// Build descriptor based on script type
	var template string
	switch MultisigScriptType(config.ScriptType) {
	case MultisigLegacy:
		template = "sh(sortedmulti(%d,%s))"
	case MultisigNestedSegwit:
		template = "sh(wsh(sortedmulti(%d,%s)))"
	case MultisigNativeSegwit:
		template = "wsh(sortedmulti(%d,%s))"
	default:
		return "", fmt.Errorf("unknown script type: %d", config.ScriptType)
	}

	// Build cosigner key list
	// Format per BIP48/Sparrow: [fingerprint/48h/coin/0h/script]xpub/<0;1>/*
	// The /<0;1>/* suffix indicates receive (0) and change (1) derivation paths
	var keys string
	for i, cs := range config.Cosigners {
		if i > 0 {
			keys += ","
		}
		// Use full key origin if provided by user, otherwise bare xpub
		if cs.KeyOrigin != "" {
			// Full key origin from user input: [origin]xpub/<0;1>/*
			keys += fmt.Sprintf("[%s]%s/<0;1>/*", cs.KeyOrigin, cs.Xpub)
		} else {
			// Bare xpub with derivation suffix
			keys += cs.Xpub + "/<0;1>/*"
		}
	}

	// Build descriptor
	desc := fmt.Sprintf(template, config.M, keys)

	// Add checksum
	checksum := DescriptorChecksum(desc)
	if checksum != "" {
		desc = desc + "#" + checksum
	}

	return desc, nil
}

// ExportColdcardMultisig generates the Coldcard multisig wallet config file format.
// This must be imported onto the Coldcard (via SD card or NFC) before it will sign
// multisig transactions. Returns the file contents as a string.
func (m *WalletManager) ExportColdcardMultisig() (string, error) {
	if !m.IsMultisig() {
		return "", fmt.Errorf("not a multisig wallet")
	}

	config := m.data.Multisig
	coinType := uint32(1) // testnet
	if m.data.Network == NetworkMainnet {
		coinType = 0
	}
	scriptType := MultisigScriptType(config.ScriptType)

	var format string
	switch scriptType {
	case MultisigLegacy:
		format = "P2SH"
	case MultisigNestedSegwit:
		format = "P2SH-P2WSH"
	case MultisigNativeSegwit:
		format = "P2WSH"
	default:
		return "", fmt.Errorf("unknown script type: %d", config.ScriptType)
	}

	// Determine the common derivation path
	// Use the first cosigner's path as reference
	derivPath := fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptType)
	if len(config.Cosigners) > 0 {
		p := cosignerDerivationPath(config.Cosigners[0], coinType, scriptType)
		if p != "" {
			derivPath = p
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Name: %s\n", m.data.Name))
	sb.WriteString(fmt.Sprintf("Policy: %d of %d\n", config.M, config.N))
	sb.WriteString(fmt.Sprintf("Format: %s\n", format))
	sb.WriteString("\n")
	// Use Coldcard h notation for hardened
	ccPath := strings.ReplaceAll(derivPath, "'", "h")
	sb.WriteString(fmt.Sprintf("Derivation: %s\n", ccPath))

	for _, cs := range config.Cosigners {
		fp := cs.Fingerprint
		if fp == "" {
			fp = "00000000"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(fp), cs.Xpub))
	}

	return sb.String(), nil
}

// getNetwork returns the network params for the current wallet
func (m *WalletManager) getNetwork() *chaincfg.Params {
	if m.data != nil && m.data.Network == "mainnet" {
		return &chaincfg.MainNetParams
	}
	return &TestNet4Params
}

// ValidateCosignerXpub validates an xpub string and returns parsed info
func ValidateCosignerXpub(xpub string) (fingerprint string, err error) {
	key, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return "", fmt.Errorf("invalid xpub: %w", err)
	}

	// Check it's a public key (not private)
	if key.IsPrivate() {
		return "", fmt.Errorf("expected public key (xpub/tpub), got private key")
	}

	// For now return empty fingerprint - would need the full key origin
	// In practice, the user would provide [fingerprint/path]xpub format
	return "", nil
}

// CosignerValidationResult contains the result of cosigner validation
type CosignerValidationResult struct {
	Config  MultisigCosignerConfig
	Warning string // Non-fatal warning (e.g., "bare xpub without key origin")
	Error   error  // Fatal error (e.g., "wrong derivation path for multisig")
	IsBIP48 bool   // True if derivation path is proper BIP48
}

// ParseCosignerString parses a cosigner string in format:
// [fingerprint/48h/1h/0h/2h]tpub... or just tpub...
// Standard format from Coldcard/Sparrow exports: [0f056943/48h/1h/0h/2h]tpub...
func ParseCosignerString(input string) (MultisigCosignerConfig, error) {
	result := ValidateCosignerInput(input)
	if result.Error != nil {
		return result.Config, result.Error
	}
	return result.Config, nil
}

// ValidateCosignerInput validates a cosigner input string and returns detailed results
// This implements Sparrow-like validation: reject non-BIP48 paths for multisig
func ValidateCosignerInput(input string) CosignerValidationResult {
	var result CosignerValidationResult
	var config MultisigCosignerConfig

	// Check for key origin format: [fingerprint/path]xpub
	if len(input) > 0 && input[0] == '[' {
		endBracket := -1
		for i, c := range input {
			if c == ']' {
				endBracket = i
				break
			}
		}

		if endBracket == -1 {
			result.Error = fmt.Errorf("malformed key origin: missing closing bracket")
			return result
		}

		// Store the full key origin (everything inside brackets)
		origin := input[1:endBracket]
		config.KeyOrigin = origin

		// Extract fingerprint and path components
		parts := splitOrigin(origin)
		if len(parts) > 0 {
			config.Fingerprint = parts[0]
		}

		config.Xpub = input[endBracket+1:]

		// Validate the derivation path - should be BIP48 for multisig (warn if not)
		if len(parts) >= 2 {
			purpose := parts[1]
			// Normalize: remove 'h', 'H', or '\'' suffix
			purpose = trimHardened(purpose)

			switch purpose {
			case "48":
				result.IsBIP48 = true
			case "84":
				result.Warning = "⚠ Derivation m/84' is for single-sig native SegWit, not multisig.\n" +
					"Standard multisig uses m/48'/coin'/account'/script'.\n" +
					"Signing may require the seed to be imported with correct path detection."
			case "49":
				result.Warning = "⚠ Derivation m/49' is for single-sig nested SegWit, not multisig.\n" +
					"Standard multisig uses m/48'/coin'/account'/script'.\n" +
					"Signing may require the seed to be imported with correct path detection."
			case "44":
				result.Warning = "⚠ Derivation m/44' is for single-sig legacy, not multisig.\n" +
					"Standard multisig uses m/48'/coin'/account'/script'.\n" +
					"Signing may require the seed to be imported with correct path detection."
			case "86":
				result.Warning = "⚠ Derivation m/86' is for single-sig Taproot, not multisig.\n" +
					"Standard multisig uses m/48'/coin'/account'/script'.\n" +
					"Signing may require the seed to be imported with correct path detection."
			default:
				result.Warning = fmt.Sprintf("Unknown derivation purpose '%s'. Expected m/48' for multisig.", purpose)
			}
		}
	} else {
		// Plain xpub - no key origin info
		config.Xpub = input
		config.Fingerprint = ""
		config.KeyOrigin = ""
		result.Warning = "⚠ Bare xpub without key origin info. For safety, use format: [fingerprint/48h/coin/account/script]xpub\n" +
			"Without key origin, we cannot verify the derivation path is correct for multisig signing."
	}

	// Validate xpub
	_, err := hdkeychain.NewKeyFromString(config.Xpub)
	if err != nil {
		result.Error = fmt.Errorf("invalid xpub: %w", err)
		return result
	}

	result.Config = config
	return result
}

// splitOrigin splits a key origin like "fingerprint/48h/1h/0h/2h" into parts
func splitOrigin(origin string) []string {
	var parts []string
	var current string

	for _, c := range origin {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// DeriveMultisigCosignerFromMnemonic derives a MultisigCosignerConfig from a seed phrase
// This automatically derives the BIP48 xpub at the correct path for the given script type
// scriptType: 0=Legacy, 1=Nested SegWit, 2=Native SegWit (P2WSH)
func DeriveMultisigCosignerFromMnemonic(mnemonic string, scriptType MultisigScriptType, isTestnet bool, customPath ...string) (MultisigCosignerConfig, error) {
	var config MultisigCosignerConfig

	// Validate mnemonic
	if !ValidateMnemonic(mnemonic) {
		return config, fmt.Errorf("invalid mnemonic phrase")
	}

	// Convert to seed
	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		return config, fmt.Errorf("failed to derive seed: %w", err)
	}

	// Create HD wallet from seed
	wallet, err := NewHDWallet(seed, isTestnet)
	if err != nil {
		return config, fmt.Errorf("failed to create HD wallet: %w", err)
	}

	// Get master fingerprint
	fingerprint, err := wallet.GetMasterFingerprintHex()
	if err != nil {
		return config, fmt.Errorf("failed to get fingerprint: %w", err)
	}

	var xpub string
	var keyOrigin string

	// Use custom derivation path if provided
	if len(customPath) > 0 && customPath[0] != "" {
		derivPath := customPath[0]
		xpub, err = wallet.GetXpubAtPath(derivPath)
		if err != nil {
			return config, fmt.Errorf("failed to derive xpub at custom path %s: %w", derivPath, err)
		}
		// Build key origin from custom path (convert ' to h for Sparrow compatibility)
		pathWithoutM := strings.TrimPrefix(derivPath, "m/")
		pathWithH := strings.ReplaceAll(pathWithoutM, "'", "h")
		keyOrigin = fmt.Sprintf("%s/%s", fingerprint, pathWithH)
		config.ActualDerivation = derivPath
	} else {
		// Standard path: BIP45 for Legacy, BIP48 for SegWit variants
		derivPath := DefaultMultisigAccountPath(scriptType, isTestnet)
		xpub, err = wallet.GetXpubAtPath(derivPath)
		if err != nil {
			return config, fmt.Errorf("failed to derive xpub at %s: %w", derivPath, err)
		}

		// Build key origin string (convert ' to h for Sparrow compatibility)
		pathWithoutM := strings.TrimPrefix(derivPath, "m/")
		pathWithH := strings.ReplaceAll(pathWithoutM, "'", "h")
		keyOrigin = fmt.Sprintf("%s/%s", fingerprint, pathWithH)
	}

	config.Fingerprint = fingerprint
	config.KeyOrigin = keyOrigin
	config.Xpub = xpub
	config.IsMine = true       // We have the seed, so we can sign
	config.Mnemonic = mnemonic // Store mnemonic for local signing (encrypted with wallet)
	config.DeviceType = "local"

	return config, nil
}
