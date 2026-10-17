package wallet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
)

type WalletManager struct {
	storage *WalletStorage
	backend backend.BitcoinBackend

	wallet   *HDWallet
	data     *WalletData
	unlocked bool

	// encKey/encSalt are the AES key derived from the wallet password at
	// unlock, kept so the session can re-persist. The password itself is
	// never retained. Both are wiped on lock. Guarded by keyMu.
	encKey []byte

	netMu       sync.Mutex
	netVerified bool // backend genesis matched wallet network
	encSalt     []byte
	keyMu       sync.RWMutex

	// dataMu protects m.data and m.data.Accounts.
	dataMu sync.RWMutex

	state *WalletState

	cache *WalletCache

	syncStopChan chan struct{}
	syncMu       sync.Mutex
	syncWg       sync.WaitGroup

	refreshing int32

	scanCancel context.CancelFunc
	scanWg     sync.WaitGroup

	persistWg sync.WaitGroup

	gapLimit int
}

// NewWalletManager creates a new wallet manager with the specified backend.
// If backend is nil, a default API backend (mempool.space) will be created.
func NewWalletManager(walletDir string, b backend.BitcoinBackend) (*WalletManager, error) {
	storage, err := NewWalletStorage(walletDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	if b == nil {
		b, err = backend.NewDefaultBackend("testnet")
		if err != nil {
			return nil, fmt.Errorf("failed to create default backend: %w", err)
		}
	}

	return &WalletManager{
		storage:  storage,
		backend:  b,
		unlocked: false,
		state:    NewWalletState(),
	}, nil
}

// saveWalletTracked wraps storage.SaveWallet with WaitGroup tracking so
// in-flight persist operations can be awaited during shutdown.
func (m *WalletManager) saveWalletTracked(name string, data *WalletData) error {
	m.persistWg.Add(1)
	defer m.persistWg.Done()
	m.keyMu.RLock()
	defer m.keyMu.RUnlock()
	return m.storage.SaveWalletWithKey(name, data, m.encSalt, m.encKey)
}

// hasKey reports whether the session holds an encryption key (i.e. is unlocked).
func (m *WalletManager) hasKey() bool {
	m.keyMu.RLock()
	defer m.keyMu.RUnlock()
	return len(m.encKey) > 0
}

func (m *WalletManager) setKey(salt, key []byte) {
	m.keyMu.Lock()
	Wipe(m.encKey)
	m.encSalt, m.encKey = salt, key
	m.keyMu.Unlock()
}

func (m *WalletManager) wipeKey() {
	m.keyMu.Lock()
	Wipe(m.encKey)
	Wipe(m.encSalt)
	m.encKey, m.encSalt = nil, nil
	m.keyMu.Unlock()
}

// WaitForPersist blocks until all in-flight persist operations complete.
func (m *WalletManager) WaitForPersist() {
	m.persistWg.Wait()
}

// SetBackend switches backends and refreshes wallet state against the new source.
func (m *WalletManager) SetBackend(b backend.BitcoinBackend) error {
	if b == nil {
		return fmt.Errorf("backend cannot be nil")
	}

	// Test the new backend first so a bad server leaves the current one
	// connected and syncing.
	if err := b.TestConnection(); err != nil {
		return fmt.Errorf("backend connection test failed: %w", err)
	}
	// An open wallet only switches to a server on its own network.
	if network := m.GetNetwork(); m.unlocked && network != "" {
		if err := checkServerNetwork(b, network); err != nil {
			_ = b.Disconnect()
			return err
		}
	}

	m.StopBackgroundSync()

	if m.backend != nil {
		_ = m.backend.Disconnect()
	}

	m.backend = b
	m.resetNetworkVerified()

	m.invalidateBlockchainCache()

	if m.unlocked {
		if m.scanCancel != nil {
			m.scanCancel()
			m.scanWg.Wait()
		}

		ctx, cancel := context.WithCancel(context.Background())
		m.scanCancel = cancel
		m.scanWg.Add(1)
		go func() {
			defer m.scanWg.Done()
			m.scanAddressesWithContext(ctx)
		}()

		m.StartBackgroundSync()
	}

	return nil
}

// invalidateBlockchainCache clears chain-derived data while preserving address structure.
func (m *WalletManager) invalidateBlockchainCache() {
	if m.cache == nil {
		return
	}

	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()

	m.cache.TotalBalance = 0
	m.cache.Transactions = nil
	m.cache.UTXOs = nil
	m.cache.FeeEstimates = nil
	m.cache.BlockHeight = 0

	m.cache.BalanceUpdatedAt = time.Time{}
	m.cache.TransactionsUpdatedAt = time.Time{}
	m.cache.UTXOsUpdatedAt = time.Time{}
	m.cache.FeesUpdatedAt = time.Time{}

	for addr := range m.cache.Addresses {
		m.cache.Addresses[addr].Balance = 0
		m.cache.Addresses[addr].LastChecked = time.Time{}
	}

	m.cache.LastSyncAt = time.Time{}
	m.cache.SyncError = nil
}

// GetBackend returns the current backend
func (m *WalletManager) GetBackend() backend.BitcoinBackend {
	return m.backend
}

// SetGapLimit sets the address gap limit for scanning
func (m *WalletManager) SetGapLimit(n int) {
	if n > 0 {
		m.gapLimit = n
	}
}

// getGapLimit returns the configured gap limit, defaulting to AddressGapLimit
func (m *WalletManager) getGapLimit() int {
	if m.gapLimit > 0 {
		return m.gapLimit
	}
	return AddressGapLimit
}

// CreateWallet creates a new wallet with a generated mnemonic (no BIP39 passphrase).
func (m *WalletManager) CreateWallet(name string, password string, wordCount int) (string, error) {
	return m.CreateWalletOnNetwork(name, password, wordCount, NetworkTestnet, "")
}

// CreateWalletWithPassphrase creates a new wallet with an optional BIP39 passphrase.
func (m *WalletManager) CreateWalletWithPassphrase(name, password, passphrase string, wordCount int) (string, error) {
	return m.CreateWalletOnNetwork(name, password, wordCount, NetworkTestnet, passphrase)
}

// CreateWalletOnNetwork creates a new software wallet for network
// (NetworkMainnet or NetworkTestnet) with an optional BIP39 passphrase.
// The network is persisted in the wallet file and fixes derivation coin type
// and address encoding for its lifetime. The passphrase is stored only inside
// the encrypted wallet file and is honored on every unlock.
func (m *WalletManager) CreateWalletOnNetwork(name, password string, wordCount int, network, passphrase string) (string, error) {
	if _, err := ParamsForNetwork(network); err != nil {
		return "", err
	}
	if err := ValidateWalletName(name); err != nil {
		return "", err
	}
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	// Check if wallet already exists
	if m.WalletExists(name) {
		return "", fmt.Errorf("wallet '%s' already exists", name)
	}

	// Determine mnemonic strength
	var strength MnemonicStrength
	switch wordCount {
	case 12:
		strength = Mnemonic12Words
	case 24:
		strength = Mnemonic24Words
	default:
		return "", fmt.Errorf("unsupported word count: %d (use 12 or 24)", wordCount)
	}

	// Generate mnemonic
	mnemonic, err := GenerateMnemonic(strength)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	// Create wallet data
	walletData := &WalletData{
		Version:    1,
		Name:       name,
		Mnemonic:   mnemonic,
		Passphrase: passphrase,
		CreatedAt:  time.Now().Unix(),
		Network:    network,
		Accounts:   make([]AccountData, 0),
	}

	// Save to disk
	if err := m.storage.SaveWallet(name, walletData, password); err != nil {
		return "", fmt.Errorf("failed to save wallet: %w", err)
	}

	return mnemonic, nil
}

// RestoreWallet creates a wallet from an existing mnemonic (no BIP39 passphrase).
func (m *WalletManager) RestoreWallet(name string, password string, mnemonic string) error {
	return m.RestoreWalletWithPassphrase(name, password, mnemonic, "")
}

// RestoreWalletWithPassphrase creates a wallet from an existing mnemonic and an
// optional BIP39 passphrase, which is honored on every unlock.
func (m *WalletManager) RestoreWalletWithPassphrase(name, password, mnemonic, passphrase string) error {
	return m.RestoreWalletOnNetwork(name, password, mnemonic, passphrase, NetworkTestnet)
}

// RestoreWalletOnNetwork restores a software wallet for network.
func (m *WalletManager) RestoreWalletOnNetwork(name, password, mnemonic, passphrase, network string) error {
	if _, err := ParamsForNetwork(network); err != nil {
		return err
	}
	if err := ValidateWalletName(name); err != nil {
		return err
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	// Check if wallet already exists
	if m.WalletExists(name) {
		return fmt.Errorf("wallet '%s' already exists", name)
	}

	// Validate mnemonic
	if !ValidateMnemonic(mnemonic) {
		return fmt.Errorf("invalid mnemonic phrase")
	}

	// Create wallet data
	walletData := &WalletData{
		Version:    1,
		Name:       name,
		Mnemonic:   mnemonic,
		Passphrase: passphrase,
		CreatedAt:  time.Now().Unix(),
		Network:    network,
		Accounts:   make([]AccountData, 0),
	}

	// Save to disk
	if err := m.storage.SaveWallet(name, walletData, password); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	return nil
}

// CreateHardwareWallet creates a watch-only single-sig wallet from a hardware device xpub
// This wallet can derive addresses but requires the hardware device for signing
func (m *WalletManager) CreateHardwareWallet(
	name string,
	password string,
	xpub string,
	fingerprint string,
	keyOrigin string,
	addrType AddressType,
	deviceType string,
) error {
	return m.CreateHardwareWalletOnNetwork(name, password, xpub, fingerprint, keyOrigin, addrType, deviceType, NetworkTestnet)
}

// CreateHardwareWalletOnNetwork creates a hardware-backed watch-only wallet
// for network. The xpub must belong to that network (xpub vs tpub).
func (m *WalletManager) CreateHardwareWalletOnNetwork(
	name string,
	password string,
	xpub string,
	fingerprint string,
	keyOrigin string,
	addrType AddressType,
	deviceType string,
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
	if xpub == "" {
		return fmt.Errorf("xpub cannot be empty")
	}

	xkey, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return fmt.Errorf("invalid xpub: %w", err)
	}
	if !xkey.IsForNet(params) {
		return fmt.Errorf("xpub is for a different network than this wallet (%s)", expectedXpubHint(network))
	}

	var addrTypeStr string
	switch addrType {
	case AddressTypeLegacy:
		addrTypeStr = "legacy"
	case AddressTypeSegWitNested:
		addrTypeStr = "segwit_nested"
	case AddressTypeSegWitNative:
		addrTypeStr = "segwit_native"
	case AddressTypeTaproot:
		addrTypeStr = "taproot"
	default:
		return fmt.Errorf("unsupported address type")
	}

	// Create hardware wallet config
	hwConfig := &HardwareWalletConfig{
		Fingerprint: fingerprint,
		KeyOrigin:   keyOrigin,
		Xpub:        xpub,
		DeviceType:  deviceType,
		AddressType: addrTypeStr,
	}

	// Create wallet data (no mnemonic for watch-only)
	walletData := &WalletData{
		Version:        1,
		Name:           name,
		Mnemonic:       "", // Empty for watch-only
		CreatedAt:      time.Now().Unix(),
		Network:        network,
		Accounts:       make([]AccountData, 0),
		HardwareWallet: hwConfig,
	}

	// Create default account
	accountData := AccountData{
		Index:       0,
		Name:        "Main Account",
		AddressType: addrTypeStr,
		Addresses:   make([]AddressData, 0),
		Labels:      make(map[string]string),
	}
	walletData.Accounts = append(walletData.Accounts, accountData)

	// Save to disk
	if err := m.storage.SaveWallet(name, walletData, password); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	return nil
}

// IsHardwareWallet returns true if the current wallet is a hardware-backed watch-only wallet
func (m *WalletManager) IsHardwareWallet() bool {
	if !m.unlocked || m.data == nil {
		return false
	}
	return m.data.HardwareWallet != nil
}

// GetHardwareWalletConfig returns the hardware wallet configuration for the current wallet
func (m *WalletManager) GetHardwareWalletConfig() *HardwareWalletConfig {
	if !m.unlocked || m.data == nil {
		return nil
	}
	return m.data.HardwareWallet
}

// IsWatchOnly returns true if the wallet has no mnemonic (hardware wallet or descriptor import)
func (m *WalletManager) IsWatchOnly() bool {
	if !m.unlocked || m.data == nil {
		return true
	}
	return m.data.Mnemonic == ""
}

// GetWalletType returns a human-readable description of the wallet type
func (m *WalletManager) GetWalletType() string {
	if !m.unlocked || m.data == nil {
		return "Unknown"
	}
	if m.data.Multisig != nil {
		if m.CanSignMultisigLocally() {
			return "Multisig (has signing keys)"
		}
		return "Multisig (watch-only)"
	}
	if m.data.Mnemonic != "" {
		return "Full (has seed)"
	}
	if m.data.HardwareWallet != nil {
		deviceType := m.data.HardwareWallet.DeviceType
		if deviceType == "descriptor" {
			return "Watch-only (descriptor)"
		}
		return "Watch-only (hardware: " + deviceType + ")"
	}
	return "Watch-only"
}

// GetScriptType returns the script type and multisig parameters for fee estimation
// Returns (ScriptType, M, N) where M and N are 0 for single-sig wallets
func (m *WalletManager) GetScriptType() (ScriptType, int, int) {
	if m.data == nil {
		return ScriptTypeP2WPKH, 0, 0
	}

	// Multisig wallets
	if m.data.Multisig != nil {
		config := m.data.Multisig
		switch MultisigScriptType(config.ScriptType) {
		case MultisigLegacy:
			return ScriptTypeP2SH, config.M, config.N
		case MultisigNestedSegwit:
			return ScriptTypeP2SHP2WSH, config.M, config.N
		case MultisigNativeSegwit:
			return ScriptTypeP2WSH, config.M, config.N
		default:
			return ScriptTypeP2WSH, config.M, config.N
		}
	}

	// Single-sig: check address type from account or hardware wallet config
	addrType := m.getAddressTypeString()
	switch addrType {
	case "legacy":
		return ScriptTypeP2PKH, 0, 0
	case "segwit_nested":
		return ScriptTypeP2SHP2WPKH, 0, 0
	case "taproot":
		return ScriptTypeP2TR, 0, 0
	default: // "segwit_native" or unknown
		return ScriptTypeP2WPKH, 0, 0
	}
}

// getAddressTypeString returns the address type string for the current wallet
func (m *WalletManager) getAddressTypeString() string {
	if m.data == nil {
		return "segwit_native"
	}

	if m.data.HardwareWallet != nil {
		return m.data.HardwareWallet.AddressType
	}

	if len(m.data.Accounts) > 0 {
		return m.data.Accounts[0].AddressType
	}

	return "segwit_native"
}

// ImportSeedToWatchOnlyWallet upgrades a matching watch-only single-sig wallet to a full wallet.
func (m *WalletManager) ImportSeedToWatchOnlyWallet(mnemonic string) error {
	if !m.unlocked || m.data == nil {
		return fmt.Errorf("wallet not unlocked")
	}

	if m.data.Mnemonic != "" {
		return fmt.Errorf("wallet already has a seed phrase")
	}

	if m.data.HardwareWallet == nil {
		return fmt.Errorf("not a watch-only wallet")
	}

	if !ValidateMnemonic(mnemonic) {
		return fmt.Errorf("invalid seed phrase")
	}

	hwConfig := m.data.HardwareWallet

	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		return fmt.Errorf("failed to derive seed: %w", err)
	}

	hdWallet, err := NewHDWallet(seed, m.data.Network == "testnet")
	if err != nil {
		return fmt.Errorf("failed to create HD wallet: %w", err)
	}

	addrType := ParseAddressTypeString(hwConfig.AddressType)

	derivedXpub, err := hdWallet.GetExtendedPublicKey(0, addrType)
	if err != nil {
		return fmt.Errorf("failed to derive xpub: %w", err)
	}

	if derivedXpub != hwConfig.Xpub {
		return fmt.Errorf("seed does not match this wallet's xpub")
	}

	m.dataMu.Lock()
	m.data.Mnemonic = mnemonic
	m.data.HardwareWallet = nil
	m.wallet = hdWallet
	m.dataMu.Unlock()

	if err := m.saveWalletTracked(m.data.Name, m.data); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	return nil
}

// UnlockWallet loads and unlocks a wallet. The password is used only to
// derive the encryption key and is not retained.
func (m *WalletManager) UnlockWallet(name string, password string) error {
	pw := []byte(password)
	defer Wipe(pw)
	return m.UnlockWalletBytes(name, pw)
}

// UnlockWalletBytes is UnlockWallet for a password held in a []byte. The
// caller may wipe password after it returns.
func (m *WalletManager) UnlockWalletBytes(name string, password []byte) error {
	if m.unlocked {
		m.LockWallet()
	}

	m.state.SetStatus(SyncStatusConnecting, "Loading wallet...")

	data, salt, key, err := m.storage.LoadWalletKey(name, password)
	if err != nil {
		m.state.SetError(fmt.Errorf("failed to load wallet: %w", err))
		return fmt.Errorf("failed to load wallet: %w", err)
	}

	var hd *HDWallet
	if data.Multisig == nil && data.HardwareWallet == nil {
		seed, err := seedFromWalletData(data)
		if err != nil {
			Wipe(key)
			m.state.SetError(fmt.Errorf("failed to derive seed: %w", err))
			return fmt.Errorf("failed to derive seed: %w", err)
		}
		hd, err = NewHDWallet(seed, data.Network == "testnet")
		Wipe(seed)
		if err != nil {
			Wipe(key)
			m.state.SetError(fmt.Errorf("failed to create HD wallet: %w", err))
			return fmt.Errorf("failed to create HD wallet: %w", err)
		}
	}

	// Hard fail if the server is on another chain. If the server is
	// unreachable the check is deferred to the first sync connection.
	m.resetNetworkVerified()
	verified, err := m.alignBackendNetwork(data.Network)
	if err != nil {
		Wipe(key)
		m.state.SetError(err)
		return err
	}
	m.netVerified = verified

	m.setKey(salt, key)
	m.wallet = hd
	m.data = data
	m.unlocked = true
	if data.Multisig != nil {
		m.migrateMultisigPaths()
	}

	m.cache = NewWalletCache()

	m.loadPersistentCache()

	m.syncStateFromCache()

	m.StartBackgroundSync()

	return nil
}

// LockWallet locks the current wallet, stops background work, and wipes the
// in-memory encryption key and derived key material.
func (m *WalletManager) LockWallet() {
	if m.scanCancel != nil {
		m.scanCancel()
	}
	m.scanWg.Wait()

	m.StopBackgroundSync()

	m.persistWg.Wait()

	m.wipeKey()
	m.dataMu.Lock()
	if m.data != nil {
		// Drop references so the GC can reclaim them; strings can't be zeroed.
		m.data.Mnemonic = ""
		m.data.Passphrase = ""
		if m.data.Multisig != nil {
			for i := range m.data.Multisig.Cosigners {
				m.data.Multisig.Cosigners[i].Mnemonic = ""
			}
		}
	}
	m.data = nil
	m.dataMu.Unlock()
	m.wallet = nil
	m.unlocked = false
	m.cache = nil
}

// GetState returns a snapshot of the current wallet state.
func (m *WalletManager) GetState() WalletStateSnapshot {
	if m.state == nil {
		return WalletStateSnapshot{Status: SyncStatusDisconnected}
	}
	return m.state.Snapshot()
}

func (m *WalletManager) GetStatePtr() *WalletState {
	return m.state
}

// IsUnlocked returns whether a wallet is currently unlocked
func (m *WalletManager) IsUnlocked() bool {
	return m.unlocked
}

// GetWalletName returns the name of the currently loaded wallet
func (m *WalletManager) GetWalletName() string {
	if m.data == nil {
		return ""
	}
	return m.data.Name
}

// CreateAccount creates a new account. Pass "" for the standard derivation path.
func (m *WalletManager) CreateAccount(name string, addrType AddressType, customDerivationPath string) (uint32, error) {
	if !m.unlocked {
		return 0, fmt.Errorf("wallet is locked")
	}

	if customDerivationPath != "" {
		if err := ValidateAccountPath(customDerivationPath); err != nil {
			return 0, fmt.Errorf("invalid custom derivation path: %w", err)
		}
	}

	addrTypeStr := "legacy"
	switch addrType {
	case AddressTypeSegWitNested:
		addrTypeStr = "segwit_nested"
	case AddressTypeSegWitNative:
		addrTypeStr = "segwit_native"
	case AddressTypeTaproot:
		addrTypeStr = "taproot"
	}

	m.dataMu.Lock()
	accountIndex := uint32(len(m.data.Accounts)) // #nosec G115 -- account count is tiny
	account := AccountData{
		Index:                accountIndex,
		Name:                 name,
		AddressType:          addrTypeStr,
		CustomDerivationPath: customDerivationPath,
		Addresses:            make([]AddressData, 0),
		Labels:               make(map[string]string),
	}

	m.data.Accounts = append(m.data.Accounts, account)

	err := m.saveWalletTracked(m.data.Name, m.data)
	m.dataMu.Unlock()

	if err != nil {
		return 0, fmt.Errorf("failed to save account: %w", err)
	}

	return accountIndex, nil
}

// ListWallets returns all available wallet names
func (m *WalletManager) ListWallets() ([]string, error) {
	return m.storage.ListWallets()
}

// WalletExists checks if a wallet exists
func (m *WalletManager) WalletExists(name string) bool {
	return m.storage.WalletExists(name)
}

// GetAccounts returns all accounts in the current wallet
func (m *WalletManager) GetAccounts() []AccountData {
	if m.data == nil {
		return nil
	}
	accounts := make([]AccountData, 0, len(m.data.Accounts))
	for _, acc := range m.data.Accounts {
		// Return a defensive copy and sanitize duplicate addresses that may exist
		// in persisted data from older code paths.
		cloned := acc
		if acc.Labels != nil {
			cloned.Labels = make(map[string]string, len(acc.Labels))
			for k, v := range acc.Labels {
				cloned.Labels[k] = v
			}
		}

		seen := make(map[string]struct{}, len(acc.Addresses))
		cloned.Addresses = make([]AddressData, 0, len(acc.Addresses))
		for _, addr := range acc.Addresses {
			key := addr.Address
			if key == "" {
				key = fmt.Sprintf("%t:%d:%s", addr.Change, addr.Index, addr.Path)
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			cloned.Addresses = append(cloned.Addresses, addr)
		}

		accounts = append(accounts, cloned)
	}
	return accounts
}

// DisplayUnit represents the unit for displaying Bitcoin amounts (BIP-177)
type DisplayUnit string

const (
	DisplayUnitBTC     DisplayUnit = "btc"     // Legacy: 1.00000000 BTC (100M base units)
	DisplayUnitBitcoin DisplayUnit = "bitcoin" // BIP-177: ₿112,987 (base unit with symbol)
	DisplayUnitSats    DisplayUnit = "sats"    // Deprecated alias for bitcoin base unit
)

// formatWithCommas adds thousand separators to an integer
func formatWithCommas(n api.Satoshi) string {
	if n < 0 {
		return "-" + formatWithCommas(-n)
	}
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}
	var result strings.Builder
	remainder := len(str) % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
		if len(str) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(str); i += 3 {
		result.WriteString(str[i : i+3])
		if i+3 < len(str) {
			result.WriteString(",")
		}
	}
	return result.String()
}

// FormatAmount formats a satoshi amount according to the specified display unit (BIP-177)
func FormatAmount(satoshis api.Satoshi, unit DisplayUnit) string {
	switch unit {
	case DisplayUnitBitcoin, DisplayUnitSats:
		// BIP-177: Display as integer with ₿ symbol and thousands separators
		return "₿" + formatWithCommas(satoshis)
	case DisplayUnitBTC:
		fallthrough
	default:
		// Legacy BTC format with 8 decimal places
		btc := float64(satoshis) / 100000000.0
		return fmt.Sprintf("%.8f BTC", btc)
	}
}

// FormatAmountWithSign formats a satoshi amount with +/- sign prefix
func FormatAmountWithSign(satoshis api.Satoshi, unit DisplayUnit) string {
	sign := ""
	if satoshis > 0 {
		sign = "+"
	}
	switch unit {
	case DisplayUnitBitcoin, DisplayUnitSats:
		// BIP-177: Display as integer with ₿ symbol
		return sign + "₿" + formatWithCommas(satoshis)
	case DisplayUnitBTC:
		fallthrough
	default:
		btc := float64(satoshis) / 100000000.0
		return fmt.Sprintf("%s%.8f BTC", sign, btc)
	}
}

// FormatAmountCompact formats a satoshi amount in a compact form
func FormatAmountCompact(satoshis api.Satoshi, unit DisplayUnit) string {
	switch unit {
	case DisplayUnitBitcoin, DisplayUnitSats:
		// BIP-177: Display as integer with ₿ symbol and thousands separators
		return "₿" + formatWithCommas(satoshis)
	case DisplayUnitBTC:
		fallthrough
	default:
		btc := float64(satoshis) / 100000000.0
		return fmt.Sprintf("%.8f BTC", btc)
	}
}

// GetUnitLabel returns the label for a display unit
func GetUnitLabel(unit DisplayUnit) string {
	switch unit {
	case DisplayUnitBitcoin, DisplayUnitSats:
		return "₿ (bitcoin)"
	case DisplayUnitBTC:
		fallthrough
	default:
		return "BTC"
	}
}

// GetExtendedPublicKey returns the extended public key for an account
// For multisig wallets, returns the output descriptor instead
// For hardware wallets, returns the stored xpub
func (m *WalletManager) GetExtendedPublicKey(accountIndex uint32) (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	// For multisig wallets, return the descriptor
	if m.IsMultisig() {
		return m.GetMultisigDescriptor()
	}

	// For hardware wallets, return the stored xpub
	if m.IsHardwareWallet() {
		hwConfig := m.data.HardwareWallet
		// Return with key origin if available
		if hwConfig.KeyOrigin != "" {
			return fmt.Sprintf("[%s]%s", hwConfig.KeyOrigin, hwConfig.Xpub), nil
		}
		return hwConfig.Xpub, nil
	}

	// For software wallets, derive the xpub
	if m.wallet == nil {
		return "", fmt.Errorf("HD wallet not initialized")
	}

	if int(accountIndex) >= len(m.data.Accounts) {
		return "", fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]

	// Use custom derivation path if set
	if account.CustomDerivationPath != "" {
		return m.wallet.GetExtendedPublicKeyAtPath(account.CustomDerivationPath)
	}

	// Determine address type (default to Legacy to match original BIP44 behavior)
	var addrType AddressType
	switch account.AddressType {
	case "legacy":
		addrType = AddressTypeLegacy
	case "segwit_nested":
		addrType = AddressTypeSegWitNested
	case "segwit_native":
		addrType = AddressTypeSegWitNative
	case "taproot":
		addrType = AddressTypeTaproot
	default:
		addrType = AddressTypeLegacy
	}

	return m.wallet.GetExtendedPublicKey(accountIndex, addrType)
}

// GetCreatedAt returns the wallet creation timestamp
func (m *WalletManager) GetCreatedAt() int64 {
	if m.data == nil {
		return 0
	}
	return m.data.CreatedAt
}

// GetNetwork returns the wallet network (testnet/mainnet)
func (m *WalletManager) GetNetwork() string {
	if m.data == nil {
		return ""
	}
	return m.data.Network
}

// GetMnemonic returns the wallet's mnemonic seed phrase
// WARNING: This exposes sensitive data - use with caution and appropriate security UI
func (m *WalletManager) GetMnemonic() (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	m.dataMu.RLock()
	defer m.dataMu.RUnlock()

	if m.data == nil || m.data.Mnemonic == "" {
		return "", fmt.Errorf("no mnemonic available")
	}

	return m.data.Mnemonic, nil
}

// seedFromWalletData derives the BIP39 seed for a software wallet, honoring the
// optional BIP39 passphrase stored inside the encrypted wallet file.
func seedFromWalletData(data *WalletData) ([]byte, error) {
	return MnemonicToSeed(data.Mnemonic, data.Passphrase)
}
