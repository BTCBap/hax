package wallet

import (
	"bytes"
	"fmt"

	"github.com/BTCBap/hax/internal/api"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// Feerates and fee shares above these get a warning before signing.
const (
	inspectHighFeeRate    = 1000.0 // sat/vB
	inspectHighFeePercent = 10     // of the amount leaving the wallet
)

// OutputOwner says who an output pays.
type OutputOwner int

const (
	OutputExternal OutputOwner = iota // someone else
	OutputReceive                     // a receive address of this wallet
	OutputChange                      // a change address of this wallet
)

// InspectedInput is one input of an inspected PSBT.
type InspectedInput struct {
	OutPoint   string
	Value      api.Satoshi
	ValueKnown bool
	Address    string
	Mine       bool
	Signed     bool
}

// InspectedOutput is one output of an inspected PSBT.
type InspectedOutput struct {
	Address string // empty for scripts without an address (OP_RETURN)
	Value   api.Satoshi
	Owner   OutputOwner
}

// PSBTInspection is what a PSBT does, worked out from the PSBT itself and
// the wallet's keys, never from what the PSBT claims about itself.
type PSBTInspection struct {
	Inputs   []InspectedInput
	Outputs  []InspectedOutput
	Fee      api.Satoshi
	FeeKnown bool
	VSize    int
	FeeRate  float64     // sat/vB, when FeeKnown
	Sent     api.Satoshi // total paid to outputs that aren't ours
	RBF      bool        // signals BIP125 replaceability
	LockTime uint32
	Warnings []string
}

// InspectPSBT describes a PSBT for review before signing. Ownership of
// every input and output is checked by deriving our key for the claimed
// path and comparing scripts, so a PSBT can't pass off a payment as
// change.
func (m *WalletManager) InspectPSBT(packet *psbt.Packet) *PSBTInspection {
	insp := &PSBTInspection{}
	if packet == nil || packet.UnsignedTx == nil {
		return insp
	}
	tx := packet.UnsignedTx
	network := m.getNetwork()
	insp.LockTime = tx.LockTime
	insp.VSize = GetPSBTVSize(packet)
	warn := func(format string, a ...interface{}) {
		insp.Warnings = append(insp.Warnings, fmt.Sprintf(format, a...))
	}

	var inTotal int64
	allKnown := true
	for i, txIn := range tx.TxIn {
		if txIn.Sequence <= maxRBFSequence {
			insp.RBF = true
		}
		in := &packet.Inputs[i]
		ii := InspectedInput{
			OutPoint: txIn.PreviousOutPoint.String(),
			Signed: len(in.PartialSigs) > 0 || len(in.TaprootKeySpendSig) > 0 ||
				len(in.FinalScriptSig) > 0 || len(in.FinalScriptWitness) > 0,
		}
		for _, w := range checkInputUTXOs(in, txIn.PreviousOutPoint) {
			warn("Input %d: %s.", i+1, w)
		}
		prevOut := inputPrevOut(packet, i)
		if prevOut == nil {
			allKnown = false
			warn("Input %d has no amount, so the fee can't be checked.", i+1)
		} else {
			ii.Value, ii.ValueKnown = api.Satoshi(prevOut.Value), true
			inTotal += prevOut.Value
			ii.Address = scriptAddress(prevOut.PkScript, network)
			mine, spoofed := m.ownsScript(prevOut.PkScript,
				in.Bip32Derivation, in.TaprootBip32Derivation)
			ii.Mine = mine != OutputExternal
			if spoofed {
				warn("Input %d claims a key of this wallet that doesn't "+
					"match its script.", i+1)
			}
			if !ii.Mine {
				warn("Input %d isn't from this wallet.", i+1)
			}
			if m.IsHardwareWallet() && in.NonWitnessUtxo == nil &&
				!txscript.IsPayToTaproot(prevOut.PkScript) {
				warn("Input %d has no previous transaction; hardware "+
					"wallets may refuse to sign it.", i+1)
			}
		}
		if s := in.SighashType; s != 0 && s != txscript.SigHashAll {
			warn("Input %d uses sighash %s: parts of the transaction can "+
				"change after you sign.", i+1, sighashName(s))
		}
		insp.Inputs = append(insp.Inputs, ii)
	}

	var outTotal int64
	for i, txOut := range tx.TxOut {
		out := &packet.Outputs[i]
		owner, spoofed := m.ownsScript(txOut.PkScript, out.Bip32Derivation,
			out.TaprootBip32Derivation)
		io := InspectedOutput{
			Address: scriptAddress(txOut.PkScript, network),
			Value:   api.Satoshi(txOut.Value),
			Owner:   owner,
		}
		if spoofed {
			warn("Output %d claims to be this wallet's but isn't: it "+
				"pays someone else.", i+1)
		}
		if owner == OutputExternal {
			insp.Sent += io.Value
		}
		if io.Address != "" && io.Value < GetDustLimit(io.Address) {
			warn("Output %d is below the dust limit; nodes won't relay "+
				"the transaction.", i+1)
		}
		outTotal += txOut.Value
		insp.Outputs = append(insp.Outputs, io)
	}

	if allKnown && len(tx.TxIn) > 0 {
		if inTotal < outTotal {
			warn("The outputs are worth more than the inputs.")
		} else {
			insp.FeeKnown = true
			insp.Fee = api.Satoshi(inTotal - outTotal)
			if insp.VSize > 0 {
				insp.FeeRate = float64(insp.Fee) / float64(insp.VSize)
			}
			switch {
			case insp.FeeRate > inspectHighFeeRate:
				warn("The fee rate is very high (%.0f sat/vB).", insp.FeeRate)
			case insp.FeeRate < incrementalRelayFee:
				warn("The fee rate is below 1 sat/vB; most nodes won't " +
					"relay it.")
			}
			if insp.Sent > 0 &&
				insp.Fee*100 > insp.Sent*inspectHighFeePercent {
				warn("The fee is more than %d%% of the amount sent.",
					inspectHighFeePercent)
			}
		}
	}
	return insp
}

// checkInputUTXOs cross-checks an input's previous transaction against
// its outpoint and witness UTXO.
func checkInputUTXOs(in *psbt.PInput, op wire.OutPoint) []string {
	if in.NonWitnessUtxo == nil {
		return nil
	}
	if in.NonWitnessUtxo.TxHash() != op.Hash {
		return []string{"its previous transaction doesn't match the outpoint"}
	}
	if int(op.Index) >= len(in.NonWitnessUtxo.TxOut) {
		return []string{"its outpoint is past the previous transaction's outputs"}
	}
	full := in.NonWitnessUtxo.TxOut[op.Index]
	if w := in.WitnessUtxo; w != nil &&
		(w.Value != full.Value || !bytes.Equal(w.PkScript, full.PkScript)) {
		return []string{"its amount differs between the witness UTXO and " +
			"the previous transaction"}
	}
	return nil
}

// ownsScript reports whether pkScript pays this wallet, judged by
// deriving our key or multisig script for each path the PSBT gives and
// comparing. spoofed is set when a path claims our fingerprint but our
// key at that path doesn't produce the script. Scripts without a path
// are matched against the wallet's known addresses.
func (m *WalletManager) ownsScript(pkScript []byte, derivs []*psbt.Bip32Derivation, tapDerivs []*psbt.TaprootBip32Derivation) (owner OutputOwner, spoofed bool) {
	ours := m.ownFingerprints()
	type claim struct {
		fp   uint32
		path []uint32
	}
	var claims []claim
	for _, d := range derivs {
		claims = append(claims, claim{d.MasterKeyFingerprint, d.Bip32Path})
	}
	for _, d := range tapDerivs {
		claims = append(claims, claim{d.MasterKeyFingerprint, d.Bip32Path})
	}
	claimed := false
	for _, c := range claims {
		if !ours[c.fp] || len(c.path) < 2 {
			continue
		}
		claimed = true
		change, index := c.path[len(c.path)-2], c.path[len(c.path)-1]
		if change > 1 || index >= 1<<31 {
			continue
		}
		if m.scriptAt(c.path, pkScript) {
			if change == 1 {
				return OutputChange, false
			}
			return OutputReceive, false
		}
	}
	if addr := m.ownAddressForScript(pkScript, m.getNetwork()); addr != nil {
		if addr.Change {
			return OutputChange, false
		}
		return OutputReceive, false
	}
	return OutputExternal, claimed
}

// scriptAt reports whether our key (or multisig script) at path pays to
// pkScript.
func (m *WalletManager) scriptAt(path []uint32, pkScript []byte) bool {
	change, index := path[len(path)-2], path[len(path)-1]
	if m.data != nil && m.data.Multisig != nil {
		addr, _, err := m.GetMultisigAddress(change == 1, index)
		if err != nil {
			return false
		}
		return bytes.Equal(addressScript(addr, m.getNetwork()), pkScript)
	}
	if len(path) != 5 {
		return false
	}
	p := BIP44Path{
		Purpose:      path[0] &^ hardenedBit,
		CoinType:     path[1] &^ hardenedBit,
		Account:      path[2] &^ hardenedBit,
		Change:       change,
		AddressIndex: index,
	}
	_, err := m.singleSigKeyData(p.String(), pkScript, 0)
	return err == nil
}

const hardenedBit = 1 << 31

// ownFingerprints are the master fingerprints (PSBT byte order) of the
// keys this wallet holds or coordinates.
func (m *WalletManager) ownFingerprints() map[uint32]bool {
	fps := map[uint32]bool{}
	if fp, ok := m.masterFingerprint(); ok {
		fps[fp] = true
	}
	if m.data != nil && m.data.Multisig != nil {
		for _, cs := range m.data.Multisig.Cosigners {
			if cs.Fingerprint != "" {
				fps[fingerprintHexToUint32(cs.Fingerprint)] = true
			}
		}
	}
	return fps
}

func scriptAddress(pkScript []byte, network *chaincfg.Params) string {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, network)
	if err != nil || len(addrs) != 1 {
		return ""
	}
	return addrs[0].EncodeAddress()
}

func addressScript(addr string, network *chaincfg.Params) []byte {
	a, err := btcutil.DecodeAddress(addr, network)
	if err != nil {
		return nil
	}
	pk, err := txscript.PayToAddrScript(a)
	if err != nil {
		return nil
	}
	return pk
}

func sighashName(s txscript.SigHashType) string {
	names := map[txscript.SigHashType]string{
		txscript.SigHashNone:                                  "NONE",
		txscript.SigHashSingle:                                "SINGLE",
		txscript.SigHashAll | txscript.SigHashAnyOneCanPay:    "ALL|ANYONECANPAY",
		txscript.SigHashNone | txscript.SigHashAnyOneCanPay:   "NONE|ANYONECANPAY",
		txscript.SigHashSingle | txscript.SigHashAnyOneCanPay: "SINGLE|ANYONECANPAY",
	}
	if n, ok := names[s]; ok {
		return n
	}
	return fmt.Sprintf("0x%02x", uint32(s))
}
