package wallet

import (
	"errors"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type genesisBackend struct {
	mockBroadcastBackend
	h   chainhash.Hash
	err error
}

func (g *genesisBackend) GenesisHash() (chainhash.Hash, error) { return g.h, g.err }

func TestCheckBackendNetwork(t *testing.T) {
	main := *chaincfg.MainNetParams.GenesisHash
	t4 := *TestNet4Params.GenesisHash
	cases := []struct {
		h        chainhash.Hash
		gerr     error
		network  string
		verified bool
		mismatch bool
	}{
		{main, nil, NetworkMainnet, true, false},
		{t4, nil, NetworkTestnet, true, false},
		{t4, nil, NetworkMainnet, false, true},
		{main, nil, NetworkTestnet, false, true},
		{*chaincfg.TestNet3Params.GenesisHash, nil, NetworkTestnet, false, true},
		{chainhash.Hash{}, errors.New("offline"), NetworkMainnet, false, false},
	}
	for i, c := range cases {
		ok, err := checkBackendNetwork(&genesisBackend{h: c.h, err: c.gerr}, c.network)
		if ok != c.verified || errors.Is(err, ErrNetworkMismatch) != c.mismatch {
			t.Errorf("case %d: ok=%v err=%v", i, ok, err)
		}
	}
	// Backends without genesis support are not verified but not rejected.
	if ok, err := checkBackendNetwork(&mockBroadcastBackend{}, NetworkMainnet); ok || err != nil {
		t.Fatalf("unsupported backend: %v %v", ok, err)
	}
}

func TestCreateAndRestoreOnNetwork_Persisted(t *testing.T) {
	m, err := NewWalletManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateWalletOnNetwork("main1", "correct horse battery", 12, NetworkMainnet, ""); err != nil {
		t.Fatal(err)
	}
	mn := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if err := m.RestoreWalletOnNetwork("t4", "correct horse battery", mn, "", NetworkTestnet); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateWalletOnNetwork("bad", "correct horse battery", 12, "regtest", ""); err == nil {
		t.Fatal("unknown network accepted")
	}
	for name, want := range map[string]string{"main1": NetworkMainnet, "t4": NetworkTestnet} {
		d, err := m.storage.LoadWallet(name, "correct horse battery")
		if err != nil || d.Network != want {
			t.Fatalf("%s: network %q err %v", name, d.Network, err)
		}
	}
}
