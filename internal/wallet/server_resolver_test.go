package wallet

import (
	"errors"
	"testing"

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
		name     string
		start    *netBackend
		wallet   string
		mismatch bool
	}{
		{"testnet server, mainnet wallet", newNetBackend("testnet", t4G), NetworkMainnet, true},
		{"mainnet server, testnet wallet", newNetBackend("mainnet", mainG), NetworkTestnet, true},
		{"mislabelled server rejected", newNetBackend("testnet", mainG), NetworkTestnet, true},
		{"matching testnet server", newNetBackend("testnet", t4G), NetworkTestnet, false},
		{"matching mainnet server", newNetBackend("mainnet", mainG), NetworkMainnet, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := c.start
			m := &WalletManager{backend: start}
			ok, err := m.alignBackendNetwork(c.wallet)
			if c.mismatch {
				if !errors.Is(err, ErrNetworkMismatch) || ok {
					t.Fatalf("want mismatch, got ok=%v err=%v", ok, err)
				}
			} else if err != nil || !ok {
				t.Fatalf("align: ok=%v err=%v", ok, err)
			}
			if m.backend != start {
				t.Fatal("backend was switched automatically")
			}
		})
	}
}
