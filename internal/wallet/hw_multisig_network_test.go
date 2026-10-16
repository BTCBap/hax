package wallet

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

func testXpub(t *testing.T, network string, seedByte byte) string {
	t.Helper()
	params, err := ParamsForNetwork(network)
	if err != nil {
		t.Fatal(err)
	}
	k, err := hdkeychain.NewMaster(bytes.Repeat([]byte{seedByte}, 32), params)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := k.Neuter()
	if err != nil {
		t.Fatal(err)
	}
	return pub.String()
}

func TestCreateHardwareWalletOnNetwork(t *testing.T) {
	m, err := NewWalletManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery"
	mainX, testX := testXpub(t, NetworkMainnet, 1), testXpub(t, NetworkTestnet, 1)
	if err := m.CreateHardwareWalletOnNetwork("hwmain", pw, mainX, "aabbccdd", "aabbccdd/84'/0'/0'", AddressTypeSegWitNative, "coldcard", NetworkMainnet); err != nil {
		t.Fatalf("mainnet xpub on mainnet: %v", err)
	}
	if err := m.CreateHardwareWalletOnNetwork("hwbad", pw, testX, "aabbccdd", "", AddressTypeSegWitNative, "coldcard", NetworkMainnet); err == nil {
		t.Fatal("tpub accepted for mainnet wallet")
	}
	if err := m.CreateHardwareWalletOnNetwork("hwbad2", pw, mainX, "aabbccdd", "", AddressTypeSegWitNative, "coldcard", NetworkTestnet); err == nil {
		t.Fatal("xpub accepted for testnet4 wallet")
	}
	d, err := m.storage.LoadWallet("hwmain", pw)
	if err != nil || d.Network != NetworkMainnet {
		t.Fatalf("stored network %q err %v", d.Network, err)
	}
}

func TestCreateMultisigWalletOnNetwork(t *testing.T) {
	m, err := NewWalletManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery"
	cos := func(network string) []MultisigCosignerConfig {
		return []MultisigCosignerConfig{
			{Name: "a", Xpub: testXpub(t, network, 1), Fingerprint: "00000001"},
			{Name: "b", Xpub: testXpub(t, network, 2), Fingerprint: "00000002"},
		}
	}
	if err := m.CreateMultisigWalletOnNetwork("msmain", pw, 2, cos(NetworkMainnet), MultisigNativeSegwit, NetworkMainnet); err != nil {
		t.Fatalf("mainnet multisig: %v", err)
	}
	if err := m.CreateMultisigWalletOnNetwork("msbad", pw, 2, cos(NetworkTestnet), MultisigNativeSegwit, NetworkMainnet); err == nil {
		t.Fatal("testnet cosigners accepted for mainnet multisig")
	}
	d, err := m.storage.LoadWallet("msmain", pw)
	if err != nil || d.Network != NetworkMainnet {
		t.Fatalf("stored network %q err %v", d.Network, err)
	}
}

func TestNetworkOfXpub(t *testing.T) {
	if NetworkOfXpub(testXpub(t, NetworkMainnet, 3)) != NetworkMainnet {
		t.Fatal("mainnet xpub")
	}
	if NetworkOfXpub(testXpub(t, NetworkTestnet, 3)) != NetworkTestnet {
		t.Fatal("tpub")
	}
	_ = chaincfg.MainNetParams
}
