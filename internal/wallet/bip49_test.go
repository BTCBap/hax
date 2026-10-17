package wallet

import (
	"testing"

	"github.com/BTCBap/hax/internal/api"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// Test mnemonic for reproducible tests
const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestBIP49AddressGeneration(t *testing.T) {
	// Create wallet from test mnemonic
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true) // testnet
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Derive BIP49 address at m/49'/1'/0'/0/0
	path := SegWitNestedPath(true, 0, 0, 0)
	addr, err := wallet.GetAddress(path, AddressTypeSegWitNested)
	if err != nil {
		t.Fatalf("Failed to derive address: %v", err)
	}

	t.Logf("BIP49 address at m/49'/1'/0'/0/0: %s", addr)

	// Verify it's a P2SH address (starts with 2 on testnet)
	if addr[0] != '2' {
		t.Errorf("Expected P2SH address starting with '2', got: %s", addr)
	}

	// Decode and verify the address
	decoded, err := btcutil.DecodeAddress(addr, &TestNet4Params)
	if err != nil {
		t.Fatalf("Failed to decode address: %v", err)
	}

	_, ok := decoded.(*btcutil.AddressScriptHash)
	if !ok {
		t.Errorf("Expected AddressScriptHash, got %T", decoded)
	}

	// Known good address for this mnemonic (from Sparrow/Electrum)
	// You may need to verify this against a reference implementation
	t.Logf("Address type: %T", decoded)
}

func TestBIP49SigningRoundTrip(t *testing.T) {
	// Create wallet from test mnemonic
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true) // testnet
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	network := &TestNet4Params

	// Derive BIP49 address and key at m/49'/1'/0'/0/0
	path := SegWitNestedPath(true, 0, 0, 0)

	// Get the address
	addr, err := wallet.GetAddress(path, AddressTypeSegWitNested)
	if err != nil {
		t.Fatalf("Failed to derive address: %v", err)
	}
	t.Logf("Address: %s", addr)

	// Get the key
	extKey, err := wallet.DeriveKey(path)
	if err != nil {
		t.Fatalf("Failed to derive key: %v", err)
	}

	privKey, err := extKey.ECPrivKey()
	if err != nil {
		t.Fatalf("Failed to get private key: %v", err)
	}

	pubKey := privKey.PubKey().SerializeCompressed()
	pubKeyHash := btcutil.Hash160(pubKey)

	t.Logf("PubKey: %x", pubKey)
	t.Logf("PubKeyHash: %x", pubKeyHash)

	// Create the redeemScript (P2WPKH witness program)
	redeemScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(pubKeyHash).
		Script()
	if err != nil {
		t.Fatalf("Failed to create redeemScript: %v", err)
	}
	t.Logf("RedeemScript: %x (len=%d)", redeemScript, len(redeemScript))

	// Verify redeemScript is valid P2WPKH
	if !txscript.IsPayToWitnessPubKeyHash(redeemScript) {
		t.Errorf("RedeemScript is not valid P2WPKH witness program")
	}

	// Create the P2SH address from redeemScript
	p2shAddr, err := btcutil.NewAddressScriptHash(redeemScript, network)
	if err != nil {
		t.Fatalf("Failed to create P2SH address: %v", err)
	}
	t.Logf("P2SH address from redeemScript: %s", p2shAddr.EncodeAddress())

	// Verify it matches the derived address
	if p2shAddr.EncodeAddress() != addr {
		t.Errorf("Address mismatch!\n  Derived: %s\n  From redeemScript: %s", addr, p2shAddr.EncodeAddress())
	} else {
		t.Logf("Address matches!")
	}

	// Create the P2SH pkScript (what would be in the UTXO)
	pkScript, err := txscript.PayToAddrScript(p2shAddr)
	if err != nil {
		t.Fatalf("Failed to create pkScript: %v", err)
	}
	t.Logf("pkScript: %x", pkScript)

	// Create a fake transaction to spend
	fakeTxHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")

	// Create the spending transaction
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *fakeTxHash,
			Index: 0,
		},
		Sequence: 0xfffffffd,
	})

	// Add a dummy output
	tx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: pkScript, // Send back to same address for simplicity
	})

	// Sign the input using BIP143 (SegWit signing)
	inputValue := int64(2000) // Fake input value

	// For P2SH-P2WPKH, the scriptCode is the P2PKH form
	scriptCode, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(pubKeyHash).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	if err != nil {
		t.Fatalf("Failed to create scriptCode: %v", err)
	}
	t.Logf("scriptCode (P2PKH form): %x", scriptCode)

	// Create signature using BIP143
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, inputValue)
	sigHashes := txscript.NewTxSigHashes(tx, prevOutputFetcher)

	sig, err := txscript.RawTxInWitnessSignature(
		tx,
		sigHashes,
		0,
		inputValue,
		scriptCode, // Use P2PKH scriptCode for BIP143
		txscript.SigHashAll,
		privKey,
	)
	if err != nil {
		t.Fatalf("Failed to create signature: %v", err)
	}
	t.Logf("Signature: %x (len=%d)", sig, len(sig))

	// Set up the transaction with scriptSig and witness
	// scriptSig = push(redeemScript)
	scriptSigBuilder := txscript.NewScriptBuilder()
	scriptSigBuilder.AddData(redeemScript)
	scriptSig, err := scriptSigBuilder.Script()
	if err != nil {
		t.Fatalf("Failed to create scriptSig: %v", err)
	}
	tx.TxIn[0].SignatureScript = scriptSig
	t.Logf("scriptSig: %x", scriptSig)

	// witness = [signature, pubkey]
	tx.TxIn[0].Witness = wire.TxWitness{sig, pubKey}
	t.Logf("Witness[0] (sig): %x", sig)
	t.Logf("Witness[1] (pubkey): %x", pubKey)

	// Verify the transaction
	vm, err := txscript.NewEngine(
		pkScript,
		tx,
		0,
		txscript.StandardVerifyFlags,
		nil,
		sigHashes,
		inputValue,
		prevOutputFetcher,
	)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}

	if err := vm.Execute(); err != nil {
		t.Errorf("Script execution failed: %v", err)
	} else {
		t.Logf("Script execution SUCCEEDED!")
	}

	// Serialize and print the transaction
	var txBuf []byte
	txBuf, err = serializeTxForTest(tx)
	if err != nil {
		t.Fatalf("Failed to serialize tx: %v", err)
	}
	t.Logf("Signed transaction hex: %x", txBuf)
}

func serializeTxForTest(tx *wire.MsgTx) ([]byte, error) {
	var buf []byte
	w := &byteWriter{buf: &buf}
	if err := tx.Serialize(w); err != nil {
		return nil, err
	}
	return buf, nil
}

type byteWriter struct {
	buf *[]byte
}

func (w *byteWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func TestCompareOldVsNewAddressGeneration(t *testing.T) {
	// This test shows the difference between old buggy and new correct address generation
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true) // testnet
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	network := &TestNet4Params

	// Derive key
	path := SegWitNestedPath(true, 0, 0, 0)
	extKey, err := wallet.DeriveKey(path)
	if err != nil {
		t.Fatalf("Failed to derive key: %v", err)
	}

	pubKey, err := extKey.ECPubKey()
	if err != nil {
		t.Fatalf("Failed to get pubkey: %v", err)
	}
	pubKeyBytes := pubKey.SerializeCompressed()
	pubKeyHash := btcutil.Hash160(pubKeyBytes)

	// OLD BUGGY METHOD:
	// witnessAddr.ScriptAddress() returns just 20 bytes (pubKeyHash)
	// Then we hash those 20 bytes to get the P2SH address
	oldBuggyAddr, err := btcutil.NewAddressScriptHash(pubKeyHash, network)
	if err != nil {
		t.Fatalf("Failed to create old buggy address: %v", err)
	}

	// NEW CORRECT METHOD:
	// Create proper 22-byte witness program (OP_0 + PUSH20 + pubKeyHash)
	redeemScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(pubKeyHash).
		Script()
	if err != nil {
		t.Fatalf("Failed to create redeemScript: %v", err)
	}
	newCorrectAddr, err := btcutil.NewAddressScriptHash(redeemScript, network)
	if err != nil {
		t.Fatalf("Failed to create new correct address: %v", err)
	}

	// Also get what GetAddress returns now
	derivedAddr, err := wallet.GetAddress(path, AddressTypeSegWitNested)
	if err != nil {
		t.Fatalf("Failed to get address: %v", err)
	}

	t.Logf("PubKeyHash: %x", pubKeyHash)
	t.Logf("RedeemScript: %x", redeemScript)
	t.Logf("")
	t.Logf("OLD BUGGY address (hash of 20 bytes):   %s", oldBuggyAddr.EncodeAddress())
	t.Logf("NEW CORRECT address (hash of 22 bytes): %s", newCorrectAddr.EncodeAddress())
	t.Logf("GetAddress() returns:                   %s", derivedAddr)
	t.Logf("")

	if derivedAddr == newCorrectAddr.EncodeAddress() {
		t.Logf("GetAddress is now using CORRECT method")
	} else if derivedAddr == oldBuggyAddr.EncodeAddress() {
		t.Errorf("GetAddress is still using OLD BUGGY method!")
	} else {
		t.Errorf("GetAddress returns something unexpected!")
	}

	// Verify against known good value from reference wallet (e.g., Sparrow)
	// For "abandon abandon..." mnemonic, BIP49 m/49'/1'/0'/0/0 should be:
	// (You should verify this with Sparrow or another reference wallet)
	t.Logf("\nTo verify: import mnemonic '%s' into Sparrow", testMnemonic)
	t.Logf("Check BIP49 (P2SH-SegWit) first receive address")
}

// Test that shows how to spend from a P2SH-P2WPKH address
func TestP2SH_P2WPKH_SpendingFormat(t *testing.T) {
	t.Log("P2SH-P2WPKH Spending Format:")
	t.Log("")
	t.Log("For a P2SH-P2WPKH input:")
	t.Log("  pkScript (in UTXO): OP_HASH160 <20-byte-script-hash> OP_EQUAL")
	t.Log("  redeemScript: OP_0 <20-byte-pubkey-hash> (22 bytes)")
	t.Log("")
	t.Log("To spend:")
	t.Log("  scriptSig: <push of redeemScript> (23 bytes)")
	t.Log("  witness: [signature, compressed-pubkey]")
	t.Log("")
	t.Log("BIP143 signing:")
	t.Log("  scriptCode = OP_DUP OP_HASH160 <pubkeyhash> OP_EQUALVERIFY OP_CHECKSIG (P2PKH form)")
	t.Log("  value = input value in satoshis")
}

func TestBIP49WithRealAddress(t *testing.T) {
	// Test with the actual mnemonic that created address 2N4uVb8r...
	// Skip if no mnemonic provided
	t.Skip("Requires real mnemonic - run manually with actual wallet seed")

	// To test with real wallet:
	// 1. Export mnemonic from wallet
	// 2. Put it here
	// 3. Run test to see what addresses are generated

	realMnemonic := "your twelve word mnemonic phrase here"

	seed, err := MnemonicToSeed(realMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Check first few addresses
	for i := uint32(0); i < 5; i++ {
		path := SegWitNestedPath(true, 0, 0, i)
		addr, err := wallet.GetAddress(path, AddressTypeSegWitNested)
		if err != nil {
			t.Fatalf("Failed to get address %d: %v", i, err)
		}
		t.Logf("Address %d: %s", i, addr)
	}
}

// Test that uses the actual PSBT flow (CreatePSBT -> SignPSBT -> FinalizePSBT)
func TestBIP49PSBTFlow(t *testing.T) {
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true) // testnet
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	network := &TestNet4Params

	// Get address at m/49'/1'/0'/0/0
	path := SegWitNestedPath(true, 0, 0, 0)
	addr, err := wallet.GetAddress(path, AddressTypeSegWitNested)
	if err != nil {
		t.Fatalf("Failed to get address: %v", err)
	}
	t.Logf("Spending from address: %s (path: %s)", addr, path.String())

	// Create a fake UTXO
	fakeTxHash := "0000000000000000000000000000000000000000000000000000000000000001"
	utxos := []UTXOWithPath{
		{
			UTXO: api.UTXO{
				TxID:  fakeTxHash,
				Vout:  0,
				Value: 2000,
			},
			Address: addr,
			Path:    path.String(),
		},
	}

	// Create output
	outputs := []TxOutput{
		{
			Address: addr, // send back to self for simplicity
			Amount:  1000,
		},
	}

	// Create PSBT (P2SH-P2WPKH is BIP49 nested SegWit)
	psbtResult, err := CreatePSBT(utxos, outputs, addr, 1.0, network, nil, ScriptTypeP2SHP2WPKH, 0, 0)
	if err != nil {
		t.Fatalf("Failed to create PSBT: %v", err)
	}
	t.Logf("Created PSBT with %d inputs, %d outputs", len(psbtResult.Packet.Inputs), len(psbtResult.Packet.UnsignedTx.TxOut))

	// Check PSBT input state before signing
	input := &psbtResult.Packet.Inputs[0]
	t.Logf("Before signing:")
	t.Logf("  WitnessUtxo: %v", input.WitnessUtxo != nil)
	if input.WitnessUtxo != nil {
		t.Logf("  WitnessUtxo.PkScript: %x", input.WitnessUtxo.PkScript)
	}
	t.Logf("  RedeemScript: %x", input.RedeemScript)
	t.Logf("  PartialSigs: %d", len(input.PartialSigs))

	// Sign PSBT
	err = SignPSBT(psbtResult.Packet, wallet, utxos, network)
	if err != nil {
		t.Fatalf("Failed to sign PSBT: %v", err)
	}

	// Check PSBT input state after signing
	t.Logf("After signing:")
	t.Logf("  RedeemScript: %x (len=%d)", input.RedeemScript, len(input.RedeemScript))
	t.Logf("  PartialSigs: %d", len(input.PartialSigs))
	if len(input.PartialSigs) > 0 {
		t.Logf("  Sig[0].PubKey: %x", input.PartialSigs[0].PubKey)
		t.Logf("  Sig[0].Signature: %x", input.PartialSigs[0].Signature)
	}

	// Finalize PSBT
	finalTx, err := FinalizePSBT(psbtResult.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	// Check the finalized transaction
	t.Logf("Finalized transaction:")
	t.Logf("  TxIn[0].SignatureScript: %x (len=%d)", finalTx.TxIn[0].SignatureScript, len(finalTx.TxIn[0].SignatureScript))
	t.Logf("  TxIn[0].Witness: %d items", len(finalTx.TxIn[0].Witness))
	for i, w := range finalTx.TxIn[0].Witness {
		t.Logf("    Witness[%d]: %x (len=%d)", i, w, len(w))
	}

	// Verify the scriptSig format
	// For P2SH-P2WPKH, scriptSig should be: OP_PUSHBYTES_22 <22-byte-redeemScript>
	// Which is 23 bytes total
	scriptSig := finalTx.TxIn[0].SignatureScript
	if len(scriptSig) != 23 {
		t.Errorf("scriptSig length should be 23, got %d", len(scriptSig))
	}
	if scriptSig[0] != 0x16 { // OP_PUSHBYTES_22
		t.Errorf("scriptSig should start with 0x16 (PUSHBYTES_22), got 0x%02x", scriptSig[0])
	}
	// The pushed data should be the redeemScript (witness program)
	if scriptSig[1] != 0x00 || scriptSig[2] != 0x14 {
		t.Errorf("Pushed redeemScript should start with 0x00 0x14, got 0x%02x 0x%02x", scriptSig[1], scriptSig[2])
	}

	// Verify witness format
	// For P2WPKH (inside P2SH), witness should be: [signature, pubkey]
	witness := finalTx.TxIn[0].Witness
	if len(witness) != 2 {
		t.Errorf("Witness should have 2 items, got %d", len(witness))
	}
	// Signature should be 71-73 bytes (DER + sighash byte)
	if len(witness) >= 1 && (len(witness[0]) < 70 || len(witness[0]) > 73) {
		t.Errorf("Signature length should be 70-73, got %d", len(witness[0]))
	}
	// Pubkey should be 33 bytes (compressed)
	if len(witness) >= 2 && len(witness[1]) != 33 {
		t.Errorf("Pubkey length should be 33, got %d", len(witness[1]))
	}

	// Serialize and show hex
	txHex, err := SerializeTx(finalTx)
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}
	t.Logf("Transaction hex: %s", txHex)

	// Now verify the transaction using btcd's script engine
	// We need the pkScript from the UTXO
	decodedAddr, _ := btcutil.DecodeAddress(addr, network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)

	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 2000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(
		pkScript,
		finalTx,
		0,
		txscript.StandardVerifyFlags,
		nil,
		sigHashes,
		2000,
		prevOutputFetcher,
	)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}

	if err := vm.Execute(); err != nil {
		t.Errorf("Script verification FAILED: %v", err)
	} else {
		t.Logf("Script verification PASSED!")
	}
}

// TestBIP49SignFromDerivation signs a P2SH-P2WPKH input the way an
// imported PSBT is signed: from its BIP32 derivation, with no UTXO path.
// The BIP143 script code must come from the redeem script.
func TestBIP49SignFromDerivation(t *testing.T) {
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatal(err)
	}
	params := &TestNet4Params
	path := PathForAddressType(AddressTypeSegWitNested, true, 0, 0, 0)
	addr, err := w.GetAddress(path, AddressTypeSegWitNested)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := btcutil.DecodeAddress(addr, params)
	pkScript, _ := txscript.PayToAddrScript(a)
	fund := wire.NewMsgTx(2)
	fund.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, nil, nil))
	fund.AddTxOut(wire.NewTxOut(100_000, pkScript))
	utxos := []UTXOWithPath{{
		UTXO:    api.UTXO{TxID: fund.TxHash().String(), Vout: 0, Value: 100_000},
		Address: addr, Path: path.String(),
	}}
	res, err := CreatePSBT(utxos, []TxOutput{{Address: addr, Amount: 50_000}}, addr, 2, params, nil, ScriptTypeP2SHP2WPKH, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := w.DeriveKey(path)
	pub, _ := key.ECPubKey()
	h := uint32(hdkeychain.HardenedKeyStart)
	res.Packet.Inputs[0].Bip32Derivation = []*psbt.Bip32Derivation{{
		PubKey:    pub.SerializeCompressed(),
		Bip32Path: []uint32{h + 49, h + 1, h, 0, 0},
	}}
	if err := SignPSBTFromDerivation(res.Packet, w, params); err != nil {
		t.Fatal(err)
	}
	final, err := FinalizePSBT(res.Packet)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 100_000)
	vm, err := txscript.NewEngine(pkScript, final, 0, txscript.StandardVerifyFlags, nil,
		txscript.NewTxSigHashes(final, fetcher), 100_000, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("script verification failed: %v", err)
	}
}
