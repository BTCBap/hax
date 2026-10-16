package wallet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWalletName(t *testing.T) {
	valid := []string{"main", "My Wallet", "cold-1", "test_2", "v1.2"}
	for _, n := range valid {
		if err := ValidateWalletName(n); err != nil {
			t.Errorf("ValidateWalletName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{"", "../escape", "../../tmp/x", "foo/bar", `a\b`, ".hidden", " lead", "a..b", "x\x00y", strings.Repeat("a", MaxWalletNameLength+1)}
	for _, n := range invalid {
		if err := ValidateWalletName(n); err == nil {
			t.Errorf("ValidateWalletName(%q) = nil, want error", n)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Error("expected error for short password")
	}
	if err := ValidatePassword(strings.Repeat("x", MinPasswordLength)); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStorage_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "wallets")
	s, err := NewWalletStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := &WalletData{Version: 1, Name: "x", Network: "testnet"}
	for _, name := range []string{"../escape", "../../tmp/x", "sub/dir"} {
		if err := s.SaveWallet(name, data, "password123"); err == nil {
			t.Errorf("SaveWallet(%q) succeeded, want error", name)
		}
		if _, err := s.LoadWallet(name, "password123"); err == nil {
			t.Errorf("LoadWallet(%q) succeeded, want error", name)
		}
		if err := s.DeleteWallet(name); err == nil {
			t.Errorf("DeleteWallet(%q) succeeded, want error", name)
		}
		if s.WalletExists(name) {
			t.Errorf("WalletExists(%q) = true", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "escape.wallet")); !os.IsNotExist(err) {
		t.Fatalf("file escaped wallet dir: %v", err)
	}
}

func TestStorage_RoundTripAndPermissions(t *testing.T) {
	s, err := NewWalletStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := &WalletData{Version: 1, Name: "w", Mnemonic: testMnemonic, Passphrase: "TREZOR", Network: "testnet"}
	if err := s.SaveWallet("w", in, "correct horse"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.walletDir, "w.wallet"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("wallet file mode = %o, want 600", info.Mode().Perm())
	}
	out, err := s.LoadWallet("w", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if out.Mnemonic != in.Mnemonic || out.Passphrase != in.Passphrase {
		t.Error("round-trip mismatch")
	}
	if _, err := s.LoadWallet("w", "wrong password"); err == nil {
		t.Error("expected error with wrong password")
	}
}

func TestCreateAndRestore_EnforcePolicy(t *testing.T) {
	m := &WalletManager{storage: mustStorage(t)}
	if _, err := m.CreateWallet("ok", "short", 12); err == nil {
		t.Error("CreateWallet accepted short password")
	}
	if _, err := m.CreateWallet("../bad", "longenough", 12); err == nil {
		t.Error("CreateWallet accepted traversal name")
	}
	if err := m.RestoreWallet("ok", "short", testMnemonic); err == nil {
		t.Error("RestoreWallet accepted short password")
	}
}

func TestPassphraseHonoredOnUnlock(t *testing.T) {
	m := &WalletManager{storage: mustStorage(t)}
	if err := m.RestoreWalletWithPassphrase("pp", "longenough", testMnemonic, "TREZOR"); err != nil {
		t.Fatal(err)
	}
	data, err := m.storage.LoadWallet("pp", "longenough")
	if err != nil {
		t.Fatal(err)
	}
	got, err := seedFromWalletData(data)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := MnemonicToSeed(testMnemonic, "TREZOR")
	none, _ := MnemonicToSeed(testMnemonic, "")
	if string(got) != string(want) {
		t.Error("seed did not use stored passphrase")
	}
	if string(got) == string(none) {
		t.Error("passphrase seed equals empty-passphrase seed")
	}
}

func TestValidateBitcoinAddress_StrictNetwork(t *testing.T) {
	if err := ValidateBitcoinAddress("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", true); err == nil {
		t.Error("mainnet address accepted on testnet")
	}
	if err := ValidateBitcoinAddress("1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", true); err == nil {
		t.Error("mainnet P2PKH accepted on testnet")
	}
	if err := ValidateBitcoinAddress("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", true); err != nil {
		t.Errorf("testnet address rejected: %v", err)
	}
}

func mustStorage(t *testing.T) *WalletStorage {
	t.Helper()
	s, err := NewWalletStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
