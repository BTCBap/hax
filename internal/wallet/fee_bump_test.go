package wallet

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// chainBackend is a toy node: a UTXO set with a mempool. Broadcasts
// spend their inputs and evict conflicting mempool transactions, like a
// replacement would.
type chainBackend struct {
	mockBroadcastBackend
	params  *chaincfg.Params
	utxos   map[wire.OutPoint]chainCoin
	spentBy map[wire.OutPoint]string // mempool spends
	mempool map[string]bool
	fees    map[string]api.Satoshi // served by MempoolEntryFee when set
	nextIn  uint32
}

type chainCoin struct {
	out    *wire.TxOut
	height int64
}

func newChainBackend(params *chaincfg.Params) *chainBackend {
	return &chainBackend{
		mockBroadcastBackend: mockBroadcastBackend{rawTxs: map[string]*wire.MsgTx{}},
		params:               params,
		utxos:                map[wire.OutPoint]chainCoin{},
		spentBy:              map[wire.OutPoint]string{},
		mempool:              map[string]bool{},
	}
}

func (b *chainBackend) script(t *testing.T, addr string) []byte {
	t.Helper()
	a, err := btcutil.DecodeAddress(addr, b.params)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := txscript.PayToAddrScript(a)
	if err != nil {
		t.Fatal(err)
	}
	return pk
}

// fund confirms a transaction paying value to addr.
func (b *chainBackend) fund(t *testing.T, addr string, value int64) string {
	t.Helper()
	tx := wire.NewMsgTx(2)
	prev := wire.OutPoint{Index: b.nextIn}
	b.nextIn++
	tx.AddTxIn(wire.NewTxIn(&prev, nil, nil))
	tx.AddTxOut(wire.NewTxOut(value, b.script(t, addr)))
	id := tx.TxHash().String()
	b.rawTxs[id] = tx
	b.utxos[wire.OutPoint{Hash: tx.TxHash()}] = chainCoin{tx.TxOut[0], 100}
	return id
}

// addMempool puts tx in the mempool without checking it.
func (b *chainBackend) addMempool(tx *wire.MsgTx) string {
	id := tx.TxHash().String()
	for _, in := range tx.TxIn {
		if other, ok := b.spentBy[in.PreviousOutPoint]; ok && other != id {
			b.evict(other)
		}
		b.spentBy[in.PreviousOutPoint] = id
		delete(b.utxos, in.PreviousOutPoint)
	}
	for i, out := range tx.TxOut {
		op := wire.OutPoint{Hash: tx.TxHash(), Index: uint32(i)} // #nosec G115 -- test
		b.utxos[op] = chainCoin{out, 0}
	}
	b.rawTxs[id] = tx
	b.mempool[id] = true
	return id
}

func (b *chainBackend) evict(id string) {
	tx := b.rawTxs[id]
	delete(b.mempool, id)
	for i := range tx.TxOut {
		op := wire.OutPoint{Hash: tx.TxHash(), Index: uint32(i)} // #nosec G115 -- test
		if child, ok := b.spentBy[op]; ok {
			b.evict(child)
		}
		delete(b.utxos, op)
	}
	for _, in := range tx.TxIn {
		delete(b.spentBy, in.PreviousOutPoint)
		if prev, ok := b.rawTxs[in.PreviousOutPoint.Hash.String()]; ok {
			b.utxos[in.PreviousOutPoint] = chainCoin{
				prev.TxOut[in.PreviousOutPoint.Index], 100}
		}
	}
}

func (b *chainBackend) BroadcastTransaction(rawHex string) (string, error) {
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return "", err
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return "", err
	}
	return b.addMempool(&tx), nil
}

func (b *chainBackend) GetAddressUTXOs(address string) ([]api.UTXO, error) {
	var out []api.UTXO
	for op, c := range b.utxos {
		_, addrs, _, err := txscript.ExtractPkScriptAddrs(c.out.PkScript, b.params)
		if err != nil || len(addrs) != 1 || addrs[0].EncodeAddress() != address {
			continue
		}
		out = append(out, api.UTXO{
			TxID: op.Hash.String(), Vout: op.Index, Value: api.Satoshi(c.out.Value),
			Status: api.Status{Confirmed: c.height > 0, BlockHeight: c.height},
		})
	}
	return out, nil
}

func (b *chainBackend) GetTransaction(txid string) (*api.Transaction, error) {
	if _, ok := b.rawTxs[txid]; !ok {
		return nil, fmt.Errorf("not found")
	}
	return &api.Transaction{TxID: txid, Status: api.Status{
		Confirmed: !b.mempool[txid], BlockHeight: 100}}, nil
}

// chainFeeBackend also answers MempoolEntryFee, like Bitcoin Core.
type chainFeeBackend struct{ *chainBackend }

func (b chainFeeBackend) MempoolEntryFee(txid string) (api.Satoshi, error) {
	if fee, ok := b.fees[txid]; ok {
		return fee, nil
	}
	return 0, fmt.Errorf("not in mempool")
}

func externalAddress(t *testing.T, params *chaincfg.Params) string {
	t.Helper()
	k, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	h := btcutil.Hash160(k.PubKey().SerializeCompressed())
	a, err := btcutil.NewAddressWitnessPubKeyHash(h, params)
	if err != nil {
		t.Fatal(err)
	}
	return a.EncodeAddress()
}

func newBumpTestManager(t *testing.T, addrType AddressType, b backend.BitcoinBackend) *WalletManager {
	t.Helper()
	m, err := NewWalletManager(t.TempDir(), b)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery"
	if err := m.RestoreWalletOnNetwork("w", pw, testMnemonic, "", NetworkTestnet); err != nil {
		t.Fatal(err)
	}
	if err := m.UnlockWallet("w", pw); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateAccount("Main Account", addrType, ""); err != nil {
		t.Fatal(err)
	}
	m.StopBackgroundSync()
	t.Cleanup(m.LockWallet)
	return m
}

// signAndSend signs a PSBT with the seed, checks every input script, and
// broadcasts it. It returns the transaction.
func signAndSend(t *testing.T, m *WalletManager, b *chainBackend, res *PSBTResult) *wire.MsgTx {
	t.Helper()
	if err := m.SignPSBTTransaction(res); err != nil {
		t.Fatal(err)
	}
	txHex, err := m.FinalizePSBTTransaction(res)
	if err != nil {
		t.Fatal(err)
	}
	tx := decodeTxHex(t, txHex)
	prevOuts := map[wire.OutPoint]*wire.TxOut{}
	for _, in := range tx.TxIn {
		prev := b.rawTxs[in.PreviousOutPoint.Hash.String()]
		prevOuts[in.PreviousOutPoint] = prev.TxOut[in.PreviousOutPoint.Index]
	}
	verifyFinalTx(t, tx, prevOuts)
	if _, err := b.BroadcastTransaction(txHex); err != nil {
		t.Fatal(err)
	}
	return tx
}

func txFee(b *chainBackend, tx *wire.MsgTx) int64 {
	var fee int64
	for _, in := range tx.TxIn {
		fee += b.rawTxs[in.PreviousOutPoint.Hash.String()].
			TxOut[in.PreviousOutPoint.Index].Value
	}
	for _, out := range tx.TxOut {
		fee -= out.Value
	}
	return fee
}

// sendTestPayment funds the wallet with the given coins and sends
// amount to an external address at 2 sat/vB.
func sendTestPayment(t *testing.T, m *WalletManager, b *chainBackend, coins []int64, amount api.Satoshi) (*wire.MsgTx, string) {
	t.Helper()
	for _, v := range coins {
		addr, err := m.GetNewAddress(0)
		if err != nil {
			t.Fatal(err)
		}
		b.fund(t, addr, v)
	}
	dest := externalAddress(t, b.params)
	res, err := m.CreatePSBTBatchTransaction(0,
		[]TxRecipient{{Address: dest, Amount: amount}}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	return signAndSend(t, m, b, res), dest
}

func outputTo(t *testing.T, b *chainBackend, tx *wire.MsgTx, addr string) int64 {
	t.Helper()
	pk := b.script(t, addr)
	for _, out := range tx.TxOut {
		if bytes.Equal(out.PkScript, pk) {
			return out.Value
		}
	}
	return -1
}

// TestBuildRBF replaces a payment at a higher feerate for every address
// type: same inputs, same payment, the extra fee comes out of change and
// the BIP125 fee rules hold for the signed replacement.
func TestBuildRBF(t *testing.T) {
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			b := newChainBackend(&TestNet4Params)
			m := newBumpTestManager(t, tt.addr, b)
			orig, dest := sendTestPayment(t, m, b, []int64{100_000}, 40_000)
			origID := orig.TxHash().String()

			info, err := m.AnalyzeFeeBump(origID)
			if err != nil {
				t.Fatal(err)
			}
			if !info.CanRBF || !info.CanCPFP {
				t.Fatalf("want RBF and CPFP, got %+v", info)
			}
			if info.Fee != api.Satoshi(txFee(b, orig)) {
				t.Fatalf("fee = %d, want %d", info.Fee, txFee(b, orig))
			}
			if len(info.Recipients) != 1 || info.Recipients[0].Address != dest {
				t.Fatalf("recipients = %+v", info.Recipients)
			}

			if _, err := m.BuildRBF(origID, info.MinRBFRate-0.5); err == nil {
				t.Fatal("accepted a feerate below the BIP125 minimum")
			}
			res, err := m.BuildRBF(origID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if res.Replaces != origID {
				t.Fatalf("Replaces = %q", res.Replaces)
			}
			repl := signAndSend(t, m, b, res)

			if len(repl.TxIn) != len(orig.TxIn) {
				t.Fatalf("replacement has %d inputs, want %d", len(repl.TxIn), len(orig.TxIn))
			}
			for i := range orig.TxIn {
				if repl.TxIn[i].PreviousOutPoint != orig.TxIn[i].PreviousOutPoint {
					t.Fatalf("input %d changed", i)
				}
				if repl.TxIn[i].Sequence > maxRBFSequence {
					t.Fatalf("input %d doesn't signal RBF", i)
				}
			}
			if got := outputTo(t, b, repl, dest); got != 40_000 {
				t.Fatalf("payment = %d, want 40000", got)
			}
			oldFee, newFee := txFee(b, orig), txFee(b, repl)
			vsize := int64(txVSize(repl))
			if newFee < oldFee+vsize {
				t.Fatalf("fee %d doesn't pay for its own relay over %d (vsize %d)",
					newFee, oldFee, vsize)
			}
			if rate := float64(newFee) / float64(vsize); rate < 10 {
				t.Fatalf("feerate %.2f < 10", rate)
			}
			if int64(res.Fee) != newFee {
				t.Fatalf("result fee %d, actual %d", res.Fee, newFee)
			}
			if b.mempool[origID] {
				t.Fatal("original still in the mempool")
			}
		})
	}
}

// TestBuildRBF_AddsConfirmedInput: a payment without change can only be
// bumped by adding a confirmed coin.
func TestBuildRBF_AddsConfirmedInput(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, b)
	addr, _ := m.GetNewAddress(0)
	b.fund(t, addr, 50_000)
	dest := externalAddress(t, b.params)
	// 300 sats over the payment: the change would be dust, so none.
	res, err := m.CreatePSBTBatchTransaction(0,
		[]TxRecipient{{Address: dest, Amount: 49_700}}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasChange {
		t.Fatal("setup: expected no change")
	}
	orig := signAndSend(t, m, b, res)
	origID := orig.TxHash().String()

	if _, err := m.BuildRBF(origID, 20); !errors.Is(err, ErrBumpNeedsFunds) {
		t.Fatalf("err = %v, want ErrBumpNeedsFunds", err)
	}

	spare, _ := m.GetNewAddress(0)
	b.fund(t, spare, 30_000)
	res, err = m.BuildRBF(origID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Packet.UnsignedTx.TxIn) != 2 || !res.HasChange {
		t.Fatalf("want 2 inputs and change, got %d inputs, change=%v",
			len(res.Packet.UnsignedTx.TxIn), res.HasChange)
	}
	repl := signAndSend(t, m, b, res)
	if got := outputTo(t, b, repl, dest); got != 49_700 {
		t.Fatalf("payment = %d, want 49700", got)
	}
	if txFee(b, repl) < txFee(b, orig)+int64(txVSize(repl)) {
		t.Fatal("BIP125 rule 4 violated")
	}
}

// TestBuildCPFP spends the change of a stuck payment so the package pays
// the target feerate.
func TestBuildCPFP(t *testing.T) {
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			b := newChainBackend(&TestNet4Params)
			m := newBumpTestManager(t, tt.addr, b)
			parent, _ := sendTestPayment(t, m, b, []int64{100_000}, 40_000)
			parentID := parent.TxHash().String()

			if _, err := m.BuildCPFP(parentID, 1); err == nil {
				t.Fatal("accepted a target below the parent's feerate")
			}
			res, err := m.BuildCPFP(parentID, 25)
			if err != nil {
				t.Fatal(err)
			}
			if res.ChildOf != parentID || !res.HasChange {
				t.Fatalf("ChildOf=%q HasChange=%v", res.ChildOf, res.HasChange)
			}
			child := signAndSend(t, m, b, res)
			if len(child.TxOut) != 1 ||
				child.TxIn[0].PreviousOutPoint.Hash != parent.TxHash() {
				t.Fatal("child doesn't spend the parent to a single output")
			}
			fee := txFee(b, parent) + txFee(b, child)
			vsize := int64(txVSize(parent) + txVSize(child))
			if rate := float64(fee) / float64(vsize); rate < 25 {
				t.Fatalf("package feerate %.2f < 25", rate)
			}
		})
	}
}

// TestBuildCPFP_Incoming speeds up a payment from someone else. Its
// inputs can't be looked up, so the fee comes from the node's mempool.
func TestBuildCPFP_Incoming(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, chainFeeBackend{b})
	addr, _ := m.GetNewAddress(0)

	in := wire.NewMsgTx(2)
	in.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: 999}, nil, nil))
	in.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: 998}, nil, nil))
	in.TxIn[0].Sequence = wire.MaxTxInSequenceNum
	in.TxIn[1].Sequence = wire.MaxTxInSequenceNum
	in.AddTxOut(wire.NewTxOut(80_000, b.script(t, addr)))
	in.AddTxOut(wire.NewTxOut(5_000, b.script(t, externalAddress(t, b.params))))
	id := b.addMempool(in)

	if _, err := m.AnalyzeFeeBump(id); err == nil {
		t.Fatal("expected an error without a fee source")
	}
	b.fees = map[string]api.Satoshi{id: 200}
	info, err := m.AnalyzeFeeBump(id)
	if err != nil {
		t.Fatal(err)
	}
	if info.CanRBF || !strings.Contains(info.RBFReason, "not every input") {
		t.Fatalf("RBF = %v (%s)", info.CanRBF, info.RBFReason)
	}
	if !info.CanCPFP || info.CPFPValue != 80_000 || info.Fee != 200 {
		t.Fatalf("info = %+v", info)
	}
	res, err := m.BuildCPFP(id, 15)
	if err != nil {
		t.Fatal(err)
	}
	child := signAndSend(t, m, b, res)
	fee := 200 + txFee(b, child)
	if rate := float64(fee) / float64(txVSize(in)+txVSize(child)); rate < 15 {
		t.Fatalf("package feerate %.2f < 15", rate)
	}
}

func TestAnalyzeFeeBump_Confirmed(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, b)
	addr, _ := m.GetNewAddress(0)
	id := b.fund(t, addr, 10_000)
	if _, err := m.AnalyzeFeeBump(id); err == nil ||
		!strings.Contains(err.Error(), "already confirmed") {
		t.Fatalf("err = %v", err)
	}
}

func TestReplacementFee(t *testing.T) {
	// The feerate decides when it pays more than the minimum ...
	if got := replacementFee(200, 10, 141); got != 1410 {
		t.Fatalf("got %d, want 1410", got)
	}
	// ... otherwise the old fee plus 1 sat/vB of the new size.
	if got := replacementFee(1000, 3, 141); got != 1141 {
		t.Fatalf("got %d, want 1141", got)
	}
}
