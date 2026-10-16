package wallet

import (
	"testing"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type netBackend struct {
	genesisBackend
	network string
}

func (n *netBackend) ConfiguredNetwork() string { return n.network }

func newNetBackend(network string, genesis chainhash.Hash) *netBackend {
	return &netBackend{genesisBackend: genesisBackend{h: genesis}, network: network}
}

func TestAlignBackendNetwork(t *testing.T) {
	mainG := *chaincfg.MainNetParams.GenesisHash
	t4G := *TestNet4Params.GenesisHash
	cases := []struct {
		name        string
		start       *netBackend
		wallet      string
		wantResolve string
	}{
		{"testnet server, mainnet wallet", newNetBackend("testnet", t4G), NetworkMainnet, NetworkMainnet},
		{"mainnet server, testnet wallet", newNetBackend("mainnet", mainG), NetworkTestnet, NetworkTestnet},
		{"matching server kept", newNetBackend("testnet", t4G), NetworkTestnet, ""},
		{"mislabelled server replaced", newNetBackend("testnet", mainG), NetworkTestnet, NetworkTestnet},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &WalletManager{backend: c.start}
			resolved := ""
			m.SetServerResolver(func(network string) (backend.BitcoinBackend, error) {
				resolved = network
				g := t4G
				if network == NetworkMainnet {
					g = mainG
				}
				return newNetBackend(network, g), nil
			})
			ok, err := m.alignBackendNetwork(c.wallet)
			if err != nil || !ok {
				t.Fatalf("align: ok=%v err=%v", ok, err)
			}
			if resolved != c.wantResolve {
				t.Fatalf("resolved %q, want %q", resolved, c.wantResolve)
			}
			if got := backend.ConfiguredNetwork(m.backend); got != c.wallet {
				t.Fatalf("backend network %q, want %q", got, c.wallet)
			}
		})
	}
}

func TestAlignBackendNetwork_NoResolverStillRejectsMismatch(t *testing.T) {
	m := &WalletManager{backend: newNetBackend("testnet", *TestNet4Params.GenesisHash)}
	if _, err := m.alignBackendNetwork(NetworkMainnet); err == nil {
		t.Fatal("expected mismatch error without a resolver")
	}
}
