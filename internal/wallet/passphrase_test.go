package wallet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHasPassphrasePersists(t *testing.T) {
	const password = "correct horse battery"
	dir := t.TempDir()
	m, err := NewWalletManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.CreateWalletOnNetwork("with-pass", password, 12, NetworkTestnet, "my passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := m.UnlockWallet("with-pass", password); err != nil {
		t.Fatal(err)
	}
	if !m.HasPassphrase() {
		t.Fatal("wallet created with a passphrase reports HasPassphrase=false")
	}
	m.LockWallet()
	if m.HasPassphrase() {
		t.Fatal("locked wallet should not report a passphrase")
	}
	if err := m.UnlockWallet("with-pass", password); err != nil {
		t.Fatal(err)
	}
	if !m.HasPassphrase() {
		t.Fatal("passphrase flag was not persisted through reload")
	}

	m.LockWallet()
	if _, err := m.CreateWalletOnNetwork("without-pass", password, 12, NetworkTestnet, ""); err != nil {
		t.Fatal(err)
	}
	if err := m.UnlockWallet("without-pass", password); err != nil {
		t.Fatal(err)
	}
	if m.HasPassphrase() {
		t.Fatal("wallet created without a passphrase reports HasPassphrase=true")
	}
}

func TestHasPassphraseOmittedWhenFalse(t *testing.T) {
	data, err := json.Marshal(WalletData{Version: 1, Name: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "has_passphrase") {
		t.Fatalf("false HasPassphrase should be omitted: %s", data)
	}
}
