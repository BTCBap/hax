package wallet

import (
	"bytes"
	"fmt"
	"testing"
)

// TestMultisigAddressOrder checks that multi(...) keeps the descriptor's key
// order while sortedmulti(...) applies BIP67 sorting (H3-05).
func TestMultisigAddressOrder(t *testing.T) {
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatal(err)
	}
	hd, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatal(err)
	}
	path := BIP48Path{CoinType: 1, Account: 0, ScriptType: 2}
	var pubKeys [][]byte
	for i := uint32(1); i <= 3; i++ {
		key, err := hd.DeriveBIP48Key(path, 0, i)
		if err != nil {
			t.Fatal(err)
		}
		pub, err := key.ECPubKey()
		if err != nil {
			t.Fatal(err)
		}
		pubKeys = append(pubKeys, pub.SerializeCompressed())
	}

	reversed := [][]byte{pubKeys[2], pubKeys[1], pubKeys[0]}

	sortedAddr, sortedScript, err := generateP2WSHMultisigAddress(
		2, reversed, true, &TestNet4Params)
	if err != nil {
		t.Fatal(err)
	}
	unsortedAddr, unsortedScript, err := generateP2WSHMultisigAddress(
		2, reversed, false, &TestNet4Params)
	if err != nil {
		t.Fatal(err)
	}
	if sortedAddr == unsortedAddr || bytes.Equal(sortedScript, unsortedScript) {
		t.Fatal("multi() and sortedmulti() must produce different scripts")
	}

	cfg := &MultisigConfig{M: 2, N: 3, ScriptType: MultisigNativeSegwit, Unsorted: true}
	gotAddr, _, err := GenerateMultisigAddress(cfg, reversed, &TestNet4Params)
	if err != nil {
		t.Fatal(err)
	}
	if gotAddr != unsortedAddr {
		t.Fatalf("Unsorted config used sorted order: got %s, want %s", gotAddr, unsortedAddr)
	}

	cfg.Unsorted = false
	gotAddr, _, err = GenerateMultisigAddress(cfg, reversed, &TestNet4Params)
	if err != nil {
		t.Fatal(err)
	}
	if gotAddr != sortedAddr {
		t.Fatalf("sorted config used given order: got %s, want %s", gotAddr, sortedAddr)
	}
}

func TestParseDescriptor_MultiVsSortedMulti(t *testing.T) {
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatal(err)
	}
	hd, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatal(err)
	}
	path := BIP48Path{CoinType: 1, Account: 0, ScriptType: 2}
	xpubs := make([]string, 3)
	for i := uint32(0); i < 3; i++ {
		xpub, err := hd.GetMultisigAccountXpub(path)
		if err != nil {
			t.Fatal(err)
		}
		xpubs[i] = xpub
	}

	for _, tc := range []struct {
		fn       string
		unsorted bool
	}{
		{"multi", true},
		{"sortedmulti", false},
	} {
		body := fmt.Sprintf("wsh(%s(2,%s/<0;1>/*,%s/<0;1>/*,%s/<0;1>/*))",
			tc.fn, xpubs[0], xpubs[1], xpubs[2])
		result, err := ParseDescriptor(withDescriptorChecksum(body))
		if err != nil {
			t.Fatalf("%s: %v", tc.fn, err)
		}
		if result.MultisigConfig == nil {
			t.Fatalf("%s: no multisig config", tc.fn)
		}
		if result.MultisigConfig.Unsorted != tc.unsorted {
			t.Errorf("%s: Unsorted = %v, want %v", tc.fn, result.MultisigConfig.Unsorted, tc.unsorted)
		}
	}
}
