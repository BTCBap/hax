package wallet

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func readFile(t *testing.T, dir, name string) EncryptedWalletFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name+".wallet")) // #nosec G304 -- test temp dir
	if err != nil {
		t.Fatal(err)
	}
	var f EncryptedWalletFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func writeFile(t *testing.T, dir, name string, f EncryptedWalletFile) {
	t.Helper()
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".wallet"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

// writeLegacyV1 writes a wallet exactly as hax did before Argon2id.
func writeLegacyV1(t *testing.T, dir, name, password string, data *WalletData) {
	t.Helper()
	plain, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, saltSize)
	for i := range salt {
		salt[i] = byte(i)
	}
	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iter, keySize, sha256.New)
	ct, nonce, err := encrypt(plain, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, name, EncryptedWalletFile{Version: 1, Salt: salt, Nonce: nonce, Data: ct})
}

func TestStorage_NewFilesUseArgon2id(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewWalletStorage(dir)
	if err := s.SaveWallet("w", &WalletData{Name: "w"}, "correct horse"); err != nil {
		t.Fatal(err)
	}
	f := readFile(t, dir, "w")
	if f.Version != 2 || f.KDF == nil || *f.KDF != defaultKDF || len(f.Salt) != saltSize {
		t.Fatalf("unexpected header: version=%d kdf=%+v salt=%d", f.Version, f.KDF, len(f.Salt))
	}
	if !f.isCurrent() {
		t.Fatal("fresh file should be current")
	}
}

func TestStorage_UpgradesLegacyPBKDF2File(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewWalletStorage(dir)
	in := &WalletData{Name: "old", Network: NetworkMainnet, Mnemonic: "abandon abandon about"}
	writeLegacyV1(t, dir, "old", "legacy password", in)

	if _, _, _, err := s.LoadWalletKey("old", []byte("wrong")); err == nil {
		t.Fatal("wrong password accepted on v1 file")
	}
	if readFile(t, dir, "old").Version != 1 {
		t.Fatal("failed unlock must not rewrite the file")
	}

	got, salt, key, err := s.LoadWalletKey("old", []byte("legacy password"))
	if err != nil {
		t.Fatalf("load v1: %v", err)
	}
	if got.Mnemonic != in.Mnemonic || got.Network != in.Network {
		t.Fatalf("data mismatch: %+v", got)
	}
	f := readFile(t, dir, "old")
	if !f.isCurrent() {
		t.Fatalf("file not upgraded: version=%d kdf=%+v", f.Version, f.KDF)
	}
	if string(f.Salt) != string(salt) {
		t.Fatal("returned salt does not match upgraded file")
	}
	// The session key must keep working for later saves.
	got.Name = "renamed"
	if err := s.SaveWalletWithKey("old", got, salt, key); err != nil {
		t.Fatal(err)
	}
	again, err := s.LoadWallet("old", "legacy password")
	if err != nil || again.Name != "renamed" || again.Mnemonic != in.Mnemonic {
		t.Fatalf("reload after upgrade: %v %+v", err, again)
	}
}

func TestStorage_HeaderTamperingFailsDecryption(t *testing.T) {
	cases := map[string]func(f *EncryptedWalletFile){
		"lower memory": func(f *EncryptedWalletFile) { f.KDF.Memory = minArgonMemory },
		"lower time":   func(f *EncryptedWalletFile) { f.KDF.Time = 1 },
		"threads":      func(f *EncryptedWalletFile) { f.KDF.Threads = 1 },
		"salt":         func(f *EncryptedWalletFile) { f.Salt[0] ^= 1 },
		"downgrade v1": func(f *EncryptedWalletFile) { f.Version = 1; f.KDF = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s, _ := NewWalletStorage(dir)
			if err := s.SaveWallet("w", &WalletData{Name: "w"}, "pw-pw-pw"); err != nil {
				t.Fatal(err)
			}
			f := readFile(t, dir, "w")
			mutate(&f)
			writeFile(t, dir, "w", f)
			if _, err := s.LoadWallet("w", "pw-pw-pw"); err == nil {
				t.Fatal("tampered header accepted")
			}
		})
	}
}

func TestStorage_RejectsBadHeaders(t *testing.T) {
	ok := defaultKDF
	huge := defaultKDF
	huge.Memory = maxArgonMemory + 1
	weak := defaultKDF
	weak.Memory = minArgonMemory - 1
	zeroT := defaultKDF
	zeroT.Time = 0
	other := defaultKDF
	other.Name = "scrypt"
	salt := make([]byte, saltSize)

	cases := []struct {
		name string
		f    EncryptedWalletFile
		want string
	}{
		{"future version", EncryptedWalletFile{Version: 3, KDF: &ok, Salt: salt}, "newer than this hax"},
		{"version zero", EncryptedWalletFile{Version: 0, Salt: salt}, "unsupported wallet file version"},
		{"missing kdf", EncryptedWalletFile{Version: 2, Salt: salt}, "missing key derivation"},
		{"unknown kdf", EncryptedWalletFile{Version: 2, KDF: &other, Salt: salt}, "unsupported key derivation"},
		{"huge memory", EncryptedWalletFile{Version: 2, KDF: &huge, Salt: salt}, "memory"},
		{"weak memory", EncryptedWalletFile{Version: 2, KDF: &weak, Salt: salt}, "memory"},
		{"zero time", EncryptedWalletFile{Version: 2, KDF: &zeroT, Salt: salt}, "time cost"},
		{"short salt", EncryptedWalletFile{Version: 2, KDF: &ok, Salt: salt[:8]}, "salt length"},
		{"v1 no salt", EncryptedWalletFile{Version: 1}, "no salt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s, _ := NewWalletStorage(dir)
			writeFile(t, dir, "w", tc.f)
			_, err := s.LoadWallet("w", "whatever")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestStorage_BadNonceDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewWalletStorage(dir)
	if err := s.SaveWallet("w", &WalletData{Name: "w"}, "pw-pw-pw"); err != nil {
		t.Fatal(err)
	}
	f := readFile(t, dir, "w")
	f.Nonce = f.Nonce[:4]
	writeFile(t, dir, "w", f)
	if _, err := s.LoadWallet("w", "pw-pw-pw"); err == nil {
		t.Fatal("short nonce accepted")
	}
}
