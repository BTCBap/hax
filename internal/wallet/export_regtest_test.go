//go:build emulator

package wallet

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// coreExportCheck has Core parse the exported descriptor, derive its
// receive and change addresses, and import it into a watch-only wallet,
// and checks every address against hax's own derivation.
func coreExportCheck(t *testing.T, node *regtestNode, m *WalletManager, name string,
	receive, change func(i uint32) string) {
	t.Helper()
	desc, err := m.ExportDescriptor(0)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		IsRange bool `json:"isrange"`
	}
	coreJSON(t, node, &info, "getdescriptorinfo", desc)
	if !info.IsRange {
		t.Fatalf("Core sees %s as a single address, want a range", desc)
	}
	var derived [][]string
	coreJSON(t, node, &derived, "deriveaddresses", desc, "[0,4]")
	if len(derived) != 2 {
		t.Fatalf("deriveaddresses gave %d paths, want receive and change: %s", len(derived), desc)
	}
	for i := uint32(0); i < 5; i++ {
		if got, want := derived[0][i], regtestAddr(t, receive(i)); got != want {
			t.Fatalf("receive %d: Core %s, hax %s (%s)", i, got, want, desc)
		}
		if got, want := derived[1][i], regtestAddr(t, change(i)); got != want {
			t.Fatalf("change %d: Core %s, hax %s (%s)", i, got, want, desc)
		}
	}

	w := "export-" + name
	coreJSON(t, node, nil, "-named", "createwallet", "wallet_name="+w, "disable_private_keys=true", "blank=true")
	req, _ := json.Marshal([]map[string]interface{}{{
		"desc": desc, "timestamp": "now", "active": true, "range": []int{0, 10}}})
	var res []struct {
		Success bool `json:"success"`
	}
	coreJSON(t, node, &res, "-rpcwallet="+w, "importdescriptors", string(req))
	if len(res) != 1 || !res[0].Success {
		t.Fatalf("importdescriptors failed for %s", desc)
	}
	// Core's wallet owns hax's addresses, each from the descriptor for
	// its chain (getaddressinfo's ischange is only an address book
	// heuristic, so check parent_desc).
	for _, c := range []struct {
		addr   string
		change bool
	}{{receive(0), false}, {receive(4), false}, {change(0), true}} {
		var ai struct {
			IsMine     bool   `json:"ismine"`
			ParentDesc string `json:"parent_desc"`
		}
		coreJSON(t, node, &ai, "-rpcwallet="+w, "getaddressinfo", regtestAddr(t, c.addr))
		if !ai.IsMine || strings.Contains(ai.ParentDesc, "/1/*") != c.change {
			t.Fatalf("Core wallet: %s ismine=%v from %s", c.addr, ai.IsMine, ai.ParentDesc)
		}
	}
}

// coreJSON runs bitcoin-cli and decodes its output into v (if not nil),
// failing t rather than the test that started the node.
func coreJSON(t *testing.T, node *regtestNode, v interface{}, args ...string) {
	t.Helper()
	out, err := node.call(args...)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		if err := json.Unmarshal([]byte(out), v); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

// TestRegtestDescriptorExport checks exported descriptors against Core
// for software, hardware (watch-only) and multisig wallets.
func TestRegtestDescriptorExport(t *testing.T) {
	node := startRegtest(t)
	for _, tt := range signerTestTypes {
		for _, hw := range []bool{false, true} {
			name := tt.name
			if hw {
				name = "hw-" + name
			}
			t.Run(name, func(t *testing.T) {
				m, _ := newSignerTestManager(t, NetworkTestnet, tt.addr, tt.purpose, hw)
				acct := m.GetAccounts()[0]
				derive := func(change bool) func(uint32) string {
					return func(i uint32) string {
						a, _, err := m.deriveAddress(0, i, change, acct.AddressType, acct.CustomDerivationPath)
						if err != nil {
							t.Fatal(err)
						}
						return a
					}
				}
				coreExportCheck(t, node, m, name, derive(false), derive(true))
			})
		}
	}
	for _, st := range []MultisigScriptType{MultisigLegacy, MultisigNestedSegwit, MultisigNativeSegwit} {
		name := fmt.Sprintf("multisig-%d", st)
		t.Run(name, func(t *testing.T) {
			m := newExportMultisig(t, st)
			derive := func(change bool) func(uint32) string {
				return func(i uint32) string {
					a, _, err := m.GetMultisigAddress(change, i)
					if err != nil {
						t.Fatal(err)
					}
					return a
				}
			}
			coreExportCheck(t, node, m, name, derive(false), derive(true))
		})
	}
}

// newExportMultisig returns a watch-only 2-of-3 from three test seeds.
func newExportMultisig(t *testing.T, st MultisigScriptType) *WalletManager {
	t.Helper()
	var cos []MultisigCosignerConfig
	for i, mn := range []string{testMnemonic,
		"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
		"all all all all all all all all all all all all"} {
		seed, _ := MnemonicToSeed(mn, "")
		w, _ := NewHDWallet(seed, true)
		x, err := w.GetExtendedPublicKeyAtPath(fmt.Sprintf("m/48'/1'/0'/%d'", st))
		if err != nil {
			t.Fatal(err)
		}
		fp, _ := w.GetMasterFingerprintHex()
		cos = append(cos, MultisigCosignerConfig{Name: fmt.Sprintf("seed%d", i),
			Fingerprint: fp, Xpub: x, KeyOrigin: fmt.Sprintf("%s/48h/1h/0h/%dh", fp, st)})
	}
	m, err := NewWalletManager(t.TempDir(), newFundedBackend(&TestNet4Params))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateMultisigWalletOnNetwork("ms", "export-pw", 2, cos, st, NetworkTestnet); err != nil {
		t.Fatal(err)
	}
	if err := m.UnlockWallet("ms", "export-pw"); err != nil {
		t.Fatal(err)
	}
	m.StopBackgroundSync()
	t.Cleanup(m.LockWallet)
	return m
}
