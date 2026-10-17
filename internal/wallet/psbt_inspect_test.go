package wallet

import (
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func hasWarning(insp *PSBTInspection, substr string) bool {
	for _, w := range insp.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// inspectTestPSBT builds a payment to an external address with change.
func inspectTestPSBT(t *testing.T, m *WalletManager, b *fundedBackend) (*PSBTResult, string) {
	t.Helper()
	dest := externalAddress(t, b.params)
	res, err := m.CreatePSBTBatchTransaction(0,
		[]TxRecipient{{Address: dest, Amount: 40_000}}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasChange {
		t.Fatal("setup: expected change")
	}
	return res, dest
}

func checkCleanInspection(t *testing.T, insp *PSBTInspection, res *PSBTResult, dest string) {
	t.Helper()
	if len(insp.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", insp.Warnings)
	}
	if !insp.FeeKnown || insp.Fee != res.Fee {
		t.Fatalf("fee = %d (known %v), want %d", insp.Fee, insp.FeeKnown, res.Fee)
	}
	if insp.Sent != 40_000 || !insp.RBF {
		t.Fatalf("sent = %d, rbf = %v", insp.Sent, insp.RBF)
	}
	for i, in := range insp.Inputs {
		if !in.Mine || !in.ValueKnown || in.Signed {
			t.Fatalf("input %d: %+v", i, in)
		}
	}
	if len(insp.Outputs) != 2 ||
		insp.Outputs[0].Address != dest || insp.Outputs[0].Owner != OutputExternal ||
		insp.Outputs[1].Address != res.ChangeAddress || insp.Outputs[1].Owner != OutputChange {
		t.Fatalf("outputs = %+v", insp.Outputs)
	}
}

// TestInspectPSBT_HaxBuilt: a fresh hax PSBT inspects clean, the same
// after a base64 round trip (an imported PSBT), for every single-sig
// type, software and hardware.
func TestInspectPSBT_HaxBuilt(t *testing.T) {
	for _, hw := range []bool{false, true} {
		for _, tt := range signerTestTypes {
			t.Run(fmt.Sprintf("%s/hw=%v", tt.name, hw), func(t *testing.T) {
				m, b := newSignerTestManager(t, NetworkTestnet, tt.addr, tt.purpose, hw)
				res, dest := inspectTestPSBT(t, m, b)
				checkCleanInspection(t, m.InspectPSBT(res.Packet), res, dest)

				b64, err := m.ExportPSBTBase64(res)
				if err != nil {
					t.Fatal(err)
				}
				imported, err := m.ImportPSBTFromBase64(b64)
				if err != nil {
					t.Fatal(err)
				}
				checkCleanInspection(t, m.InspectPSBT(imported.Packet), res, dest)

				if !hw {
					if err := m.SignPSBTTransaction(res); err != nil {
						t.Fatal(err)
					}
					if insp := m.InspectPSBT(res.Packet); !insp.Inputs[0].Signed {
						t.Fatal("signed input not reported as signed")
					}
				}
			})
		}
	}
}

// TestInspectPSBT_ChangeIsVerified: change must be proven by our keys.
// A PSBT that keeps our change derivation but pays the output elsewhere
// is a payment, with a warning; an output with no derivation paying one
// of our addresses is still recognised.
func TestInspectPSBT_ChangeIsVerified(t *testing.T) {
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			m, b := newSignerTestManager(t, NetworkTestnet, tt.addr, tt.purpose, false)
			res, _ := inspectTestPSBT(t, m, b)
			p := res.Packet
			p.UnsignedTx.TxOut[1].PkScript = addressScript(
				externalAddress(t, b.params), b.params)
			insp := m.InspectPSBT(p)
			if insp.Outputs[1].Owner != OutputExternal {
				t.Fatal("swapped change still shown as ours")
			}
			if !hasWarning(insp, "Output 2 claims to be this wallet's") {
				t.Fatalf("warnings = %v", insp.Warnings)
			}
			if insp.Sent != 40_000+res.Change {
				t.Fatalf("sent = %d", insp.Sent)
			}
		})
	}

	m, b := newSignerTestManager(t, NetworkTestnet, AddressTypeSegWitNative, 84, false)
	res, _ := inspectTestPSBT(t, m, b)
	p := res.Packet
	p.Outputs[1].Bip32Derivation = nil
	recv, _, _ := m.GetAddressByIndex(0, 3)
	p.UnsignedTx.TxOut[0].PkScript = addressScript(recv, b.params)
	insp := m.InspectPSBT(p)
	if insp.Outputs[0].Owner != OutputReceive || insp.Outputs[1].Owner != OutputChange {
		t.Fatalf("outputs = %+v", insp.Outputs)
	}
	if insp.Sent != 0 || len(insp.Warnings) != 0 {
		t.Fatalf("sent = %d, warnings = %v", insp.Sent, insp.Warnings)
	}
}

func TestInspectPSBT_Warnings(t *testing.T) {
	cases := []struct {
		name   string
		addr   AddressType
		mutate func(t *testing.T, res *PSBTResult, b *fundedBackend)
		want   string
	}{
		{"amount mismatch", AddressTypeSegWitNative, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			res.Packet.Inputs[0].WitnessUtxo.Value += 50_000
		}, "amount differs"},
		{"wrong prev tx", AddressTypeLegacy, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			other := wire.NewMsgTx(2)
			other.AddTxOut(wire.NewTxOut(1, nil))
			res.Packet.Inputs[0].NonWitnessUtxo = other
		}, "doesn't match the outpoint"},
		{"no amount", AddressTypeSegWitNative, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			res.Packet.Inputs[0].WitnessUtxo = nil
			res.Packet.Inputs[0].NonWitnessUtxo = nil
		}, "has no amount"},
		{"sighash none", AddressTypeSegWitNative, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			res.Packet.Inputs[0].SighashType = txscript.SigHashNone
		}, "sighash NONE"},
		{"high fee", AddressTypeSegWitNative, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			res.Packet.UnsignedTx.TxOut[1].Value -= 20_000
		}, "more than 10%"},
		{"dust output", AddressTypeSegWitNative, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			res.Packet.UnsignedTx.TxOut[0].Value = 100
		}, "below the dust limit"},
		{"foreign input", AddressTypeSegWitNative, func(t *testing.T, res *PSBTResult, b *fundedBackend) {
			in := &res.Packet.Inputs[0]
			in.WitnessUtxo.PkScript = addressScript(externalAddress(t, b.params), b.params)
			in.NonWitnessUtxo = nil
		}, "Input 1 isn't from this wallet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			purpose := uint32(84)
			if tc.addr == AddressTypeLegacy {
				purpose = 44
			}
			m, b := newSignerTestManager(t, NetworkTestnet, tc.addr, purpose, false)
			res, _ := inspectTestPSBT(t, m, b)
			tc.mutate(t, res, b)
			insp := m.InspectPSBT(res.Packet)
			if !hasWarning(insp, tc.want) {
				t.Fatalf("want a warning containing %q, got %v", tc.want, insp.Warnings)
			}
		})
	}
}

// TestInspectPSBT_Multisig: change of a multisig wallet is verified by
// rebuilding the multisig script.
func TestInspectPSBT_Multisig(t *testing.T) {
	for _, st := range []MultisigScriptType{MultisigLegacy, MultisigNestedSegwit, MultisigNativeSegwit} {
		t.Run(fmt.Sprint(st), func(t *testing.T) {
			s := setupMultisigTest(t, 2, 3, st)
			b := newFundedBackend(&TestNet4Params)
			m, err := NewWalletManager(t.TempDir(), b)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.CreateMultisigWalletOnNetwork("ms", "pw", 2, s.cosigners, st, NetworkTestnet); err != nil {
				t.Fatal(err)
			}
			if err := m.UnlockWallet("ms", "pw"); err != nil {
				t.Fatal(err)
			}
			m.StopBackgroundSync()
			t.Cleanup(m.LockWallet)
			addr, err := m.GetNewAddress(0)
			if err != nil {
				t.Fatal(err)
			}
			b.fund(t, addr, 100_000)

			res, dest := inspectTestPSBT(t, m, b)
			checkCleanInspection(t, m.InspectPSBT(res.Packet), res, dest)

			res.Packet.UnsignedTx.TxOut[1].PkScript = addressScript(
				externalAddress(t, b.params), b.params)
			insp := m.InspectPSBT(res.Packet)
			if insp.Outputs[1].Owner != OutputExternal ||
				!hasWarning(insp, "claims to be this wallet's") {
				t.Fatalf("swapped multisig change: %+v %v", insp.Outputs[1], insp.Warnings)
			}
		})
	}
}
