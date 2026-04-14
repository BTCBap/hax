package wallet

import (
	"testing"

	"hax/internal/api"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

func TestBIP44AddressGeneration(t *testing.T) {
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Derive BIP44 address at m/44'/1'/0'/0/0
	path := StandardBIP44Path(true, 0, 0, 0)
	addr, err := wallet.GetAddress(path, AddressTypeLegacy)
	if err != nil {
		t.Fatalf("Failed to derive address: %v", err)
	}

	// Verify it's a P2PKH address (starts with m or n on testnet)
	if addr[0] != 'm' && addr[0] != 'n' {
		t.Errorf("Expected P2PKH testnet address starting with 'm' or 'n', got: %s", addr)
	}

	// Decode and verify the address type
	decoded, err := btcutil.DecodeAddress(addr, &TestNet4Params)
	if err != nil {
		t.Fatalf("Failed to decode address: %v", err)
	}

	if _, ok := decoded.(*btcutil.AddressPubKeyHash); !ok {
		t.Errorf("Expected AddressPubKeyHash (P2PKH), got %T", decoded)
	}

	// Verify multiple addresses are unique
	seen := make(map[string]bool)
	for i := uint32(0); i < 5; i++ {
		p := StandardBIP44Path(true, 0, 0, i)
		a, err := wallet.GetAddress(p, AddressTypeLegacy)
		if err != nil {
			t.Fatalf("Failed to derive address %d: %v", i, err)
		}
		if seen[a] {
			t.Errorf("Duplicate address at index %d: %s", i, a)
		}
		seen[a] = true
	}
}

// TestBIP44PSBTSigningRoundTrip tests the full PSBT flow for single-sig Legacy P2PKH:
// CreatePSBT -> SignPSBT -> FinalizePSBT -> script engine verification
func TestBIP44PSBTSigningRoundTrip(t *testing.T) {
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	wallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	network := &TestNet4Params

	// Get address at m/44'/1'/0'/0/0
	path := StandardBIP44Path(true, 0, 0, 0)
	addr, err := wallet.GetAddress(path, AddressTypeLegacy)
	if err != nil {
		t.Fatalf("Failed to get address: %v", err)
	}

	// Create a fake UTXO
	fakeTxHash := "0000000000000000000000000000000000000000000000000000000000000001"
	utxos := []UTXOWithPath{
		{
			UTXO: api.UTXO{
				TxID:  fakeTxHash,
				Vout:  0,
				Value: 50000,
			},
			Address: addr,
			Path:    path.String(),
		},
	}

	outputs := []TxOutput{
		{Address: addr, Amount: 10000},
	}

	// Create PSBT (Legacy P2PKH)
	psbtResult, err := CreatePSBT(utxos, outputs, addr, 1.0, network, nil, ScriptTypeP2PKH, 0, 0)
	if err != nil {
		t.Fatalf("Failed to create PSBT: %v", err)
	}

	// Sign PSBT
	err = SignPSBT(psbtResult.Packet, wallet, utxos, network)
	if err != nil {
		t.Fatalf("Failed to sign PSBT: %v", err)
	}

	// Verify we have exactly one signature
	input := &psbtResult.Packet.Inputs[0]
	if len(input.PartialSigs) != 1 {
		t.Fatalf("Expected 1 partial signature, got %d", len(input.PartialSigs))
	}

	// Finalize PSBT
	finalTx, err := FinalizePSBT(psbtResult.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	// For Legacy P2PKH: scriptSig present, no witness
	if len(finalTx.TxIn[0].SignatureScript) == 0 {
		t.Errorf("Expected non-empty scriptSig for P2PKH")
	}
	if len(finalTx.TxIn[0].Witness) != 0 {
		t.Errorf("Expected no witness for P2PKH, got %d items", len(finalTx.TxIn[0].Witness))
	}

	// Verify with btcd's script engine
	decodedAddr, _ := btcutil.DecodeAddress(addr, network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 50000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(pkScript, finalTx, 0, txscript.StandardVerifyFlags, nil, sigHashes, 50000, prevOutputFetcher)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("Script verification failed: %v", err)
	}
}

// TestLegacyP2SHMultisigSigningRoundTrip tests the full PSBT flow for Legacy P2SH multisig:
// CreatePSBT -> SignMultisigPSBT -> FinalizePSBT -> script engine verification
func TestLegacyP2SHMultisigSigningRoundTrip(t *testing.T) {
	mnemonics := []string{
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
		"all all all all all all all all all all all all",
	}

	network := &TestNet4Params
	bip48Path := BIP48Path{CoinType: 1, Account: 0, ScriptType: MultisigLegacy}

	var cosigners []MultisigCosignerConfig
	var pubKeys [][]byte

	for i, mnemonic := range mnemonics {
		seed, err := MnemonicToSeed(mnemonic, "")
		if err != nil {
			t.Fatalf("Failed to create seed %d: %v", i, err)
		}

		hdWallet, err := NewHDWallet(seed, true)
		if err != nil {
			t.Fatalf("Failed to create wallet %d: %v", i, err)
		}

		xpub, err := hdWallet.GetMultisigAccountXpub(bip48Path)
		if err != nil {
			t.Fatalf("Failed to get xpub %d: %v", i, err)
		}

		fp, err := hdWallet.GetMasterFingerprintHex()
		if err != nil {
			t.Fatalf("Failed to get fingerprint %d: %v", i, err)
		}

		extKey, err := hdWallet.DeriveBIP48Key(bip48Path, 0, 0)
		if err != nil {
			t.Fatalf("Failed to derive key %d: %v", i, err)
		}
		pubKey, err := extKey.ECPubKey()
		if err != nil {
			t.Fatalf("Failed to get pubkey %d: %v", i, err)
		}
		pubKeys = append(pubKeys, pubKey.SerializeCompressed())

		cosigners = append(cosigners, MultisigCosignerConfig{
			Name:        "Cosigner " + mnemonics[i][:4],
			Xpub:        xpub,
			Fingerprint: fp,
			IsMine:      i < 2, // First two cosigners have local keys
			Mnemonic:    mnemonic,
		})
	}

	// Generate 2-of-3 P2SH multisig address
	addr, redeemScript, err := GenerateP2SHMultisigAddress(2, pubKeys, network)
	if err != nil {
		t.Fatalf("Failed to generate P2SH multisig address: %v", err)
	}

	// Verify it's a P2SH address (starts with '2' on testnet)
	if addr[0] != '2' {
		t.Errorf("Expected P2SH address starting with '2', got: %s", addr)
	}

	// Create a fake UTXO
	fakeTxHash := "0000000000000000000000000000000000000000000000000000000000000001"
	utxos := []UTXOWithPath{
		{
			UTXO: api.UTXO{
				TxID:  fakeTxHash,
				Vout:  0,
				Value: 100000,
			},
			Address: addr,
			Path:    bip48Path.ChildPath(0, 0),
		},
	}

	outputs := []TxOutput{
		{Address: addr, Amount: 50000},
	}

	// Create PSBT with Legacy P2SH script type
	psbtResult, err := CreatePSBT(utxos, outputs, addr, 1.0, network, [][]byte{redeemScript}, ScriptTypeP2SH, 2, 3)
	if err != nil {
		t.Fatalf("Failed to create PSBT: %v", err)
	}

	// Verify redeemScript in correct PSBT field (RedeemScript, not WitnessScript)
	input := &psbtResult.Packet.Inputs[0]
	if len(input.RedeemScript) == 0 {
		t.Fatalf("Expected RedeemScript to be set for Legacy P2SH")
	}
	if len(input.WitnessScript) > 0 {
		t.Errorf("Expected WitnessScript to be empty for Legacy P2SH")
	}

	// Sign with first two cosigners (2-of-3)
	sigsAdded, err := SignMultisigPSBT(psbtResult.Packet, cosigners, utxos, network, MultisigLegacy)
	if err != nil {
		t.Fatalf("Failed to sign multisig PSBT: %v", err)
	}
	if sigsAdded != 2 {
		t.Fatalf("Expected 2 signatures added, got %d", sigsAdded)
	}

	// Verify PSBT is complete
	if !IsPSBTComplete(psbtResult.Packet) {
		t.Fatalf("PSBT should be complete with 2-of-3 signatures")
	}

	// Finalize PSBT
	finalTx, err := FinalizePSBT(psbtResult.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	// For Legacy P2SH: scriptSig present, no witness
	if len(finalTx.TxIn[0].SignatureScript) == 0 {
		t.Errorf("Expected non-empty scriptSig for P2SH multisig")
	}
	if len(finalTx.TxIn[0].Witness) != 0 {
		t.Errorf("Expected no witness for P2SH multisig, got %d items", len(finalTx.TxIn[0].Witness))
	}

	// Verify with btcd's script engine
	decodedAddr, _ := btcutil.DecodeAddress(addr, network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 100000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(pkScript, finalTx, 0, txscript.StandardVerifyFlags, nil, sigHashes, 100000, prevOutputFetcher)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("Script verification failed: %v", err)
	}
}
