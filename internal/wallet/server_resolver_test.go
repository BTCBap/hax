package wallet

import (
	"errors"
	"testing"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type netBackend struct {
	genesisBackend
	network    string
	connectErr error
}

func (n *netBackend) ConfiguredNetwork() string { return n.network }

func (n *netBackend) Connect() error { return n.connectErr }

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
		resolve  *netBackend
		mismatch bool
	}{
		{"testnet server, mainnet wallet", newNetBackend("testnet", t4G), NetworkMainnet, newNetBackend("mainnet", mainG), false},
		{"mainnet server, testnet wallet", newNetBackend("mainnet", mainG), NetworkTestnet, newNetBackend("testnet", t4G), false},
		{"mislabelled server rejected", newNetBackend("testnet", mainG), NetworkTestnet, nil, true},
		{"matching testnet server", newNetBackend("testnet", t4G), NetworkTestnet, nil, false},
		{"matching mainnet server", newNetBackend("mainnet", mainG), NetworkMainnet, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := c.start
			m := &WalletManager{backend: start}
			if c.resolve != nil {
				resolved := c.resolve
				m.SetBackendResolver(func(network string) (backend.BitcoinBackend, error) {
					if network != backend.NormalizeNetwork(c.wallet) {
						t.Errorf("resolver network = %q, want %q", network, backend.NormalizeNetwork(c.wallet))
					}
					return resolved, nil
				})
			}
			ok, err := m.alignBackendNetwork(c.wallet)
			if c.mismatch {
				if !errors.Is(err, ErrNetworkMismatch) || ok {
					t.Fatalf("want mismatch, got ok=%v err=%v", ok, err)
				}
			} else if err != nil || !ok {
				t.Fatalf("align: ok=%v err=%v", ok, err)
			}
			if c.resolve != nil {
				if m.backend != c.resolve {
					t.Fatal("backend was not switched to the wallet network")
				}
			} else if m.backend != start {
				t.Fatal("matching backend was switched")
			}
		})
	}
}

func TestAlignBackendNetwork_FallbackWhenDefaultUnreachable(t *testing.T) {
	mainG := *chaincfg.MainNetParams.GenesisHash
	primary := newNetBackend("mainnet", mainG)
	primary.connectErr = errors.New("dial failed")
	second := newNetBackend("mainnet", mainG)

	m := &WalletManager{backend: primary}
	m.SetBackendFallbacks(func(network string) []backend.BitcoinBackend {
		if network != NetworkMainnet {
			t.Errorf("fallback network = %q, want %q", network, NetworkMainnet)
		}
		return []backend.BitcoinBackend{second}
	})

	ok, err := m.alignBackendNetwork(NetworkMainnet)
	if err != nil || !ok {
		t.Fatalf("align: ok=%v err=%v", ok, err)
	}
	if m.backend != second {
		t.Fatal("backend was not switched to the fallback server")
	}
}

func TestAlignBackendNetwork_FallbackSkipsWrongGenesis(t *testing.T) {
	mainG := *chaincfg.MainNetParams.GenesisHash
	t4G := *TestNet4Params.GenesisHash
	primary := newNetBackend("mainnet", mainG)
	primary.connectErr = errors.New("dial failed")
	mislabelled := newNetBackend("mainnet", t4G) // claims mainnet but serves testnet4
	good := newNetBackend("mainnet", mainG)

	m := &WalletManager{backend: primary}
	m.SetBackendFallbacks(func(string) []backend.BitcoinBackend {
		return []backend.BitcoinBackend{mislabelled, good}
	})

	ok, err := m.alignBackendNetwork(NetworkMainnet)
	if err != nil || !ok {
		t.Fatalf("align: ok=%v err=%v", ok, err)
	}
	if m.backend != good {
		t.Fatal("backend was not switched to the correctly-labelled server")
	}
}

func TestOpenWalletUsesWalletNetworkNotSavedServerNetwork(t *testing.T) {
	mainG := *chaincfg.MainNetParams.GenesisHash
	t4G := *TestNet4Params.GenesisHash
	m, err := NewWalletManager(t.TempDir(), newNetBackend("testnet", t4G))
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery"
	if _, err := m.CreateWalletOnNetwork("w", pw, 12, NetworkMainnet, ""); err != nil {
		t.Fatal(err)
	}

	saved := &backend.BackendConfig{
		Type:    backend.BackendTypeElectrum,
		Network: "testnet",
		Electrum: &backend.ElectrumConfig{
			Host: "mempool.space", Port: 40002, UseSSL: true,
		},
	}
	m.SetBackendResolver(func(network string) (backend.BitcoinBackend, error) {
		resolved := backend.ResolveBackendConfig(saved, network)
		if resolved.Network != NetworkMainnet {
			t.Fatalf("resolved network = %q, want mainnet", resolved.Network)
		}
		return newNetBackend(resolved.Network, mainG), nil
	})

	if err := m.UnlockWallet("w", pw); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	defer m.LockWallet()

	if got := backend.ConfiguredNetwork(m.GetBackend()); got != NetworkMainnet {
		t.Fatalf("backend network = %q, want mainnet", got)
	}
}

func TestAlignBackendNetwork_RejectsMislabelledMainnetServer(t *testing.T) {
	server := newNetBackend("mainnet", *TestNet4Params.GenesisHash)

	m := &WalletManager{backend: server}
	ok, err := m.alignBackendNetwork(NetworkMainnet)
	if ok || !errors.Is(err, ErrNetworkMismatch) {
		t.Fatalf("ok=%v err=%v, want network mismatch", ok, err)
	}
	if m.backend != server {
		t.Fatal("rejected server must not be replaced without a fallback list")
	}
}

func TestUnlockSwitchesBackendToWalletNetwork(t *testing.T) {
	mainG := *chaincfg.MainNetParams.GenesisHash
	t4G := *TestNet4Params.GenesisHash
	cases := []struct {
		name       string
		start      *netBackend
		walletNet  string
		resolved   *netBackend
		wantSwitch bool
	}{
		{"testnet backend, mainnet wallet", newNetBackend("testnet", t4G), NetworkMainnet, newNetBackend("mainnet", mainG), true},
		{"mainnet backend, testnet4 wallet", newNetBackend("mainnet", mainG), NetworkTestnet, newNetBackend("testnet", t4G), true},
		{"matching backend kept", newNetBackend("mainnet", mainG), NetworkMainnet, newNetBackend("mainnet", mainG), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewWalletManager(t.TempDir(), c.start)
			if err != nil {
				t.Fatal(err)
			}
			const pw = "correct horse battery"
			if _, err := m.CreateWalletOnNetwork("w", pw, 12, c.walletNet, ""); err != nil {
				t.Fatal(err)
			}

			resolved := c.resolved
			called := false
			m.SetBackendResolver(func(network string) (backend.BitcoinBackend, error) {
				called = true
				if network != backend.NormalizeNetwork(c.walletNet) {
					t.Errorf("resolver network = %q, want %q", network, backend.NormalizeNetwork(c.walletNet))
				}
				return resolved, nil
			})

			if err := m.UnlockWallet("w", pw); err != nil {
				t.Fatalf("unlock: %v", err)
			}
			defer m.LockWallet()

			if got := backend.ConfiguredNetwork(m.GetBackend()); got != backend.NormalizeNetwork(c.walletNet) {
				t.Fatalf("backend network = %q, want %q", got, backend.NormalizeNetwork(c.walletNet))
			}
			if c.wantSwitch {
				if !called || m.GetBackend() != resolved {
					t.Fatal("backend was not replaced with one for the wallet's network")
				}
			} else if called || m.GetBackend() != c.start {
				t.Fatal("matching backend should be kept")
			}
		})
	}
}

func TestSetBackend_RejectsOtherNetworkWhileUnlocked(t *testing.T) {
	old := newNetBackend("mainnet", *chaincfg.MainNetParams.GenesisHash)
	m := &WalletManager{backend: old, unlocked: true, data: &WalletData{Network: NetworkMainnet}}
	for _, b := range []*netBackend{
		newNetBackend("testnet", *TestNet4Params.GenesisHash), // configured for testnet4
		newNetBackend("mainnet", *TestNet4Params.GenesisHash), // mislabelled testnet4 server
	} {
		if err := m.SetBackend(b); !errors.Is(err, ErrNetworkMismatch) {
			t.Fatalf("SetBackend err = %v, want network mismatch", err)
		}
		if m.backend != old {
			t.Fatal("current backend replaced by a server on another network")
		}
	}
}
