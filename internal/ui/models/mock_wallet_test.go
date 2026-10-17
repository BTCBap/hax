package models

import (
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/btcsuite/btcd/btcutil/psbt"
)

// MockWalletManager implements WalletService for testing.
// Each method delegates to an optional function field; if nil, returns a safe zero value.
// Tests set only the function fields they need.
type MockWalletManager struct {
	// State
	GetStateFn             func() wallet.WalletStateSnapshot
	GetStatePtrFn          func() *wallet.WalletState
	IsUnlockedFn           func() bool
	GetWalletNameFn        func() string
	GetWalletTypeFn        func() string
	GetNetworkFn           func() string
	CreatedNetwork         string // last network passed to *OnNetwork
	CreatedPassphrase      string // last BIP39 passphrase passed to create/restore
	ValidateAddressFn      func(string) error
	GetCreatedAtFn         func() int64
	IsHardwareWalletFn     func() bool
	GetHardwareWalletCfgFn func() *wallet.HardwareWalletConfig
	IsWatchOnlyFn          func() bool
	IsMultisigFn           func() bool
	GetMultisigConfigFn    func() *wallet.MultisigWalletConfig
	GetScriptTypeFn        func() (wallet.ScriptType, int, int)

	// Backend
	SetBackendFn func(backend.BitcoinBackend) error
	GetBackendFn func() backend.BitcoinBackend

	// Wallet lifecycle
	CreateWalletFn          func(string, string, int) (string, error)
	RestoreWalletFn         func(string, string, string) error
	CreateHardwareWalletFn  func(string, string, string, string, string, wallet.AddressType, string) error
	CreateMultisigWalletFn  func(string, string, int, []wallet.MultisigCosignerConfig, wallet.MultisigScriptType) error
	ImportSeedToWatchOnlyFn func(string) error
	UnlockWalletFn          func(string, string) error
	LockWalletFn            func()
	ListWalletsFn           func() ([]string, error)
	WalletExistsFn          func(string) bool
	CreateAccountFn         func(string, wallet.AddressType, string) (uint32, error)
	SetGapLimitFn           func(int)

	// Accounts & keys
	GetAccountsFn          func() []wallet.AccountData
	GetExtendedPublicKeyFn func(uint32) (string, error)
	GetMnemonicFn          func() (string, error)

	// Addresses
	GetNewAddressFn           func(uint32) (string, error)
	GetCurrentReceiveAddrFn   func(uint32) (string, string, uint32, error)
	GetCurrentChangeAddrFn    func(uint32) (string, string, uint32, error)
	GetAddressByIndexFn       func(uint32, uint32) (string, string, error)
	VerifyReceiveAddrCacheFn  func(uint32)
	GetCachedReceiveAddrsFn   func() []*wallet.AddressCacheEntry
	GetCachedAddrBalanceFn    func(string) (api.Satoshi, bool)
	GetAddressStatusFn        func(string) string
	SyncAddressUsageFn        func(uint32) error
	GetNextUnusedReceiveIdxFn func() uint32
	IsAddressScannedFn        func() bool

	// Sync & balance
	StartBackgroundSyncFn   func()
	StopBackgroundSyncFn    func()
	RefreshCacheFn          func()
	RefreshCacheWithOptsFn  func(wallet.RefreshOptions)
	GetCachedBalanceFn      func() (api.Satoshi, bool)
	GetBalanceFn            func(uint32) (api.Satoshi, error)
	GetCachedTransactionsFn func() ([]api.Transaction, bool)
	GetTransactionsFn       func(uint32) ([]api.Transaction, error)
	GetCachedUTXOsFn        func() ([]api.UTXO, bool)
	GetUTXOsFn              func(uint32) ([]api.UTXO, error)
	GetCachedFeeEstimatesFn func() (*api.FeeEstimates, bool)
	GetFeeEstimatesFn       func() (*api.FeeEstimates, error)
	GetTotalBalanceChangeFn func() (api.Satoshi, error)
	IsSyncingFn             func() bool

	// PSBT
	PreviewBatchTxFn         func(uint32, []wallet.TxRecipient, float64, []api.UTXO) (*wallet.PSBTResult, error)
	CreatePSBTBatchTxFn      func(uint32, []wallet.TxRecipient, float64, []api.UTXO) (*wallet.PSBTResult, error)
	SignPSBTFn               func(*wallet.PSBTResult) error
	SignMultisigPSBTFn       func(*wallet.PSBTResult) (int, error)
	GetCosignerSigningStatFn func(*wallet.PSBTResult) []wallet.CosignerSigningStatus
	CanSignMultisigLocallyFn func() bool
	GetMultisigSigningStatFn func() (int, int, int)
	ExportPSBTBase64Fn       func(*wallet.PSBTResult) (string, error)
	ImportPSBTFromBase64Fn   func(string) (*wallet.PSBTResult, error)
	FinalizePSBTFn           func(*wallet.PSBTResult) (string, error)
	BroadcastPSBTFn          func(*wallet.PSBTResult) (string, error)
	InspectPSBTFn            func(*psbt.Packet) *wallet.PSBTInspection

	// Hardware
	GetHWIClientFn        func() *wallet.HWIClient
	ListHardwareWalletsFn func() ([]wallet.HardwareDevice, error)
	SignPSBTWithHWFn      func(string, wallet.HardwareDevice) (string, error)
	DisplayAddrOnDeviceFn func(string, string) error

	// Labels
	GetTransactionLabelFn func(string) string
	SetTransactionLabelFn func(string, string) error
	GetAddressLabelFn     func(string) string
	SetAddressLabelFn     func(string, string) error
	ExportLabelsBIP329Fn  func() (string, error)
	ImportLabelsBIP329Fn  func(string) (int, error)
	ExportKoinlyFn        func([]api.Transaction) (string, error)

	// Cosigners
	ImportCosignerSeedFn    func(string) (int, error)
	GetCosignersFn          func() []wallet.MultisigCosignerConfig
	UpdateCosignerFn        func(int, string, string) error
	ScanLocalForCosignersFn func(string) ([]wallet.LocalWalletMatch, error)
	ImportCosignerLocalFn   func(string, string) (int, error)

	// Export
	ExportDescriptorFn     func(uint32) (string, error)
	ExportDescriptorURFn   func() (string, error)
	ExportMultisigConfigFn func() ([]byte, error)
	ExportColdcardFn       func() (string, error)

	// Internal state for tests
	state *wallet.WalletState
}

// NewMockWalletManager creates a MockWalletManager with sensible defaults.
func NewMockWalletManager() *MockWalletManager {
	return &MockWalletManager{
		state: wallet.NewWalletState(),
	}
}

// --- State ---

func (m *MockWalletManager) GetState() wallet.WalletStateSnapshot {
	if m.GetStateFn != nil {
		return m.GetStateFn()
	}
	return m.state.Snapshot()
}

func (m *MockWalletManager) GetStatePtr() *wallet.WalletState {
	if m.GetStatePtrFn != nil {
		return m.GetStatePtrFn()
	}
	return m.state
}

func (m *MockWalletManager) IsUnlocked() bool {
	if m.IsUnlockedFn != nil {
		return m.IsUnlockedFn()
	}
	return true
}

func (m *MockWalletManager) GetWalletName() string {
	if m.GetWalletNameFn != nil {
		return m.GetWalletNameFn()
	}
	return "test-wallet"
}

func (m *MockWalletManager) GetWalletType() string {
	if m.GetWalletTypeFn != nil {
		return m.GetWalletTypeFn()
	}
	return "Software"
}

func (m *MockWalletManager) GetNetwork() string {
	if m.GetNetworkFn != nil {
		return m.GetNetworkFn()
	}
	return "testnet"
}

func (m *MockWalletManager) GetCreatedAt() int64 {
	if m.GetCreatedAtFn != nil {
		return m.GetCreatedAtFn()
	}
	return 0
}

func (m *MockWalletManager) IsHardwareWallet() bool {
	if m.IsHardwareWalletFn != nil {
		return m.IsHardwareWalletFn()
	}
	return false
}

func (m *MockWalletManager) GetHardwareWalletConfig() *wallet.HardwareWalletConfig {
	if m.GetHardwareWalletCfgFn != nil {
		return m.GetHardwareWalletCfgFn()
	}
	return nil
}

func (m *MockWalletManager) IsWatchOnly() bool {
	if m.IsWatchOnlyFn != nil {
		return m.IsWatchOnlyFn()
	}
	return false
}

func (m *MockWalletManager) IsMultisig() bool {
	if m.IsMultisigFn != nil {
		return m.IsMultisigFn()
	}
	return false
}

func (m *MockWalletManager) GetMultisigConfig() *wallet.MultisigWalletConfig {
	if m.GetMultisigConfigFn != nil {
		return m.GetMultisigConfigFn()
	}
	return nil
}

func (m *MockWalletManager) GetScriptType() (wallet.ScriptType, int, int) {
	if m.GetScriptTypeFn != nil {
		return m.GetScriptTypeFn()
	}
	return 0, 0, 0
}

// --- Backend ---

func (m *MockWalletManager) SetBackend(b backend.BitcoinBackend) error {
	if m.SetBackendFn != nil {
		return m.SetBackendFn(b)
	}
	return nil
}

func (m *MockWalletManager) GetBackend() backend.BitcoinBackend {
	if m.GetBackendFn != nil {
		return m.GetBackendFn()
	}
	return nil
}

// --- Wallet lifecycle ---

func (m *MockWalletManager) CreateWallet(name, password string, wordCount int) (string, error) {
	if m.CreateWalletFn != nil {
		return m.CreateWalletFn(name, password, wordCount)
	}
	return "mock mnemonic", nil
}

func (m *MockWalletManager) RestoreWallet(name, password, mnemonic string) error {
	if m.RestoreWalletFn != nil {
		return m.RestoreWalletFn(name, password, mnemonic)
	}
	return nil
}

func (m *MockWalletManager) CreateHardwareWallet(name, password, xpub, fingerprint, keyOrigin string, addrType wallet.AddressType, deviceType string) error {
	if m.CreateHardwareWalletFn != nil {
		return m.CreateHardwareWalletFn(name, password, xpub, fingerprint, keyOrigin, addrType, deviceType)
	}
	return nil
}

func (m *MockWalletManager) CreateMultisigWallet(name, password string, requiredSigs int, cosigners []wallet.MultisigCosignerConfig, scriptType wallet.MultisigScriptType) error {
	if m.CreateMultisigWalletFn != nil {
		return m.CreateMultisigWalletFn(name, password, requiredSigs, cosigners, scriptType)
	}
	return nil
}

func (m *MockWalletManager) ImportSeedToWatchOnlyWallet(mnemonic string) error {
	if m.ImportSeedToWatchOnlyFn != nil {
		return m.ImportSeedToWatchOnlyFn(mnemonic)
	}
	return nil
}

func (m *MockWalletManager) UnlockWallet(name, password string) error {
	if m.UnlockWalletFn != nil {
		return m.UnlockWalletFn(name, password)
	}
	return nil
}

func (m *MockWalletManager) LockWallet() {
	if m.LockWalletFn != nil {
		m.LockWalletFn()
	}
}

func (m *MockWalletManager) ListWallets() ([]string, error) {
	if m.ListWalletsFn != nil {
		return m.ListWalletsFn()
	}
	return []string{"test-wallet"}, nil
}

func (m *MockWalletManager) WalletExists(name string) bool {
	if m.WalletExistsFn != nil {
		return m.WalletExistsFn(name)
	}
	return false
}

func (m *MockWalletManager) CreateAccount(name string, addrType wallet.AddressType, customPath string) (uint32, error) {
	if m.CreateAccountFn != nil {
		return m.CreateAccountFn(name, addrType, customPath)
	}
	return 0, nil
}

func (m *MockWalletManager) SetGapLimit(n int) {
	if m.SetGapLimitFn != nil {
		m.SetGapLimitFn(n)
	}
}

// --- Accounts & keys ---

func (m *MockWalletManager) GetAccounts() []wallet.AccountData {
	if m.GetAccountsFn != nil {
		return m.GetAccountsFn()
	}
	return []wallet.AccountData{{
		Index:     0,
		Name:      "Default",
		Addresses: []wallet.AddressData{},
		Labels:    map[string]string{},
	}}
}

func (m *MockWalletManager) GetExtendedPublicKey(accountIndex uint32) (string, error) {
	if m.GetExtendedPublicKeyFn != nil {
		return m.GetExtendedPublicKeyFn(accountIndex)
	}
	return "tpub_mock_xpub", nil
}

func (m *MockWalletManager) GetMnemonic() (string, error) {
	if m.GetMnemonicFn != nil {
		return m.GetMnemonicFn()
	}
	return "mock mnemonic words", nil
}

// --- Addresses ---

func (m *MockWalletManager) GetNewAddress(accountIndex uint32) (string, error) {
	if m.GetNewAddressFn != nil {
		return m.GetNewAddressFn(accountIndex)
	}
	return "tb1q_mock_new_address", nil
}

func (m *MockWalletManager) GetCurrentReceiveAddress(accountIndex uint32) (string, string, uint32, error) {
	if m.GetCurrentReceiveAddrFn != nil {
		return m.GetCurrentReceiveAddrFn(accountIndex)
	}
	return "tb1q_mock_receive", "m/84'/1'/0'/0/0", 0, nil
}

func (m *MockWalletManager) GetCurrentChangeAddress(accountIndex uint32) (string, string, uint32, error) {
	if m.GetCurrentChangeAddrFn != nil {
		return m.GetCurrentChangeAddrFn(accountIndex)
	}
	return "tb1q_mock_change", "m/84'/1'/0'/1/0", 0, nil
}

func (m *MockWalletManager) GetAddressByIndex(accountIndex, addressIndex uint32) (string, string, error) {
	if m.GetAddressByIndexFn != nil {
		return m.GetAddressByIndexFn(accountIndex, addressIndex)
	}
	return "tb1q_mock_addr", "m/84'/1'/0'/0/0", nil
}

func (m *MockWalletManager) VerifyReceiveAddressCache(accountIndex uint32) {
	if m.VerifyReceiveAddrCacheFn != nil {
		m.VerifyReceiveAddrCacheFn(accountIndex)
	}
}

func (m *MockWalletManager) GetCachedReceiveAddresses() []*wallet.AddressCacheEntry {
	if m.GetCachedReceiveAddrsFn != nil {
		return m.GetCachedReceiveAddrsFn()
	}
	return nil
}

func (m *MockWalletManager) GetCachedAddressBalance(address string) (api.Satoshi, bool) {
	if m.GetCachedAddrBalanceFn != nil {
		return m.GetCachedAddrBalanceFn(address)
	}
	return 0, false
}

func (m *MockWalletManager) GetAddressStatus(address string) string {
	if m.GetAddressStatusFn != nil {
		return m.GetAddressStatusFn(address)
	}
	return ""
}

func (m *MockWalletManager) SyncAddressUsage(accountIndex uint32) error {
	if m.SyncAddressUsageFn != nil {
		return m.SyncAddressUsageFn(accountIndex)
	}
	return nil
}

func (m *MockWalletManager) GetNextUnusedReceiveIndex() uint32 {
	if m.GetNextUnusedReceiveIdxFn != nil {
		return m.GetNextUnusedReceiveIdxFn()
	}
	return 0
}

func (m *MockWalletManager) IsAddressScanned() bool {
	if m.IsAddressScannedFn != nil {
		return m.IsAddressScannedFn()
	}
	return true
}

// --- Sync & balance ---

func (m *MockWalletManager) StartBackgroundSync() {
	if m.StartBackgroundSyncFn != nil {
		m.StartBackgroundSyncFn()
	}
}

func (m *MockWalletManager) StopBackgroundSync() {
	if m.StopBackgroundSyncFn != nil {
		m.StopBackgroundSyncFn()
	}
}

func (m *MockWalletManager) RefreshCache() {
	if m.RefreshCacheFn != nil {
		m.RefreshCacheFn()
	}
}

func (m *MockWalletManager) RefreshCacheWithOptions(opts wallet.RefreshOptions) {
	if m.RefreshCacheWithOptsFn != nil {
		m.RefreshCacheWithOptsFn(opts)
	}
}

func (m *MockWalletManager) GetCachedBalance() (api.Satoshi, bool) {
	if m.GetCachedBalanceFn != nil {
		return m.GetCachedBalanceFn()
	}
	return 0, false
}

func (m *MockWalletManager) GetBalance(accountIndex uint32) (api.Satoshi, error) {
	if m.GetBalanceFn != nil {
		return m.GetBalanceFn(accountIndex)
	}
	return 0, nil
}

func (m *MockWalletManager) GetCachedTransactions() ([]api.Transaction, bool) {
	if m.GetCachedTransactionsFn != nil {
		return m.GetCachedTransactionsFn()
	}
	return nil, false
}

func (m *MockWalletManager) GetTransactions(accountIndex uint32) ([]api.Transaction, error) {
	if m.GetTransactionsFn != nil {
		return m.GetTransactionsFn(accountIndex)
	}
	return nil, nil
}

func (m *MockWalletManager) GetCachedUTXOs() ([]api.UTXO, bool) {
	if m.GetCachedUTXOsFn != nil {
		return m.GetCachedUTXOsFn()
	}
	return nil, false
}

func (m *MockWalletManager) GetUTXOs(accountIndex uint32) ([]api.UTXO, error) {
	if m.GetUTXOsFn != nil {
		return m.GetUTXOsFn(accountIndex)
	}
	return nil, nil
}

func (m *MockWalletManager) GetCachedFeeEstimates() (*api.FeeEstimates, bool) {
	if m.GetCachedFeeEstimatesFn != nil {
		return m.GetCachedFeeEstimatesFn()
	}
	return nil, false
}

func (m *MockWalletManager) GetFeeEstimates() (*api.FeeEstimates, error) {
	if m.GetFeeEstimatesFn != nil {
		return m.GetFeeEstimatesFn()
	}
	return nil, nil
}

func (m *MockWalletManager) GetTotalBalanceChange() (api.Satoshi, error) {
	if m.GetTotalBalanceChangeFn != nil {
		return m.GetTotalBalanceChangeFn()
	}
	return 0, nil
}

func (m *MockWalletManager) IsSyncing() bool {
	if m.IsSyncingFn != nil {
		return m.IsSyncingFn()
	}
	return false
}

// --- PSBT ---

func (m *MockWalletManager) PreviewBatchTransaction(accountIndex uint32, recipients []wallet.TxRecipient, feeRate float64, utxos []api.UTXO) (*wallet.PSBTResult, error) {
	if m.PreviewBatchTxFn != nil {
		return m.PreviewBatchTxFn(accountIndex, recipients, feeRate, utxos)
	}
	return nil, nil
}

func (m *MockWalletManager) CreatePSBTBatchTransaction(accountIndex uint32, recipients []wallet.TxRecipient, feeRate float64, utxos []api.UTXO) (*wallet.PSBTResult, error) {
	if m.CreatePSBTBatchTxFn != nil {
		return m.CreatePSBTBatchTxFn(accountIndex, recipients, feeRate, utxos)
	}
	return nil, nil
}

func (m *MockWalletManager) SignPSBTTransaction(result *wallet.PSBTResult) error {
	if m.SignPSBTFn != nil {
		return m.SignPSBTFn(result)
	}
	return nil
}

func (m *MockWalletManager) SignMultisigPSBTTransaction(result *wallet.PSBTResult) (int, error) {
	if m.SignMultisigPSBTFn != nil {
		return m.SignMultisigPSBTFn(result)
	}
	return 0, nil
}

func (m *MockWalletManager) GetCosignerSigningStatus(result *wallet.PSBTResult) []wallet.CosignerSigningStatus {
	if m.GetCosignerSigningStatFn != nil {
		return m.GetCosignerSigningStatFn(result)
	}
	return nil
}

func (m *MockWalletManager) CanSignMultisigLocally() bool {
	if m.CanSignMultisigLocallyFn != nil {
		return m.CanSignMultisigLocallyFn()
	}
	return false
}

func (m *MockWalletManager) GetMultisigSigningStatus() (local, hardware, external int) {
	if m.GetMultisigSigningStatFn != nil {
		return m.GetMultisigSigningStatFn()
	}
	return 0, 0, 0
}

func (m *MockWalletManager) ExportPSBTBase64(result *wallet.PSBTResult) (string, error) {
	if m.ExportPSBTBase64Fn != nil {
		return m.ExportPSBTBase64Fn(result)
	}
	return "", nil
}

func (m *MockWalletManager) ImportPSBTFromBase64(base64Data string) (*wallet.PSBTResult, error) {
	if m.ImportPSBTFromBase64Fn != nil {
		return m.ImportPSBTFromBase64Fn(base64Data)
	}
	return nil, nil
}

func (m *MockWalletManager) FinalizePSBTTransaction(result *wallet.PSBTResult) (string, error) {
	if m.FinalizePSBTFn != nil {
		return m.FinalizePSBTFn(result)
	}
	return "", nil
}

func (m *MockWalletManager) InspectPSBT(packet *psbt.Packet) *wallet.PSBTInspection {
	if m.InspectPSBTFn != nil {
		return m.InspectPSBTFn(packet)
	}
	return nil
}

func (m *MockWalletManager) BroadcastPSBT(result *wallet.PSBTResult) (string, error) {
	if m.BroadcastPSBTFn != nil {
		return m.BroadcastPSBTFn(result)
	}
	return "mock_txid", nil
}

// --- Hardware ---

func (m *MockWalletManager) GetHWIClient() *wallet.HWIClient {
	if m.GetHWIClientFn != nil {
		return m.GetHWIClientFn()
	}
	return nil
}

func (m *MockWalletManager) ListHardwareWallets() ([]wallet.HardwareDevice, error) {
	if m.ListHardwareWalletsFn != nil {
		return m.ListHardwareWalletsFn()
	}
	return nil, nil
}

func (m *MockWalletManager) SignPSBTWithHardwareWallet(psbtBase64 string, device wallet.HardwareDevice) (string, error) {
	if m.SignPSBTWithHWFn != nil {
		return m.SignPSBTWithHWFn(psbtBase64, device)
	}
	return "", nil
}

func (m *MockWalletManager) DisplayAddressOnDevice(addr, derivPath string) error {
	if m.DisplayAddrOnDeviceFn != nil {
		return m.DisplayAddrOnDeviceFn(addr, derivPath)
	}
	return nil
}

// --- Labels ---

func (m *MockWalletManager) GetTransactionLabel(txid string) string {
	if m.GetTransactionLabelFn != nil {
		return m.GetTransactionLabelFn(txid)
	}
	return ""
}

func (m *MockWalletManager) SetTransactionLabel(txid, label string) error {
	if m.SetTransactionLabelFn != nil {
		return m.SetTransactionLabelFn(txid, label)
	}
	return nil
}

func (m *MockWalletManager) ExportLabelsBIP329() (string, error) {
	if m.ExportLabelsBIP329Fn != nil {
		return m.ExportLabelsBIP329Fn()
	}
	return "", nil
}

func (m *MockWalletManager) ImportLabelsBIP329(data string) (int, error) {
	if m.ImportLabelsBIP329Fn != nil {
		return m.ImportLabelsBIP329Fn(data)
	}
	return 0, nil
}

func (m *MockWalletManager) ExportKoinly(transactions []api.Transaction) (string, error) {
	if m.ExportKoinlyFn != nil {
		return m.ExportKoinlyFn(transactions)
	}
	return "", nil
}

// --- Cosigners ---

func (m *MockWalletManager) ImportCosignerSeed(mnemonic string) (int, error) {
	if m.ImportCosignerSeedFn != nil {
		return m.ImportCosignerSeedFn(mnemonic)
	}
	return 0, nil
}

func (m *MockWalletManager) GetCosigners() []wallet.MultisigCosignerConfig {
	if m.GetCosignersFn != nil {
		return m.GetCosignersFn()
	}
	return nil
}

func (m *MockWalletManager) UpdateCosigner(index int, name, fingerprint string) error {
	if m.UpdateCosignerFn != nil {
		return m.UpdateCosignerFn(index, name, fingerprint)
	}
	return nil
}

func (m *MockWalletManager) ScanLocalWalletsForCosigners(password string) ([]wallet.LocalWalletMatch, error) {
	if m.ScanLocalForCosignersFn != nil {
		return m.ScanLocalForCosignersFn(password)
	}
	return nil, nil
}

func (m *MockWalletManager) ImportCosignerFromLocalWallet(walletName, password string) (int, error) {
	if m.ImportCosignerLocalFn != nil {
		return m.ImportCosignerLocalFn(walletName, password)
	}
	return 0, nil
}

// --- Export ---

func (m *MockWalletManager) ExportDescriptor(accountIndex uint32) (string, error) {
	if m.ExportDescriptorFn != nil {
		return m.ExportDescriptorFn(accountIndex)
	}
	return "", nil
}

func (m *MockWalletManager) ExportDescriptorUR() (string, error) {
	if m.ExportDescriptorURFn != nil {
		return m.ExportDescriptorURFn()
	}
	return "", nil
}

func (m *MockWalletManager) ExportMultisigConfig() ([]byte, error) {
	if m.ExportMultisigConfigFn != nil {
		return m.ExportMultisigConfigFn()
	}
	return nil, nil
}

func (m *MockWalletManager) ExportColdcardMultisig() (string, error) {
	if m.ExportColdcardFn != nil {
		return m.ExportColdcardFn()
	}
	return "", nil
}

// Compile-time interface check
var _ WalletService = (*MockWalletManager)(nil)

func (m *MockWalletManager) ValidateAddress(addr string) error {
	if m.ValidateAddressFn != nil {
		return m.ValidateAddressFn(addr)
	}
	network := m.GetNetwork()
	if network == "" {
		network = wallet.NetworkTestnet
	}
	return wallet.ValidateAddressForNetwork(addr, network)
}

func (m *MockWalletManager) CreateWalletOnNetwork(name, password string, wordCount int, network, passphrase string) (string, error) {
	m.CreatedNetwork = network
	m.CreatedPassphrase = passphrase
	return m.CreateWallet(name, password, wordCount)
}

func (m *MockWalletManager) RestoreWalletOnNetwork(name, password, mnemonic, passphrase, network string) error {
	m.CreatedNetwork = network
	m.CreatedPassphrase = passphrase
	return m.RestoreWallet(name, password, mnemonic)
}

func (m *MockWalletManager) CreateHardwareWalletOnNetwork(name, password, xpub, fingerprint, keyOrigin string, addrType wallet.AddressType, deviceType, network string) error {
	m.CreatedNetwork = network
	return m.CreateHardwareWallet(name, password, xpub, fingerprint, keyOrigin, addrType, deviceType)
}

func (m *MockWalletManager) CreateMultisigWalletOnNetwork(name, password string, requiredSigs int, cosigners []wallet.MultisigCosignerConfig, scriptType wallet.MultisigScriptType, network string) error {
	m.CreatedNetwork = network
	return m.CreateMultisigWallet(name, password, requiredSigs, cosigners, scriptType)
}

func (m *MockWalletManager) GetAddressLabel(address string) string {
	if m.GetAddressLabelFn != nil {
		return m.GetAddressLabelFn(address)
	}
	return ""
}

func (m *MockWalletManager) SetAddressLabel(address, label string) error {
	if m.SetAddressLabelFn != nil {
		return m.SetAddressLabelFn(address, label)
	}
	return nil
}
