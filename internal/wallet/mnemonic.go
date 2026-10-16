package wallet

import (
	"crypto/rand"
	"fmt"

	"github.com/tyler-smith/go-bip39"
)

// MnemonicStrength represents the entropy bits for mnemonic generation
type MnemonicStrength int

const (
	// Mnemonic12Words provides 128 bits of entropy (12 words)
	Mnemonic12Words MnemonicStrength = 128
	// Mnemonic15Words provides 160 bits of entropy (15 words)
	Mnemonic15Words MnemonicStrength = 160
	// Mnemonic18Words provides 192 bits of entropy (18 words)
	Mnemonic18Words MnemonicStrength = 192
	// Mnemonic21Words provides 224 bits of entropy (21 words)
	Mnemonic21Words MnemonicStrength = 224
	// Mnemonic24Words provides 256 bits of entropy (24 words)
	Mnemonic24Words MnemonicStrength = 256
)

// GenerateMnemonic creates a new BIP39 mnemonic seed phrase
// Uses cryptographically secure random number generation
func GenerateMnemonic(strength MnemonicStrength) (string, error) {
	// Generate entropy
	entropy, err := generateEntropy(int(strength))
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	defer Wipe(entropy)

	// Convert entropy to mnemonic
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to create mnemonic: %w", err)
	}

	return mnemonic, nil
}

// generateEntropy creates cryptographically secure random entropy
func generateEntropy(bitSize int) ([]byte, error) {
	if bitSize%32 != 0 || bitSize < 128 || bitSize > 256 {
		return nil, fmt.Errorf("invalid bit size: must be 128, 160, 192, 224, or 256")
	}

	byteSize := bitSize / 8
	entropy := make([]byte, byteSize)

	// Use crypto/rand for cryptographically secure randomness
	_, err := rand.Read(entropy)
	if err != nil {
		return nil, fmt.Errorf("failed to read random bytes: %w", err)
	}

	return entropy, nil
}

// ValidateMnemonic checks if a mnemonic phrase is valid according to BIP39
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// MnemonicToSeed converts a mnemonic phrase to a BIP39 seed
// The seed is used for BIP32 hierarchical deterministic wallet generation
// Password can be empty string for no passphrase
func MnemonicToSeed(mnemonic string, password string) ([]byte, error) {
	if !ValidateMnemonic(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// BIP39 seed generation with optional passphrase
	seed := bip39.NewSeed(mnemonic, password)
	return seed, nil
}

// GetWordCount returns the number of words in a mnemonic based on entropy strength
func GetWordCount(strength MnemonicStrength) int {
	switch strength {
	case Mnemonic12Words:
		return 12
	case Mnemonic15Words:
		return 15
	case Mnemonic18Words:
		return 18
	case Mnemonic21Words:
		return 21
	case Mnemonic24Words:
		return 24
	default:
		return 0
	}
}
