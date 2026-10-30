package wallet

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// AddressType represents different Bitcoin address formats
type AddressType int

const (
	// AddressTypeLegacy is the original P2PKH address format (starts with 1)
	AddressTypeLegacy AddressType = iota
	// AddressTypeSegWitNested is P2SH-wrapped SegWit (starts with 3)
	AddressTypeSegWitNested
	// AddressTypeSegWitNative is native SegWit bech32 (starts with bc1)
	AddressTypeSegWitNative
	// AddressTypeTaproot is P2TR Taproot (starts with bc1p)
	AddressTypeTaproot
)

// MultisigScriptType represents the 3 Sparrow-compatible multisig options
type MultisigScriptType int

const (
	// MultisigLegacy is P2SH multisig - BIP48 script index 0'
	MultisigLegacy MultisigScriptType = 0
	// MultisigNestedSegwit is P2SH-P2WSH - BIP48 script index 1'
	MultisigNestedSegwit MultisigScriptType = 1
	// MultisigNativeSegwit is P2WSH - BIP48 script index 2' (recommended)
	MultisigNativeSegwit MultisigScriptType = 2
)

// BIP44Path represents a BIP44 derivation path
// Format: m / purpose' / coin_type' / account' / change / address_index
type BIP44Path struct {
	Purpose      uint32 // Usually 44' for BIP44, 49' for P2SH-SegWit, 84' for Native SegWit
	CoinType     uint32 // 0' for Bitcoin mainnet, 1' for testnet
	Account      uint32 // Account number
	Change       uint32 // 0 for external (receiving), 1 for internal (change)
	AddressIndex uint32 // Address index
}

// StandardBIP44Path returns the standard BIP44 path for Bitcoin
func StandardBIP44Path(testnet bool, account, change, addressIndex uint32) BIP44Path {
	coinType := uint32(0) // Mainnet
	if testnet {
		coinType = 1 // Testnet
	}

	return BIP44Path{
		Purpose:      44,
		CoinType:     coinType,
		Account:      account,
		Change:       change,
		AddressIndex: addressIndex,
	}
}

// SegWitNestedPath returns BIP49 path for P2SH-wrapped SegWit
func SegWitNestedPath(testnet bool, account, change, addressIndex uint32) BIP44Path {
	coinType := uint32(0)
	if testnet {
		coinType = 1
	}

	return BIP44Path{
		Purpose:      49,
		CoinType:     coinType,
		Account:      account,
		Change:       change,
		AddressIndex: addressIndex,
	}
}

// SegWitNativePath returns BIP84 path for native SegWit
func SegWitNativePath(testnet bool, account, change, addressIndex uint32) BIP44Path {
	coinType := uint32(0)
	if testnet {
		coinType = 1
	}

	return BIP44Path{
		Purpose:      84,
		CoinType:     coinType,
		Account:      account,
		Change:       change,
		AddressIndex: addressIndex,
	}
}

// TaprootPath returns BIP86 path for Taproot (P2TR)
func TaprootPath(testnet bool, account, change, addressIndex uint32) BIP44Path {
	coinType := uint32(0)
	if testnet {
		coinType = 1
	}

	return BIP44Path{
		Purpose:      86,
		CoinType:     coinType,
		Account:      account,
		Change:       change,
		AddressIndex: addressIndex,
	}
}

// ParseAddressTypeString converts a string like "legacy", "segwit_nested",
// "segwit_native", "taproot" to the corresponding AddressType enum.
// Defaults to AddressTypeSegWitNative for unknown values.
func ParseAddressTypeString(s string) AddressType {
	switch s {
	case "legacy":
		return AddressTypeLegacy
	case "segwit_nested":
		return AddressTypeSegWitNested
	case "segwit_native":
		return AddressTypeSegWitNative
	case "taproot":
		return AddressTypeTaproot
	default:
		return AddressTypeSegWitNative
	}
}

// PurposeForAddressType returns the BIP purpose number for a given address type.
func PurposeForAddressType(addrType AddressType) uint32 {
	switch addrType {
	case AddressTypeLegacy:
		return 44
	case AddressTypeSegWitNested:
		return 49
	case AddressTypeSegWitNative:
		return 84
	case AddressTypeTaproot:
		return 86
	default:
		return 84
	}
}

// PathForAddressType creates the correct BIP44 derivation path for the given address type.
func PathForAddressType(addrType AddressType, isTestnet bool, account, change, index uint32) BIP44Path {
	switch addrType {
	case AddressTypeLegacy:
		return StandardBIP44Path(isTestnet, account, change, index)
	case AddressTypeSegWitNested:
		return SegWitNestedPath(isTestnet, account, change, index)
	case AddressTypeTaproot:
		return TaprootPath(isTestnet, account, change, index)
	default:
		return SegWitNativePath(isTestnet, account, change, index)
	}
}

// trimHardened removes hardened notation (', h, H) from a path component.
func trimHardened(s string) string {
	if len(s) > 0 {
		switch s[len(s)-1] {
		case '\'', 'h', 'H':
			return s[:len(s)-1]
		}
	}
	return s
}

// isHardened returns true if the path component has hardened notation.
func isHardened(s string) bool {
	if len(s) == 0 {
		return false
	}
	switch s[len(s)-1] {
	case '\'', 'h', 'H':
		return true
	}
	return false
}

// keyOriginToPath converts a key origin string (e.g. "fingerprint/48h/1h/0h/2h")
// to a derivation path (e.g. "m/48'/1'/0'/2'"). Returns "" if the origin has
// no path components (fingerprint only or empty).
func keyOriginToPath(origin string) string {
	parts := splitOrigin(origin)
	if len(parts) < 2 {
		return ""
	}
	var pathParts []string
	for _, p := range parts[1:] { // skip fingerprint
		if isHardened(p) {
			pathParts = append(pathParts, trimHardened(p)+"'")
		} else {
			pathParts = append(pathParts, p)
		}
	}
	return "m/" + strings.Join(pathParts, "/")
}

// cosignerDerivationPath returns the derivation path for a cosigner, resolving
// ActualDerivation → KeyOrigin → BIP48 default (in priority order).
func cosignerDerivationPath(cs MultisigCosignerConfig, coinType uint32, scriptType MultisigScriptType) string {
	if cs.ActualDerivation != "" {
		return cs.ActualDerivation
	}
	if path := keyOriginToPath(cs.KeyOrigin); path != "" {
		return path
	}
	return fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptType)
}

// ParseAccountPath parses an account-level derivation path string like "m/84'/1'/0'"
// into its purpose, coinType, and account components.
// Accepts hardened notation with ', h, or H.
// Returns an error if the path doesn't have at least 3 components.
func ParseAccountPath(pathStr string) (purpose, coinType, account uint32, err error) {
	pathStr = strings.TrimPrefix(pathStr, "m/")
	if pathStr == "" {
		return 0, 0, 0, fmt.Errorf("empty path")
	}

	parts := strings.Split(pathStr, "/")
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("account path requires at least 3 components (purpose/cointype/account), got %d", len(parts))
	}

	val, parseErr := strconv.ParseUint(trimHardened(parts[0]), 10, 32)
	if parseErr != nil {
		return 0, 0, 0, fmt.Errorf("invalid purpose %q: %w", parts[0], parseErr)
	}
	purpose = uint32(val)

	val, parseErr = strconv.ParseUint(trimHardened(parts[1]), 10, 32)
	if parseErr != nil {
		return 0, 0, 0, fmt.Errorf("invalid coin type %q: %w", parts[1], parseErr)
	}
	coinType = uint32(val)

	val, parseErr = strconv.ParseUint(trimHardened(parts[2]), 10, 32)
	if parseErr != nil {
		return 0, 0, 0, fmt.Errorf("invalid account %q: %w", parts[2], parseErr)
	}
	account = uint32(val)

	return purpose, coinType, account, nil
}

// AccountPathForAddressType returns the standard account-level derivation path
// for a given address type, e.g. "m/84'/1'/0'" for native SegWit on testnet.
func AccountPathForAddressType(addrType AddressType, isTestnet bool, account uint32) string {
	purpose := PurposeForAddressType(addrType)
	coinType := uint32(0)
	if isTestnet {
		coinType = 1
	}
	return fmt.Sprintf("m/%d'/%d'/%d'", purpose, coinType, account)
}

// BuildFullPath combines an account-level path with change and index to produce
// a full derivation path, e.g. "m/84'/1'/0'" + (0, 5) → "m/84'/1'/0'/0/5".
func BuildFullPath(accountPath string, change, index uint32) string {
	return fmt.Sprintf("%s/%d/%d", accountPath, change, index)
}

// ValidateAccountPath checks that a path string is a valid account-level derivation path.
// Returns nil if valid, or an error describing the issue.
func ValidateAccountPath(pathStr string) error {
	if pathStr == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(pathStr, "m/") {
		return fmt.Errorf("path must start with 'm/'")
	}

	_, _, _, err := ParseAccountPath(pathStr)
	if err != nil {
		return err
	}
	return nil
}

// ResolveAccountPath returns the effective account-level path for a wallet.
// If customPath is non-empty, it's used directly. Otherwise, a standard BIP path
// is constructed from the address type.
func ResolveAccountPath(customPath string, addrType AddressType, isTestnet bool, account uint32) string {
	if customPath != "" {
		return customPath
	}
	return AccountPathForAddressType(addrType, isTestnet, account)
}

// GetExtendedPublicKeyAtPath returns the extended public key at an arbitrary account path.
// The path should be an account-level path like "m/84'/1'/0'" or "m/49'/0'/3'".
func (w *HDWallet) GetExtendedPublicKeyAtPath(accountPath string) (string, error) {
	accountPath = strings.TrimPrefix(accountPath, "m/")
	parts := strings.Split(accountPath, "/")

	key := w.masterKey
	var err error
	for _, part := range parts {
		hardened := isHardened(part)
		numStr := trimHardened(part)
		val, parseErr := strconv.ParseUint(numStr, 10, 31)
		if parseErr != nil {
			return "", fmt.Errorf("invalid path component %q: %w", part, parseErr)
		}
		idx := uint32(val)
		if hardened {
			idx += hdkeychain.HardenedKeyStart
		}
		key, err = key.Derive(idx)
		if err != nil {
			return "", fmt.Errorf("failed to derive at component %q: %w", part, err)
		}
	}

	pubKey, err := key.Neuter()
	if err != nil {
		return "", fmt.Errorf("failed to neuter key: %w", err)
	}
	return pubKey.String(), nil
}

// BIP48Path represents a BIP48 derivation path for multisig
// Format: m / 48' / coin_type' / account' / script_type'
type BIP48Path struct {
	CoinType   uint32             // 0' for mainnet, 1' for testnet
	Account    uint32             // Account number
	ScriptType MultisigScriptType // 0=Legacy, 1=Nested, 2=Native
}

// String returns the string representation of the BIP48 path (account level)
func (p BIP48Path) String() string {
	return fmt.Sprintf("m/48'/%d'/%d'/%d'",
		p.CoinType, p.Account, p.ScriptType)
}

// DefaultMultisigAccountPath returns the standard derivation path for a multisig script type.
// Legacy (P2SH) uses BIP45: m/45'
// Nested SegWit and Native SegWit use BIP48: m/48'/coin'/account'/script'
func DefaultMultisigAccountPath(scriptType MultisigScriptType, isTestnet bool) string {
	if scriptType == MultisigLegacy {
		return "m/45'"
	}
	coinType := uint32(0)
	if isTestnet {
		coinType = 1
	}
	return fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptType)
}

// ChildPath returns the full path including change and index
func (p BIP48Path) ChildPath(change, index uint32) string {
	return fmt.Sprintf("m/48'/%d'/%d'/%d'/%d/%d",
		p.CoinType, p.Account, p.ScriptType, change, index)
}

// String returns the string representation of the path
func (p BIP44Path) String() string {
	return fmt.Sprintf("m/%d'/%d'/%d'/%d/%d",
		p.Purpose, p.CoinType, p.Account, p.Change, p.AddressIndex)
}

// ParseBIP44Path parses a BIP44 path string like "m/84'/1'/0'/0/5"
func ParseBIP44Path(pathStr string) (BIP44Path, error) {
	var path BIP44Path

	// Remove "m/" prefix if present
	pathStr = strings.TrimPrefix(pathStr, "m/")

	// Remove hardened indicators (')
	pathStr = strings.ReplaceAll(pathStr, "'", "")

	// Split by /
	parts := strings.Split(pathStr, "/")
	if len(parts) != 5 {
		return path, fmt.Errorf("invalid BIP44 path: expected 5 parts, got %d", len(parts))
	}

	// Parse each component
	var err error
	var val uint64

	val, err = strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return path, fmt.Errorf("invalid purpose: %w", err)
	}
	path.Purpose = uint32(val)

	val, err = strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return path, fmt.Errorf("invalid coin type: %w", err)
	}
	path.CoinType = uint32(val)

	val, err = strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return path, fmt.Errorf("invalid account: %w", err)
	}
	path.Account = uint32(val)

	val, err = strconv.ParseUint(parts[3], 10, 32)
	if err != nil {
		return path, fmt.Errorf("invalid change: %w", err)
	}
	path.Change = uint32(val)

	val, err = strconv.ParseUint(parts[4], 10, 32)
	if err != nil {
		return path, fmt.Errorf("invalid address index: %w", err)
	}
	path.AddressIndex = uint32(val)

	return path, nil
}

// HDWallet represents a hierarchical deterministic wallet
type HDWallet struct {
	masterKey *hdkeychain.ExtendedKey
	network   *chaincfg.Params
}

// NewHDWallet creates a new HD wallet from a seed
func NewHDWallet(seed []byte, testnet bool) (*HDWallet, error) {
	var network *chaincfg.Params
	if testnet {
		network = &TestNet4Params // Using Testnet4 (2024+)
	} else {
		network = &chaincfg.MainNetParams
	}

	masterKey, err := hdkeychain.NewMaster(seed, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %w", err)
	}

	return &HDWallet{
		masterKey: masterKey,
		network:   network,
	}, nil
}

// Destroy wipes the extended private key material. The wallet must not be
// used after this call.
func (w *HDWallet) Destroy() {
	if w == nil || w.masterKey == nil {
		return
	}
	w.masterKey.Zero()
	w.masterKey = nil
}

// DeriveKey derives a key at the specified BIP44 path
func (w *HDWallet) DeriveKey(path BIP44Path) (*hdkeychain.ExtendedKey, error) {
	key := w.masterKey

	indices := []uint32{
		hdkeychain.HardenedKeyStart + path.Purpose,
		hdkeychain.HardenedKeyStart + path.CoinType,
		hdkeychain.HardenedKeyStart + path.Account,
		path.Change,
		path.AddressIndex,
	}

	var err error
	for _, index := range indices {
		key, err = key.Derive(index)
		if err != nil {
			return nil, fmt.Errorf("failed to derive child key at index %d: %w", index, err)
		}
	}

	return key, nil
}

// GetAddress derives and returns a Bitcoin address at the specified path
func (w *HDWallet) GetAddress(path BIP44Path, addrType AddressType) (string, error) {
	key, err := w.DeriveKey(path)
	if err != nil {
		return "", err
	}

	pubKey, err := key.ECPubKey()
	if err != nil {
		return "", fmt.Errorf("failed to get public key: %w", err)
	}

	pubKeyBytes := pubKey.SerializeCompressed()
	btcPubKey, err := btcutil.NewAddressPubKey(pubKeyBytes, w.network)
	if err != nil {
		return "", fmt.Errorf("failed to create address from public key: %w", err)
	}

	var address btcutil.Address
	switch addrType {
	case AddressTypeLegacy:
		address = btcPubKey.AddressPubKeyHash()

	case AddressTypeSegWitNative:
		witnessProgram := btcutil.Hash160(pubKeyBytes)
		address, err = btcutil.NewAddressWitnessPubKeyHash(witnessProgram, w.network)
		if err != nil {
			return "", fmt.Errorf("failed to create witness address: %w", err)
		}

	case AddressTypeSegWitNested:
		// P2SH-P2WPKH: redeemScript wraps the P2WPKH witness program
		pubKeyHash := btcutil.Hash160(pubKeyBytes)
		redeemScript, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(pubKeyHash).
			Script()
		if err != nil {
			return "", fmt.Errorf("failed to create witness program: %w", err)
		}

		address, err = btcutil.NewAddressScriptHash(redeemScript, w.network)
		if err != nil {
			return "", fmt.Errorf("failed to create script hash address: %w", err)
		}

	case AddressTypeTaproot:
		// BIP86: normalize internal key to even Y before tweaking
		// (btcd's ComputeTaprootKeyNoScript doesn't normalize internally)
		xOnlyBytes := schnorr.SerializePubKey(pubKey)
		internalKey, err := schnorr.ParsePubKey(xOnlyBytes)
		if err != nil {
			return "", fmt.Errorf("failed to normalize taproot internal key: %w", err)
		}

		// Compute: Q = lift_x(bytes(P)) + H_TapTweak(bytes(P)) * G
		taprootKey := txscript.ComputeTaprootKeyNoScript(internalKey)

		address, err = btcutil.NewAddressTaproot(
			schnorr.SerializePubKey(taprootKey),
			w.network,
		)
		if err != nil {
			return "", fmt.Errorf("failed to create taproot address: %w", err)
		}

	default:
		return "", fmt.Errorf("unsupported address type")
	}

	return address.EncodeAddress(), nil
}

// GetPrivateKey returns the WIF-encoded private key at the specified path
// WARNING: Handle with extreme care - never expose this in logs or UI
func (w *HDWallet) GetPrivateKey(path BIP44Path) (string, error) {
	key, err := w.DeriveKey(path)
	if err != nil {
		return "", err
	}

	privKey, err := key.ECPrivKey()
	if err != nil {
		return "", fmt.Errorf("failed to get private key: %w", err)
	}

	wif, err := btcutil.NewWIF(privKey, w.network, true)
	if err != nil {
		return "", fmt.Errorf("failed to encode private key: %w", err)
	}

	return wif.String(), nil
}

// GetMasterFingerprint returns the 4-byte fingerprint of the master key.
func (w *HDWallet) GetMasterFingerprint() ([]byte, error) {
	pubKey, err := w.masterKey.ECPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get master public key: %w", err)
	}
	// Hash160 of compressed public key, first 4 bytes
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	return pubKeyHash[:4], nil
}

// GetMasterFingerprintHex returns the fingerprint as a lowercase hex string
func (w *HDWallet) GetMasterFingerprintHex() (string, error) {
	fp, err := w.GetMasterFingerprint()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x", fp), nil
}

// GetExtendedPublicKey returns the extended public key for an account.
func (w *HDWallet) GetExtendedPublicKey(account uint32, addrType AddressType) (string, error) {
	purpose := PurposeForAddressType(addrType)

	coinType := uint32(1)
	if w.network == &chaincfg.MainNetParams {
		coinType = 0
	}

	key := w.masterKey
	indices := []uint32{
		hdkeychain.HardenedKeyStart + purpose,
		hdkeychain.HardenedKeyStart + coinType,
		hdkeychain.HardenedKeyStart + account,
	}

	var err error
	for _, index := range indices {
		key, err = key.Derive(index)
		if err != nil {
			return "", fmt.Errorf("failed to derive account key: %w", err)
		}
	}

	pubKey, err := key.Neuter()
	if err != nil {
		return "", fmt.Errorf("failed to neuter key: %w", err)
	}

	return pubKey.String(), nil
}

// =============================================================================
// Multisig Support (BIP48)
// =============================================================================

// DeriveBIP48Key derives a key at the specified BIP48 path with change and index
func (w *HDWallet) DeriveBIP48Key(path BIP48Path, change, index uint32) (*hdkeychain.ExtendedKey, error) {
	key := w.masterKey

	// Derive: m/48'/coin'/account'/script'/change/index
	indices := []uint32{
		hdkeychain.HardenedKeyStart + 48, // BIP48 purpose
		hdkeychain.HardenedKeyStart + path.CoinType,
		hdkeychain.HardenedKeyStart + path.Account,
		hdkeychain.HardenedKeyStart + uint32(path.ScriptType), // #nosec G115 -- BIP48 script types are 1 or 2
		change,
		index,
	}

	var err error
	for _, idx := range indices {
		key, err = key.Derive(idx)
		if err != nil {
			return nil, fmt.Errorf("failed to derive BIP48 key: %w", err)
		}
	}

	return key, nil
}

// GetMultisigAccountXpub returns the extended public key at BIP48 account level
// Format: [fingerprint/48h/coin'h/account'h/script'h]xpub...
func (w *HDWallet) GetMultisigAccountXpub(path BIP48Path) (string, error) {
	key := w.masterKey

	// Derive to account level: m/48'/coin'/account'/script'
	indices := []uint32{
		hdkeychain.HardenedKeyStart + 48,
		hdkeychain.HardenedKeyStart + path.CoinType,
		hdkeychain.HardenedKeyStart + path.Account,
		hdkeychain.HardenedKeyStart + uint32(path.ScriptType), // #nosec G115 -- BIP48 script types are 1 or 2
	}

	var err error
	for _, idx := range indices {
		key, err = key.Derive(idx)
		if err != nil {
			return "", fmt.Errorf("failed to derive BIP48 account key: %w", err)
		}
	}

	// Neuter to get public key
	pubKey, err := key.Neuter()
	if err != nil {
		return "", fmt.Errorf("failed to neuter key: %w", err)
	}

	return pubKey.String(), nil
}

// GetXpubAtPath derives the extended public key at an arbitrary account-level path string.
// The path should be like "m/48'/1'/0'/2'" — all components after m/ are treated as hardened
// if they end with ' or h.
func (w *HDWallet) GetXpubAtPath(pathStr string) (string, error) {
	pathStr = strings.TrimPrefix(pathStr, "m/")
	parts := strings.Split(pathStr, "/")

	key := w.masterKey
	var err error
	for _, part := range parts {
		hardened := strings.HasSuffix(part, "'") || strings.HasSuffix(part, "h") || strings.HasSuffix(part, "H")
		numStr := strings.TrimRight(part, "'hH")
		val, parseErr := strconv.ParseUint(numStr, 10, 32)
		if parseErr != nil {
			return "", fmt.Errorf("invalid path component %q: %w", part, parseErr)
		}
		idx := uint32(val)
		if hardened {
			idx += hdkeychain.HardenedKeyStart
		}
		key, err = key.Derive(idx)
		if err != nil {
			return "", fmt.Errorf("failed to derive key at %s: %w", part, err)
		}
	}

	pubKey, err := key.Neuter()
	if err != nil {
		return "", fmt.Errorf("failed to neuter key: %w", err)
	}
	return pubKey.String(), nil
}

// Cosigner represents a cosigner in a multisig wallet
type Cosigner struct {
	Name        string // Human-readable name (e.g., "Hardware Wallet")
	Fingerprint string // 8 hex chars master key fingerprint
	Xpub        string // Extended public key at BIP48 account level
	IsMine      bool   // True if we have the private key
}

// MultisigConfig represents a multisig wallet configuration
type MultisigConfig struct {
	M          int                // Required signatures
	N          int                // Total cosigners
	ScriptType MultisigScriptType // 0=Legacy, 1=Nested, 2=Native
	Unsorted   bool               // multi(...) keeps the supplied key order
	Cosigners  []Cosigner
}

// orderedPublicKeys returns pubKeys in BIP67 order unless the descriptor
// used multi(...), whose key order is significant (H3-05).
func orderedPublicKeys(pubKeys [][]byte, sorted bool) [][]byte {
	if sorted {
		return SortPublicKeys(pubKeys)
	}
	out := make([][]byte, len(pubKeys))
	copy(out, pubKeys)
	return out
}

// SortPublicKeys sorts public keys lexicographically per BIP67
func SortPublicKeys(pubKeys [][]byte) [][]byte {
	sorted := make([][]byte, len(pubKeys))
	copy(sorted, pubKeys)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i], sorted[j]) < 0
	})
	return sorted
}

// BuildMultisigScript creates a multisig redeem script from sorted public keys
// Returns: OP_M <pubkey1> <pubkey2> ... <pubkeyN> OP_N OP_CHECKMULTISIG
func BuildMultisigScript(m int, sortedPubKeys [][]byte) ([]byte, error) {
	if m < 1 || m > len(sortedPubKeys) {
		return nil, fmt.Errorf("invalid M value: %d for %d keys", m, len(sortedPubKeys))
	}
	if len(sortedPubKeys) > 15 {
		return nil, fmt.Errorf("too many keys: %d (max 15)", len(sortedPubKeys))
	}

	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_1 - 1 + byte(m)) // #nosec G115 -- 1 <= m <= 15 checked above

	for _, pk := range sortedPubKeys {
		builder.AddData(pk)
	}

	builder.AddOp(txscript.OP_1 - 1 + byte(len(sortedPubKeys))) // #nosec G115 -- at most 15 keys checked above
	builder.AddOp(txscript.OP_CHECKMULTISIG)

	return builder.Script()
}

// GenerateP2SHMultisigAddress creates a P2SH multisig address (Legacy)
func GenerateP2SHMultisigAddress(m int, pubKeys [][]byte, network *chaincfg.Params) (string, []byte, error) {
	return generateP2SHMultisigAddress(m, pubKeys, true, network)
}

func generateP2SHMultisigAddress(m int, pubKeys [][]byte, sorted bool, network *chaincfg.Params) (string, []byte, error) {
	ordered := orderedPublicKeys(pubKeys, sorted)
	redeemScript, err := BuildMultisigScript(m, ordered)
	if err != nil {
		return "", nil, err
	}

	// P2SH: address = Base58Check(HASH160(redeemScript))
	// NewAddressScriptHash hashes the script internally — pass raw script, not pre-hashed
	addr, err := btcutil.NewAddressScriptHash(redeemScript, network)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create P2SH address: %w", err)
	}

	return addr.EncodeAddress(), redeemScript, nil
}

// GenerateP2SHWitnessMultisigAddress creates a P2SH-P2WSH multisig address (Nested SegWit)
func GenerateP2SHWitnessMultisigAddress(m int, pubKeys [][]byte, network *chaincfg.Params) (string, []byte, error) {
	return generateP2SHWitnessMultisigAddress(m, pubKeys, true, network)
}

func generateP2SHWitnessMultisigAddress(m int, pubKeys [][]byte, sorted bool, network *chaincfg.Params) (string, []byte, error) {
	ordered := orderedPublicKeys(pubKeys, sorted)
	witnessScript, err := BuildMultisigScript(m, ordered)
	if err != nil {
		return "", nil, err
	}

	// P2WSH: SHA256(witnessScript)
	witnessHash := sha256.Sum256(witnessScript)

	// Create the P2WSH address first
	witnessAddr, err := btcutil.NewAddressWitnessScriptHash(witnessHash[:], network)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create P2WSH address: %w", err)
	}

	// Get the witness program (0x00 0x20 <32-byte-hash>)
	witnessProgram, err := txscript.PayToAddrScript(witnessAddr)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create witness program: %w", err)
	}

	// Wrap in P2SH: address = Base58Check(HASH160(witnessProgram))
	// NewAddressScriptHash hashes the script internally — pass raw script, not pre-hashed
	addr, err := btcutil.NewAddressScriptHash(witnessProgram, network)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create P2SH-P2WSH address: %w", err)
	}

	return addr.EncodeAddress(), witnessScript, nil
}

// GenerateP2WSHMultisigAddress creates a P2WSH multisig address (Native SegWit)
func GenerateP2WSHMultisigAddress(m int, pubKeys [][]byte, network *chaincfg.Params) (string, []byte, error) {
	return generateP2WSHMultisigAddress(m, pubKeys, true, network)
}

func generateP2WSHMultisigAddress(m int, pubKeys [][]byte, sorted bool, network *chaincfg.Params) (string, []byte, error) {
	ordered := orderedPublicKeys(pubKeys, sorted)
	witnessScript, err := BuildMultisigScript(m, ordered)
	if err != nil {
		return "", nil, err
	}

	// P2WSH: SHA256(witnessScript)
	witnessHash := sha256.Sum256(witnessScript)

	addr, err := btcutil.NewAddressWitnessScriptHash(witnessHash[:], network)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create P2WSH address: %w", err)
	}

	return addr.EncodeAddress(), witnessScript, nil
}

// GenerateMultisigAddress generates a multisig address based on script type.
// sorted=false keeps the supplied key order, as multi(...) requires.
func GenerateMultisigAddress(config *MultisigConfig, pubKeys [][]byte, network *chaincfg.Params) (string, []byte, error) {
	sorted := !config.Unsorted
	switch config.ScriptType {
	case MultisigLegacy:
		return generateP2SHMultisigAddress(config.M, pubKeys, sorted, network)
	case MultisigNestedSegwit:
		return generateP2SHWitnessMultisigAddress(config.M, pubKeys, sorted, network)
	case MultisigNativeSegwit:
		return generateP2WSHMultisigAddress(config.M, pubKeys, sorted, network)
	default:
		return "", nil, fmt.Errorf("unsupported multisig script type: %d", config.ScriptType)
	}
}

// DeriveChildPubKey derives a child public key from an xpub at change/index
func DeriveChildPubKey(xpub string, change, index uint32, network *chaincfg.Params) ([]byte, error) {
	key, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, fmt.Errorf("invalid xpub: %w", err)
	}

	// Derive change level
	changeKey, err := key.Derive(change)
	if err != nil {
		return nil, fmt.Errorf("failed to derive change key: %w", err)
	}

	// Derive index level
	indexKey, err := changeKey.Derive(index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive index key: %w", err)
	}

	// Get public key
	pubKey, err := indexKey.ECPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	return pubKey.SerializeCompressed(), nil
}

// DeriveAddressFromXpub derives a Bitcoin address from an xpub at change/index
// This is used for watch-only wallets (hardware-backed or imported xpub)
func DeriveAddressFromXpub(xpub string, change, index uint32, addrType AddressType, network *chaincfg.Params) (string, error) {
	// Parse the xpub
	key, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return "", fmt.Errorf("invalid xpub: %w", err)
	}

	// Derive change level
	changeKey, err := key.Derive(change)
	if err != nil {
		return "", fmt.Errorf("failed to derive change key: %w", err)
	}

	// Derive index level
	indexKey, err := changeKey.Derive(index)
	if err != nil {
		return "", fmt.Errorf("failed to derive index key: %w", err)
	}

	// Get public key
	pubKey, err := indexKey.ECPubKey()
	if err != nil {
		return "", fmt.Errorf("failed to get public key: %w", err)
	}

	pubKeyBytes := pubKey.SerializeCompressed()

	// Generate address based on type
	var address btcutil.Address
	switch addrType {
	case AddressTypeLegacy:
		// P2PKH (Pay to Public Key Hash)
		btcPubKey, err := btcutil.NewAddressPubKey(pubKeyBytes, network)
		if err != nil {
			return "", fmt.Errorf("failed to create address from public key: %w", err)
		}
		address = btcPubKey.AddressPubKeyHash()

	case AddressTypeSegWitNative:
		// P2WPKH (Pay to Witness Public Key Hash) - bech32
		witnessProgram := btcutil.Hash160(pubKeyBytes)
		address, err = btcutil.NewAddressWitnessPubKeyHash(witnessProgram, network)
		if err != nil {
			return "", fmt.Errorf("failed to create witness address: %w", err)
		}

	case AddressTypeSegWitNested:
		// P2SH-P2WPKH (SegWit nested in P2SH)
		pubKeyHash := btcutil.Hash160(pubKeyBytes)
		redeemScript, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(pubKeyHash).
			Script()
		if err != nil {
			return "", fmt.Errorf("failed to create witness program: %w", err)
		}
		address, err = btcutil.NewAddressScriptHash(redeemScript, network)
		if err != nil {
			return "", fmt.Errorf("failed to create script hash address: %w", err)
		}

	case AddressTypeTaproot:
		// P2TR (Pay to Taproot) - bech32m
		// Normalize internal key to even Y per BIP340/BIP341
		xOnlyBytes := schnorr.SerializePubKey(pubKey)
		internalKey, err := schnorr.ParsePubKey(xOnlyBytes)
		if err != nil {
			return "", fmt.Errorf("failed to normalize taproot internal key: %w", err)
		}
		taprootKey := txscript.ComputeTaprootKeyNoScript(internalKey)
		address, err = btcutil.NewAddressTaproot(
			schnorr.SerializePubKey(taprootKey),
			network,
		)
		if err != nil {
			return "", fmt.Errorf("failed to create taproot address: %w", err)
		}

	default:
		return "", fmt.Errorf("unsupported address type: %d", addrType)
	}

	return address.EncodeAddress(), nil
}

// =============================================================================
// Taproot Support (BIP86)
// =============================================================================

// GetTaprootInternalKey returns the internal key for a Taproot address
// This is needed for signing Taproot transactions
func (w *HDWallet) GetTaprootInternalKey(path BIP44Path) (*btcec.PublicKey, error) {
	key, err := w.DeriveKey(path)
	if err != nil {
		return nil, err
	}

	pubKey, err := key.ECPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	return pubKey, nil
}

// GetTaprootOutputKey returns the tweaked output key for a Taproot address
// This is the key that appears in the scriptPubKey
func (w *HDWallet) GetTaprootOutputKey(path BIP44Path) (*btcec.PublicKey, error) {
	internalKey, err := w.GetTaprootInternalKey(path)
	if err != nil {
		return nil, err
	}

	// Normalize internal key to even Y per BIP340/BIP341
	// btcd's ComputeTaprootKeyNoScript does NOT normalize internally
	xOnlyBytes := schnorr.SerializePubKey(internalKey)
	normalizedKey, err := schnorr.ParsePubKey(xOnlyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize internal key: %w", err)
	}

	// Compute output key with no script tree (BIP86)
	outputKey := txscript.ComputeTaprootKeyNoScript(normalizedKey)

	return outputKey, nil
}
