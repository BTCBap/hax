package wallet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportUnifiedBackup_Locked(t *testing.T) {
	mgr := &WalletManager{
		unlocked: false,
		data: &WalletData{
			Multisig: &MultisigWalletConfig{M: 2, N: 3},
		},
	}

	_, err := mgr.ExportUnifiedBackup()
	if err == nil {
		t.Fatal("expected error for locked wallet")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("error should mention locked, got: %s", err.Error())
	}
}

func TestExportUnifiedBackup_Multisig(t *testing.T) {
	mgr := &WalletManager{
		unlocked: true,
		data: &WalletData{
			Name:      "test-multisig",
			Network:   "testnet",
			CreatedAt: 1700000000,
			Multisig: &MultisigWalletConfig{
				M:          2,
				N:          3,
				ScriptType: int(MultisigNativeSegwit),
				Cosigners: []MultisigCosignerConfig{
					{
						Name:        "Alice",
						Fingerprint: "aabbccdd",
						KeyOrigin:   "aabbccdd/48h/1h/0h/2h",
						Xpub:        "tpub1111",
						DeviceType:  "local",
						Mnemonic:    "secret seed phrase that must not leak",
					},
					{
						Name:        "Bob",
						Fingerprint: "11223344",
						KeyOrigin:   "11223344/48h/1h/0h/2h",
						Xpub:        "tpub2222",
						DeviceType:  "trezor",
					},
					{
						Name:        "Carol",
						Fingerprint: "55667788",
						KeyOrigin:   "55667788/48h/1h/0h/2h",
						Xpub:        "tpub3333",
					},
				},
			},
			TransactionLabels: map[string]string{
				"abc123": "payment to Alice",
				"def456": "refund from Bob",
			},
			Accounts: []AccountData{
				{
					AddressType: "multisig_native",
					Labels: map[string]string{
						"tb1qaddr1": "cold storage",
					},
				},
			},
		},
	}

	data, err := mgr.ExportUnifiedBackup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var backup WalletBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	// Check top-level structure
	if backup.Version != 1 {
		t.Errorf("expected version 1, got %d", backup.Version)
	}
	if backup.Format != "haxwallet-backup" {
		t.Errorf("expected format 'haxwallet-backup', got %q", backup.Format)
	}
	if backup.ExportedAt == "" {
		t.Error("exported_at should not be empty")
	}

	// Check wallet metadata
	if backup.Wallet.Name != "test-multisig" {
		t.Errorf("expected name 'test-multisig', got %q", backup.Wallet.Name)
	}
	if backup.Wallet.WalletType != "multisig" {
		t.Errorf("expected wallet_type 'multisig', got %q", backup.Wallet.WalletType)
	}
	if backup.Wallet.Network != "testnet" {
		t.Errorf("expected network 'testnet', got %q", backup.Wallet.Network)
	}

	// Check descriptor is present
	if backup.Descriptor == "" {
		t.Error("descriptor should not be empty")
	}
	if !strings.Contains(backup.Descriptor, "wsh(sortedmulti(2,") {
		t.Errorf("descriptor should be wsh(sortedmulti(...)), got %q", backup.Descriptor)
	}

	// Check multisig section
	if backup.Multisig == nil {
		t.Fatal("multisig section should not be nil")
	}
	if backup.Multisig.M != 2 {
		t.Errorf("expected M=2, got %d", backup.Multisig.M)
	}
	if backup.Multisig.N != 3 {
		t.Errorf("expected N=3, got %d", backup.Multisig.N)
	}
	if backup.Multisig.ScriptType != "native_segwit" {
		t.Errorf("expected script_type 'native_segwit', got %q", backup.Multisig.ScriptType)
	}
	if len(backup.Multisig.Cosigners) != 3 {
		t.Fatalf("expected 3 cosigners, got %d", len(backup.Multisig.Cosigners))
	}
	if backup.Multisig.Cosigners[0].Name != "Alice" {
		t.Errorf("expected first cosigner 'Alice', got %q", backup.Multisig.Cosigners[0].Name)
	}

	// Hardware section should be nil for multisig
	if backup.Hardware != nil {
		t.Error("hardware section should be nil for multisig wallet")
	}

	// Check labels
	if backup.Labels == nil {
		t.Fatal("labels should not be nil")
	}
	if len(backup.Labels.Transactions) != 2 {
		t.Errorf("expected 2 transaction labels, got %d", len(backup.Labels.Transactions))
	}
	if len(backup.Labels.Addresses) != 1 {
		t.Errorf("expected 1 address label, got %d", len(backup.Labels.Addresses))
	}
}

func TestExportUnifiedBackup_SingleSig(t *testing.T) {
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("failed to create seed: %v", err)
	}
	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("failed to create HD wallet: %v", err)
	}

	mgr := &WalletManager{
		unlocked: true,
		wallet:   hdWallet,
		data: &WalletData{
			Name:      "my-wallet",
			Network:   "testnet",
			CreatedAt: 1700000000,
			Mnemonic:  testMnemonic,
			Accounts: []AccountData{
				{
					Index:       0,
					AddressType: "segwit_native",
					Labels: map[string]string{
						"tb1qtest": "savings",
					},
				},
			},
			TransactionLabels: map[string]string{
				"tx123": "coffee",
			},
		},
	}

	data, err := mgr.ExportUnifiedBackup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var backup WalletBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if backup.Wallet.WalletType != "single_sig" {
		t.Errorf("expected wallet_type 'single_sig', got %q", backup.Wallet.WalletType)
	}
	if backup.Wallet.AddressType != "segwit_native" {
		t.Errorf("expected address_type 'segwit_native', got %q", backup.Wallet.AddressType)
	}
	if backup.Descriptor == "" {
		t.Error("descriptor should not be empty")
	}
	if backup.Multisig != nil {
		t.Error("multisig section should be nil for single-sig")
	}
	if backup.Hardware != nil {
		t.Error("hardware section should be nil for single-sig")
	}

	// Check labels present
	if backup.Labels == nil {
		t.Fatal("labels should not be nil")
	}
	if len(backup.Labels.Transactions) != 1 {
		t.Errorf("expected 1 transaction label, got %d", len(backup.Labels.Transactions))
	}
	if len(backup.Labels.Addresses) != 1 {
		t.Errorf("expected 1 address label, got %d", len(backup.Labels.Addresses))
	}

	// Secrets must not leak
	jsonStr := string(data)
	if strings.Contains(jsonStr, "abandon") {
		t.Error("backup contains wallet mnemonic — secrets must never be exported")
	}
	if strings.Contains(jsonStr, "mnemonic") {
		t.Error("backup JSON contains 'mnemonic' key — secrets must never be exported")
	}
}

func TestExportUnifiedBackup_Hardware(t *testing.T) {
	mgr := &WalletManager{
		unlocked: true,
		data: &WalletData{
			Name:      "my-trezor",
			Network:   "testnet",
			CreatedAt: 1700000000,
			HardwareWallet: &HardwareWalletConfig{
				Fingerprint: "deadbeef",
				KeyOrigin:   "deadbeef/84h/1h/0h",
				Xpub:        "tpub6BXPerqvA1234",
				DeviceType:  "trezor",
				AddressType: "segwit_native",
			},
			Accounts: []AccountData{
				{
					Index:       0,
					AddressType: "segwit_native",
				},
			},
		},
	}

	data, err := mgr.ExportUnifiedBackup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var backup WalletBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if backup.Wallet.WalletType != "hardware" {
		t.Errorf("expected wallet_type 'hardware', got %q", backup.Wallet.WalletType)
	}
	if backup.Wallet.AddressType != "segwit_native" {
		t.Errorf("expected address_type 'segwit_native', got %q", backup.Wallet.AddressType)
	}
	if backup.Descriptor == "" {
		t.Error("descriptor should not be empty")
	}
	if backup.Multisig != nil {
		t.Error("multisig section should be nil for hardware wallet")
	}
	if backup.Hardware == nil {
		t.Fatal("hardware section should not be nil")
	}
	if backup.Hardware.DeviceType != "trezor" {
		t.Errorf("expected device_type 'trezor', got %q", backup.Hardware.DeviceType)
	}
	if backup.Hardware.Fingerprint != "deadbeef" {
		t.Errorf("expected fingerprint 'deadbeef', got %q", backup.Hardware.Fingerprint)
	}
	if backup.Hardware.Xpub != "tpub6BXPerqvA1234" {
		t.Errorf("expected xpub 'tpub6BXPerqvA1234', got %q", backup.Hardware.Xpub)
	}
}

func TestExportUnifiedBackup_NoSecrets(t *testing.T) {
	mgr := &WalletManager{
		unlocked: true,
		data: &WalletData{
			Name:      "secret-test",
			Network:   "testnet",
			CreatedAt: 1700000000,
			Mnemonic:  "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
			Multisig: &MultisigWalletConfig{
				M:          2,
				N:          2,
				ScriptType: int(MultisigNativeSegwit),
				Cosigners: []MultisigCosignerConfig{
					{
						Name:        "Cosigner1",
						Fingerprint: "aabbccdd",
						KeyOrigin:   "aabbccdd/48h/1h/0h/2h",
						Xpub:        "tpub1111",
						Mnemonic:    "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
					},
					{
						Name:        "Cosigner2",
						Fingerprint: "11223344",
						KeyOrigin:   "11223344/48h/1h/0h/2h",
						Xpub:        "tpub2222",
						Mnemonic:    "legal winner thank year wave sausage worth useful legal winner thank yellow",
					},
				},
			},
			Accounts: []AccountData{
				{AddressType: "multisig_native"},
			},
		},
	}

	data, err := mgr.ExportUnifiedBackup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonStr := string(data)

	if strings.Contains(jsonStr, "abandon") {
		t.Error("backup contains wallet mnemonic — secrets must never be exported")
	}
	if strings.Contains(jsonStr, "zoo") {
		t.Error("backup contains cosigner mnemonic — secrets must never be exported")
	}
	if strings.Contains(jsonStr, "legal winner") {
		t.Error("backup contains cosigner mnemonic — secrets must never be exported")
	}
	if strings.Contains(jsonStr, "mnemonic") {
		t.Error("backup JSON contains 'mnemonic' key — secrets must never be exported")
	}
}

func TestExportUnifiedBackup_NoLabels(t *testing.T) {
	mgr := &WalletManager{
		unlocked: true,
		data: &WalletData{
			Name:      "no-labels",
			Network:   "testnet",
			CreatedAt: 1700000000,
			Multisig: &MultisigWalletConfig{
				M:          2,
				N:          2,
				ScriptType: int(MultisigNativeSegwit),
				Cosigners: []MultisigCosignerConfig{
					{
						Name:        "A",
						Fingerprint: "aabbccdd",
						KeyOrigin:   "aabbccdd/48h/1h/0h/2h",
						Xpub:        "tpub1111",
					},
					{
						Name:        "B",
						Fingerprint: "11223344",
						KeyOrigin:   "11223344/48h/1h/0h/2h",
						Xpub:        "tpub2222",
					},
				},
			},
			Accounts: []AccountData{
				{AddressType: "multisig_native"},
			},
		},
	}

	data, err := mgr.ExportUnifiedBackup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var backup WalletBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if backup.Labels != nil {
		t.Error("labels should be nil/omitted when no labels exist")
	}
	if strings.Contains(string(data), `"labels"`) {
		t.Error("labels key should not appear in JSON when empty")
	}
}

func TestExportUnifiedBackup_NoAccounts(t *testing.T) {
	mgr := &WalletManager{
		unlocked: true,
		data: &WalletData{
			Name:    "empty",
			Network: "testnet",
		},
	}

	_, err := mgr.ExportUnifiedBackup()
	if err == nil {
		t.Fatal("expected error for wallet with no accounts")
	}
}
