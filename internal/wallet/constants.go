package wallet

import "hax/internal/api"

// Bitcoin protocol constants
const (
	// Dust limits by script type
	// These are the minimum economical output values - below this, outputs
	// are considered "dust" and may be rejected by nodes.
	// Formula: (input_size * 3 + output_size) * minRelayFeeRate
	// With minRelayFeeRate = 1 sat/vB (conservative estimate)
	DustLimitP2PKH  api.Satoshi = 546 // Legacy (1-prefix)
	DustLimitP2SH   api.Satoshi = 540 // Script hash (3-prefix)
	DustLimitP2WPKH api.Satoshi = 294 // Native SegWit (bc1q)
	DustLimitP2WSH  api.Satoshi = 330 // Witness Script Hash (bc1q, multisig)
	DustLimitP2TR   api.Satoshi = 330 // Taproot (bc1p)

	// DustLimitSats is the default/legacy dust limit (kept for backwards compatibility)
	DustLimitSats api.Satoshi = 546

	// AddressGapLimit is the number of consecutive unused addresses
	// to scan before stopping (BIP44 standard is 20)
	AddressGapLimit = 20

	// Transaction size constants for P2WPKH (native SegWit)
	InputVSizeP2WPKH  = 68 // vBytes per P2WPKH input
	OutputVSizeP2WPKH = 31 // vBytes per P2WPKH output
	TxOverheadVSize   = 11 // Transaction overhead vBytes

	// SatsPerBTC is the number of satoshis in one bitcoin
	SatsPerBTC api.Satoshi = 100_000_000
)

// ScriptType represents different Bitcoin script types for transaction size estimation
type ScriptType int

const (
	ScriptTypeP2PKH      ScriptType = iota // Legacy single-sig (BIP44)
	ScriptTypeP2SHP2WPKH                   // Nested SegWit single-sig (BIP49)
	ScriptTypeP2WPKH                       // Native SegWit single-sig (BIP84)
	ScriptTypeP2TR                         // Taproot single-sig (BIP86)
	ScriptTypeP2SH                         // Legacy multisig
	ScriptTypeP2SHP2WSH                    // Nested SegWit multisig
	ScriptTypeP2WSH                        // Native SegWit multisig (BIP48)
)

// Input vSize by script type (single-sig)
const (
	InputVSizeP2PKH      = 148 // Legacy P2PKH input (~148 vB)
	InputVSizeP2SHP2WPKH = 91  // Nested SegWit P2SH-P2WPKH input (~91 vB)
	// InputVSizeP2WPKH = 68 (already defined above)
	InputVSizeP2TR = 58 // Taproot P2TR input (~58 vB)
)

// Output vSize by script type
const (
	OutputVSizeP2PKH = 34 // Legacy P2PKH output
	OutputVSizeP2SH  = 32 // P2SH output (nested SegWit or legacy multisig)
	// OutputVSizeP2WPKH = 31 (already defined above)
	OutputVSizeP2WSH = 43 // P2WSH output (native SegWit multisig)
	OutputVSizeP2TR  = 43 // Taproot P2TR output
)

// GetDustLimit returns the appropriate dust limit for an address
func GetDustLimit(address string) api.Satoshi {
	if len(address) == 0 {
		return DustLimitSats
	}

	// Check address prefix to determine type
	switch {
	case address[0] == '1' || address[0] == 'm' || address[0] == 'n':
		// Legacy P2PKH (mainnet 1, testnet m/n)
		return DustLimitP2PKH
	case address[0] == '3' || address[0] == '2':
		// P2SH (mainnet 3, testnet 2)
		return DustLimitP2SH
	case len(address) >= 4 && (address[:4] == "bc1q" || address[:4] == "tb1q"):
		// Native SegWit - could be P2WPKH or P2WSH
		// P2WPKH addresses are 42 chars, P2WSH are 62 chars
		if len(address) > 50 {
			return DustLimitP2WSH // Longer address = P2WSH (multisig)
		}
		return DustLimitP2WPKH
	case len(address) >= 4 && (address[:4] == "bc1p" || address[:4] == "tb1p"):
		// Taproot
		return DustLimitP2TR
	default:
		return DustLimitSats
	}
}

// Sync timing constants
const (
	// BackgroundSyncInterval is the time between background cache refreshes
	BackgroundSyncInterval = 30 // seconds
)

// Bitcoin script opcodes used in multisig and witness scripts
const (
	// Number opcodes (OP_1 through OP_16 for multisig M-of-N values)
	OP1  byte = 0x51 // Push number 1
	OP16 byte = 0x60 // Push number 16

	// OPSmallIntBase is subtracted from OP_N to get the integer value
	// e.g., OP_3 (0x53) - 0x50 = 3
	OPSmallIntBase byte = 0x50

	// Data push opcodes
	OPPushBytes33 byte = 0x21 // Push 33 bytes (compressed pubkey)

	// Crypto opcodes
	OPCheckMultiSig byte = 0xAE // Verify multisig
)
