package wallet

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/BTCBap/hax/internal/api"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// Test vectors from BIP84 and BIP86, both for the mnemonic
// "abandon ... about" with no passphrase.

func vectorWallet(t *testing.T, testnet bool) *HDWallet {
	t.Helper()
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewHDWallet(seed, testnet)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestBIP84Vectors(t *testing.T) {
	w := vectorWallet(t, false)

	xpub, err := w.GetExtendedPublicKeyAtPath("m/84'/0'/0'")
	if err != nil {
		t.Fatal(err)
	}
	// BIP84 lists the account key as a zpub; this is the same key with
	// the standard xpub version bytes.
	if want := "xpub6CatWdiZiodmUeTDp8LT5or8nmbKNcuyvz7WyksVFkKB4RHwCD3XyuvPEbvqAQY3rAPshWcMLoP2fMFMKHPJ4ZeZXYVUhLv1VMrjPC7PW6V"; xpub != want {
		t.Errorf("account xpub = %s, want %s", xpub, want)
	}

	for _, v := range []struct {
		change, index uint32
		pubKey, addr  string
	}{
		{0, 0, "0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c", "bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu"},
		{0, 1, "03e775fd51f0dfb8cd865d9ff1cca2a158cf651fe997fdc9fee9c1d3b5e995ea77", "bc1qnjg0jd8228aq7egyzacy8cys3knf9xvrerkf9g"},
		{1, 0, "03025324888e429ab8e3dbaf1f7802648b9cd01e9b418485c5fa4c1b9b5700e1a6", "bc1q8c6fshw2dlwun7ekn9qwf37cu2rn755upcp6el"},
	} {
		path := PathForAddressType(AddressTypeSegWitNative, false, 0, v.change, v.index)
		key, err := w.DeriveKey(path)
		if err != nil {
			t.Fatal(err)
		}
		pub, err := key.ECPubKey()
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(pub.SerializeCompressed()); got != v.pubKey {
			t.Errorf("%s pubkey = %s, want %s", path, got, v.pubKey)
		}
		addr, err := w.GetAddress(path, AddressTypeSegWitNative)
		if err != nil {
			t.Fatal(err)
		}
		if addr != v.addr {
			t.Errorf("%s address = %s, want %s", path, addr, v.addr)
		}
	}
}

func TestBIP86Vectors(t *testing.T) {
	w := vectorWallet(t, false)

	xpub, err := w.GetExtendedPublicKeyAtPath("m/86'/0'/0'")
	if err != nil {
		t.Fatal(err)
	}
	if want := "xpub6BgBgsespWvERF3LHQu6CnqdvfEvtMcQjYrcRzx53QJjSxarj2afYWcLteoGVky7D3UKDP9QyrLprQ3VCECoY49yfdDEHGCtMMj92pReUsQ"; xpub != want {
		t.Errorf("account xpub = %s, want %s", xpub, want)
	}

	for _, v := range []struct {
		change, index          uint32
		internalKey, outputKey string
		addr                   string
	}{
		{0, 0,
			"cc8a4bc64d897bddc5fbc2f670f7a8ba0b386779106cf1223c6fc5d7cd6fc115",
			"a60869f0dbcf1dc659c9cecbaf8050135ea9e8cdc487053f1dc6880949dc684c",
			"bc1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr"},
		{0, 1,
			"83dfe85a3151d2517290da461fe2815591ef69f2b18a2ce63f01697a8b313145",
			"a82f29944d65b86ae6b5e5cc75e294ead6c59391a1edc5e016e3498c67fc7bbb",
			"bc1p4qhjn9zdvkux4e44uhx8tc55attvtyu358kutcqkudyccelu0was9fqzwh"},
		{1, 0,
			"399f1b2f4393f29a18c937859c5dd8a77350103157eb880f02e8c08214277cef",
			"882d74e5d0572d5a816cef0041a96b6c1de832f6f9676d9605c44d5e9a97d3dc",
			"bc1p3qkhfews2uk44qtvauqyr2ttdsw7svhkl9nkm9s9c3x4ax5h60wqwruhk7"},
	} {
		path := PathForAddressType(AddressTypeTaproot, false, 0, v.change, v.index)
		internal, err := w.GetTaprootInternalKey(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(schnorr.SerializePubKey(internal)); got != v.internalKey {
			t.Errorf("%s internal key = %s, want %s", path, got, v.internalKey)
		}
		output := txscript.ComputeTaprootKeyNoScript(internal)
		if got := hex.EncodeToString(schnorr.SerializePubKey(output)); got != v.outputKey {
			t.Errorf("%s output key = %s, want %s", path, got, v.outputKey)
		}
		addr, err := w.GetAddress(path, AddressTypeTaproot)
		if err != nil {
			t.Fatal(err)
		}
		if addr != v.addr {
			t.Errorf("%s address = %s, want %s", path, addr, v.addr)
		}
	}
}

// TestSingleSigSigningRoundTrip signs and finalizes P2WPKH and P2TR
// spends on both networks and checks the result with the script
// interpreter. The two-input case matters for Taproot, whose signature
// hash commits to every input's amount and script.
func TestSingleSigSigningRoundTrip(t *testing.T) {
	for _, network := range []string{NetworkMainnet, NetworkTestnet} {
		params, err := ParamsForNetwork(network)
		if err != nil {
			t.Fatal(err)
		}
		testnet := network != NetworkMainnet
		w := vectorWallet(t, testnet)
		for _, tc := range []struct {
			name string
			addr AddressType
			st   ScriptType
		}{
			{"p2wpkh", AddressTypeSegWitNative, ScriptTypeP2WPKH},
			{"p2tr", AddressTypeTaproot, ScriptTypeP2TR},
		} {
			for _, inputs := range []int{1, 2} {
				name := fmt.Sprintf("%s/%s/%d-input", network, tc.name, inputs)
				t.Run(name, func(t *testing.T) {
					roundTrip(t, w, params, testnet, tc.addr, tc.st, inputs)
				})
			}
		}
	}
}

func roundTrip(t *testing.T, w *HDWallet, params *chaincfg.Params, testnet bool, at AddressType, st ScriptType, inputs int) {
	t.Helper()
	var utxos []UTXOWithPath
	prevOuts := map[wire.OutPoint]*wire.TxOut{}
	for i := 0; i < inputs; i++ {
		path := PathForAddressType(at, testnet, 0, 0, uint32(i))
		addr, err := w.GetAddress(path, at)
		if err != nil {
			t.Fatal(err)
		}
		a, err := btcutil.DecodeAddress(addr, params)
		if err != nil {
			t.Fatal(err)
		}
		pk, err := txscript.PayToAddrScript(a)
		if err != nil {
			t.Fatal(err)
		}
		fund := wire.NewMsgTx(2)
		fund.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: uint32(i)}, nil, nil))
		value := int64(60_000 + 10_000*i)
		fund.AddTxOut(wire.NewTxOut(value, pk))
		op := wire.OutPoint{Hash: fund.TxHash(), Index: 0}
		prevOuts[op] = fund.TxOut[0]
		utxos = append(utxos, UTXOWithPath{
			UTXO:    api.UTXO{TxID: fund.TxHash().String(), Vout: 0, Value: api.Satoshi(value)},
			Address: addr,
			Path:    path.String(),
		})
	}
	change, err := w.GetAddress(PathForAddressType(at, testnet, 0, 1, 0), at)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := w.GetAddress(PathForAddressType(at, testnet, 0, 0, 9), at)
	if err != nil {
		t.Fatal(err)
	}

	res, err := CreatePSBT(utxos, []TxOutput{{Address: dest, Amount: 50_000}}, change, 2, params, nil, st, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := SignPSBT(res.Packet, w, utxos, params); err != nil {
		t.Fatal(err)
	}
	if !IsPSBTComplete(res.Packet) {
		t.Fatal("PSBT should be complete after signing")
	}
	final, err := FinalizePSBT(res.Packet)
	if err != nil {
		t.Fatal(err)
	}
	verifyFinalTx(t, final, prevOuts)

	// The fee paid matches what CreatePSBT reported.
	var in, out int64
	for _, p := range prevOuts {
		in += p.Value
	}
	for _, o := range final.TxOut {
		out += o.Value
	}
	if api.Satoshi(in-out) != res.Fee {
		t.Errorf("fee paid %d, PSBT reported %d", in-out, res.Fee)
	}

	// Tampering with an amount must break verification.
	final.TxOut[0].Value--
	fetcher := txscript.NewMultiPrevOutFetcher(prevOuts)
	hashes := txscript.NewTxSigHashes(final, fetcher)
	prev := prevOuts[final.TxIn[0].PreviousOutPoint]
	vm, err := txscript.NewEngine(prev.PkScript, final, 0, txscript.StandardVerifyFlags, nil, hashes, prev.Value, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Execute() == nil {
		t.Fatal("tampered transaction still verifies")
	}
}
