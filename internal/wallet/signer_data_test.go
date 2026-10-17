package wallet

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/BTCBap/hax/internal/api"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// signerTestFingerprint is the master fingerprint of testMnemonic.
const signerTestFingerprint = "73c5da0a"

// fundedBackend serves one confirmed UTXO per funded address and the full
// funding transactions by txid.
type fundedBackend struct {
	mockBroadcastBackend
	params *chaincfg.Params
	funded map[string]*wire.MsgTx // address -> funding tx (output 0)
	// utxoFor serves explicit UTXOs, e.g. ones funded on a real node.
	utxoFor map[string]api.UTXO
	nextIn  uint32
}

func newFundedBackend(params *chaincfg.Params) *fundedBackend {
	return &fundedBackend{
		mockBroadcastBackend: mockBroadcastBackend{rawTxs: map[string]*wire.MsgTx{}},
		params:               params,
		funded:               map[string]*wire.MsgTx{},
	}
}

// fund creates a transaction paying value to addr at output 0.
func (b *fundedBackend) fund(t *testing.T, addr string, value int64) *wire.MsgTx {
	t.Helper()
	a, err := btcutil.DecodeAddress(addr, b.params)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := txscript.PayToAddrScript(a)
	if err != nil {
		t.Fatal(err)
	}
	tx := wire.NewMsgTx(2)
	// Distinct prevouts give distinct txids per funded address.
	prev := wire.OutPoint{Index: b.nextIn}
	b.nextIn++
	tx.AddTxIn(wire.NewTxIn(&prev, nil, nil))
	tx.AddTxOut(wire.NewTxOut(value, pk))
	b.funded[addr] = tx
	b.rawTxs[tx.TxHash().String()] = tx
	return tx
}

func (b *fundedBackend) GetAddressUTXOs(address string) ([]api.UTXO, error) {
	if u, ok := b.utxoFor[address]; ok {
		return []api.UTXO{u}, nil
	}
	tx, ok := b.funded[address]
	if !ok {
		return nil, nil
	}
	return []api.UTXO{{
		TxID:   tx.TxHash().String(),
		Vout:   0,
		Value:  api.Satoshi(tx.TxOut[0].Value),
		Status: api.Status{Confirmed: true, BlockHeight: 100},
	}}, nil
}

var signerTestTypes = []struct {
	name    string
	addr    AddressType
	purpose uint32
}{
	{"legacy", AddressTypeLegacy, 44},
	{"nested", AddressTypeSegWitNested, 49},
	{"native", AddressTypeSegWitNative, 84},
	{"taproot", AddressTypeTaproot, 86},
}

// accountXpub returns the account xpub for purpose on network, as a
// hardware wallet would export it.
func accountXpub(t *testing.T, purpose uint32, params *chaincfg.Params) string {
	t.Helper()
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatal(err)
	}
	k, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		t.Fatal(err)
	}
	coin := uint32(1)
	if params.Net == chaincfg.MainNetParams.Net {
		coin = 0
	}
	for _, c := range []uint32{purpose, coin, 0} {
		if k, err = k.Derive(hdkeychain.HardenedKeyStart + c); err != nil {
			t.Fatal(err)
		}
	}
	pub, err := k.Neuter()
	if err != nil {
		t.Fatal(err)
	}
	return pub.String()
}

// newSignerTestManager returns an unlocked wallet of the given type with
// its first receive address funded. hardware selects a watch-only wallet
// built from the account xpub.
func newSignerTestManager(t *testing.T, network string, addrType AddressType, purpose uint32, hardware bool) (*WalletManager, *fundedBackend) {
	t.Helper()
	params, err := ParamsForNetwork(network)
	if err != nil {
		t.Fatal(err)
	}
	b := newFundedBackend(params)
	m, err := NewWalletManager(t.TempDir(), b)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery"
	if hardware {
		coin := "1h"
		if network == NetworkMainnet {
			coin = "0h"
		}
		origin := fmt.Sprintf("%s/%dh/%s/0h", signerTestFingerprint, purpose, coin)
		if err := m.CreateHardwareWalletOnNetwork("w", pw, accountXpub(t, purpose, params),
			signerTestFingerprint, origin, addrType, "trezor", network); err != nil {
			t.Fatal(err)
		}
		if err := m.UnlockWallet("w", pw); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := m.RestoreWalletOnNetwork("w", pw, testMnemonic, "", network); err != nil {
			t.Fatal(err)
		}
		if err := m.UnlockWallet("w", pw); err != nil {
			t.Fatal(err)
		}
		if _, err := m.CreateAccount("Main Account", addrType, ""); err != nil {
			t.Fatal(err)
		}
	}
	m.StopBackgroundSync()
	t.Cleanup(m.LockWallet)

	addr, err := m.GetNewAddress(0)
	if err != nil {
		t.Fatal(err)
	}
	b.fund(t, addr, 100_000)
	return m, b
}

// verifyFinalTx runs every input of tx through the script interpreter.
func verifyFinalTx(t *testing.T, tx *wire.MsgTx, prevOuts map[wire.OutPoint]*wire.TxOut) {
	t.Helper()
	fetcher := txscript.NewMultiPrevOutFetcher(prevOuts)
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, in := range tx.TxIn {
		prev := prevOuts[in.PreviousOutPoint]
		vm, err := txscript.NewEngine(prev.PkScript, tx, i,
			txscript.StandardVerifyFlags, nil, hashes, prev.Value, fetcher)
		if err != nil {
			t.Fatalf("input %d: engine: %v", i, err)
		}
		if err := vm.Execute(); err != nil {
			t.Fatalf("input %d: script verification failed: %v", i, err)
		}
	}
}

func decodeTxHex(t *testing.T, s string) *wire.MsgTx {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	return &tx
}

func wantPath(purpose, coin, change, index uint32) []uint32 {
	h := uint32(hdkeychain.HardenedKeyStart)
	return []uint32{h + purpose, h + coin, h, change, index}
}

func equalPath(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCreatePSBT_SignerData checks that hax-built PSBTs carry the fields
// BIP174/BIP371 and hardware signers need, for every single-sig type,
// for software and hardware wallets, on mainnet and testnet4.
func TestCreatePSBT_SignerData(t *testing.T) {
	fp := fingerprintHexToUint32(signerTestFingerprint)
	for _, network := range []string{NetworkMainnet, NetworkTestnet} {
		coin := uint32(1)
		if network == NetworkMainnet {
			coin = 0
		}
		for _, hw := range []bool{false, true} {
			for _, tt := range signerTestTypes {
				name := fmt.Sprintf("%s/%s/hw=%v", network, tt.name, hw)
				t.Run(name, func(t *testing.T) {
					m, b := newSignerTestManager(t, network, tt.addr, tt.purpose, hw)
					dest, _, err := m.GetAddressByIndex(0, 5)
					if err != nil {
						t.Fatal(err)
					}
					res, err := m.CreatePSBTBatchTransaction(0,
						[]TxRecipient{{Address: dest, Amount: 40_000}}, 2, nil)
					if err != nil {
						t.Fatal(err)
					}
					if len(res.Warnings) != 0 {
						t.Fatalf("unexpected warnings: %v", res.Warnings)
					}
					if !res.HasChange {
						t.Fatal("expected a change output")
					}
					p := res.Packet
					in := p.Inputs[0]
					inPath, err := ParseBIP44Path(res.SelectedUTXOs[0].Path)
					if err != nil {
						t.Fatal(err)
					}
					_, changePath, _, err := m.GetCurrentChangeAddress(0)
					if err != nil {
						t.Fatal(err)
					}
					chPath, _ := ParseBIP44Path(changePath)
					funding := b.rawTxs[p.UnsignedTx.TxIn[0].PreviousOutPoint.Hash.String()]

					switch tt.addr {
					case AddressTypeTaproot:
						if in.NonWitnessUtxo != nil {
							t.Error("taproot input should not carry the previous transaction")
						}
						if in.WitnessUtxo == nil {
							t.Fatal("taproot input needs PSBT_IN_WITNESS_UTXO")
						}
					case AddressTypeLegacy:
						if in.WitnessUtxo != nil {
							t.Error("legacy input must not carry PSBT_IN_WITNESS_UTXO")
						}
						fallthrough
					default:
						if in.NonWitnessUtxo == nil || in.NonWitnessUtxo.TxHash() != funding.TxHash() {
							t.Fatal("missing or wrong PSBT_IN_NON_WITNESS_UTXO")
						}
					}

					if tt.addr == AddressTypeTaproot {
						if len(in.Bip32Derivation) != 0 {
							t.Error("taproot input should use PSBT_IN_TAP_BIP32_DERIVATION only")
						}
						if len(in.TaprootBip32Derivation) != 1 {
							t.Fatalf("taproot derivations = %d, want 1", len(in.TaprootBip32Derivation))
						}
						d := in.TaprootBip32Derivation[0]
						if d.MasterKeyFingerprint != fp || !equalPath(d.Bip32Path, wantPath(86, coin, 0, inPath.AddressIndex)) {
							t.Errorf("tap derivation fp=%08x path=%v", d.MasterKeyFingerprint, d.Bip32Path)
						}
						if len(d.LeafHashes) != 0 {
							t.Error("key-path only wallet should have no leaf hashes")
						}
						if !bytes.Equal(in.TaprootInternalKey, d.XOnlyPubKey) {
							t.Error("PSBT_IN_TAP_INTERNAL_KEY must equal the derived x-only key")
						}
						internal, err := schnorr.ParsePubKey(in.TaprootInternalKey)
						if err != nil {
							t.Fatal(err)
						}
						want, _ := txscript.PayToTaprootScript(txscript.ComputeTaprootKeyNoScript(internal))
						if !bytes.Equal(want, in.WitnessUtxo.PkScript) {
							t.Error("internal key does not tweak to the spent output key")
						}
						if in.SighashType != 0 {
							t.Errorf("taproot sighash = %d, want SIGHASH_DEFAULT", in.SighashType)
						}
					} else {
						if len(in.Bip32Derivation) != 1 {
							t.Fatalf("bip32 derivations = %d, want 1", len(in.Bip32Derivation))
						}
						d := in.Bip32Derivation[0]
						if d.MasterKeyFingerprint != fp || !equalPath(d.Bip32Path, wantPath(tt.purpose, coin, 0, inPath.AddressIndex)) {
							t.Errorf("derivation fp=%08x path=%v", d.MasterKeyFingerprint, d.Bip32Path)
						}
					}
					if tt.addr == AddressTypeSegWitNested {
						if len(in.RedeemScript) != 22 || !txscript.IsWitnessProgram(in.RedeemScript) {
							t.Errorf("nested input redeem script = %x", in.RedeemScript)
						}
					}

					// Change output: the signer must be able to recognise it.
					changeIdx := len(p.UnsignedTx.TxOut) - 1
					out := p.Outputs[changeIdx]
					if tt.addr == AddressTypeTaproot {
						if len(out.TaprootBip32Derivation) != 1 || len(out.TaprootInternalKey) != 32 {
							t.Fatal("taproot change needs PSBT_OUT_TAP_BIP32_DERIVATION and PSBT_OUT_TAP_INTERNAL_KEY")
						}
						if !equalPath(out.TaprootBip32Derivation[0].Bip32Path, wantPath(86, coin, 1, chPath.AddressIndex)) {
							t.Errorf("change path %v", out.TaprootBip32Derivation[0].Bip32Path)
						}
					} else {
						if len(out.Bip32Derivation) != 1 ||
							!equalPath(out.Bip32Derivation[0].Bip32Path, wantPath(tt.purpose, coin, 1, chPath.AddressIndex)) {
							t.Fatalf("change derivation = %+v", out.Bip32Derivation)
						}
						if tt.addr == AddressTypeSegWitNested && len(out.RedeemScript) != 22 {
							t.Error("nested change needs PSBT_OUT_REDEEM_SCRIPT")
						}
					}

					// The PSBT survives a serialization round trip.
					b64, err := ExportPSBT(p)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := ImportPSBT(b64); err != nil {
						t.Fatal(err)
					}

					prevOuts := map[wire.OutPoint]*wire.TxOut{
						p.UnsignedTx.TxIn[0].PreviousOutPoint: funding.TxOut[0],
					}
					if hw {
						// Stand in for the external signer: sign from the
						// PSBT's own derivation data, as an imported PSBT.
						seed, _ := MnemonicToSeed(testMnemonic, "")
						signer, _ := NewHDWallet(seed, network != NetworkMainnet)
						imported, _ := ImportPSBT(b64)
						if err := SignPSBTFromDerivation(imported, signer, m.getNetwork()); err != nil {
							t.Fatal(err)
						}
						final, err := FinalizePSBT(imported)
						if err != nil {
							t.Fatal(err)
						}
						verifyFinalTx(t, final, prevOuts)
						return
					}
					if err := m.SignPSBTTransaction(res); err != nil {
						t.Fatal(err)
					}
					txHex, err := m.FinalizePSBTTransaction(res)
					if err != nil {
						t.Fatal(err)
					}
					verifyFinalTx(t, decodeTxHex(t, txHex), prevOuts)
				})
			}
		}
	}
}

// TestCreatePSBT_PrevTxChecks covers the previous-transaction rules: a
// legacy input cannot be built without it, a segwit input gets a warning,
// and a substituted transaction is rejected.
func TestCreatePSBT_PrevTxChecks(t *testing.T) {
	t.Run("legacy without prev tx fails", func(t *testing.T) {
		m, b := newSignerTestManager(t, NetworkTestnet, AddressTypeLegacy, 44, false)
		b.rawTxs = map[string]*wire.MsgTx{}
		dest, _, _ := m.GetAddressByIndex(0, 5)
		if _, err := m.CreatePSBTBatchTransaction(0, []TxRecipient{{Address: dest, Amount: 40_000}}, 2, nil); err == nil {
			t.Fatal("expected an error without the previous transaction")
		}
	})
	t.Run("segwit without prev tx warns", func(t *testing.T) {
		m, b := newSignerTestManager(t, NetworkTestnet, AddressTypeSegWitNative, 84, false)
		b.rawTxs = map[string]*wire.MsgTx{}
		dest, _, _ := m.GetAddressByIndex(0, 5)
		res, err := m.CreatePSBTBatchTransaction(0, []TxRecipient{{Address: dest, Amount: 40_000}}, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Warnings) != 1 || res.Packet.Inputs[0].NonWitnessUtxo != nil {
			t.Fatalf("warnings=%v", res.Warnings)
		}
	})
	t.Run("mismatched prev tx is rejected", func(t *testing.T) {
		m, b := newSignerTestManager(t, NetworkTestnet, AddressTypeSegWitNative, 84, false)
		for txid, tx := range b.rawTxs {
			forged := tx.Copy()
			forged.TxOut[0].Value++ // what a lying server would send
			b.rawTxs[txid] = forged
		}
		dest, _, _ := m.GetAddressByIndex(0, 5)
		res, err := m.CreatePSBTBatchTransaction(0, []TxRecipient{{Address: dest, Amount: 40_000}}, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.Packet.Inputs[0].NonWitnessUtxo != nil || len(res.Warnings) != 1 {
			t.Fatal("a previous transaction that does not hash to the txid must not be attached")
		}
	})
}

// TestSignPSBTFromDerivation_NestedSegwit signs an imported P2SH-P2WPKH
// PSBT, which must use the BIP143 P2PKH script code.
func TestSignPSBTFromDerivation_NestedSegwit(t *testing.T) {
	m, b := newSignerTestManager(t, NetworkTestnet, AddressTypeSegWitNested, 49, true)
	dest, _, _ := m.GetAddressByIndex(0, 5)
	res, err := m.CreatePSBTBatchTransaction(0, []TxRecipient{{Address: dest, Amount: 40_000}}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := MnemonicToSeed(testMnemonic, "")
	signer, _ := NewHDWallet(seed, true)
	if err := SignPSBTFromDerivation(res.Packet, signer, m.getNetwork()); err != nil {
		t.Fatal(err)
	}
	final, err := FinalizePSBT(res.Packet)
	if err != nil {
		t.Fatal(err)
	}
	funding := b.rawTxs[final.TxIn[0].PreviousOutPoint.Hash.String()]
	verifyFinalTx(t, final, map[wire.OutPoint]*wire.TxOut{final.TxIn[0].PreviousOutPoint: funding.TxOut[0]})
}
