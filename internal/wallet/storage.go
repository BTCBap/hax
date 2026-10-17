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

	"golang.org/x/crypto/pbkdf2"
)

const (
	// PBKDF2 parameters
	saltSize   = 32
	keySize    = 32
	pbkdf2Iter = 100000
	nonceSize  = 12
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
	M          int                      `json:"m"`           // Required signatures
	N          int                      `json:"n"`           // Total cosigners
	ScriptType int                      `json:"script_type"` // 0=Legacy, 1=Nested, 2=Native
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

// EncryptedWalletFile represents the encrypted wallet file format
type EncryptedWalletFile struct {
	Version int    `json:"version"`
	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"` // Encrypted WalletData
}

// WalletStorage handles secure wallet persistence
type WalletStorage struct {
	walletDir string
}

// NewWalletStorage creates a new wallet storage handler
func NewWalletStorage(walletDir string) (*WalletStorage, error) {
	// Create wallet directory if it doesn't exist
	if err := os.MkdirAll(walletDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create wallet directory: %w", err)
	}

	return &WalletStorage{
		walletDir: walletDir,
	}, nil
}

// SaveWallet encrypts and saves a wallet to disk under a fresh salt derived
// from password. Used when creating a wallet.
func (s *WalletStorage) SaveWallet(walletName string, data *WalletData, password string) error {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}
	pw := []byte(password)
	key := deriveKey(pw, salt)
	defer Wipe(key)
	defer Wipe(pw)
	return s.SaveWalletWithKey(walletName, data, salt, key)
}

// SaveWalletWithKey encrypts and saves a wallet with an already-derived key
// and its salt. A fresh random nonce is used on every save. This lets an
// unlocked session re-persist without retaining the password.
func (s *WalletStorage) SaveWalletWithKey(walletName string, data *WalletData, salt, key []byte) error {
	walletPath, err := s.walletPath(walletName)
	if err != nil {
		return err
	}
	if len(key) != keySize || len(salt) == 0 {
		return fmt.Errorf("wallet encryption key not available (wallet locked?)")
	}

	plaintext, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal wallet data: %w", err)
	}
	defer Wipe(plaintext)

	ciphertext, nonce, err := encrypt(plaintext, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt wallet: %w", err)
	}

	fileData, err := json.MarshalIndent(EncryptedWalletFile{
		Version: 1,
		Salt:    salt,
		Nonce:   nonce,
		Data:    ciphertext,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal encrypted wallet: %w", err)
	}

	// Write atomically with restricted permissions.
	tmp := walletPath + ".tmp"
	if err := os.WriteFile(tmp, fileData, 0600); err != nil {
		return fmt.Errorf("failed to write wallet file: %w", err)
	}
	if err := os.Rename(tmp, walletPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to write wallet file: %w", err)
	}
	return nil
}

// LoadWallet decrypts and loads a wallet from disk.
func (s *WalletStorage) LoadWallet(walletName string, password string) (*WalletData, error) {
	data, salt, key, err := s.LoadWalletKey(walletName, []byte(password))
	if err != nil {
		return nil, err
	}
	Wipe(key)
	Wipe(salt)
	return data, nil
}

// LoadWalletKey decrypts a wallet and also returns the salt and derived key so
// the caller can re-save without keeping the password. The caller owns key and
// must Wipe it when done. password is not retained.
func (s *WalletStorage) LoadWalletKey(walletName string, password []byte) (*WalletData, []byte, []byte, error) {
	walletPath, err := s.walletPath(walletName)
	if err != nil {
		return nil, nil, nil, err
	}

	fileData, err := os.ReadFile(walletPath) // #nosec G304 -- path built from a validated wallet name inside the wallet dir
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read wallet file: %w", err)
	}

	var encryptedWallet EncryptedWalletFile
	if err := json.Unmarshal(fileData, &encryptedWallet); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse wallet file: %w", err)
	}

	key := deriveKey(password, encryptedWallet.Salt)

	plaintext, err := decrypt(encryptedWallet.Data, encryptedWallet.Nonce, key)
	if err != nil {
		Wipe(key)
		return nil, nil, nil, fmt.Errorf("failed to decrypt wallet (incorrect password?): %w", err)
	}
	defer Wipe(plaintext)

	var walletData WalletData
	if err := json.Unmarshal(plaintext, &walletData); err != nil {
		Wipe(key)
		return nil, nil, nil, fmt.Errorf("failed to parse wallet data: %w", err)
	}

	return &walletData, encryptedWallet.Salt, key, nil
}

func deriveKey(password, salt []byte) []byte {
	return pbkdf2.Key(password, salt, pbkdf2Iter, keySize, sha256.New)
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

// encrypt encrypts data using AES-GCM
func encrypt(plaintext []byte, key []byte) (ciphertext []byte, nonce []byte, err error) {
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
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// decrypt decrypts data using AES-GCM
func decrypt(ciphertext []byte, nonce []byte, key []byte) (plaintext []byte, err error) {
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
	plaintext, err = gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
