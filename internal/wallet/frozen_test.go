package wallet

import (
	"errors"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/api"
)

func TestFrozenCoinsPersist(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, b)
	id := strings.Repeat("ab", 32)

	if err := m.SetUTXOFrozen(id, 1, true); err != nil {
		t.Fatal(err)
	}
	if !m.IsUTXOFrozen(id, 1) || m.IsUTXOFrozen(id, 0) {
		t.Fatal("frozen state wrong in memory")
	}
	d, err := m.storage.LoadWallet("w", "correct horse battery")
	if err != nil || !d.FrozenOutputs[id+":1"] {
		t.Fatalf("not in the encrypted wallet file: %v %v", d.FrozenOutputs, err)
	}
	if err := m.SetUTXOFrozen(id, 1, false); err != nil {
		t.Fatal(err)
	}
	d, _ = m.storage.LoadWallet("w", "correct horse battery")
	if m.IsUTXOFrozen(id, 1) || len(d.FrozenOutputs) != 0 {
		t.Fatal("unfreeze not saved")
	}
}

// TestFrozenCoinsNotSpent: automatic selection skips a frozen coin,
// naming it explicitly is refused, and with every coin frozen there is
// nothing to spend.
func TestFrozenCoinsNotSpent(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, b)
	big, _ := m.GetNewAddress(0)
	bigID := b.fund(t, big, 200_000)
	small, _ := m.GetNewAddress(0)
	b.fund(t, small, 60_000)
	if err := m.SetUTXOFrozen(bigID, 0, true); err != nil {
		t.Fatal(err)
	}
	dest := externalAddress(t, b.params)
	pay := []TxRecipient{{Address: dest, Amount: 50_000}}

	res, err := m.CreatePSBTBatchTransaction(0, pay, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range res.Packet.UnsignedTx.TxIn {
		if in.PreviousOutPoint.Hash.String() == bigID {
			t.Fatal("automatic selection spent the frozen coin")
		}
	}
	// More than the unfrozen coin holds.
	if _, err := m.CreatePSBTBatchTransaction(0,
		[]TxRecipient{{Address: dest, Amount: 100_000}}, 2, nil); err == nil {
		t.Fatal("spent the frozen coin to cover a larger payment")
	}
	frozen := api.UTXO{TxID: bigID, Vout: 0, Value: 200_000}
	if _, err := m.CreatePSBTBatchTransaction(0, pay, 2,
		[]api.UTXO{frozen}); !errors.Is(err, ErrCoinFrozen) {
		t.Fatalf("explicit frozen coin: err = %v, want ErrCoinFrozen", err)
	}

	for _, u := range mustUTXOs(t, m) {
		_ = m.SetUTXOFrozen(u.TxID, u.Vout, true)
	}
	if _, err := m.CreatePSBTBatchTransaction(0, pay, 2, nil); err == nil ||
		!strings.Contains(err.Error(), "frozen") {
		t.Fatalf("all frozen: err = %v", err)
	}
}

func mustUTXOs(t *testing.T, m *WalletManager) []UTXOWithPath {
	t.Helper()
	u, err := m.GetUTXOsWithPath(0)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestFrozenCoinsNotUsedForFeeBumps: RBF won't add a frozen coin and
// CPFP won't spend a frozen output of the stuck transaction.
func TestFrozenCoinsNotUsedForFeeBumps(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, b)
	addr, _ := m.GetNewAddress(0)
	b.fund(t, addr, 50_000)
	dest := externalAddress(t, b.params)
	res, err := m.CreatePSBTBatchTransaction(0,
		[]TxRecipient{{Address: dest, Amount: 49_700}}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	orig := signAndSend(t, m, b, res)
	origID := orig.TxHash().String()

	spare, _ := m.GetNewAddress(0)
	spareID := b.fund(t, spare, 30_000)
	_ = m.SetUTXOFrozen(spareID, 0, true)
	if _, err := m.BuildRBF(origID, 20); !errors.Is(err, ErrBumpNeedsFunds) {
		t.Fatalf("RBF with only a frozen spare: err = %v, want ErrBumpNeedsFunds", err)
	}

	parent, _ := sendTestPayment(t, m, b, []int64{100_000}, 40_000)
	parentID := parent.TxHash().String()
	for i := range parent.TxOut {
		_ = m.SetUTXOFrozen(parentID, uint32(i), true) // #nosec G115 -- test tx index
	}
	info, err := m.AnalyzeFeeBump(parentID)
	if err != nil {
		t.Fatal(err)
	}
	if info.CanCPFP || !strings.Contains(info.CPFPReason, "frozen") {
		t.Fatalf("CPFP = %v (%s), want refused as frozen", info.CanCPFP, info.CPFPReason)
	}
}

func TestBIP329_FrozenOutputs(t *testing.T) {
	b := newChainBackend(&TestNet4Params)
	m := newBumpTestManager(t, AddressTypeSegWitNative, b)
	id := strings.Repeat("cd", 32)
	_ = m.SetUTXOFrozen(id, 2, true)

	out, err := m.ExportLabelsBIP329()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"output","ref":"` + id + `:2","spendable":false}` + "\n"
	if out != want {
		t.Fatalf("export:\n%s\nwant:\n%s", out, want)
	}

	other := strings.Repeat("ef", 32)
	in := `{"type":"output","ref":"` + id + `:2","spendable":true}` + "\n" +
		`{"type":"output","ref":"` + other + `:0","label":"x","spendable":false}` + "\n" +
		`{"type":"output","ref":"` + other + `:1","label":"no flag"}` + "\n" +
		`{"type":"output","ref":"nonsense","spendable":false}` + "\n"
	n, err := m.ImportLabelsBIP329(in)
	if err != nil || n != 2 {
		t.Fatalf("imported %d, %v; want 2", n, err)
	}
	if m.IsUTXOFrozen(id, 2) || !m.IsUTXOFrozen(other, 0) || m.IsUTXOFrozen(other, 1) {
		t.Fatal("spendable flags not applied")
	}
}
