package wallet

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// inputPrevOut returns the output spent by input i, from the witness UTXO
// or, for non-witness inputs, from the full previous transaction.
//
// Signing paths must use verifiedInputPrevOut instead: this helper is for
// metadata and display where any available amount is acceptable.
func inputPrevOut(packet *psbt.Packet, i int) *wire.TxOut {
	if packet == nil || i < 0 || i >= len(packet.Inputs) {
		return nil
	}
	in := &packet.Inputs[i]
	if in.WitnessUtxo != nil {
		return in.WitnessUtxo
	}
	if in.NonWitnessUtxo == nil || i >= len(packet.UnsignedTx.TxIn) {
		return nil
	}
	idx := packet.UnsignedTx.TxIn[i].PreviousOutPoint.Index
	if int(idx) >= len(in.NonWitnessUtxo.TxOut) {
		return nil
	}
	return in.NonWitnessUtxo.TxOut[idx]
}

// verifiedInputPrevOut returns the previous output for input i after
// checking it against the outpoint. A non-witness (legacy) input must carry
// NonWitnessUtxo: its signature does not commit to the amount, so the
// witness UTXO alone would let a counterparty understate the real input and
// hide the fee (H3-04). When both forms are present they must agree.
func verifiedInputPrevOut(packet *psbt.Packet, i int) (*wire.TxOut, error) {
	if packet == nil || packet.UnsignedTx == nil || i < 0 || i >= len(packet.Inputs) ||
		i >= len(packet.UnsignedTx.TxIn) {
		return nil, fmt.Errorf("input %d is missing", i)
	}
	in := &packet.Inputs[i]
	op := packet.UnsignedTx.TxIn[i].PreviousOutPoint

	var fromPrev *wire.TxOut
	if in.NonWitnessUtxo != nil {
		if in.NonWitnessUtxo.TxHash() != op.Hash {
			return nil, fmt.Errorf("input %d: previous transaction does not match the outpoint", i+1)
		}
		if int(op.Index) >= len(in.NonWitnessUtxo.TxOut) {
			return nil, fmt.Errorf("input %d: outpoint is past the previous transaction's outputs", i+1)
		}
		fromPrev = in.NonWitnessUtxo.TxOut[op.Index]
		if in.WitnessUtxo != nil &&
			(in.WitnessUtxo.Value != fromPrev.Value || !bytes.Equal(in.WitnessUtxo.PkScript, fromPrev.PkScript)) {
			return nil, fmt.Errorf("input %d: witness UTXO and previous transaction disagree", i+1)
		}
	}

	if in.WitnessUtxo == nil && fromPrev == nil {
		return nil, fmt.Errorf("input %d has no UTXO data", i+1)
	}

	// The script determines whether the signature commits to the amount.
	script := in.WitnessUtxo
	if fromPrev != nil {
		script = fromPrev
	}
	witness := txscript.IsWitnessProgram(script.PkScript)
	if !witness && txscript.IsPayToScriptHash(script.PkScript) {
		witness = txscript.IsWitnessProgram(in.RedeemScript) || len(in.WitnessScript) > 0
	}

	if witness {
		if in.WitnessUtxo != nil {
			return in.WitnessUtxo, nil
		}
		return fromPrev, nil
	}
	if fromPrev == nil {
		// A P2SH output may be nested segwit (BIP49), whose redeem script
		// is not always present before signing. Accept the witness UTXO
		// here; legacy signing paths separately require NonWitnessUtxo.
		if txscript.IsPayToScriptHash(script.PkScript) && in.WitnessUtxo != nil {
			return in.WitnessUtxo, nil
		}
		return nil, fmt.Errorf("input %d is a legacy input without its previous transaction", i+1)
	}
	return fromPrev, nil
}

// isWitnessInput reports whether an input spends a segwit output, judged
// from its scriptPubKey and, for P2SH, its redeem script.
func isWitnessInput(in *psbt.PInput, prevOut *wire.TxOut) bool {
	if txscript.IsWitnessProgram(prevOut.PkScript) {
		return true
	}
	if txscript.IsPayToScriptHash(prevOut.PkScript) {
		return len(in.RedeemScript) > 0 &&
			txscript.IsWitnessProgram(in.RedeemScript)
	}
	return false
}

// verifyMultisigScript checks that script is the one committed by prevOut.
// A PSBT can otherwise supply a different script containing the same keys
// (for example a lower threshold) and collect signatures over it (H3-21).
func verifyMultisigScript(script []byte, scriptType MultisigScriptType, prevOut *wire.TxOut) error {
	if len(script) == 0 || prevOut == nil {
		return fmt.Errorf("missing multisig script or previous output")
	}
	switch scriptType {
	case MultisigNativeSegwit:
		sum := sha256.Sum256(script)
		if !bytes.Equal(prevOut.PkScript, append([]byte{0x00, 0x20}, sum[:]...)) {
			return fmt.Errorf("multisig script does not match the spent output")
		}
	case MultisigNestedSegwit:
		sum := sha256.Sum256(script)
		witnessProgram := append([]byte{0x00, 0x20}, sum[:]...)
		if !p2shPkScriptMatches(prevOut.PkScript, btcutil.Hash160(witnessProgram)) {
			return fmt.Errorf("multisig script does not match the spent output")
		}
	case MultisigLegacy:
		if !p2shPkScriptMatches(prevOut.PkScript, btcutil.Hash160(script)) {
			return fmt.Errorf("multisig script does not match the spent output")
		}
	default:
		return fmt.Errorf("unknown multisig script type %d", scriptType)
	}
	return nil
}

func p2shPkScriptMatches(pkScript, scriptHash []byte) bool {
	return len(pkScript) == 23 && pkScript[0] == txscript.OP_HASH160 &&
		pkScript[1] == 0x14 && bytes.Equal(pkScript[2:22], scriptHash)
}

// prevTxCache holds full previous transactions by txid. A txid commits
// to the transaction, so entries never go stale.
type prevTxCache struct {
	mu  sync.Mutex
	txs map[string]*wire.MsgTx
}

func (c *prevTxCache) get(txid string) *wire.MsgTx {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.txs[txid]
}

func (c *prevTxCache) put(txid string, tx *wire.MsgTx) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.txs == nil {
		c.txs = make(map[string]*wire.MsgTx)
	}
	c.txs[txid] = tx
}

// attachPrevTxs adds PSBT_IN_NON_WITNESS_UTXO to every non-Taproot input.
//
// BIP174 requires it for non-witness inputs. Hardware signers also require
// it for segwit v0 inputs, because the witness UTXO alone lets a host lie
// about input amounts across two signing sessions. Taproot signatures
// commit to every input amount, so P2TR inputs keep only the witness UTXO.
//
// The previous transaction is fetched and checked for every input: a
// failure is fatal, so no PSBT is built from unverified values (H3-06).
func (m *WalletManager) attachPrevTxs(result *PSBTResult) error {
	packet := result.Packet
	for i, utxo := range result.SelectedUTXOs {
		if i >= len(packet.Inputs) {
			break
		}
		in := &packet.Inputs[i]
		prevOut := inputPrevOut(packet, i)
		if prevOut == nil {
			return fmt.Errorf("input %d has no UTXO data", i)
		}
		witness := isWitnessInput(in, prevOut)

		tx, err := m.fetchPrevTx(utxo.TxID, utxo.Status.BlockHeight)
		if err == nil {
			err = checkPrevTx(tx, utxo.Vout, prevOut)
		}
		if err != nil {
			return fmt.Errorf("input %s:%d: %w", utxo.TxID, utxo.Vout, err)
		}
		if txscript.IsPayToTaproot(prevOut.PkScript) {
			continue
		}
		in.NonWitnessUtxo = tx
		if !witness {
			in.WitnessUtxo = nil
		}
	}
	return nil
}

// checkPrevTx confirms the previous transaction pays the expected output.
func checkPrevTx(tx *wire.MsgTx, vout uint32, want *wire.TxOut) error {
	if int(vout) >= len(tx.TxOut) {
		return fmt.Errorf("previous transaction has no output %d", vout)
	}
	got := tx.TxOut[vout]
	if got.Value != want.Value || !bytes.Equal(got.PkScript, want.PkScript) {
		return fmt.Errorf("previous transaction output %d does not match the UTXO", vout)
	}
	return nil
}

func (m *WalletManager) fetchPrevTx(txid string, height int64) (*wire.MsgTx, error) {
	if tx := m.prevTxs.get(txid); tx != nil {
		return tx, nil
	}
	if m.backend == nil {
		return nil, fmt.Errorf("no backend connected")
	}
	tx, err := m.backend.GetRawTransaction(txid, height)
	if err != nil {
		return nil, err
	}
	if tx.TxHash().String() != txid {
		return nil, fmt.Errorf("backend returned the wrong transaction for %s", txid)
	}
	m.prevTxs.put(txid, tx)
	return tx, nil
}

func shortTxID(txid string) string {
	if len(txid) <= 16 {
		return txid
	}
	return txid[:8] + "…" + txid[len(txid)-8:]
}

// addSingleSigSignerData adds the key origin data external signers need to
// sign inputs and recognise change, for single-sig wallets:
//
//   - P2PKH, P2WPKH: PSBT_{IN,OUT}_BIP32_DERIVATION
//   - P2SH-P2WPKH: the same plus PSBT_{IN,OUT}_REDEEM_SCRIPT
//   - P2TR (BIP371): PSBT_{IN,OUT}_TAP_BIP32_DERIVATION and
//     PSBT_{IN,OUT}_TAP_INTERNAL_KEY
//
// Every entry is checked against the scriptPubKey it describes, so a key
// that does not match is left out rather than misdescribing the output.
// Pass changeOutputIdx < 0 when there is no change output.
func (m *WalletManager) addSingleSigSignerData(packet *psbt.Packet, utxos []UTXOWithPath, changePath string, changeOutputIdx int) {
	fingerprint, ok := m.masterFingerprint()
	if !ok {
		return
	}
	for i, utxo := range utxos {
		if i >= len(packet.Inputs) {
			break
		}
		prevOut := inputPrevOut(packet, i)
		if prevOut == nil {
			continue
		}
		d, err := m.singleSigKeyData(utxo.Path, prevOut.PkScript, fingerprint)
		if err != nil {
			continue
		}
		in := &packet.Inputs[i]
		if d.tap != nil {
			in.TaprootBip32Derivation = append(in.TaprootBip32Derivation, d.tap)
			in.TaprootInternalKey = d.tap.XOnlyPubKey
			// BIP371 key-path spends use SIGHASH_DEFAULT; an explicit
			// SIGHASH_ALL would change the signature hash.
			in.SighashType = 0
			continue
		}
		in.Bip32Derivation = append(in.Bip32Derivation, d.ecdsa)
		if d.redeemScript != nil {
			in.RedeemScript = d.redeemScript
		}
	}

	if changeOutputIdx < 0 || changeOutputIdx >= len(packet.Outputs) || changePath == "" {
		return
	}
	pkScript := packet.UnsignedTx.TxOut[changeOutputIdx].PkScript
	d, err := m.singleSigKeyData(changePath, pkScript, fingerprint)
	if err != nil {
		return
	}
	out := &packet.Outputs[changeOutputIdx]
	if d.tap != nil {
		out.TaprootBip32Derivation = append(out.TaprootBip32Derivation, d.tap)
		out.TaprootInternalKey = d.tap.XOnlyPubKey
		return
	}
	out.Bip32Derivation = append(out.Bip32Derivation, d.ecdsa)
	if d.redeemScript != nil {
		out.RedeemScript = d.redeemScript
	}
}

type singleSigKey struct {
	ecdsa        *psbt.Bip32Derivation
	tap          *psbt.TaprootBip32Derivation
	redeemScript []byte
}

// singleSigKeyData derives the key at path and describes it for the given
// scriptPubKey, failing if the key does not produce that script.
func (m *WalletManager) singleSigKeyData(path string, pkScript []byte, fingerprint uint32) (*singleSigKey, error) {
	bip44Path, err := ParseBIP44Path(path)
	if err != nil {
		return nil, err
	}
	pubKey, err := m.singleSigPubKey(bip44Path)
	if err != nil {
		return nil, err
	}
	fullPath := []uint32{
		hdkeychain.HardenedKeyStart + bip44Path.Purpose,
		hdkeychain.HardenedKeyStart + bip44Path.CoinType,
		hdkeychain.HardenedKeyStart + bip44Path.Account,
		bip44Path.Change,
		bip44Path.AddressIndex,
	}
	pubKeyHash := btcutil.Hash160(pubKey)

	switch {
	case txscript.IsPayToTaproot(pkScript):
		internal, err := schnorr.ParsePubKey(pubKey[1:])
		if err != nil {
			return nil, err
		}
		want, err := txscript.PayToTaprootScript(txscript.ComputeTaprootKeyNoScript(internal))
		if err != nil || !bytes.Equal(want, pkScript) {
			return nil, fmt.Errorf("key at %s does not match taproot output", path)
		}
		return &singleSigKey{tap: &psbt.TaprootBip32Derivation{
			XOnlyPubKey:          schnorr.SerializePubKey(internal),
			MasterKeyFingerprint: fingerprint,
			Bip32Path:            fullPath,
		}}, nil

	case txscript.IsPayToScriptHash(pkScript):
		redeem, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).AddData(pubKeyHash).Script()
		if err != nil {
			return nil, err
		}
		want, err := txscript.NewScriptBuilder().AddOp(txscript.OP_HASH160).
			AddData(btcutil.Hash160(redeem)).AddOp(txscript.OP_EQUAL).Script()
		if err != nil || !bytes.Equal(want, pkScript) {
			return nil, fmt.Errorf("key at %s does not match P2SH output", path)
		}
		return &singleSigKey{
			ecdsa:        &psbt.Bip32Derivation{PubKey: pubKey, MasterKeyFingerprint: fingerprint, Bip32Path: fullPath},
			redeemScript: redeem,
		}, nil

	case txscript.IsPayToWitnessPubKeyHash(pkScript):
		if !bytes.Equal(pkScript[2:], pubKeyHash) {
			return nil, fmt.Errorf("key at %s does not match P2WPKH output", path)
		}
	case txscript.IsPayToPubKeyHash(pkScript):
		if !bytes.Equal(pkScript[3:23], pubKeyHash) {
			return nil, fmt.Errorf("key at %s does not match P2PKH output", path)
		}
	default:
		return nil, fmt.Errorf("unsupported output script for %s", path)
	}
	return &singleSigKey{ecdsa: &psbt.Bip32Derivation{
		PubKey: pubKey, MasterKeyFingerprint: fingerprint, Bip32Path: fullPath,
	}}, nil
}

// singleSigPubKey returns the compressed public key at path, from the seed
// or, for hardware wallets, from the account xpub.
func (m *WalletManager) singleSigPubKey(path BIP44Path) ([]byte, error) {
	if m.wallet != nil {
		key, err := m.wallet.DeriveKey(path)
		if err != nil {
			return nil, err
		}
		pub, err := key.ECPubKey()
		if err != nil {
			return nil, err
		}
		return pub.SerializeCompressed(), nil
	}
	if m.data != nil && m.data.HardwareWallet != nil {
		return DeriveChildPubKey(m.data.HardwareWallet.Xpub,
			path.Change, path.AddressIndex, m.getNetwork())
	}
	return nil, fmt.Errorf("no keys available")
}

// masterFingerprint returns the wallet's master key fingerprint in PSBT
// (little-endian) form.
func (m *WalletManager) masterFingerprint() (uint32, bool) {
	if m.wallet != nil {
		fp, err := m.wallet.GetMasterFingerprint()
		if err != nil || len(fp) < 4 {
			return 0, false
		}
		return uint32(fp[3])<<24 | uint32(fp[2])<<16 |
			uint32(fp[1])<<8 | uint32(fp[0]), true
	}
	if m.data != nil && m.data.HardwareWallet != nil {
		return fingerprintHexToUint32(m.data.HardwareWallet.Fingerprint), true
	}
	return 0, false
}
