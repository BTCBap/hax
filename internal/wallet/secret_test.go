package wallet

import (
	"bytes"
	"testing"
)

func TestWipe(t *testing.T) {
	b := []byte("hunter2hunter2")
	Wipe(b)
	if !bytes.Equal(b, make([]byte, len(b))) {
		t.Fatal("not zeroed")
	}
	Wipe(nil)
}

func TestStorage_KeyRoundTripWithoutPassword(t *testing.T) {
	s, err := NewWalletStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := &WalletData{Name: "w", Network: NetworkTestnet}
	if err := s.SaveWallet("w", data, "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	pw := []byte("correct horse battery")
	got, salt, key, err := s.LoadWalletKey("w", pw)
	if err != nil || got.Name != "w" || len(key) != 32 || len(salt) == 0 {
		t.Fatalf("load: %v", err)
	}
	// Re-save using only the derived key; must still open with the password.
	got.Network = NetworkMainnet
	if err := s.SaveWalletWithKey("w", got, salt, key); err != nil {
		t.Fatal(err)
	}
	again, err := s.LoadWallet("w", "correct horse battery")
	if err != nil || again.Network != NetworkMainnet {
		t.Fatalf("reload: %v", err)
	}
	if _, _, _, err := s.LoadWalletKey("w", []byte("wrong password!!")); err == nil {
		t.Fatal("wrong password accepted")
	}
}

func TestManager_WipeKeyZeroesBuffers(t *testing.T) {
	m := &WalletManager{}
	key, salt := bytes.Repeat([]byte{0xAA}, 32), bytes.Repeat([]byte{0xBB}, 16)
	m.setKey(&WalletKey{Salt: salt, Key: key})
	if !m.hasKey() {
		t.Fatal("expected key")
	}
	m.wipeKey()
	if m.hasKey() || !bytes.Equal(key, make([]byte, 32)) || !bytes.Equal(salt, make([]byte, 16)) {
		t.Fatal("key material not wiped")
	}
}
