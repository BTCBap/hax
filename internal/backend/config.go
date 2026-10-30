package backend

// MaxFeeRateSatPerVB is the highest backend-provided fee estimate hax will
// use. Anything above it is treated as unavailable, so a malicious server
// cannot turn a send into a fee drain.
const MaxFeeRateSatPerVB = 1000.0

// BackendConfig contains configuration for a backend connection
type BackendConfig struct {
	Type    BackendType `json:"type"`
	Network string      `json:"network"` // "mainnet" or "testnet"

	// Electrum Backend Config
	Electrum *ElectrumConfig `json:"electrum,omitempty"`

	// RPC Backend Config (Bitcoin Core)
	RPC *RPCConfig `json:"rpc,omitempty"`
}

// ElectrumConfig contains configuration for an Electrum server connection
type ElectrumConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	UseSSL    bool   `json:"use_ssl"`
	UseProxy  bool   `json:"use_proxy"`
	ProxyHost string `json:"proxy_host,omitempty"`
	ProxyPort int    `json:"proxy_port,omitempty"`

	// Tor SOCKS5 proxy settings
	UseTor        bool   `json:"use_tor"`
	TorSOCKS5Port string `json:"tor_socks5_port,omitempty"` // Default: "9050"
}

// RPCConfig contains configuration for Bitcoin Core RPC connection
type RPCConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	UseSSL   bool   `json:"use_ssl"`
	// AllowPlaintext permits sending RPC credentials over cleartext HTTP to
	// a public host. Local-network hosts already use HTTP by default.
	AllowPlaintext bool `json:"allow_plaintext,omitempty"`
}

// SavedServer represents a user's recently-used Electrum server
type SavedServer struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	UseSSL bool   `json:"use_ssl"`
	UseTor bool   `json:"use_tor"`
}

// PublicElectrumServer represents a well-known public Electrum server
type PublicElectrumServer struct {
	Name   string
	Host   string
	Port   int
	UseSSL bool
}

// PublicElectrumServers lists available public Electrum servers by network
var PublicElectrumServers = map[string][]PublicElectrumServer{
	"mainnet": {
		{Name: "Blockstream", Host: "electrum.blockstream.info", Port: 50002, UseSSL: true},
		{Name: "Fulcrum (sethforprivacy)", Host: "fulcrum.sethforprivacy.com", Port: 50002, UseSSL: true},
		{Name: "ACINQ", Host: "electrum.acinq.co", Port: 50002, UseSSL: true},
		{Name: "Emzy", Host: "electrum.emzy.de", Port: 50002, UseSSL: true},
		{Name: "Bitaroo", Host: "electrum.bitaroo.net", Port: 50002, UseSSL: true},
	},
	"testnet": {
		{Name: "Mempool.space", Host: "mempool.space", Port: 40002, UseSSL: true},
		{Name: "C3 Soft (Fulcrum)", Host: "blackie.c3-soft.com", Port: 57010, UseSSL: true},
	},
}

// ResolveBackendConfig returns the backend config to use for a wallet on
// walletNetwork. A saved config is honored only when it is for the same
// network; otherwise the default server for the wallet's network is used.
func ResolveBackendConfig(saved *BackendConfig, walletNetwork string) *BackendConfig {
	network := NormalizeNetwork(walletNetwork)
	if saved != nil && NormalizeNetwork(saved.Network) == network {
		return saved
	}
	return DefaultElectrumConfig(network)
}

// DefaultElectrumConfig returns the default Electrum server configuration
func DefaultElectrumConfig(network string) *BackendConfig {
	network = NormalizeNetwork(network)

	// Default to mempool.space for testnet4
	host := "mempool.space"
	port := 40002

	if network == "mainnet" {
		host = "electrum.blockstream.info"
		port = 50002
	}

	return &BackendConfig{
		Type:    BackendTypeElectrum,
		Network: network,
		Electrum: &ElectrumConfig{
			Host:   host,
			Port:   port,
			UseSSL: true,
		},
	}
}
