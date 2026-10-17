package wallet

import (
	"errors"
	"fmt"

	"github.com/BTCBap/hax/internal/backend"
)

// ErrNetworkMismatch means the connected server serves a different chain
// than the wallet was created for.
var ErrNetworkMismatch = errors.New("server network does not match wallet network")

// NetworkMismatchError is returned when the server serves another network.
// It matches ErrNetworkMismatch with errors.Is and records the wallet's
// network so the UI can offer servers for it.
type NetworkMismatchError struct {
	WalletNetwork string
	Detail        string
}

func (e *NetworkMismatchError) Error() string {
	return ErrNetworkMismatch.Error() + ": " + e.Detail
}

// Is reports whether target is ErrNetworkMismatch.
func (e *NetworkMismatchError) Is(target error) bool { return target == ErrNetworkMismatch }

// checkBackendNetwork verifies the backend's genesis block matches network.
// It returns (verified, err): verified=false with err=nil means the check
// could not be performed (offline or backend lacks support) and should be
// retried on the next connection.
func checkBackendNetwork(b backend.BitcoinBackend, network string) (bool, error) {
	if b == nil {
		return false, nil
	}
	gp, ok := b.(backend.GenesisProvider)
	if !ok {
		return false, nil
	}
	params, err := ParamsForNetwork(network)
	if err != nil {
		return false, err
	}
	got, err := gp.GenesisHash()
	if err != nil {
		return false, nil
	}
	if got != *params.GenesisHash {
		return false, &NetworkMismatchError{WalletNetwork: network, Detail: fmt.Sprintf(
			"wallet is %s but the server's genesis block is %s. Choose a %s server",
			NetworkDisplayName(network), describeGenesis(got.String()), NetworkDisplayName(network))}
	}
	return true, nil
}

func describeGenesis(h string) string {
	switch h {
	case "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f":
		return "mainnet's"
	case "00000000da84f2bafbbc53dee25a72ae507ff4914b867c565be350b0da8bf043":
		return "testnet4's"
	case "000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943":
		return "testnet3's"
	case "00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6":
		return "signet's"
	case "0f9188f13cb7b2c71f2a335e3a4fc328bf5beb436012afca590b1a11466e2206":
		return "regtest's"
	}
	return h
}

// ensureNetworkVerified runs the genesis check once per backend connection.
// A mismatch is sticky until the backend is changed.
func (m *WalletManager) ensureNetworkVerified() error {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	if m.netVerified {
		return nil
	}
	ok, err := checkBackendNetwork(m.backend, m.GetNetwork())
	if err != nil {
		return err
	}
	m.netVerified = ok
	return nil
}

func (m *WalletManager) resetNetworkVerified() {
	m.netMu.Lock()
	m.netVerified = false
	m.netMu.Unlock()
}

// alignBackendNetwork checks that the backend serves network before a
// wallet is opened. A server configured for another network, or whose genesis
// block disagrees with the wallet's network, is rejected with
// ErrNetworkMismatch so the user can pick a matching server. hax never
// switches servers on its own. Unreachable servers defer the genesis check.
func (m *WalletManager) alignBackendNetwork(network string) (verified bool, err error) {
	network = backend.NormalizeNetwork(network)
	if m.backend == nil {
		return false, nil
	}
	if cfgNet := backend.ConfiguredNetwork(m.backend); cfgNet != "" && cfgNet != network {
		return false, &NetworkMismatchError{WalletNetwork: network, Detail: fmt.Sprintf(
			"wallet is %s but the selected server is configured for %s. Choose a %s server",
			NetworkDisplayName(network), NetworkDisplayName(cfgNet), NetworkDisplayName(network))}
	}
	if err := m.backend.Connect(); err != nil {
		return false, nil
	}
	return checkBackendNetwork(m.backend, network)
}
