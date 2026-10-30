package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/fsutil"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize  = 32
	keySize   = 32
	nonceSize = 12

	// Current wallet file format. Version 1 used PBKDF2-HMAC-SHA256 with
	// 100,000 iterations and no associated data. Version 2 uses Argon2id
	// and authenticates the KDF metadata as AEAD associated data.
	walletFormatVersion   = 2
	legacyFormatVersion   = 1
	kdfArgon2id           = "argon2id"
	argon2Time            = 3
	argon2MemoryKiB       = 64 * 1024
	argon2Threads         = 4
	legacyPBKDF2Iter      = 100000
	minArgon2MemoryKiB    = 8 * 1024
	maxArgon2MemoryKiB    = 512 * 1024
	maxArgon2Time         = 10
	maxArgon2Threads      = 16
	walletAssociatedData  = "hax-wallet"
	walletDataFormatVers  = 1
	walletEncryptedSuffix = ".wallet"
)

// WalletData represents the data stored in a wallet file
type WalletData struct {
	Version  int    `json:"version"`
	Name     string `json:"name"`
	Mnemonic string `json:"mnemonic"` // Encrypted (empty for watch-only multisig)
	// Optional BIP39 passphrase ("25th word"). Stored only inside the encrypted file.
	Passphrase        string            `json:"passphrase,omitempty"`
	CreatedAt         int64             `json:"created_at"`
	Network           string            `json:"network"` // "mainnet" or "testnet"
	Accounts          []AccountData     `json:"accounts"`
	TransactionLabels map[string]string `json:"transaction_labels,omitempty"` // TxID -> Label mapping (BIP-329)
	// FrozenOutputs holds "txid:vout" of coins the wallet must not spend.
	FrozenOutputs map[string]bool `json:"frozen_outputs,omitempty"`

	// Multisig configuration (nil for single-sig wallets)
	Multisig *MultisigWalletConfig `json:"multisig,omitempty"`

	// Hardware wallet configuration (nil for mnemonic-based wallets)
	// Used for single-sig watch-only wallets backed by hardware devices
	HardwareWallet *HardwareWalletConfig `json:"hardware_wallet,omitempty"`

	// Persistent cache (loaded instantly on unlock)
	Cache *WalletDataCache `json:"cache,omitempty"`
}

// MultisigWalletConfig stores multisig wallet configuration
type MultisigWalletConfig struct {
	M          int                      `json:"m"`                  // Required signatures
	N          int                      `json:"n"`                  // Total cosigners
	ScriptType int                      `json:"script_type"`        // 0=Legacy, 1=Nested, 2=Native
	Unsorted   bool                     `json:"unsorted,omitempty"` // multi(...) keeps descriptor key order
	Cosigners  []MultisigCosignerConfig `json:"cosigners"`
}

// MultisigCosignerConfig stores a cosigner's information
type MultisigCosignerConfig struct {
	Name             string `json:"name"`                        // Human-readable name
	Fingerprint      string `json:"fingerprint"`                 // 8 hex char master key fingerprint
	KeyOrigin        string `json:"key_origin"`                  // Full origin path e.g. "0f056943/48h/1h/0h/2h"
	Xpub             string `json:"xpub"`                        // Extended public key at BIP48 account level
	IsMine           bool   `json:"is_mine"`                     // True if we have the private key
	Mnemonic         string `json:"mnemonic,omitempty"`          // Seed phrase if we can sign locally (encrypted with wallet)
	DeviceType       string `json:"device_type,omitempty"`       // "local", "coldcard", "trezor", "ledger", etc.
	ActualDerivation string `json:"actual_derivation,omitempty"` // The real derivation path used (may differ from KeyOrigin if user pasted wrong xpub)
}

// HardwareWalletConfig stores configuration for single-sig hardware-backed wallets
// These are watch-only wallets that derive addresses from an xpub but require
// a hardware device for signing transactions
type HardwareWalletConfig struct {
	Fingerprint string `json:"fingerprint"`  // 8 hex char master key fingerprint
	KeyOrigin   string `json:"key_origin"`   // Full derivation path e.g. "0f056943/84h/1h/0h"
	Xpub        string `json:"xpub"`         // Extended public key at account level
	DeviceType  string `json:"device_type"`  // "trezor", "ledger", "coldcard", etc.
	AddressType string `json:"address_type"` // "legacy", "segwit_nested", "segwit_native", "taproot"
}

// WalletDataCache stores cached blockchain data that persists to disk
// This enables instant display on wallet open without network calls
type WalletDataCache struct {
	// Cached transactions - instantly available on unlock
	Transactions []CachedTransaction `json:"transactions,omitempty"`

	// Cached UTXOs
	UTXOs []CachedUTXO `json:"utxos,omitempty"`

	// Last synced block height - enables delta sync
	LastSyncHeight int64 `json:"last_sync_height"`

	// Block timestamp cache (heights never change timestamps)
	BlockTimestamps map[int64]int64 `json:"block_timestamps,omitempty"`

	// Last full sync time
	LastSyncTime int64 `json:"last_sync_time"`
}

// CachedTransaction stores transaction data for persistence
type CachedTransaction struct {
	TxID        string `json:"txid"`
	BlockHeight int64  `json:"block_height"`
	BlockTime   int64  `json:"block_time"`
	Confirmed   bool   `json:"confirmed"`

	// Net effect on wallet (positive = received, negative = sent)
	NetAmount api.Satoshi `json:"net_amount"`
	Fee       api.Satoshi `json:"fee"`
	Type      string      `json:"type"` // "receive", "send", "self"

	// Addresses involved (for display)
	FromAddresses []string `json:"from_addresses,omitempty"`
	ToAddresses   []string `json:"to_addresses,omitempty"`
}

// CachedUTXO stores UTXO data for persistence
type CachedUTXO struct {
	TxID        string      `json:"txid"`
	Vout        uint32      `json:"vout"`
	Value       api.Satoshi `json:"value"`
	Address     string      `json:"address"`
	BlockHeight int64       `json:"block_height"`
	Confirmed   bool        `json:"confirmed"`
}

// AccountData represents a single account in the wallet
type AccountData struct {
	Index                uint32            `json:"index"`
	Name                 string            `json:"name"`
	AddressType          string            `json:"address_type"`                     // "legacy", "segwit_nested", "segwit_native", "taproot"
	CustomDerivationPath string            `json:"custom_derivation_path,omitempty"` // Optional override e.g. "m/84'/0'/3'" (empty = standard BIP path)
	Addresses            []AddressData     `json:"addresses"`
	Labels               map[string]string `json:"labels,omitempty"` // Address -> Label mapping
}

// AddressData represents a derived address
type AddressData struct {
	Index   uint32 `json:"index"`
	Address string `json:"address"`
	Path    string `json:"path"`
	Change  bool   `json:"change"` // true for change addresses
	Used    bool   `json:"used"`
}

// EncryptedWalletFile represents the encrypted wallet file format.
//
// Version 1 files (PBKDF2, no KDF fields) are still accepted. New files use
// Version 2 with Argon2id parameters stored alongside the ciphertext.
type EncryptedWalletFile struct {
	Version        int    `json:"version"`
	KDF            string `json:"kdf,omitempty"`
	KDFTime        uint32 `json:"kdf_time,omitempty"`
	KDFMemory      uint32 `json:"kdf_memory,omitempty"`      // KiB
	KDFParallelism uint8  `json:"kdf_parallelism,omitempty"` // threads
	Salt           []byte `json:"salt"`
	Nonce          []byte `json:"nonce"`
	Data           []byte `json:"data"` // Encrypted WalletData
}

// WalletKey is a derived wallet encryption key plus the exact KDF metadata
// needed to re-save the file without changing its format.
type WalletKey struct {
	Salt        []byte
	Key         []byte
	Version     int
	KDF         string
	Time        uint32
	Memory      uint32 // KiB
	Parallelism uint8
}

func defaultWalletKey(salt, key []byte) *WalletKey {
	return &WalletKey{
		Salt:        salt,
		Key:         key,
		Version:     walletFormatVersion,
		KDF:         kdfArgon2id,
		Time:        argon2Time,
		Memory:      argon2MemoryKiB,
		Parallelism: argon2Threads,
	}
}

// Wipe overwrites the key material.
func (k *WalletKey) Wipe() {
	if k == nil {
		return
	}
	Wipe(k.Key)
	Wipe(k.Salt)
}

// WalletStorage handles secure wallet persistence
type WalletStorage struct {
	walletDir string
}

// NewWalletStorage creates a new wallet storage handler
func NewWalletStorage(walletDir string) (*WalletStorage, error) {
	// Create the wallet directory owner-only and tighten it if it already
	// existed with looser permissions (H3-17).
	if err := fsutil.EnsurePrivateDir(walletDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create wallet directory: %w", err)
	}

	return &WalletStorage{
		walletDir: walletDir,
	}, nil
}

// SaveWallet encrypts and saves a wallet to disk under a fresh Argon2id salt
// derived from password. Used when creating a wallet.
func (s *WalletStorage) SaveWallet(walletName string, data *WalletData, password string) error {
	pw := []byte(password)
	defer Wipe(pw)
	return s.saveWalletBytes(walletName, data, pw)
}

func (s *WalletStorage) saveWalletBytes(walletName string, data *WalletData, password []byte) error {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}
	key := deriveKey(password, salt, kdfArgon2id, argon2Time, argon2MemoryKiB, argon2Threads)
	defer Wipe(key)
	return s.SaveWalletWithKeyInfo(walletName, data, defaultWalletKey(salt, key))
}

// SaveWalletWithKey encrypts and saves a wallet with an already-derived key
// and its salt. A fresh random nonce is used on every save. This lets an
// unlocked session re-persist without retaining the password.
//
// The key is assumed to use the current Argon2id parameters. Unlocked
// sessions are transparently migrated to those parameters, so this holds
// for every key obtained through LoadWalletKey.
func (s *WalletStorage) SaveWalletWithKey(walletName string, data *WalletData, salt, key []byte) error {
	return s.SaveWalletWithKeyInfo(walletName, data, defaultWalletKey(salt, key))
}

// SaveWalletWithKeyInfo is SaveWalletWithKey with explicit KDF metadata, so
// a wallet that could not be migrated keeps its original parameters.
func (s *WalletStorage) SaveWalletWithKeyInfo(walletName string, data *WalletData, wk *WalletKey) error {
	walletPath, err := s.walletPath(walletName)
	if err != nil {
		return err
	}
	if wk == nil || len(wk.Key) != keySize || len(wk.Salt) == 0 {
		return fmt.Errorf("wallet encryption key not available (wallet locked?)")
	}
	if data.Version == 0 {
		data.Version = walletDataFormatVers
	}

	plaintext, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal wallet data: %w", err)
	}
	defer Wipe(plaintext)

	enc := EncryptedWalletFile{
		Version:        wk.Version,
		KDF:            wk.KDF,
		KDFTime:        wk.Time,
		KDFMemory:      wk.Memory,
		KDFParallelism: wk.Parallelism,
		Salt:           wk.Salt,
	}
	ciphertext, nonce, err := encrypt(plaintext, wk.Key, walletAAD(&enc))
	if err != nil {
		return fmt.Errorf("failed to encrypt wallet: %w", err)
	}
	enc.Nonce = nonce
	enc.Data = ciphertext

	fileData, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal encrypted wallet: %w", err)
	}

	// Write atomically with restricted permissions: temp file in the same
	// directory, fsync, rename, fsync the directory (H3-16).
	if err := fsutil.WriteFileAtomic(walletPath, fileData, 0o600); err != nil {
		return fmt.Errorf("failed to write wallet file: %w", err)
	}
	return nil
}

// LoadWallet decrypts and loads a wallet from disk.
func (s *WalletStorage) LoadWallet(walletName string, password string) (*WalletData, error) {
	data, wk, err := s.LoadWalletKeyInfo(walletName, []byte(password))
	if err != nil {
		return nil, err
	}
	wk.Wipe()
	return data, nil
}

// LoadWalletKey decrypts a wallet and also returns the salt and derived key so
// the caller can re-save without keeping the password. The caller owns key and
// must Wipe it when done. password is not retained.
func (s *WalletStorage) LoadWalletKey(walletName string, password []byte) (*WalletData, []byte, []byte, error) {
	data, wk, err := s.LoadWalletKeyInfo(walletName, password)
	if err != nil {
		return nil, nil, nil, err
	}
	return data, wk.Salt, wk.Key, nil
}

// LoadWalletKeyInfo decrypts a wallet, validates the file format, and
// transparently re-encrypts old-format files with the current Argon2id
// parameters after a successful unlock. If re-encryption fails the wallet
// still opens with its original parameters.
func (s *WalletStorage) LoadWalletKeyInfo(walletName string, password []byte) (*WalletData, *WalletKey, error) {
	walletPath, err := s.walletPath(walletName)
	if err != nil {
		return nil, nil, err
	}

	// Tighten a wallet file left group/world-readable by an older version,
	// a restore, or a permissive umask (H3-17).
	if err := fsutil.TightenFile(walletPath, 0o600); err != nil {
		return nil, nil, fmt.Errorf("failed to secure wallet file: %w", err)
	}

	fileData, err := os.ReadFile(walletPath) // #nosec G304 -- path built from a validated wallet name inside the wallet dir
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read wallet file: %w", err)
	}

	var encryptedWallet EncryptedWalletFile
	if err := json.Unmarshal(fileData, &encryptedWallet); err != nil {
		return nil, nil, fmt.Errorf("failed to parse wallet file: %w", err)
	}

	wk, err := validateEncryptedWallet(&encryptedWallet)
	if err != nil {
		return nil, nil, err
	}
	wk.Key = deriveKey(password, encryptedWallet.Salt, wk.KDF, wk.Time, wk.Memory, wk.Parallelism)

	plaintext, err := decrypt(encryptedWallet.Data, encryptedWallet.Nonce, wk.Key, walletAAD(&encryptedWallet))
	if err != nil {
		wk.Wipe()
		return nil, nil, fmt.Errorf("failed to decrypt wallet (incorrect password?): %w", err)
	}
	defer Wipe(plaintext)

	var walletData WalletData
	if err := json.Unmarshal(plaintext, &walletData); err != nil {
		wk.Wipe()
		return nil, nil, fmt.Errorf("failed to parse wallet data: %w", err)
	}
	// Files written before the field was populated decode as zero; treat
	// that as the current data version so old wallets keep opening.
	if walletData.Version == 0 {
		walletData.Version = walletDataFormatVers
	}
	if walletData.Version != walletDataFormatVers {
		wk.Wipe()
		return nil, nil, fmt.Errorf("unsupported wallet data version %d", walletData.Version)
	}

	// Old files open normally and are re-encrypted with the stronger KDF.
	if needsKDFUpgrade(&encryptedWallet) {
		if upgraded, upgradeErr := s.reencrypt(walletName, &walletData, password); upgradeErr == nil {
			wk.Wipe()
			wk = upgraded
		}
	}

	return &walletData, wk, nil
}

// validateEncryptedWallet checks the format, version, KDF parameters and
// nonce/salt lengths before any key derivation (H3-43).
func validateEncryptedWallet(enc *EncryptedWalletFile) (*WalletKey, error) {
	if enc == nil {
		return nil, fmt.Errorf("invalid wallet file")
	}
	switch enc.Version {
	case legacyFormatVersion:
		if len(enc.Salt) != saltSize {
			return nil, fmt.Errorf("invalid wallet salt length %d", len(enc.Salt))
		}
		if len(enc.Nonce) != nonceSize {
			return nil, fmt.Errorf("invalid wallet nonce length %d", len(enc.Nonce))
		}
		return &WalletKey{
			Salt:    enc.Salt,
			Version: legacyFormatVersion,
		}, nil
	case walletFormatVersion:
		if enc.KDF != kdfArgon2id {
			return nil, fmt.Errorf("unsupported wallet KDF %q", enc.KDF)
		}
		if len(enc.Salt) != saltSize {
			return nil, fmt.Errorf("invalid wallet salt length %d", len(enc.Salt))
		}
		if len(enc.Nonce) != nonceSize {
			return nil, fmt.Errorf("invalid wallet nonce length %d", len(enc.Nonce))
		}
		if enc.KDFTime < 1 || enc.KDFTime > maxArgon2Time {
			return nil, fmt.Errorf("unsupported wallet KDF time %d", enc.KDFTime)
		}
		if enc.KDFMemory < minArgon2MemoryKiB || enc.KDFMemory > maxArgon2MemoryKiB {
			return nil, fmt.Errorf("unsupported wallet KDF memory %d KiB", enc.KDFMemory)
		}
		if enc.KDFParallelism < 1 || enc.KDFParallelism > maxArgon2Threads {
			return nil, fmt.Errorf("unsupported wallet KDF parallelism %d", enc.KDFParallelism)
		}
		return &WalletKey{
			Salt:        enc.Salt,
			Version:     walletFormatVersion,
			KDF:         enc.KDF,
			Time:        enc.KDFTime,
			Memory:      enc.KDFMemory,
			Parallelism: enc.KDFParallelism,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported wallet file version %d", enc.Version)
	}
}

func needsKDFUpgrade(enc *EncryptedWalletFile) bool {
	return enc.Version != walletFormatVersion ||
		enc.KDF != kdfArgon2id ||
		enc.KDFTime < argon2Time ||
		enc.KDFMemory < argon2MemoryKiB ||
		enc.KDFParallelism != argon2Threads
}

// reencrypt writes data with fresh Argon2id parameters and returns the new key.
func (s *WalletStorage) reencrypt(walletName string, data *WalletData, password []byte) (*WalletKey, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := deriveKey(password, salt, kdfArgon2id, argon2Time, argon2MemoryKiB, argon2Threads)
	wk := defaultWalletKey(salt, key)
	if err := s.SaveWalletWithKeyInfo(walletName, data, wk); err != nil {
		wk.Wipe()
		return nil, err
	}
	return wk, nil
}

// walletAAD binds the format version and KDF metadata to the ciphertext
// (H3-44). Legacy version 1 files were sealed without associated data.
func walletAAD(enc *EncryptedWalletFile) []byte {
	if enc == nil || enc.Version == legacyFormatVersion {
		return nil
	}
	return []byte(fmt.Sprintf("%s|version=%d|kdf=%s|time=%d|memory=%d|parallelism=%d|salt=%x",
		walletAssociatedData, enc.Version, enc.KDF, enc.KDFTime, enc.KDFMemory,
		enc.KDFParallelism, enc.Salt))
}

// deriveKey derives a 32-byte encryption key with the given KDF. Unknown
// KDFs fall back to the legacy PBKDF2 parameters only for version 1 files,
// which validateEncryptedWallet enforces.
func deriveKey(password, salt []byte, kdf string, t, m uint32, p uint8) []byte {
	if kdf == kdfArgon2id {
		return argon2.IDKey(password, salt, t, m, p, keySize)
	}
	return pbkdf2.Key(password, salt, legacyPBKDF2Iter, keySize, sha256.New)
}

// WalletExists checks if a wallet file exists
func (s *WalletStorage) WalletExists(walletName string) bool {
	walletPath, err := s.walletPath(walletName)
	if err != nil {
		return false
	}
	_, err = os.Stat(walletPath)
	return err == nil
}

// ListWallets returns a list of all wallet names in the wallet directory
func (s *WalletStorage) ListWallets() ([]string, error) {
	files, err := os.ReadDir(s.walletDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read wallet directory: %w", err)
	}

	var wallets []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".wallet" {
			name := file.Name()[:len(file.Name())-7] // Remove .wallet extension
			wallets = append(wallets, name)
		}
	}

	return wallets, nil
}

// DeleteWallet removes a wallet file
func (s *WalletStorage) DeleteWallet(walletName string) error {
	walletPath, err := s.walletPath(walletName)
	if err != nil {
		return err
	}
	if err := os.Remove(walletPath); err != nil {
		return fmt.Errorf("failed to delete wallet: %w", err)
	}
	return nil
}

// walletPath returns the on-disk path for a wallet, rejecting any name that
// could escape the wallet directory.
func (s *WalletStorage) walletPath(walletName string) (string, error) {
	if err := ValidateWalletName(walletName); err != nil {
		return "", err
	}
	p := filepath.Join(s.walletDir, walletName+".wallet")
	rel, err := filepath.Rel(s.walletDir, p)
	if err != nil || rel != filepath.Base(p) {
		return "", fmt.Errorf("invalid wallet name %q", walletName)
	}
	return p, nil
}

// encrypt encrypts data using AES-GCM with optional associated data.
func encrypt(plaintext []byte, key []byte, aad []byte) (ciphertext []byte, nonce []byte, err error) {
	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext = gcm.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nonce, nil
}

// decrypt decrypts data using AES-GCM with optional associated data.
func decrypt(ciphertext []byte, nonce []byte, key []byte, aad []byte) (plaintext []byte, err error) {
	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err = gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
