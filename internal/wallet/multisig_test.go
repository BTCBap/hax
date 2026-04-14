package wallet

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// TestMultisigAddressGeneration_P2WSH verifies that multisig address generation
// produces valid addresses and is consistent across runs.
//
// These tests use the standard BIP39 test mnemonic "abandon" x 11 + "about"
// and derived xpubs to create a 2-of-3 multisig.
func TestMultisigAddressGeneration_P2WSH(t *testing.T) {
	// Generate xpubs from the standard test mnemonic
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	// Create HD wallet for testnet
	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}

	// Derive BIP48 multisig account for native segwit: m/48'/1'/0'/2'
	bip48Path := BIP48Path{
		CoinType:   1, // testnet
		Account:    0,
		ScriptType: MultisigNativeSegwit, // 2' for native segwit
	}

	xpubStr, err := hdWallet.GetMultisigAccountXpub(bip48Path)
	if err != nil {
		t.Fatalf("Failed to derive BIP48 xpub: %v", err)
	}

	t.Logf("BIP48 xpub: %s", xpubStr)

	// For this test, we'll use the same xpub 3 times (simulating 3 cosigners with same seed)
	// In practice, each cosigner would have a different seed
	testXpubs := []string{xpubStr, xpubStr, xpubStr}

	network := &chaincfg.TestNet4Params

	config := &MultisigConfig{
		M:          2,
		N:          3,
		ScriptType: MultisigNativeSegwit,
	}

	// Test receive addresses (change=0)
	t.Run("ReceiveAddresses", func(t *testing.T) {
		var generatedAddresses []string
		for index := uint32(0); index < 5; index++ {
			// Derive public keys for all cosigners at this index
			pubKeys := make([][]byte, len(testXpubs))
			for i, xpubKey := range testXpubs {
				pubKey, err := DeriveChildPubKey(xpubKey, 0, index, network)
				if err != nil {
					t.Fatalf("Failed to derive pubkey for cosigner %d at index %d: %v", i, index, err)
				}
				pubKeys[i] = pubKey
			}

			// Generate multisig address
			address, witnessScript, err := GenerateMultisigAddress(config, pubKeys, network)
			if err != nil {
				t.Fatalf("Failed to generate address at index %d: %v", index, err)
			}

			// Verify address format (tb1q for native segwit on testnet)
			if len(address) < 4 || address[:4] != "tb1q" {
				t.Errorf("Invalid address format at index %d: %s (expected tb1q prefix)", index, address)
			}

			// Verify witness script is not empty
			if len(witnessScript) == 0 {
				t.Errorf("Empty witness script at index %d", index)
			}

			generatedAddresses = append(generatedAddresses, address)
			t.Logf("Receive address %d: %s", index, address)
		}

		// Verify all addresses are unique
		seen := make(map[string]bool)
		for i, addr := range generatedAddresses {
			if seen[addr] {
				t.Errorf("Duplicate address at index %d: %s", i, addr)
			}
			seen[addr] = true
		}
	})

	// Test change addresses (change=1)
	t.Run("ChangeAddresses", func(t *testing.T) {
		var generatedAddresses []string
		for index := uint32(0); index < 5; index++ {
			pubKeys := make([][]byte, len(testXpubs))
			for i, xpubKey := range testXpubs {
				pubKey, err := DeriveChildPubKey(xpubKey, 1, index, network)
				if err != nil {
					t.Fatalf("Failed to derive pubkey for cosigner %d at index %d: %v", i, index, err)
				}
				pubKeys[i] = pubKey
			}

			address, _, err := GenerateMultisigAddress(config, pubKeys, network)
			if err != nil {
				t.Fatalf("Failed to generate address at index %d: %v", index, err)
			}

			if len(address) < 4 || address[:4] != "tb1q" {
				t.Errorf("Invalid address format at index %d: %s", index, address)
			}

			generatedAddresses = append(generatedAddresses, address)
			t.Logf("Change address %d: %s", index, address)
		}

		// Verify all addresses are unique
		seen := make(map[string]bool)
		for i, addr := range generatedAddresses {
			if seen[addr] {
				t.Errorf("Duplicate address at index %d: %s", i, addr)
			}
			seen[addr] = true
		}
	})
}

// TestMultisigPubKeySorting verifies that pubkeys are sorted correctly for multisig
// BIP67 specifies lexicographic sorting of public keys
func TestMultisigPubKeySorting(t *testing.T) {
	// Generate 3 different xpubs from different paths
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}

	// Generate 3 xpubs from different accounts
	var xpubs []string
	for account := uint32(0); account < 3; account++ {
		bip48Path := BIP48Path{
			CoinType:   1,
			Account:    account,
			ScriptType: MultisigNativeSegwit,
		}

		xpub, err := hdWallet.GetMultisigAccountXpub(bip48Path)
		if err != nil {
			t.Fatalf("Failed to derive account %d: %v", account, err)
		}

		xpubs = append(xpubs, xpub)
	}

	network := &chaincfg.TestNet4Params
	config := &MultisigConfig{
		M:          2,
		N:          3,
		ScriptType: MultisigNativeSegwit,
	}

	// Derive keys in original order
	pubKeys1 := make([][]byte, 3)
	for i, xpub := range xpubs {
		pubKey, err := DeriveChildPubKey(xpub, 0, 0, network)
		if err != nil {
			t.Fatalf("Failed to derive pubkey: %v", err)
		}
		pubKeys1[i] = pubKey
	}

	// Derive keys in reversed order
	pubKeys2 := make([][]byte, 3)
	for i := 0; i < 3; i++ {
		pubKey, err := DeriveChildPubKey(xpubs[2-i], 0, 0, network)
		if err != nil {
			t.Fatalf("Failed to derive pubkey: %v", err)
		}
		pubKeys2[i] = pubKey
	}

	// Generate addresses - they should be the same due to BIP67 sorting
	addr1, _, err := GenerateMultisigAddress(config, pubKeys1, network)
	if err != nil {
		t.Fatalf("Failed to generate address from order 1: %v", err)
	}

	addr2, _, err := GenerateMultisigAddress(config, pubKeys2, network)
	if err != nil {
		t.Fatalf("Failed to generate address from order 2: %v", err)
	}

	if addr1 != addr2 {
		t.Errorf("BIP67 sorting not working - different xpub order produced different addresses:\n  order1: %s\n  order2: %s",
			addr1, addr2)
	}

	t.Logf("Address (either order): %s", addr1)
}

// TestMultisigAddressConsistency verifies that address generation is deterministic
func TestMultisigAddressConsistency(t *testing.T) {
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}

	bip48Path := BIP48Path{
		CoinType:   1,
		Account:    0,
		ScriptType: MultisigNativeSegwit,
	}

	xpubStr, err := hdWallet.GetMultisigAccountXpub(bip48Path)
	if err != nil {
		t.Fatalf("Failed to derive xpub: %v", err)
	}

	network := &chaincfg.TestNet4Params
	config := &MultisigConfig{
		M:          2,
		N:          3,
		ScriptType: MultisigNativeSegwit,
	}

	testXpubs := []string{xpubStr, xpubStr, xpubStr}

	// Generate address twice and verify they match
	var firstAddress string
	for run := 0; run < 2; run++ {
		pubKeys := make([][]byte, 3)
		for i, xpubKey := range testXpubs {
			pubKey, err := DeriveChildPubKey(xpubKey, 0, 0, network)
			if err != nil {
				t.Fatalf("Run %d: Failed to derive pubkey: %v", run, err)
			}
			pubKeys[i] = pubKey
		}

		address, _, err := GenerateMultisigAddress(config, pubKeys, network)
		if err != nil {
			t.Fatalf("Run %d: Failed to generate address: %v", run, err)
		}

		if run == 0 {
			firstAddress = address
			t.Logf("First address: %s", address)
		} else {
			if address != firstAddress {
				t.Errorf("Address generation not deterministic:\n  first:  %s\n  second: %s", firstAddress, address)
			}
			t.Logf("Second address: %s", address)
		}
	}
}

// TestMultisigDeriveChildPubKey verifies basic pubkey derivation for multisig
func TestMultisigDeriveChildPubKey(t *testing.T) {
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}

	bip48Path := BIP48Path{
		CoinType:   1,
		Account:    0,
		ScriptType: MultisigNativeSegwit,
	}

	xpubStr, err := hdWallet.GetMultisigAccountXpub(bip48Path)
	if err != nil {
		t.Fatalf("Failed to derive xpub: %v", err)
	}

	network := &chaincfg.TestNet4Params

	// Derive first receive address pubkey
	pubKey, err := DeriveChildPubKey(xpubStr, 0, 0, network)
	if err != nil {
		t.Fatalf("Failed to derive pubkey: %v", err)
	}

	// Compressed pubkey should be 33 bytes
	if len(pubKey) != 33 {
		t.Errorf("Expected 33 byte compressed pubkey, got %d bytes", len(pubKey))
	}

	// First byte should be 0x02 or 0x03 for compressed
	if pubKey[0] != 0x02 && pubKey[0] != 0x03 {
		t.Errorf("Invalid compressed pubkey prefix: 0x%02x", pubKey[0])
	}

	t.Logf("Derived pubkey (first 8 bytes): %x...", pubKey[:8])
}

// TestMultisigChangeOutputMetadata verifies that BIP32 derivation paths and
// witness/redeem scripts are added to PSBT change outputs for multisig wallets.
// This is required by BIP174 (PSBT_OUT_BIP32_DERIVATION, PSBT_OUT_WITNESS_SCRIPT)
// so that hardware wallets can verify change goes back to the same wallet.
func TestMultisigChangeOutputMetadata(t *testing.T) {
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}

	fpHex, err := hdWallet.GetMasterFingerprintHex()
	if err != nil {
		t.Fatalf("Failed to get master fingerprint: %v", err)
	}

	// Generate 3 xpubs from different accounts for a real 2-of-3 multisig
	var cosigners []MultisigCosignerConfig
	for account := uint32(0); account < 3; account++ {
		bip48Path := BIP48Path{CoinType: 1, Account: account, ScriptType: MultisigNativeSegwit}
		xpub, err := hdWallet.GetMultisigAccountXpub(bip48Path)
		if err != nil {
			t.Fatalf("Failed to derive account %d xpub: %v", account, err)
		}
		cosigners = append(cosigners, MultisigCosignerConfig{
			Name:        "cosigner",
			Fingerprint: fpHex,
			Xpub:        xpub,
		})
	}
	_ = cosigners // used below in subtests

	tests := []struct {
		name            string
		scriptType      MultisigScriptType
		wantWitness     bool
		wantRedeem      bool
	}{
		{"NativeSegwit_P2WSH", MultisigNativeSegwit, true, false},
		{"NestedSegwit_P2SH-P2WSH", MultisigNestedSegwit, true, true},
		{"Legacy_P2SH", MultisigLegacy, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build per-scriptType cosigners (each scriptType needs its own xpub)
			var stCosigners []MultisigCosignerConfig
			for account := uint32(0); account < 3; account++ {
				bip48Path := BIP48Path{CoinType: 1, Account: account, ScriptType: tt.scriptType}
				xpub, err := hdWallet.GetMultisigAccountXpub(bip48Path)
				if err != nil {
					t.Fatalf("Failed to derive account %d xpub for %s: %v", account, tt.name, err)
				}
				stCosigners = append(stCosigners, MultisigCosignerConfig{
					Name:        "cosigner",
					Fingerprint: fpHex,
					Xpub:        xpub,
				})
			}

			m := &WalletManager{
				data: &WalletData{
					Network: "testnet",
					Multisig: &MultisigWalletConfig{
						M:          2,
						N:          3,
						ScriptType: int(tt.scriptType),
						Cosigners:  stCosigners,
					},
				},
				unlocked: true,
			}

			// Create a minimal PSBT with 2 outputs (recipient + change)
			tx := wire.NewMsgTx(2)
			tx.AddTxOut(wire.NewTxOut(50000, []byte{0x00, 0x14, 0x01, 0x02, 0x03}))  // recipient
			tx.AddTxOut(wire.NewTxOut(40000, []byte{0x00, 0x14, 0x04, 0x05, 0x06}))  // change
			packet, err := psbt.NewFromUnsignedTx(tx)
			if err != nil {
				t.Fatalf("Failed to create PSBT: %v", err)
			}

			changeAddrIndex := uint32(0)
			changeOutputIdx := 1 // second output is change

			err = m.addMultisigChangeOutputMetadata(packet, changeAddrIndex, changeOutputIdx)
			if err != nil {
				t.Fatalf("addMultisigChangeOutputMetadata failed: %v", err)
			}

			output := packet.Outputs[changeOutputIdx]

			// Verify BIP32 derivation entries
			if len(output.Bip32Derivation) != 3 {
				t.Errorf("Expected 3 Bip32Derivation entries (one per cosigner), got %d", len(output.Bip32Derivation))
			}
			for i, deriv := range output.Bip32Derivation {
				if len(deriv.PubKey) != 33 {
					t.Errorf("Derivation %d: expected 33-byte compressed pubkey, got %d bytes", i, len(deriv.PubKey))
				}
				if deriv.MasterKeyFingerprint == 0 {
					t.Errorf("Derivation %d: fingerprint is zero", i)
				}
				// Last two path components should be 1 (change) and 0 (index)
				pathLen := len(deriv.Bip32Path)
				if pathLen < 2 {
					t.Errorf("Derivation %d: path too short: %v", i, deriv.Bip32Path)
					continue
				}
				if deriv.Bip32Path[pathLen-2] != 1 {
					t.Errorf("Derivation %d: expected change=1, got %d", i, deriv.Bip32Path[pathLen-2])
				}
				if deriv.Bip32Path[pathLen-1] != 0 {
					t.Errorf("Derivation %d: expected index=0, got %d", i, deriv.Bip32Path[pathLen-1])
				}
			}

			// Verify witness script
			if tt.wantWitness {
				if len(output.WitnessScript) == 0 {
					t.Error("Expected WitnessScript on change output, got empty")
				}
			} else {
				if len(output.WitnessScript) != 0 {
					t.Error("Did not expect WitnessScript on change output")
				}
			}

			// Verify redeem script
			if tt.wantRedeem {
				if len(output.RedeemScript) == 0 {
					t.Error("Expected RedeemScript on change output, got empty")
				}
			} else {
				if len(output.RedeemScript) != 0 {
					t.Error("Did not expect RedeemScript on change output")
				}
			}

			// Verify recipient output is untouched
			recipientOutput := packet.Outputs[0]
			if len(recipientOutput.Bip32Derivation) != 0 {
				t.Error("Recipient output should not have Bip32Derivation")
			}

			t.Logf("Change output has %d BIP32 derivations, WitnessScript=%d bytes, RedeemScript=%d bytes",
				len(output.Bip32Derivation), len(output.WitnessScript), len(output.RedeemScript))
		})
	}
}

// TestMultisigScriptTypes verifies all supported multisig script types
func TestMultisigScriptTypes(t *testing.T) {
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}

	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}

	network := &chaincfg.TestNet4Params

	tests := []struct {
		name           string
		scriptType     MultisigScriptType
		expectedPrefix string
	}{
		{"NativeSegwit", MultisigNativeSegwit, "tb1q"},
		{"Legacy", MultisigLegacy, "2"}, // P2SH on testnet
		{"NestedSegwit", MultisigNestedSegwit, "2"}, // P2SH-P2WSH on testnet
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bip48Path := BIP48Path{
				CoinType:   1,
				Account:    0,
				ScriptType: tt.scriptType,
			}

			xpubStr, err := hdWallet.GetMultisigAccountXpub(bip48Path)
			if err != nil {
				t.Fatalf("Failed to derive xpub: %v", err)
			}

			config := &MultisigConfig{
				M:          2,
				N:          3,
				ScriptType: tt.scriptType,
			}

			testXpubs := []string{xpubStr, xpubStr, xpubStr}

			pubKeys := make([][]byte, 3)
			for i, xpubKey := range testXpubs {
				pubKey, err := DeriveChildPubKey(xpubKey, 0, 0, network)
				if err != nil {
					t.Fatalf("Failed to derive pubkey: %v", err)
				}
				pubKeys[i] = pubKey
			}

			address, witnessScript, err := GenerateMultisigAddress(config, pubKeys, network)
			if err != nil {
				t.Fatalf("Failed to generate address: %v", err)
			}

			if len(address) < len(tt.expectedPrefix) || address[:len(tt.expectedPrefix)] != tt.expectedPrefix {
				t.Errorf("Invalid address prefix: got %s, want prefix %s", address, tt.expectedPrefix)
			}

			t.Logf("%s address: %s", tt.name, address)
			t.Logf("Witness script length: %d bytes", len(witnessScript))
		})
	}
}
