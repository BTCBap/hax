package wallet

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/BTCBap/hax/internal/api"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// incrementalRelayFee is Bitcoin Core's default -incrementalrelayfee in
// sat/vB. A replacement must add at least this much per vbyte of its own
// size on top of the fee it replaces (BIP125 rule 4).
const incrementalRelayFee = 1.0

// maxRBFSequence is the highest nSequence that signals opt-in RBF.
const maxRBFSequence = wire.MaxTxInSequenceNum - 2

// ErrBumpNeedsFunds means the wallet has no spare confirmed coins to pay
// a higher fee without reducing what the recipients get.
var ErrBumpNeedsFunds = errors.New("not enough confirmed coins to pay the higher fee")

// mempoolFeeSource is implemented by backends that report a mempool
// transaction's fee directly (Bitcoin Core's getmempoolentry). Without
// -txindex, Core cannot look up the inputs of other people's
// transactions, so this is the only way to price an incoming one.
type mempoolFeeSource interface {
	MempoolEntryFee(txid string) (api.Satoshi, error)
}

// unspentChecker is implemented by backends that can say whether an
// output is unspent counting the mempool. Bitcoin Core's scantxoutset
// only sees confirmed coins, so the UTXO list alone misses unconfirmed
// outputs there.
type unspentChecker interface {
	IsUnspent(txid string, vout uint32) (bool, error)
}

// FeeBumpInfo describes how an unconfirmed transaction can be sped up.
type FeeBumpInfo struct {
	TxID    string
	Fee     api.Satoshi
	VSize   int
	FeeRate float64 // sat/vB

	// RBF: replace the transaction with one paying more. Only possible
	// when every input is ours and the transaction signals BIP125.
	CanRBF    bool
	RBFReason string // why not, when !CanRBF
	// MinRBFRate is the lowest feerate a replacement may use.
	MinRBFRate float64
	// Recipients are the outputs a replacement keeps unchanged:
	// everything except our change.
	Recipients []TxRecipient

	// CPFP: spend our unconfirmed outputs of the transaction with a
	// fee high enough to pull both into a block.
	CanCPFP    bool
	CPFPReason string // why not, when !CPFP
	// CPFPValue is the total of our unspent outputs of the transaction.
	CPFPValue api.Satoshi
}

// bumpAnalysis is FeeBumpInfo plus what the builders need.
type bumpAnalysis struct {
	FeeBumpInfo
	tx         *wire.MsgTx
	inputs     []UTXOWithPath // our inputs, in order (RBF only)
	change     *AddressData   // our change output, if any
	ownOutputs []int          // indices of outputs paying us
	childUTXOs []UTXOWithPath // our unspent outputs of tx
	walletUTXO []UTXOWithPath
}

// AnalyzeFeeBump reports whether an unconfirmed transaction can be sped
// up by RBF, CPFP or both.
func (m *WalletManager) AnalyzeFeeBump(txid string) (*FeeBumpInfo, error) {
	a, err := m.analyzeFeeBump(txid)
	if err != nil {
		return nil, err
	}
	return &a.FeeBumpInfo, nil
}

func (m *WalletManager) analyzeFeeBump(txid string) (*bumpAnalysis, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}
	if m.backend == nil {
		return nil, fmt.Errorf("no backend connected")
	}
	if len(m.data.Accounts) == 0 {
		return nil, fmt.Errorf("account not found")
	}
	if status, err := m.backend.GetTransaction(txid); err == nil &&
		status != nil && status.Status.Confirmed {
		return nil, fmt.Errorf("transaction %s is already confirmed",
			shortTxID(txid))
	}
	tx, err := m.fetchPrevTx(txid, 0)
	if err != nil {
		return nil, fmt.Errorf("transaction %s not found: %w",
			shortTxID(txid), err)
	}

	a := &bumpAnalysis{tx: tx}
	a.TxID = txid
	a.VSize = txVSize(tx)

	utxos, err := m.GetUTXOsWithPath(0)
	if err != nil {
		return nil, err
	}
	a.walletUTXO = utxos

	fee, feeErr := m.resolveBumpInputs(a)
	if feeErr != nil {
		if src, ok := m.backend.(mempoolFeeSource); ok {
			fee, feeErr = src.MempoolEntryFee(txid)
		}
	}
	if feeErr != nil {
		return nil, fmt.Errorf("can't work out the fee of %s: %w",
			shortTxID(txid), feeErr)
	}
	a.Fee = fee
	a.FeeRate = float64(fee) / float64(a.VSize)
	a.MinRBFRate = a.FeeRate + incrementalRelayFee

	m.classifyBumpOutputs(a)
	m.checkRBF(a)
	m.checkCPFP(a)
	return a, nil
}

// resolveBumpInputs looks up every input of a.tx, records the ones that
// are ours and returns the fee. The fee is only known when every
// previous output could be fetched.
func (m *WalletManager) resolveBumpInputs(a *bumpAnalysis) (api.Satoshi, error) {
	network := m.getNetwork()
	heights := m.knownTxHeights()
	var in int64
	var lookupErr error
	allOurs := true
	for _, txIn := range a.tx.TxIn {
		op := txIn.PreviousOutPoint
		prevID := op.Hash.String()
		prev, err := m.fetchPrevTx(prevID, heights[prevID])
		if err != nil || int(op.Index) >= len(prev.TxOut) {
			if lookupErr == nil {
				lookupErr = fmt.Errorf("input %s:%d not found",
					shortTxID(prevID), op.Index)
			}
			allOurs = false
			continue
		}
		out := prev.TxOut[op.Index]
		in += out.Value
		addr := m.ownAddressForScript(out.PkScript, network)
		if addr == nil {
			allOurs = false
			continue
		}
		height := heights[prevID]
		a.inputs = append(a.inputs, UTXOWithPath{
			UTXO: api.UTXO{
				TxID:  prevID,
				Vout:  op.Index,
				Value: api.Satoshi(out.Value),
				Status: api.Status{
					Confirmed:   height > 0,
					BlockHeight: height,
				},
			},
			Address: addr.Address,
			Path:    addr.Path,
		})
	}
	if !allOurs {
		a.inputs = nil
	}
	if lookupErr != nil {
		return 0, lookupErr
	}
	var out int64
	for _, o := range a.tx.TxOut {
		out += o.Value
	}
	if in < out {
		return 0, fmt.Errorf("inputs are worth less than outputs")
	}
	return api.Satoshi(in - out), nil
}

// classifyBumpOutputs finds our outputs, our change and the recipients.
func (m *WalletManager) classifyBumpOutputs(a *bumpAnalysis) {
	network := m.getNetwork()
	for i, out := range a.tx.TxOut {
		addr := m.ownAddressForScript(out.PkScript, network)
		if addr != nil {
			a.ownOutputs = append(a.ownOutputs, i)
		}
		if addr != nil && addr.Change && a.change == nil {
			a.change = addr
			continue
		}
		recipient := TxRecipient{Amount: api.Satoshi(out.Value)}
		if addr != nil {
			recipient.Address = addr.Address
		} else if _, addrs, _, err := txscript.ExtractPkScriptAddrs(
			out.PkScript, network); err == nil && len(addrs) == 1 {
			recipient.Address = addrs[0].EncodeAddress()
		}
		a.Recipients = append(a.Recipients, recipient)
	}
}

func (m *WalletManager) checkRBF(a *bumpAnalysis) {
	signals := false
	for _, txIn := range a.tx.TxIn {
		if txIn.Sequence <= maxRBFSequence {
			signals = true
		}
	}
	for _, r := range a.Recipients {
		if r.Address == "" {
			a.RBFReason = "it has an output hax can't rebuild"
			return
		}
	}
	switch {
	case len(a.inputs) != len(a.tx.TxIn):
		a.RBFReason = "not every input is from this wallet"
	case !signals:
		a.RBFReason = "it doesn't signal replaceability (BIP125)"
	default:
		a.CanRBF = true
	}
}

func (m *WalletManager) checkCPFP(a *bumpAnalysis) {
	listed := make(map[uint32]UTXOWithPath)
	for _, u := range a.walletUTXO {
		if u.TxID == a.TxID {
			listed[u.Vout] = u
		}
	}
	checker, canCheck := m.backend.(unspentChecker)
	network := m.getNetwork()
	for _, i := range a.ownOutputs {
		vout := uint32(i) // #nosec G115 -- output index of a decoded tx
		u, ok := listed[vout]
		if !ok && canCheck {
			if unspent, err := checker.IsUnspent(a.TxID, vout); err == nil &&
				unspent {
				addr := m.ownAddressForScript(a.tx.TxOut[i].PkScript, network)
				u = UTXOWithPath{
					UTXO: api.UTXO{
						TxID:  a.TxID,
						Vout:  vout,
						Value: api.Satoshi(a.tx.TxOut[i].Value),
					},
					Address: addr.Address,
					Path:    addr.Path,
				}
				ok = true
			}
		}
		if ok {
			a.childUTXOs = append(a.childUTXOs, u)
			a.CPFPValue += u.Value
		}
	}
	if len(a.childUTXOs) == 0 {
		a.CPFPReason = "it has no unspent output paying this wallet"
		return
	}
	a.CanCPFP = true
}

// BuildRBF builds a replacement for txid that pays feeRate. It spends
// the same inputs, pays the recipients the same amounts and takes the
// extra fee from our change. When the change can't cover it, confirmed
// coins are added (BIP125 rule 2 forbids new unconfirmed inputs).
func (m *WalletManager) BuildRBF(txid string, feeRate float64) (*PSBTResult, error) {
	a, err := m.analyzeFeeBump(txid)
	if err != nil {
		return nil, err
	}
	if !a.CanRBF {
		return nil, fmt.Errorf("can't replace this transaction: %s",
			a.RBFReason)
	}
	if feeRate < a.MinRBFRate {
		return nil, fmt.Errorf("a replacement must pay at least %.1f sat/vB",
			a.MinRBFRate)
	}

	changeAddr, changePath := "", ""
	var changeIdx uint32
	if a.change != nil {
		changeAddr, changePath = a.change.Address, a.change.Path
		changeIdx = a.change.Index
	} else {
		changeAddr, changePath, changeIdx, err =
			m.GetCurrentChangeAddress(0)
		if err != nil {
			return nil, fmt.Errorf("failed to get change address: %w", err)
		}
	}

	inputs := append([]UTXOWithPath(nil), a.inputs...)
	spare := spareConfirmedUTXOs(a.walletUTXO, a)
	var recipientTotal api.Satoshi
	for _, r := range a.Recipients {
		recipientTotal += r.Amount
	}

	scriptType, ms, mn := m.GetScriptType()
	dust := GetDustLimit(changeAddr)
	var fee, change api.Satoshi
	var vsize int
	for {
		var total api.Satoshi
		for _, u := range inputs {
			total += u.Value
		}
		n := len(a.Recipients)
		vsize = EstimateVSize(len(inputs), n+1, scriptType, scriptType,
			ms, mn)
		fee = replacementFee(a.Fee, feeRate, vsize)
		if total >= recipientTotal+fee+dust {
			change = total - recipientTotal - fee
			break
		}
		vsize = EstimateVSize(len(inputs), n, scriptType, scriptType, ms, mn)
		if n > 0 && total >= recipientTotal+replacementFee(a.Fee, feeRate,
			vsize) {
			// What would have been dust change goes to the fee.
			fee = total - recipientTotal
			change = 0
			break
		}
		if len(spare) == 0 {
			return nil, ErrBumpNeedsFunds
		}
		inputs = append(inputs, spare[0])
		spare = spare[1:]
	}

	outputs := make([]TxOutput, 0, len(a.Recipients)+1)
	for _, r := range a.Recipients {
		outputs = append(outputs, TxOutput(r))
	}
	changeOutputIdx := -1
	if change > 0 {
		changeOutputIdx = len(outputs)
		outputs = append(outputs, TxOutput{Address: changeAddr, Amount: change})
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("nothing left to pay after the fee")
	}

	witnessScripts, err := m.multisigWitnessScripts(inputs)
	if err != nil {
		return nil, err
	}
	packet, err := assemblePSBT(inputs, outputs, m.getNetwork(),
		witnessScripts, scriptType)
	if err != nil {
		return nil, err
	}
	result := &PSBTResult{
		Packet:        packet,
		Fee:           fee,
		Change:        change,
		HasChange:     change > 0,
		SelectedUTXOs: inputs,
		VSize:         vsize,
		IntendedFee:   fee,
		Replaces:      txid,
	}
	if change > 0 {
		result.ChangeAddress = changeAddr
	}
	if err := m.addSignerData(result, changePath, changeIdx,
		changeOutputIdx); err != nil {
		return nil, err
	}
	if m.hasSpentOutputs(a) {
		result.Warnings = append(result.Warnings,
			"A later unconfirmed transaction spends this one's outputs. "+
				"Replacing it drops that transaction too.")
	}
	return result, nil
}

// replacementFee is the fee a replacement of vsize vbytes pays at
// feeRate, raised if needed to cover the replaced fee plus the
// incremental relay fee for its own size (BIP125 rules 3 and 4).
func replacementFee(oldFee api.Satoshi, feeRate float64, vsize int) api.Satoshi {
	fee := api.Satoshi(math.Ceil(feeRate * float64(vsize)))
	minFee := oldFee + api.Satoshi(math.Ceil(incrementalRelayFee*
		float64(vsize)))
	if fee < minFee {
		return minFee
	}
	return fee
}

// spareConfirmedUTXOs returns confirmed wallet coins not already spent
// by or created by the transaction, largest first.
func spareConfirmedUTXOs(utxos []UTXOWithPath, a *bumpAnalysis) []UTXOWithPath {
	used := make(map[string]bool, len(a.inputs))
	for _, u := range a.inputs {
		used[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] = true
	}
	var spare []UTXOWithPath
	for _, u := range utxos {
		if !u.Status.Confirmed || u.TxID == a.TxID ||
			used[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] {
			continue
		}
		spare = append(spare, u)
	}
	sort.SliceStable(spare, func(i, j int) bool {
		return spare[i].Value > spare[j].Value
	})
	return spare
}

// hasSpentOutputs reports whether one of our outputs of the transaction
// is already spent by a descendant.
func (m *WalletManager) hasSpentOutputs(a *bumpAnalysis) bool {
	return len(a.childUTXOs) < len(a.ownOutputs)
}

// BuildCPFP builds a child that spends our unconfirmed outputs of txid
// back to the wallet, with a fee that brings the parent and child
// together to feeRate. The parent's own unconfirmed ancestors are not
// counted.
func (m *WalletManager) BuildCPFP(txid string, feeRate float64) (*PSBTResult, error) {
	a, err := m.analyzeFeeBump(txid)
	if err != nil {
		return nil, err
	}
	if !a.CanCPFP {
		return nil, fmt.Errorf("can't speed this up with a child: %s",
			a.CPFPReason)
	}
	if feeRate <= a.FeeRate {
		return nil, fmt.Errorf("the transaction already pays %.1f sat/vB",
			a.FeeRate)
	}

	addr, path, idx, err := m.GetCurrentChangeAddress(0)
	if err != nil {
		return nil, fmt.Errorf("failed to get change address: %w", err)
	}
	scriptType, ms, mn := m.GetScriptType()
	vsize := EstimateVSize(len(a.childUTXOs), 1, scriptType, scriptType,
		ms, mn)
	fee := api.Satoshi(math.Ceil(feeRate*float64(a.VSize+vsize))) - a.Fee
	if minFee := api.Satoshi(math.Ceil(incrementalRelayFee *
		float64(vsize))); fee < minFee {
		fee = minFee
	}
	if a.CPFPValue < fee+GetDustLimit(addr) {
		return nil, fmt.Errorf("our outputs (%d sats) can't pay a %d sat "+
			"child fee", a.CPFPValue, fee)
	}
	amount := a.CPFPValue - fee

	inputs := append([]UTXOWithPath(nil), a.childUTXOs...)
	witnessScripts, err := m.multisigWitnessScripts(inputs)
	if err != nil {
		return nil, err
	}
	packet, err := assemblePSBT(inputs,
		[]TxOutput{{Address: addr, Amount: amount}}, m.getNetwork(),
		witnessScripts, scriptType)
	if err != nil {
		return nil, err
	}
	result := &PSBTResult{
		Packet:        packet,
		Fee:           fee,
		Change:        amount,
		HasChange:     true,
		SelectedUTXOs: inputs,
		ChangeAddress: addr,
		VSize:         vsize,
		IntendedFee:   fee,
		ChildOf:       txid,
	}
	if err := m.addSignerData(result, path, idx, 0); err != nil {
		return nil, err
	}
	return result, nil
}

// ownAddressForScript returns our address record for a scriptPubKey.
func (m *WalletManager) ownAddressForScript(pkScript []byte, network *chaincfg.Params) *AddressData {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, network)
	if err != nil || len(addrs) != 1 {
		return nil
	}
	return m.ownAddress(addrs[0].EncodeAddress())
}

// ownAddress returns a copy of our address record for addr, or nil.
func (m *WalletManager) ownAddress(addr string) *AddressData {
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	if m.data == nil || len(m.data.Accounts) == 0 {
		return nil
	}
	for _, a := range m.data.Accounts[0].Addresses {
		if a.Address == addr {
			return &a
		}
	}
	return nil
}

// knownTxHeights maps wallet transaction ids to their block heights, so
// backends without a transaction index can still find them.
func (m *WalletManager) knownTxHeights() map[string]int64 {
	state := m.GetState()
	heights := make(map[string]int64, len(state.Transactions))
	for _, tx := range state.Transactions {
		if tx.Status.Confirmed {
			heights[tx.TxID] = tx.Status.BlockHeight
		}
	}
	return heights
}

// txVSize is the BIP141 virtual size: weight / 4, rounded up.
func txVSize(tx *wire.MsgTx) int {
	weight := tx.SerializeSizeStripped()*3 + tx.SerializeSize()
	return (weight + 3) / 4
}
