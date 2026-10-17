package wallet

import (
	"errors"
	"strings"
	"testing"
)

func labelWallet(t *testing.T) (*WalletManager, string) {
	t.Helper()
	m := unlockedWithBackend(t, &flakyBackend{})
	addrs := m.GetAccounts()[0].Addresses
	if len(addrs) == 0 {
		t.Fatal("no addresses derived")
	}
	return m, addrs[0].Address
}

func TestAddressLabels_SetGetPersist(t *testing.T) {
	m, addr := labelWallet(t)
	if err := m.SetAddressLabel(addr, "  savings  "); err != nil {
		t.Fatal(err)
	}
	if got := m.GetAddressLabel(addr); got != "savings" {
		t.Fatalf("label = %q", got)
	}
	// Persisted in the encrypted wallet file.
	d, err := m.storage.LoadWallet("w", "correct horse battery")
	if err != nil || d.Accounts[0].Labels[addr] != "savings" {
		t.Fatalf("not persisted: %v %v", d.Accounts[0].Labels, err)
	}
	// Empty clears it.
	if err := m.SetAddressLabel(addr, ""); err != nil || m.GetAddressLabel(addr) != "" {
		t.Fatalf("clear failed: %v", err)
	}
	// Long labels are capped at 255 runes without splitting UTF-8.
	_ = m.SetAddressLabel(addr, strings.Repeat("é", 300))
	if got := []rune(m.GetAddressLabel(addr)); len(got) != 255 {
		t.Fatalf("len %d", len(got))
	}
}

func TestAddressLabels_RejectForeignAddress(t *testing.T) {
	m, _ := labelWallet(t)
	if err := m.SetAddressLabel("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", "x"); err == nil {
		t.Fatal("labelled an address the wallet doesn't own")
	}
}

func TestBIP329_RoundTripsAddressLabels(t *testing.T) {
	m, addr := labelWallet(t)
	if _, err := m.ExportLabelsBIP329(); !errors.Is(err, ErrNoLabels) {
		t.Fatalf("empty export err = %v", err)
	}
	_ = m.SetAddressLabel(addr, "cold storage")
	_ = m.SetTransactionLabel("aa", "rent")
	out, err := m.ExportLabelsBIP329()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"label":"rent","ref":"aa","type":"tx"}` + "\n" +
		`{"label":"cold storage","ref":"` + addr + `","type":"addr"}` + "\n"
	if out != want {
		t.Fatalf("export:\n%s\nwant:\n%s", out, want)
	}

	_ = m.SetAddressLabel(addr, "")
	in := out + `{"type":"addr","ref":"tb1qforeign","label":"skip me"}` + "\n"
	n, err := m.ImportLabelsBIP329(in)
	if err != nil || n != 2 {
		t.Fatalf("imported %d, %v; want 2 (foreign address skipped)", n, err)
	}
	if m.GetAddressLabel(addr) != "cold storage" {
		t.Fatal("address label not imported")
	}
}
