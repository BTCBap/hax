package models

import (
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/btcsuite/btcd/btcutil/psbt"
)

// WalletService defines the interface that UI models use to interact with the wallet.
// *wallet.WalletManager satisfies this interface implicitly.
type WalletService interface {
	// State
	GetState() wallet.WalletStateSnapshot
	GetStatePtr() *wallet.WalletState
	IsUnlocked() bool
	GetWalletName() string
	GetWalletType() string
	GetNetwork() string
	ValidateAddress(address string) error
	GetCreatedAt() int64
	IsHardwareWallet() bool
	GetHardwareWalletConfig() *wallet.HardwareWalletConfig
	IsWatchOnly() bool
	IsMultisig() bool
	GetMultisigConfig() *wallet.MultisigWalletConfig
	GetScriptType() (wallet.ScriptType, int, int)

	// Backend
	SetBackend(b backend.BitcoinBackend) error
	GetBackend() backend.BitcoinBackend

	// Wallet lifecycle
	CreateWallet(name, password string, wordCount int) (string, error)
	RestoreWallet(name, password, mnemonic string) error
	CreateWalletOnNetwork(name, password string, wordCount int, network, passphrase string) (string, error)
	RestoreWalletOnNetwork(name, password, mnemonic, passphrase, network string) error
	CreateHardwareWallet(name, password, xpub, fingerprint, keyOrigin string, addrType wallet.AddressType, deviceType string) error
	CreateMultisigWallet(name, password string, requiredSigs int, cosigners []wallet.MultisigCosignerConfig, scriptType wallet.MultisigScriptType) error
	CreateHardwareWalletOnNetwork(name, password, xpub, fingerprint, keyOrigin string, addrType wallet.AddressType, deviceType, network string) error
	CreateMultisigWalletOnNetwork(name, password string, requiredSigs int, cosigners []wallet.MultisigCosignerConfig, scriptType wallet.MultisigScriptType, network string) error
	ImportSeedToWatchOnlyWallet(mnemonic string) error
	UnlockWallet(name, password string) error
	LockWallet()
	ListWallets() ([]string, error)
	WalletExists(name string) bool
	CreateAccount(name string, addrType wallet.AddressType, customDerivationPath string) (uint32, error)
	SetGapLimit(n int)

	// Accounts & keys
	GetAccounts() []wallet.AccountData
	GetExtendedPublicKey(accountIndex uint32) (string, error)
	GetMnemonic() (string, error)

	// Addresses
	GetNewAddress(accountIndex uint32) (string, error)
	GetCurrentReceiveAddress(accountIndex uint32) (string, string, uint32, error)
	GetCurrentChangeAddress(accountIndex uint32) (string, string, uint32, error)
	GetAddressByIndex(accountIndex, addressIndex uint32) (string, string, error)
	VerifyReceiveAddressCache(accountIndex uint32)
	GetCachedReceiveAddresses() []*wallet.AddressCacheEntry
	GetCachedAddressBalance(address string) (api.Satoshi, bool)
	GetAddressStatus(address string) string
	SyncAddressUsage(accountIndex uint32) error
	GetNextUnusedReceiveIndex() uint32
	IsAddressScanned() bool

	// Sync & balance
	StartBackgroundSync()
	StopBackgroundSync()
	RefreshCache()
	RefreshCacheWithOptions(opts wallet.RefreshOptions)
	GetCachedBalance() (api.Satoshi, bool)
	GetBalance(accountIndex uint32) (api.Satoshi, error)
	GetCachedTransactions() ([]api.Transaction, bool)
	GetTransactions(accountIndex uint32) ([]api.Transaction, error)
	GetCachedUTXOs() ([]api.UTXO, bool)
	GetUTXOs(accountIndex uint32) ([]api.UTXO, error)
	GetCachedFeeEstimates() (*api.FeeEstimates, bool)
	GetFeeEstimates() (*api.FeeEstimates, error)
	GetTotalBalanceChange() (api.Satoshi, error)
	IsSyncing() bool

	// Transactions (PSBT)
	PreviewBatchTransaction(accountIndex uint32, recipients []wallet.TxRecipient, feeRate float64, utxos []api.UTXO) (*wallet.PSBTResult, error)
	CreatePSBTBatchTransaction(accountIndex uint32, recipients []wallet.TxRecipient, feeRate float64, utxos []api.UTXO) (*wallet.PSBTResult, error)
	SignPSBTTransaction(result *wallet.PSBTResult) error
	SignMultisigPSBTTransaction(result *wallet.PSBTResult) (int, error)
	GetCosignerSigningStatus(result *wallet.PSBTResult) []wallet.CosignerSigningStatus
	CanSignMultisigLocally() bool
	GetMultisigSigningStatus() (local, hardware, external int)
	ExportPSBTBase64(result *wallet.PSBTResult) (string, error)
	ImportPSBTFromBase64(base64Data string) (*wallet.PSBTResult, error)
	FinalizePSBTTransaction(result *wallet.PSBTResult) (string, error)
	BroadcastPSBT(result *wallet.PSBTResult) (string, error)
	InspectPSBT(packet *psbt.Packet) *wallet.PSBTInspection

	// Fee bumps
	AnalyzeFeeBump(txid string) (*wallet.FeeBumpInfo, error)
	BuildRBF(txid string, feeRate float64) (*wallet.PSBTResult, error)
	BuildCPFP(txid string, feeRate float64) (*wallet.PSBTResult, error)

	// Hardware wallet
	GetHWIClient() *wallet.HWIClient
	ListHardwareWallets() ([]wallet.HardwareDevice, error)
	SignPSBTWithHardwareWallet(psbtBase64 string, device wallet.HardwareDevice) (string, error)
	DisplayAddressOnDevice(addr, derivPath string) error

	// Labels
	GetTransactionLabel(txid string) string
	SetTransactionLabel(txid, label string) error
	GetAddressLabel(address string) string
	SetAddressLabel(address, label string) error
	ExportLabelsBIP329() (string, error)
	ImportLabelsBIP329(data string) (int, error)
	ExportKoinly(transactions []api.Transaction) (string, error)

	// Cosigners
	ImportCosignerSeed(mnemonic string) (int, error)
	GetCosigners() []wallet.MultisigCosignerConfig
	UpdateCosigner(index int, name, fingerprint string) error
	ScanLocalWalletsForCosigners(password string) ([]wallet.LocalWalletMatch, error)
	ImportCosignerFromLocalWallet(walletName, password string) (int, error)

	// Export
	ExportDescriptor(accountIndex uint32) (string, error)
	ExportDescriptorUR() (string, error)
	ExportMultisigConfig() ([]byte, error)
	ExportColdcardMultisig() (string, error)
}

// Compile-time check: *wallet.WalletManager satisfies WalletService
var _ WalletService = (*wallet.WalletManager)(nil)
