package wallet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

// Wallet file format versions.
//
//	1: PBKDF2-SHA256, 100,000 iterations. Read-only; upgraded to v2 on unlock.
//	2: Argon2id with parameters stored in the file and bound to the
//	   ciphertext as AES-GCM additional data.
const (
	fileVersionPBKDF2   = 1
	fileVersionArgon2id = 2
	currentFileVersion  = fileVersionArgon2id
)

const (
	kdfArgon2id = "argon2id"

	// Legacy v1 parameters.
	pbkdf2Iter = 100000
)

// KDFParams describes how the wallet key is derived from the password.
type KDFParams struct {
	Name    string `json:"name"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory_kib"`
	Threads uint8  `json:"threads"`
}

// defaultKDF follows the second recommended option of RFC 9106 (section 4):
// Argon2id, t=3, m=64 MiB, p=4.
var defaultKDF = KDFParams{Name: kdfArgon2id, Time: 3, Memory: 64 * 1024, Threads: 4}

// Bounds applied to parameters read from disk, so a tampered or corrupt file
// can neither weaken the KDF below a sane floor nor make unlock allocate
// unbounded memory.
const (
	minArgonTime    = 1
	maxArgonTime    = 16
	minArgonMemory  = 16 * 1024       // 16 MiB
	maxArgonMemory  = 2 * 1024 * 1024 // 2 GiB
	minArgonThreads = 1
	maxArgonThreads = 64
	minSaltSize     = 16
	maxSaltSize     = 64
)

func (p KDFParams) validate() error {
	if p.Name != kdfArgon2id {
		return fmt.Errorf("unsupported key derivation %q", p.Name)
	}
	if p.Time < minArgonTime || p.Time > maxArgonTime {
		return fmt.Errorf("argon2id time cost %d out of range", p.Time)
	}
	if p.Memory < minArgonMemory || p.Memory > maxArgonMemory {
		return fmt.Errorf("argon2id memory %d KiB out of range", p.Memory)
	}
	if p.Threads < minArgonThreads || p.Threads > maxArgonThreads {
		return fmt.Errorf("argon2id parallelism %d out of range", p.Threads)
	}
	return nil
}

func (p KDFParams) derive(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, p.Time, p.Memory, p.Threads, keySize)
}

// associatedData binds the file version, KDF parameters and salt to the
// ciphertext, so editing any of them makes decryption fail instead of
// silently changing how the key is derived.
func associatedData(version int, p KDFParams, salt []byte) []byte {
	return fmt.Appendf(nil, "hax-wallet|v%d|%s|t=%d|m=%d|p=%d|salt=%s",
		version, p.Name, p.Time, p.Memory, p.Threads, hex.EncodeToString(salt))
}

// fileKey derives the decryption key and AES-GCM additional data for an
// encrypted wallet file, rejecting unknown versions and out-of-range
// parameters before doing any expensive work.
func fileKey(f *EncryptedWalletFile, password []byte) (key, aad []byte, err error) {
	switch f.Version {
	case fileVersionPBKDF2:
		if len(f.Salt) == 0 {
			return nil, nil, fmt.Errorf("wallet file has no salt")
		}
		return pbkdf2.Key(password, f.Salt, pbkdf2Iter, keySize, sha256.New), nil, nil
	case fileVersionArgon2id:
		if f.KDF == nil {
			return nil, nil, fmt.Errorf("wallet file is missing key derivation parameters")
		}
		if err := f.KDF.validate(); err != nil {
			return nil, nil, err
		}
		if len(f.Salt) < minSaltSize || len(f.Salt) > maxSaltSize {
			return nil, nil, fmt.Errorf("wallet file salt length %d out of range", len(f.Salt))
		}
		return f.KDF.derive(password, f.Salt), associatedData(f.Version, *f.KDF, f.Salt), nil
	default:
		if f.Version > currentFileVersion {
			return nil, nil, fmt.Errorf("wallet file version %d is newer than this hax supports; please upgrade", f.Version)
		}
		return nil, nil, fmt.Errorf("unsupported wallet file version %d", f.Version)
	}
}

// isCurrent reports whether f already uses the current format and default
// parameters, i.e. needs no upgrade.
func (f *EncryptedWalletFile) isCurrent() bool {
	return f.Version == currentFileVersion && f.KDF != nil && *f.KDF == defaultKDF && len(f.Salt) == saltSize
}
