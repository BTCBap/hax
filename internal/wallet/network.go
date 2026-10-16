package wallet

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

// Network identifiers stored in wallet files. "testnet" means testnet4.
const (
	NetworkMainnet = "mainnet"
	NetworkTestnet = "testnet"
)

// ParamsForNetwork maps a stored network name to chain params. Unknown names
// are an error rather than a silent default.
func ParamsForNetwork(name string) (*chaincfg.Params, error) {
	switch name {
	case NetworkMainnet:
		return &chaincfg.MainNetParams, nil
	case NetworkTestnet, "testnet4":
		return &TestNet4Params, nil
	default:
		return nil, fmt.Errorf("unknown network %q", name)
	}
}

// NetworkDisplayName returns a human label for UI ("Mainnet", "Testnet4").
func NetworkDisplayName(name string) string {
	if name == NetworkMainnet {
		return "Mainnet"
	}
	return "Testnet4"
}

// ExplorerTxURL returns a mempool.space URL for txid on the given network.
func ExplorerTxURL(network, txid string) string {
	if network == NetworkMainnet {
		return "https://mempool.space/tx/" + txid
	}
	return "https://mempool.space/testnet4/tx/" + txid
}

// ValidateAddressForNetwork decodes address strictly for network: it must
// parse with that network's params and IsForNet must hold.
func ValidateAddressForNetwork(address, network string) error {
	params, err := ParamsForNetwork(network)
	if err != nil {
		return err
	}
	addr, err := btcutil.DecodeAddress(address, params)
	if err != nil {
		return fmt.Errorf("invalid %s address: %w", NetworkDisplayName(network), err)
	}
	if !addr.IsForNet(params) {
		return fmt.Errorf("address is not a %s address", NetworkDisplayName(network))
	}
	return nil
}

// ValidateAddress checks a recipient address against the unlocked wallet's network.
func (m *WalletManager) ValidateAddress(address string) error {
	return ValidateAddressForNetwork(address, m.GetNetwork())
}
