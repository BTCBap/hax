package wallet

import "testing"

const passphraseTestMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestHasPassphraseRoundTrip(t *testing.T) {
	const password = "correct horse battery"

	cases := []struct {
		name       string
		passphrase string
		want       bool
		restore    bool
	}{
		{name: "created with passphrase", passphrase: "my passphrase", want: true},
		{name: "created without passphrase", passphrase: "", want: false},
		{name: "restored with passphrase", passphrase: "my passphrase", want: true, restore: true},
		{name: "restored without passphrase", passphrase: "", want: false, restore: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewWalletManager(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.restore {
				err = m.RestoreWalletOnNetwork("wallet", password, passphraseTestMnemonic, tc.passphrase, NetworkTestnet)
			} else {
				_, err = m.CreateWalletOnNetwork("wallet", password, 12, NetworkTestnet, tc.passphrase)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := m.UnlockWallet("wallet", password); err != nil {
				t.Fatal(err)
			}
			if got := m.HasPassphrase(); got != tc.want {
				t.Fatalf("HasPassphrase after unlock = %v, want %v", got, tc.want)
			}

			// Locking drops the loaded data, so the second unlock proves the
			// answer survives a reload from the encrypted file.
			m.LockWallet()
			if m.HasPassphrase() {
				t.Fatal("locked wallet should not report a passphrase")
			}
			if err := m.UnlockWallet("wallet", password); err != nil {
				t.Fatal(err)
			}
			if got := m.HasPassphrase(); got != tc.want {
				t.Fatalf("HasPassphrase after reload = %v, want %v", got, tc.want)
			}
		})
	}
}
