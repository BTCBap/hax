package wallet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/ur"
)

// testMultiCosigners derives three distinct BIP48 cosigners from fixed test
// seeds for the given script type, returned in seed order.
func testMultiCosigners(t *testing.T, st MultisigScriptType) []MultisigCosignerConfig {
	t.Helper()
	seeds := []string{
		testMnemonic,
		"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
		"all all all all all all all all all all all all",
	}
	cos := make([]MultisigCosignerConfig, 0, len(seeds))
	for i, mn := range seeds {
		seed, err := MnemonicToSeed(mn, "")
		if err != nil {
			t.Fatal(err)
		}
		w, err := NewHDWallet(seed, true)
		if err != nil {
			t.Fatal(err)
		}
		xpub, err := w.GetExtendedPublicKeyAtPath(fmt.Sprintf("m/48'/1'/0'/%d'", st))
		if err != nil {
			t.Fatal(err)
		}
		fp, err := w.GetMasterFingerprintHex()
		if err != nil {
			t.Fatal(err)
		}
		cos = append(cos, MultisigCosignerConfig{
			Name:        fmt.Sprintf("cosigner%d", i+1),
			Fingerprint: fp,
			Xpub:        xpub,
			KeyOrigin:   fmt.Sprintf("%s/48h/1h/0h/%dh", fp, st),
		})
	}
	return cos
}

// unsortedMultiCosigners returns the same cosigners in an order that differs
// from BIP67 sorting, so multi(...) and sortedmulti(...) derive different
// scripts.
func unsortedMultiCosigners(t *testing.T, st MultisigScriptType) []MultisigCosignerConfig {
	t.Helper()
	cos := testMultiCosigners(t, st)

	type keyedCosigner struct {
		config MultisigCosignerConfig
		pubKey []byte
	}
	keyed := make([]keyedCosigner, len(cos))
	for i, cs := range cos {
		pubKey, err := DeriveChildPubKey(cs.Xpub, 0, 0, &TestNet4Params)
		if err != nil {
			t.Fatal(err)
		}
		keyed[i] = keyedCosigner{config: cs, pubKey: pubKey}
	}
	sort.Slice(keyed, func(i, j int) bool {
		return bytes.Compare(keyed[i].pubKey, keyed[j].pubKey) < 0
	})

	// Rotate the BIP67 order so the result is guaranteed unsorted.
	ordered := make([]MultisigCosignerConfig, len(keyed))
	for i := range keyed {
		ordered[i] = keyed[(i+1)%len(keyed)].config
	}
	return ordered
}

func multiDescriptorKeys(cos []MultisigCosignerConfig) string {
	keys := make([]string, len(cos))
	for i, cs := range cos {
		keys[i] = fmt.Sprintf("[%s]%s/<0;1>/*", cs.KeyOrigin, cs.Xpub)
	}
	return strings.Join(keys, ",")
}

func walletFromMultisigConfig(t *testing.T, name string, config *MultisigWalletConfig) *WalletManager {
	t.Helper()
	m, err := NewWalletManager(t.TempDir(), newFundedBackend(&TestNet4Params))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateMultisigWalletWithOrderOnNetwork(name, "multi-pw", config.M,
		config.Cosigners, MultisigScriptType(config.ScriptType), NetworkTestnet,
		config.Unsorted); err != nil {
		t.Fatal(err)
	}
	if err := m.UnlockWallet(name, "multi-pw"); err != nil {
		t.Fatal(err)
	}
	m.StopBackgroundSync()
	t.Cleanup(m.LockWallet)
	return m
}

// TestMultiDescriptorRoundTrip imports a multi(...) descriptor, exports it,
// re-imports it, and checks that the key order and derived addresses survive.
func TestMultiDescriptorRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		st     MultisigScriptType
		prefix string
		suffix string
	}{
		{"native", MultisigNativeSegwit, "wsh(multi(", "))"},
		{"nested", MultisigNestedSegwit, "sh(wsh(multi(", ")))"},
		{"legacy", MultisigLegacy, "sh(multi(", "))"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos := unsortedMultiCosigners(t, tt.st)
			body := tt.prefix + "2," + multiDescriptorKeys(cos) + tt.suffix

			parsed, err := ParseDescriptor(withDescriptorChecksum(body))
			if err != nil {
				t.Fatal(err)
			}
			if !parsed.IsMultisig || !parsed.MultisigConfig.Unsorted {
				t.Fatalf("parsed config: multisig=%v unsorted=%v", parsed.IsMultisig,
					parsed.MultisigConfig.Unsorted)
			}

			original := walletFromMultisigConfig(t, "original-"+tt.name, parsed.MultisigConfig)

			descriptor, err := original.GetMultisigDescriptor()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(descriptor, "multi(") || strings.Contains(descriptor, "sortedmulti(") {
				t.Fatalf("descriptor lost multi(): %s", descriptor)
			}
			exportedBody, checksum, found := strings.Cut(descriptor, "#")
			if !found {
				t.Fatalf("descriptor has no checksum: %s", descriptor)
			}
			if exportedBody != body {
				t.Fatalf("descriptor body changed key order:\n got %s\nwant %s", exportedBody, body)
			}
			if checksum != DescriptorChecksum(exportedBody) {
				t.Fatalf("bad checksum %q for %q", checksum, exportedBody)
			}

			urStr, err := original.ExportDescriptorUR()
			if err != nil {
				t.Fatal(err)
			}
			payload, err := ur.DecodeSinglePart(strings.TrimPrefix(urStr, "ur:crypto-output/"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(payload, []byte{0xd9, 0x01, 0x96}) {
				t.Fatal("UR descriptor does not use multi tag 406")
			}

			reparsed, err := ParseDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			if !reparsed.MultisigConfig.Unsorted {
				t.Fatal("re-imported descriptor is no longer unsorted")
			}
			if len(reparsed.MultisigConfig.Cosigners) != len(cos) {
				t.Fatalf("got %d cosigners, want %d", len(reparsed.MultisigConfig.Cosigners), len(cos))
			}
			for i, want := range cos {
				got := reparsed.MultisigConfig.Cosigners[i]
				if got.Xpub != want.Xpub || got.KeyOrigin != want.KeyOrigin {
					t.Fatalf("cosigner %d order changed: got %s/%s, want %s/%s", i,
						got.KeyOrigin, got.Xpub, want.KeyOrigin, want.Xpub)
				}
			}

			reimported := walletFromMultisigConfig(t, "reimported-"+tt.name, reparsed.MultisigConfig)
			for i := uint32(0); i < 5; i++ {
				for _, change := range []bool{false, true} {
					wantAddr, _, err := original.GetMultisigAddress(change, i)
					if err != nil {
						t.Fatal(err)
					}
					gotAddr, _, err := reimported.GetMultisigAddress(change, i)
					if err != nil {
						t.Fatal(err)
					}
					if gotAddr != wantAddr {
						t.Fatalf("change=%v index=%d: got %s, want %s", change, i, gotAddr, wantAddr)
					}
				}
			}
		})
	}
}

// TestSortedMultiDescriptorGolden pins the descriptor text for a sorted
// wallet: it must still use sortedmulti and keep the stored key order.
func TestSortedMultiDescriptorGolden(t *testing.T) {
	cos := testMultiCosigners(t, MultisigNativeSegwit)

	config := &MultisigWalletConfig{
		M:          2,
		N:          len(cos),
		ScriptType: int(MultisigNativeSegwit),
		Unsorted:   false,
		Cosigners:  cos,
	}
	m := walletFromMultisigConfig(t, "sorted-golden", config)

	got, err := m.GetMultisigDescriptor()
	if err != nil {
		t.Fatal(err)
	}
	const golden = `wsh(sortedmulti(2,[73c5da0a/48h/1h/0h/2h]tpubDFH9dgzveyD8zTbPUFuLrGmCydNvxehyNdUXKJAQN8x4aZ4j6UZqGfnqFrD4NqyaTVGKbvEW54tsvPTK2UoSbCC1PJY8iCNiwTL3RWZEheQ/<0;1>/*,[3f635a63/48h/1h/0h/2h]tpubDFPtPArj4GzBEFHohegg1Xatrc1Fi9oSox5LzuSRX91miwQxuUrEpBxpvDRsmZYJKYFhgdK3UStsjC8JKXfUbMinjFqiEM4uNwzVaCaHpys/<0;1>/*,[5c9e228d/48h/1h/0h/2h]tpubDEGquuorgFNbDrg8vepq1HnaV2mgQu9TcSBgBYfXw4AX8VMgkWqvkxHNuJmiah8iVnA3Hgj4cSvaGAXEnq814yC6hMEreckLsd7zyLL3o76/<0;1>/*))#e2mxr002`
	if got != golden {
		t.Fatalf("sorted multisig descriptor changed:\n got %s\nwant %s", got, golden)
	}
}

func TestExportFormatsRejectUnsortedMultisig(t *testing.T) {
	unsortedCos := unsortedMultiCosigners(t, MultisigNativeSegwit)
	unsorted := &MultisigWalletConfig{
		M:          2,
		N:          len(unsortedCos),
		ScriptType: int(MultisigNativeSegwit),
		Unsorted:   true,
		Cosigners:  unsortedCos,
	}
	m := walletFromMultisigConfig(t, "unsorted-export", unsorted)

	if _, err := m.ExportMultisigConfig(); !errors.Is(err, ErrUnsortedMultisigFormat) {
		t.Fatalf("ExportMultisigConfig error = %v, want ErrUnsortedMultisigFormat", err)
	}
	if _, err := m.ExportColdcardMultisig(); !errors.Is(err, ErrUnsortedMultisigFormat) {
		t.Fatalf("ExportColdcardMultisig error = %v, want ErrUnsortedMultisigFormat", err)
	}

	sortedCos := testMultiCosigners(t, MultisigNativeSegwit)
	sorted := &MultisigWalletConfig{
		M:          2,
		N:          len(sortedCos),
		ScriptType: int(MultisigNativeSegwit),
		Unsorted:   false,
		Cosigners:  sortedCos,
	}
	sm := walletFromMultisigConfig(t, "sorted-export", sorted)

	if _, err := sm.ExportMultisigConfig(); err != nil {
		t.Fatalf("ExportMultisigConfig for sorted wallet: %v", err)
	}
	if _, err := sm.ExportColdcardMultisig(); err != nil {
		t.Fatalf("ExportColdcardMultisig for sorted wallet: %v", err)
	}
}

// TestExportUnifiedBackupUnsortedDescriptor checks that the unified backup
// inherits the multi(...) descriptor from GetMultisigDescriptor.
func TestExportUnifiedBackupUnsortedDescriptor(t *testing.T) {
	cos := unsortedMultiCosigners(t, MultisigNativeSegwit)
	config := &MultisigWalletConfig{
		M:          2,
		N:          len(cos),
		ScriptType: int(MultisigNativeSegwit),
		Unsorted:   true,
		Cosigners:  cos,
	}
	m := walletFromMultisigConfig(t, "unsorted-backup", config)

	data, err := m.ExportUnifiedBackup()
	if err != nil {
		t.Fatal(err)
	}
	var backup WalletBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(backup.Descriptor, "wsh(multi(2,") {
		t.Fatalf("backup descriptor does not use multi(): %s", backup.Descriptor)
	}
	if strings.Contains(backup.Descriptor, "sortedmulti(") {
		t.Fatalf("backup descriptor uses sortedmulti(): %s", backup.Descriptor)
	}
}
