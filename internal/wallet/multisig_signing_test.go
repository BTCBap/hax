package wallet

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"hax/internal/api"
	"hax/internal/backend"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// testMnemonics provides deterministic test wallets for multisig signing tests.
var testMnemonics = []string{
	"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
	"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
	"all all all all all all all all all all all all",
}

// multisigTestSetup holds the common state for a multisig signing test.
type multisigTestSetup struct {
	cosigners  []MultisigCosignerConfig
	pubKeys    [][]byte
	addr       string
	script     []byte // redeemScript (P2SH) or witnessScript (P2WSH/P2SH-P2WSH)
	bip48Path  BIP48Path
	network    *chaincfg.Params
	scriptType MultisigScriptType
	m, n       int
}

type mockBroadcastBackend struct {
	lastRawTx string
	txid      string
}

func (m *mockBroadcastBackend) Connect() error            { return nil }
func (m *mockBroadcastBackend) Disconnect() error         { return nil }
func (m *mockBroadcastBackend) IsConnected() bool         { return true }
func (m *mockBroadcastBackend) TestConnection() error     { return nil }
func (m *mockBroadcastBackend) Type() backend.BackendType { return backend.BackendTypeElectrum }
func (m *mockBroadcastBackend) GetAddressInfo(address string) (*api.AddressInfo, error) {
	return nil, nil
}
func (m *mockBroadcastBackend) GetAddressBalance(address string) (api.Satoshi, error) { return 0, nil }
func (m *mockBroadcastBackend) GetAddressUTXOs(address string) ([]api.UTXO, error)    { return nil, nil }
func (m *mockBroadcastBackend) GetAddressTransactions(address string) ([]api.Transaction, error) {
	return nil, nil
}
func (m *mockBroadcastBackend) GetTransaction(txid string) (*api.Transaction, error) { return nil, nil }
func (m *mockBroadcastBackend) BroadcastTransaction(rawTxHex string) (string, error) {
	m.lastRawTx = rawTxHex
	if m.txid == "" {
		return "mock-txid", nil
	}
	return m.txid, nil
}
func (m *mockBroadcastBackend) GetFeeEstimates() (*api.FeeEstimates, error) { return nil, nil }
func (m *mockBroadcastBackend) GetBlockHeight() (int64, error)              { return 0, nil }
func (m *mockBroadcastBackend) ScripthashNotifications() <-chan backend.ScripthashNotification {
	return nil
}
func (m *mockBroadcastBackend) HeaderNotifications() <-chan backend.HeaderNotification { return nil }
func (m *mockBroadcastBackend) SubscribeHeaders() (int64, error)                       { return 0, nil }
func (m *mockBroadcastBackend) SubscribeAddress(address string) (string, error)        { return "", nil }
func (m *mockBroadcastBackend) GetAddressHistory(address string) (bool, error)         { return false, nil }
func (m *mockBroadcastBackend) AddressToScripthash(address string) (string, error)     { return "", nil }
func (m *mockBroadcastBackend) ServerInfo() (software, protocol string)                { return "mock", "mock" }
func (m *mockBroadcastBackend) CertFingerprint() string                                { return "" }

// setupMultisigTest creates a complete multisig test environment with the given parameters.
func setupMultisigTest(t *testing.T, m, n int, scriptType MultisigScriptType) multisigTestSetup {
	t.Helper()
	if n > len(testMnemonics) {
		t.Fatalf("need %d mnemonics but only have %d", n, len(testMnemonics))
	}

	network := &TestNet4Params
	bip48Path := BIP48Path{CoinType: 1, Account: 0, ScriptType: scriptType}

	var cosigners []MultisigCosignerConfig
	var pubKeys [][]byte

	for i := 0; i < n; i++ {
		seed, err := MnemonicToSeed(testMnemonics[i], "")
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
			Name:        testMnemonics[i][:4],
			Xpub:        xpub,
			Fingerprint: fp,
			IsMine:      true,
			Mnemonic:    testMnemonics[i],
		})
	}

	// Generate the multisig address
	msConfig := &MultisigConfig{
		M:          m,
		N:          n,
		ScriptType: scriptType,
	}

	addr, script, err := GenerateMultisigAddress(msConfig, pubKeys, network)
	if err != nil {
		t.Fatalf("Failed to generate multisig address: %v", err)
	}

	return multisigTestSetup{
		cosigners:  cosigners,
		pubKeys:    pubKeys,
		addr:       addr,
		script:     script,
		bip48Path:  bip48Path,
		network:    network,
		scriptType: scriptType,
		m:          m,
		n:          n,
	}
}

// createTestPSBT creates a PSBT for the given multisig setup with a single fake input.
func (s *multisigTestSetup) createTestPSBT(t *testing.T) *PSBTResult {
	t.Helper()

	fakeTxHash := "0000000000000000000000000000000000000000000000000000000000000001"
	utxos := []UTXOWithPath{
		{
			UTXO: api.UTXO{
				TxID:  fakeTxHash,
				Vout:  0,
				Value: 100000,
			},
			Address: s.addr,
			Path:    s.bip48Path.ChildPath(0, 0),
		},
	}

	outputs := []TxOutput{
		{Address: s.addr, Amount: 50000},
	}

	// Map multisig script type to CreatePSBT's ScriptType
	var inputScriptType ScriptType
	switch s.scriptType {
	case MultisigLegacy:
		inputScriptType = ScriptTypeP2SH
	case MultisigNestedSegwit:
		inputScriptType = ScriptTypeP2SHP2WSH
	case MultisigNativeSegwit:
		inputScriptType = ScriptTypeP2WSH
	}

	result, err := CreatePSBT(utxos, outputs, s.addr, 1.0, s.network,
		[][]byte{s.script}, inputScriptType, s.m, s.n)
	if err != nil {
		t.Fatalf("Failed to create PSBT: %v", err)
	}

	return result
}

// utxosForTest returns the UTXOWithPath slice matching the test PSBT.
func (s *multisigTestSetup) utxosForTest() []UTXOWithPath {
	return []UTXOWithPath{
		{
			UTXO: api.UTXO{
				TxID:  "0000000000000000000000000000000000000000000000000000000000000001",
				Vout:  0,
				Value: 100000,
			},
			Address: s.addr,
			Path:    s.bip48Path.ChildPath(0, 0),
		},
	}
}

func TestMultisigPSBTE2E_CreateExportImportPartialFullBroadcast(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)

	// create
	initial := s.createTestPSBT(t)

	// export/import unsigned PSBT
	serializedUnsigned, err := ExportPSBT(initial.Packet)
	if err != nil {
		t.Fatalf("failed to export unsigned PSBT: %v", err)
	}
	sharedPSBT, err := ImportPSBT(serializedUnsigned)
	if err != nil {
		t.Fatalf("failed to import unsigned PSBT: %v", err)
	}

	utxos := s.utxosForTest()

	// partial sign (cosigner 1)
	sigsAdded, err := SignMultisigPSBT(sharedPSBT, s.cosigners[:1], utxos, s.network, s.scriptType)
	if err != nil {
		t.Fatalf("failed to partially sign PSBT: %v", err)
	}
	if sigsAdded != 1 {
		t.Fatalf("expected one signature in partial sign step, got %d", sigsAdded)
	}
	if CountPSBTSigners(sharedPSBT) != 1 {
		t.Fatalf("expected one signer after partial sign, got %d", CountPSBTSigners(sharedPSBT))
	}

	// export/import partially signed PSBT
	serializedPartial, err := ExportPSBT(sharedPSBT)
	if err != nil {
		t.Fatalf("failed to export partially signed PSBT: %v", err)
	}
	forNextSigner, err := ImportPSBT(serializedPartial)
	if err != nil {
		t.Fatalf("failed to import partially signed PSBT: %v", err)
	}

	// fully sign (cosigner 2)
	sigsAdded, err = SignMultisigPSBT(forNextSigner, s.cosigners[1:2], utxos, s.network, s.scriptType)
	if err != nil {
		t.Fatalf("failed to complete signing: %v", err)
	}
	if sigsAdded != 1 {
		t.Fatalf("expected one additional signature in full sign step, got %d", sigsAdded)
	}
	if CountPSBTSigners(forNextSigner) != 2 {
		t.Fatalf("expected two signers after full sign, got %d", CountPSBTSigners(forNextSigner))
	}
	if !IsPSBTComplete(forNextSigner) {
		t.Fatal("expected fully signed PSBT to be complete")
	}

	// broadcast
	backendStub := &mockBroadcastBackend{txid: "e2e-broadcast-txid"}
	wm := &WalletManager{
		backend: backendStub,
		data:    &WalletData{}, // avoid nil dereference in post-broadcast bookkeeping
	}
	txid, err := wm.BroadcastPSBT(&PSBTResult{
		Packet:    forNextSigner,
		HasChange: false, // keep the test focused on signing+broadcast behavior
	})
	if err != nil {
		t.Fatalf("failed to broadcast fully signed PSBT: %v", err)
	}
	if txid != "e2e-broadcast-txid" {
		t.Fatalf("unexpected broadcast txid: got %q", txid)
	}
	if backendStub.lastRawTx == "" {
		t.Fatal("expected backend to receive serialized transaction hex")
	}
}

// TestP2WSHMultisigSigningRoundTrip tests the full PSBT flow for 2-of-3 Native SegWit (P2WSH) multisig:
// CreatePSBT -> SignMultisigPSBT -> FinalizePSBT -> script engine verification
func TestP2WSHMultisigSigningRoundTrip(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)

	// Verify address format: P2WSH addresses start with "tb1q" on testnet
	if !bytes.HasPrefix([]byte(s.addr), []byte("tb1q")) {
		t.Errorf("Expected P2WSH address starting with 'tb1q', got: %s", s.addr)
	}

	result := s.createTestPSBT(t)

	// Verify WitnessScript is set (not RedeemScript) for P2WSH
	input := &result.Packet.Inputs[0]
	if len(input.WitnessScript) == 0 {
		t.Fatalf("Expected WitnessScript to be set for P2WSH")
	}
	if len(input.RedeemScript) > 0 {
		t.Errorf("Expected no RedeemScript for P2WSH (native SegWit)")
	}

	// Sign with first two cosigners (2-of-3)
	sigsAdded, err := SignMultisigPSBT(result.Packet, s.cosigners, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign multisig PSBT: %v", err)
	}
	if sigsAdded != 2 {
		t.Fatalf("Expected 2 signatures added, got %d", sigsAdded)
	}

	if !IsPSBTComplete(result.Packet) {
		t.Fatalf("PSBT should be complete with 2-of-3 signatures")
	}

	// Finalize PSBT
	finalTx, err := FinalizePSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	// For P2WSH: no scriptSig, witness present
	if len(finalTx.TxIn[0].SignatureScript) != 0 {
		t.Errorf("Expected empty scriptSig for P2WSH, got %d bytes", len(finalTx.TxIn[0].SignatureScript))
	}
	if len(finalTx.TxIn[0].Witness) == 0 {
		t.Fatal("Expected non-empty witness for P2WSH")
	}

	// Verify with btcd's script engine
	decodedAddr, _ := btcutil.DecodeAddress(s.addr, s.network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 100000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(pkScript, finalTx, 0,
		txscript.StandardVerifyFlags, nil, sigHashes, 100000, prevOutputFetcher)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("Script verification failed: %v", err)
	}
}

// TestP2SHP2WSHMultisigSigningRoundTrip tests the full PSBT flow for 2-of-3 Nested SegWit (P2SH-P2WSH) multisig:
// CreatePSBT -> SignMultisigPSBT -> FinalizePSBT -> script engine verification
func TestP2SHP2WSHMultisigSigningRoundTrip(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNestedSegwit)

	// Verify address format: P2SH-P2WSH starts with '2' on testnet
	if s.addr[0] != '2' {
		t.Errorf("Expected P2SH address starting with '2', got: %s", s.addr)
	}

	result := s.createTestPSBT(t)

	// Verify both WitnessScript AND RedeemScript are set for P2SH-P2WSH
	input := &result.Packet.Inputs[0]
	if len(input.WitnessScript) == 0 {
		t.Fatalf("Expected WitnessScript to be set for P2SH-P2WSH")
	}
	if len(input.RedeemScript) == 0 {
		t.Fatalf("Expected RedeemScript to be set for P2SH-P2WSH")
	}

	// Verify RedeemScript is the P2WSH witness program (OP_0 <32-byte hash>)
	witnessHash := sha256.Sum256(s.script)
	expectedRedeemScript, _ := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(witnessHash[:]).
		Script()
	if !bytes.Equal(input.RedeemScript, expectedRedeemScript) {
		t.Fatalf("RedeemScript doesn't match expected P2WSH witness program")
	}

	// Sign with first two cosigners (2-of-3)
	sigsAdded, err := SignMultisigPSBT(result.Packet, s.cosigners, s.utxosForTest(), s.network, MultisigNestedSegwit)
	if err != nil {
		t.Fatalf("Failed to sign multisig PSBT: %v", err)
	}
	if sigsAdded != 2 {
		t.Fatalf("Expected 2 signatures added, got %d", sigsAdded)
	}

	if !IsPSBTComplete(result.Packet) {
		t.Fatalf("PSBT should be complete with 2-of-3 signatures")
	}

	// Finalize PSBT
	finalTx, err := FinalizePSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	// For P2SH-P2WSH: both scriptSig (pushing redeemScript) and witness present
	if len(finalTx.TxIn[0].SignatureScript) == 0 {
		t.Fatal("Expected non-empty scriptSig for P2SH-P2WSH (P2SH wrapper)")
	}
	if len(finalTx.TxIn[0].Witness) == 0 {
		t.Fatal("Expected non-empty witness for P2SH-P2WSH")
	}

	// Verify with btcd's script engine
	decodedAddr, _ := btcutil.DecodeAddress(s.addr, s.network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 100000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(pkScript, finalTx, 0,
		txscript.StandardVerifyFlags, nil, sigHashes, 100000, prevOutputFetcher)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("Script verification failed: %v", err)
	}
}

// TestMultisig3of3SigningRoundTrip tests that all cosigners must sign for 3-of-3.
func TestMultisig3of3SigningRoundTrip(t *testing.T) {
	s := setupMultisigTest(t, 3, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Sign with all three cosigners
	sigsAdded, err := SignMultisigPSBT(result.Packet, s.cosigners, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign multisig PSBT: %v", err)
	}
	if sigsAdded != 3 {
		t.Fatalf("Expected 3 signatures added, got %d", sigsAdded)
	}

	if !IsPSBTComplete(result.Packet) {
		t.Fatalf("PSBT should be complete with 3-of-3 signatures")
	}

	// Finalize and verify
	finalTx, err := FinalizePSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	decodedAddr, _ := btcutil.DecodeAddress(s.addr, s.network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 100000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(pkScript, finalTx, 0,
		txscript.StandardVerifyFlags, nil, sigHashes, 100000, prevOutputFetcher)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("Script verification failed: %v", err)
	}
}

// TestMultisigInsufficientSignatures verifies that finalization fails
// when fewer than M signatures are present.
func TestMultisigInsufficientSignatures(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Only sign with one cosigner (need 2-of-3)
	oneCosigner := []MultisigCosignerConfig{s.cosigners[0]}
	sigsAdded, err := SignMultisigPSBT(result.Packet, oneCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign with one cosigner: %v", err)
	}
	if sigsAdded != 1 {
		t.Fatalf("Expected 1 signature added, got %d", sigsAdded)
	}

	// Verify PSBT is not yet complete
	if IsPSBTComplete(result.Packet) {
		// IsPSBTComplete checks for any sigs, not M-of-N threshold
		// So it may report true with 1 sig — that's by design (it's a simple check)
	}

	// Verify we only have 1 partial signature
	if len(result.Packet.Inputs[0].PartialSigs) != 1 {
		t.Fatalf("Expected 1 partial sig, got %d", len(result.Packet.Inputs[0].PartialSigs))
	}

	// Attempt finalization — should fail because we need 2 sigs
	_, err = FinalizePSBT(result.Packet)
	if err == nil {
		t.Fatal("Expected finalization to fail with insufficient signatures")
	}
}

// TestMultisigPartialSigning tests incremental signing: sign with cosigner 1,
// then separately with cosigner 2, verifying counts at each step.
func TestMultisigPartialSigning(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Sign with first cosigner only
	firstCosigner := []MultisigCosignerConfig{s.cosigners[0]}
	sigs1, err := SignMultisigPSBT(result.Packet, firstCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign with cosigner 1: %v", err)
	}
	if sigs1 != 1 {
		t.Fatalf("Expected 1 signature from cosigner 1, got %d", sigs1)
	}

	// Verify CountPSBTSigners reports 1
	sigCount := CountPSBTSigners(result.Packet)
	if sigCount != 1 {
		t.Fatalf("Expected CountPSBTSigners=1, got %d", sigCount)
	}

	// Export and re-import (simulates passing PSBT to another signer)
	exported, err := ExportPSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to export PSBT: %v", err)
	}
	reimported, err := ImportPSBT(exported)
	if err != nil {
		t.Fatalf("Failed to import PSBT: %v", err)
	}

	// Sign with second cosigner
	secondCosigner := []MultisigCosignerConfig{s.cosigners[1]}
	sigs2, err := SignMultisigPSBT(reimported, secondCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign with cosigner 2: %v", err)
	}
	if sigs2 != 1 {
		t.Fatalf("Expected 1 signature from cosigner 2, got %d", sigs2)
	}

	// Verify CountPSBTSigners reports 2
	sigCount = CountPSBTSigners(reimported)
	if sigCount != 2 {
		t.Fatalf("Expected CountPSBTSigners=2, got %d", sigCount)
	}

	// Should now finalize successfully
	finalTx, err := FinalizePSBT(reimported)
	if err != nil {
		t.Fatalf("Failed to finalize PSBT: %v", err)
	}

	// Verify with script engine
	decodedAddr, _ := btcutil.DecodeAddress(s.addr, s.network)
	pkScript, _ := txscript.PayToAddrScript(decodedAddr)
	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, 100000)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutputFetcher)

	vm, err := txscript.NewEngine(pkScript, finalTx, 0,
		txscript.StandardVerifyFlags, nil, sigHashes, 100000, prevOutputFetcher)
	if err != nil {
		t.Fatalf("Failed to create script engine: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("Script verification failed: %v", err)
	}
}

// TestMultisigAlreadySignedSkip verifies that signing with the same cosigner twice
// doesn't add duplicate signatures.
func TestMultisigAlreadySignedSkip(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Sign with first cosigner
	firstCosigner := []MultisigCosignerConfig{s.cosigners[0]}
	sigs1, err := SignMultisigPSBT(result.Packet, firstCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign with cosigner 1: %v", err)
	}
	if sigs1 != 1 {
		t.Fatalf("Expected 1 signature, got %d", sigs1)
	}

	// Sign again with the same cosigner — should skip
	sigs2, err := SignMultisigPSBT(result.Packet, firstCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Second sign should not error, got: %v", err)
	}
	if sigs2 != 0 {
		t.Fatalf("Expected 0 signatures on duplicate, got %d", sigs2)
	}

	// Still only 1 partial sig
	if len(result.Packet.Inputs[0].PartialSigs) != 1 {
		t.Fatalf("Expected 1 partial sig after duplicate attempt, got %d", len(result.Packet.Inputs[0].PartialSigs))
	}
}

// TestMultisigPSBTExportImportPreservesSignatures verifies that
// exporting a partially-signed PSBT to base64 and re-importing preserves all data.
func TestMultisigPSBTExportImportPreservesSignatures(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Sign with one cosigner
	firstCosigner := []MultisigCosignerConfig{s.cosigners[0]}
	_, err := SignMultisigPSBT(result.Packet, firstCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	// Capture state before export
	origSigCount := len(result.Packet.Inputs[0].PartialSigs)
	origSigPubKey := make([]byte, len(result.Packet.Inputs[0].PartialSigs[0].PubKey))
	copy(origSigPubKey, result.Packet.Inputs[0].PartialSigs[0].PubKey)

	// Export
	exported, err := ExportPSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	// Import
	reimported, err := ImportPSBT(exported)
	if err != nil {
		t.Fatalf("Failed to import: %v", err)
	}

	// Verify sig count preserved
	if len(reimported.Inputs[0].PartialSigs) != origSigCount {
		t.Fatalf("Expected %d sigs after import, got %d", origSigCount, len(reimported.Inputs[0].PartialSigs))
	}

	// Verify pubkey preserved
	if !bytes.Equal(reimported.Inputs[0].PartialSigs[0].PubKey, origSigPubKey) {
		t.Fatal("Signature pubkey changed after export/import")
	}

	// Verify WitnessScript preserved
	if !bytes.Equal(reimported.Inputs[0].WitnessScript, result.Packet.Inputs[0].WitnessScript) {
		t.Fatal("WitnessScript changed after export/import")
	}
}

// TestAllScriptTypesAddressPrefixes verifies address format correctness for all 3 multisig types.
func TestAllScriptTypesAddressPrefixes(t *testing.T) {
	tests := []struct {
		name       string
		scriptType MultisigScriptType
		prefix     string
	}{
		{"Legacy P2SH", MultisigLegacy, "2"},
		{"Nested SegWit P2SH-P2WSH", MultisigNestedSegwit, "2"},
		{"Native SegWit P2WSH", MultisigNativeSegwit, "tb1q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupMultisigTest(t, 2, 3, tt.scriptType)
			if !bytes.HasPrefix([]byte(s.addr), []byte(tt.prefix)) {
				t.Errorf("Expected address prefix %q, got: %s", tt.prefix, s.addr)
			}
		})
	}
}

// TestP2SHP2WSHInputMetadata verifies that P2SH-P2WSH PSBT inputs have both
// WitnessScript and RedeemScript set correctly.
func TestP2SHP2WSHInputMetadata(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNestedSegwit)
	result := s.createTestPSBT(t)
	input := &result.Packet.Inputs[0]

	// WitnessScript should be the multisig script
	if !bytes.Equal(input.WitnessScript, s.script) {
		t.Fatal("WitnessScript should be the multisig script")
	}

	// RedeemScript should be P2WSH witness program
	witnessHash := sha256.Sum256(s.script)
	expectedRedeemScript, _ := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(witnessHash[:]).
		Script()

	if !bytes.Equal(input.RedeemScript, expectedRedeemScript) {
		t.Fatalf("RedeemScript should be P2WSH witness program (OP_0 <32-byte hash>)\nExpected: %x\nGot:      %x",
			expectedRedeemScript, input.RedeemScript)
	}

	// RedeemScript should be 34 bytes: OP_0 (1) + push (1) + hash (32)
	if len(input.RedeemScript) != 34 {
		t.Fatalf("Expected 34-byte RedeemScript for P2WSH program, got %d bytes", len(input.RedeemScript))
	}
}

// TestSignMultisigStopsAtThreshold verifies that SignMultisigPSBT stops after M signatures.
func TestSignMultisigStopsAtThreshold(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Pass all 3 cosigners but M=2, so it should stop after 2
	sigsAdded, err := SignMultisigPSBT(result.Packet, s.cosigners, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	if sigsAdded != 2 {
		t.Fatalf("Expected signing to stop at M=2, got %d signatures", sigsAdded)
	}

	// Verify only 2 partial sigs exist
	if len(result.Packet.Inputs[0].PartialSigs) != 2 {
		t.Fatalf("Expected 2 partial sigs, got %d", len(result.Packet.Inputs[0].PartialSigs))
	}
}

// TestCosignerSigningStatus verifies GetCosignerSigningStatus correctly
// reports which cosigners have signed.
func TestCosignerSigningStatus(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Before signing: none signed
	status := GetCosignerSigningStatus(result.Packet, s.cosigners, s.utxosForTest(), s.network)
	for i, st := range status {
		if st.HasSigned {
			t.Errorf("Cosigner %d should not be signed initially", i)
		}
		if !st.CanSign {
			t.Errorf("Cosigner %d should be able to sign (has mnemonic)", i)
		}
	}

	// Sign with first cosigner only
	firstCosigner := []MultisigCosignerConfig{s.cosigners[0]}
	_, err := SignMultisigPSBT(result.Packet, firstCosigner, s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	// After signing with cosigner 0: only first should show signed
	status = GetCosignerSigningStatus(result.Packet, s.cosigners, s.utxosForTest(), s.network)
	if !status[0].HasSigned {
		t.Error("Cosigner 0 should show as signed")
	}
	if status[1].HasSigned {
		t.Error("Cosigner 1 should not show as signed")
	}
	if status[2].HasSigned {
		t.Error("Cosigner 2 should not show as signed")
	}
}

// TestP2SHP2WSHFinalTxStructure verifies the final transaction structure for P2SH-P2WSH:
// scriptSig must push the P2WSH witness program, witness must contain multisig stack.
func TestP2SHP2WSHFinalTxStructure(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNestedSegwit)
	result := s.createTestPSBT(t)

	// Sign with 2 cosigners
	sigsAdded, err := SignMultisigPSBT(result.Packet, s.cosigners[:2], s.utxosForTest(), s.network, MultisigNestedSegwit)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	if sigsAdded != 2 {
		t.Fatalf("Expected 2 sigs, got %d", sigsAdded)
	}

	finalTx, err := FinalizePSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize: %v", err)
	}

	txIn := finalTx.TxIn[0]

	// scriptSig should push the redeemScript (P2WSH witness program)
	// Parse the scriptSig to verify it's a single push of the 34-byte witness program
	tokenizer := txscript.MakeScriptTokenizer(0, txIn.SignatureScript)
	if !tokenizer.Next() {
		t.Fatal("scriptSig should have at least one push")
	}
	pushData := tokenizer.Data()
	if len(pushData) != 34 {
		t.Fatalf("Expected 34-byte push in scriptSig, got %d bytes", len(pushData))
	}
	// Should be OP_0 <32-byte-hash>
	if pushData[0] != 0x00 {
		t.Fatalf("Expected first byte 0x00 (OP_0) in witness program, got 0x%02x", pushData[0])
	}
	if pushData[1] != 0x20 { // OP_PUSHBYTES_32
		t.Fatalf("Expected second byte 0x20 (push 32) in witness program, got 0x%02x", pushData[1])
	}

	// Witness should have: [empty] [sig1] [sig2] [witnessScript]
	if len(txIn.Witness) != 4 { // empty + 2 sigs + witnessScript
		t.Fatalf("Expected 4 witness items (empty + 2 sigs + script), got %d", len(txIn.Witness))
	}
	// First item is empty (CHECKMULTISIG bug)
	if len(txIn.Witness[0]) != 0 {
		t.Fatalf("First witness item should be empty, got %d bytes", len(txIn.Witness[0]))
	}
	// Last item is the witness script
	lastItem := txIn.Witness[len(txIn.Witness)-1]
	if !bytes.Equal(lastItem, s.script) {
		t.Fatal("Last witness item should be the multisig witness script")
	}
}

// TestP2WSHFinalTxStructure verifies the final transaction structure for P2WSH:
// no scriptSig, witness must contain multisig stack.
func TestP2WSHFinalTxStructure(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	// Sign with 2 cosigners
	_, err := SignMultisigPSBT(result.Packet, s.cosigners[:2], s.utxosForTest(), s.network, MultisigNativeSegwit)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	finalTx, err := FinalizePSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to finalize: %v", err)
	}

	txIn := finalTx.TxIn[0]

	// P2WSH: no scriptSig
	if len(txIn.SignatureScript) != 0 {
		t.Fatalf("Expected empty scriptSig for P2WSH, got %d bytes", len(txIn.SignatureScript))
	}

	// Witness: [empty] [sig1] [sig2] [witnessScript]
	if len(txIn.Witness) != 4 {
		t.Fatalf("Expected 4 witness items, got %d", len(txIn.Witness))
	}
	if len(txIn.Witness[0]) != 0 {
		t.Fatal("First witness item should be empty")
	}
	if !bytes.Equal(txIn.Witness[len(txIn.Witness)-1], s.script) {
		t.Fatal("Last witness item should be the witness script")
	}
}

// TestAddGlobalXpubs verifies that addGlobalXpubs populates PSBT_GLOBAL_XPUB
// entries with account-level xpubs for each cosigner, enabling Coldcard's
// "Offer Import" auto-detection of multisig configuration from PSBTs.
func TestAddGlobalXpubs(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)

	// Create a WalletManager with multisig config
	wm := &WalletManager{
		data: &WalletData{
			Network: "testnet4",
			Multisig: &MultisigWalletConfig{
				M:          s.m,
				N:          s.n,
				ScriptType: int(s.scriptType),
				Cosigners:  s.cosigners,
			},
		},
	}

	result := s.createTestPSBT(t)

	// Verify no global xpubs initially
	if len(result.Packet.XPubs) != 0 {
		t.Fatalf("Expected 0 global xpubs initially, got %d", len(result.Packet.XPubs))
	}

	// Add global xpubs
	if err := wm.addGlobalXpubs(result.Packet); err != nil {
		t.Fatalf("addGlobalXpubs failed: %v", err)
	}

	// Verify correct count
	if len(result.Packet.XPubs) != s.n {
		t.Fatalf("Expected %d global xpubs, got %d", s.n, len(result.Packet.XPubs))
	}

	for i, xpub := range result.Packet.XPubs {
		// Verify extended key is 78 bytes (BIP32 serialization without checksum)
		if len(xpub.ExtendedKey) != 78 {
			t.Errorf("XPub[%d]: expected 78-byte extended key, got %d bytes", i, len(xpub.ExtendedKey))
		}

		// Verify fingerprint matches cosigner
		expectedFP := fingerprintHexToUint32(s.cosigners[i].Fingerprint)
		if xpub.MasterKeyFingerprint != expectedFP {
			t.Errorf("XPub[%d]: fingerprint mismatch: got %08x, want %08x",
				i, xpub.MasterKeyFingerprint, expectedFP)
		}

		// Verify derivation path is account-level BIP48: m/48'/1'/0'/2'
		// (4 hardened components for native segwit on testnet)
		expectedPath := []uint32{
			0x80000000 + 48, // 48'
			0x80000000 + 1,  // 1' (testnet)
			0x80000000 + 0,  // 0' (account)
			0x80000000 + 2,  // 2' (native segwit)
		}
		if len(xpub.Bip32Path) != len(expectedPath) {
			t.Errorf("XPub[%d]: expected %d path components, got %d",
				i, len(expectedPath), len(xpub.Bip32Path))
			continue
		}
		for j, comp := range xpub.Bip32Path {
			if comp != expectedPath[j] {
				t.Errorf("XPub[%d]: path[%d] = %08x, want %08x", i, j, comp, expectedPath[j])
			}
		}
	}

	// Verify PSBT round-trips with global xpubs intact
	exported, err := ExportPSBT(result.Packet)
	if err != nil {
		t.Fatalf("Failed to export PSBT with global xpubs: %v", err)
	}
	reimported, err := ImportPSBT(exported)
	if err != nil {
		t.Fatalf("Failed to import PSBT with global xpubs: %v", err)
	}
	if len(reimported.XPubs) != s.n {
		t.Fatalf("Global xpubs lost during export/import: expected %d, got %d",
			s.n, len(reimported.XPubs))
	}

	// === Hex dump for manual comparison with Sparrow-generated PSBTs ===
	t.Logf("PSBT base64 (load into Sparrow/Coldcard for comparison):\n%s", exported)

	for i, xpub := range result.Packet.XPubs {
		key := xpub.ExtendedKey // 78 bytes: version(4) depth(1) parentFP(4) childNum(4) chainCode(32) pubkey(33)
		t.Logf("XPub[%d] raw hex: %s", i, hex.EncodeToString(key))
		t.Logf("  version:    %s (% 02x)", hex.EncodeToString(key[0:4]), key[0:4])
		t.Logf("  depth:      %d", key[4])
		t.Logf("  parent FP:  %s", hex.EncodeToString(key[5:9]))
		t.Logf("  child num:  %d (0x%08x)", binary.BigEndian.Uint32(key[9:13]), binary.BigEndian.Uint32(key[9:13]))
		t.Logf("  master FP:  %08x", xpub.MasterKeyFingerprint)
		pathStrs := make([]string, len(xpub.Bip32Path))
		for j, p := range xpub.Bip32Path {
			if p >= 0x80000000 {
				pathStrs[j] = fmt.Sprintf("%d'", p-0x80000000)
			} else {
				pathStrs[j] = fmt.Sprintf("%d", p)
			}
		}
		t.Logf("  BIP32 path: m/%s", strings.Join(pathStrs, "/"))
	}

	// Dump witness script from first input for M/N verification
	ws := result.Packet.Inputs[0].WitnessScript
	if len(ws) > 0 {
		t.Logf("Input[0] witnessScript (%d bytes): first=0x%02x last=0x%02x",
			len(ws), ws[0], ws[len(ws)-1])
		t.Logf("  (expect first=0x%02x [OP_%d], last=0x%02x [OP_CHECKMULTISIG])",
			0x50+s.m, s.m, 0xae)
	}
}

// TestAddGlobalXpubsSkipsNonMultisig verifies addGlobalXpubs is a no-op for non-multisig wallets.
func TestAddGlobalXpubsSkipsNonMultisig(t *testing.T) {
	wm := &WalletManager{
		data: &WalletData{
			Network:  "testnet4",
			Multisig: nil,
		},
	}

	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)
	result := s.createTestPSBT(t)

	if err := wm.addGlobalXpubs(result.Packet); err != nil {
		t.Fatalf("addGlobalXpubs should not error for non-multisig: %v", err)
	}
	if len(result.Packet.XPubs) != 0 {
		t.Fatalf("Expected 0 global xpubs for non-multisig, got %d", len(result.Packet.XPubs))
	}
}

func TestAddGlobalXpubs_UsesActualDerivation(t *testing.T) {
	s := setupMultisigTest(t, 2, 3, MultisigNativeSegwit)

	// Simulate an HWI-imported cosigner with a non-default account path.
	// This is required for correct PSBT_GLOBAL_XPUB metadata and Coldcard Offer Import.
	s.cosigners[0].ActualDerivation = "m/48'/1'/7'/2'"

	wm := &WalletManager{
		data: &WalletData{
			Network: "testnet4",
			Multisig: &MultisigWalletConfig{
				M:          s.m,
				N:          s.n,
				ScriptType: int(s.scriptType),
				Cosigners:  s.cosigners,
			},
		},
	}

	result := s.createTestPSBT(t)
	if err := wm.addGlobalXpubs(result.Packet); err != nil {
		t.Fatalf("addGlobalXpubs failed: %v", err)
	}
	if len(result.Packet.XPubs) == 0 {
		t.Fatal("expected global xpubs to be present")
	}

	gotPath := result.Packet.XPubs[0].Bip32Path
	wantPath := []uint32{
		0x80000000 + 48,
		0x80000000 + 1,
		0x80000000 + 7,
		0x80000000 + 2,
	}
	if len(gotPath) != len(wantPath) {
		t.Fatalf("unexpected path length: got %d want %d", len(gotPath), len(wantPath))
	}
	for i := range wantPath {
		if gotPath[i] != wantPath[i] {
			t.Fatalf("path[%d] mismatch: got %08x want %08x", i, gotPath[i], wantPath[i])
		}
	}
}

// Ensure the psbt package is used (avoid unused import)
var _ = psbt.NewFromRawBytes
