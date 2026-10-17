package backend

import "github.com/BTCBap/hax/internal/api"

// BackendType identifies the backend implementation
type BackendType string

const (
	BackendTypeElectrum BackendType = "electrum" // Electrum protocol
	BackendTypeRPC      BackendType = "rpc"      // Bitcoin Core RPC
)

// BitcoinBackend defines the interface for blockchain data providers.
// All implementations must satisfy this interface to be used with WalletManager.
type BitcoinBackend interface {
	// Connection lifecycle methods
	Connect() error
	Disconnect() error
	IsConnected() bool
	TestConnection() error
	Type() BackendType

	// Address operations
	GetAddressInfo(address string) (*api.AddressInfo, error)
	GetAddressBalance(address string) (api.Satoshi, error)
	GetAddressUTXOs(address string) ([]api.UTXO, error)
	GetAddressTransactions(address string) ([]api.Transaction, error)

	// Transaction operations
	GetTransaction(txid string) (*api.Transaction, error)
	BroadcastTransaction(rawTxHex string) (string, error)

	// Network operations
	GetFeeEstimates() (*api.FeeEstimates, error)
	GetBlockHeight() (int64, error)

	// Subscription operations (Electrum-specific; no-ops for other backends)
	ScripthashNotifications() <-chan ScripthashNotification
	HeaderNotifications() <-chan HeaderNotification
	SubscribeHeaders() (int64, error)
	SubscribeAddress(address string) (string, error)
	GetAddressHistory(address string) (bool, error)
	AddressToScripthash(address string) (string, error)

	// Backend display info
	ServerInfo() (software, protocol string)
	CertFingerprint() string
}

// DropNotifier is implemented by backends that hold a live connection and
// can report when it is lost unexpectedly (Electrum).
type DropNotifier interface {
	Drops() <-chan error
}
