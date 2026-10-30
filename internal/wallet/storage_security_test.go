package wallet

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func writeEncryptedWalletFile(t *testing.T, path string, enc EncryptedWalletFile, perm os.FileMode) {
	t.Helper()
	raw, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, perm); err != nil {
		t.Fatal(err)
	}
}

// TestLoadWalletKeyInfoMigratesLegacyPBKDF2File checks that a version 1
// wallet (PBKDF2, no AAD) still opens and is re-encrypted with Argon2id.
func TestLoadWalletKeyInfoMigratesLegacyPBKDF2File(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWalletStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.walletPath("legacy")
	if err != nil {
		t.Fatal(err)
	}

	// Version zero stands in for an old file that predates the field.
	data := &WalletData{Name: "legacy", Network: "testnet"}
	plaintext, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, saltSize)
	for i := range salt {
		salt[i] = byte(i + 1)
	}
	password := []byte("legacy-test-password")
	key := pbkdf2.Key(password, salt, legacyPBKDF2Iter, keySize, sha256.New)
	ciphertext, nonce, err := encrypt(plaintext, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Start with loose permissions to prove the loader tightens them.
	writeEncryptedWalletFile(t, path, EncryptedWalletFile{
		Version: legacyFormatVersion,
		Salt:    salt,
		Nonce:   nonce,
		Data:    ciphertext,
	}, 0o644)

	got, wk, err := s.LoadWalletKeyInfo("legacy", password)
	if err != nil {
		t.Fatalf("legacy wallet did not load: %v", err)
	}
	defer wk.Wipe()
	if got.Name != "legacy" {
		t.Fatalf("name = %q", got.Name)
	}

	var upgraded EncryptedWalletFile
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &upgraded); err != nil {
		t.Fatal(err)
	}
	if upgraded.Version != walletFormatVersion || upgraded.KDF != kdfArgon2id {
		t.Fatalf("file was not migrated: version=%d kdf=%q", upgraded.Version, upgraded.KDF)
	}
	if upgraded.KDFMemory < argon2MemoryKiB || upgraded.KDFTime < argon2Time {
		t.Fatalf("weaker parameters after migration: %+v", upgraded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("wallet mode = %o, want 600", perm)
	}

	// The migrated file must open with the same password.
	reloaded, wk2, err := s.LoadWalletKeyInfo("legacy", password)
	if err != nil {
		t.Fatalf("migrated wallet did not reload: %v", err)
	}
	defer wk2.Wipe()
	if reloaded.Name != "legacy" {
		t.Fatalf("reloaded name = %q", reloaded.Name)
	}
	if _, _, err := s.LoadWalletKeyInfo("legacy", []byte("wrong-test-password")); err == nil {
		t.Fatal("wrong password was accepted")
	}
}

func TestSaveWalletUsesArgon2idAndPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWalletStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := &WalletData{Version: 1, Name: "fresh", Network: "testnet"}
	if err := s.SaveWallet("fresh", data, "fresh-test-password"); err != nil {
		t.Fatal(err)
	}
	path, _ := s.walletPath("fresh")
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var enc EncryptedWalletFile
	if err := json.Unmarshal(raw, &enc); err != nil {
		t.Fatal(err)
	}
	if enc.Version != walletFormatVersion || enc.KDF != kdfArgon2id {
		t.Fatalf("new file format = version %d kdf %q", enc.Version, enc.KDF)
	}
	if enc.KDFMemory != argon2MemoryKiB || enc.KDFTime != argon2Time {
		t.Fatalf("unexpected KDF parameters: %+v", enc)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("wallet mode = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("directory mode = %o, want 700", perm)
	}
}

func TestLoadWalletRejectsInvalidFormat(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWalletStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := s.walletPath("broken")
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	base := EncryptedWalletFile{
		Version:        walletFormatVersion,
		KDF:            kdfArgon2id,
		KDFTime:        argon2Time,
		KDFMemory:      argon2MemoryKiB,
		KDFParallelism: argon2Threads,
		Salt:           salt,
		Nonce:          nonce,
		Data:           []byte("not a real ciphertext"),
	}

	cases := []struct {
		name   string
		mutate func(*EncryptedWalletFile)
	}{
		{"unknown version", func(e *EncryptedWalletFile) { e.Version = 99 }},
		{"short salt", func(e *EncryptedWalletFile) { e.Salt = []byte{1, 2, 3} }},
		{"short nonce", func(e *EncryptedWalletFile) { e.Nonce = []byte{1} }},
		{"unknown kdf", func(e *EncryptedWalletFile) { e.KDF = "scrypt" }},
		{"weak memory", func(e *EncryptedWalletFile) { e.KDFMemory = minArgon2MemoryKiB - 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := base
			tc.mutate(&enc)
			writeEncryptedWalletFile(t, path, enc, 0o600)
			if _, _, err := s.LoadWalletKeyInfo("broken", []byte("irrelevant-test-password")); err == nil {
				t.Fatal("invalid wallet file was accepted")
			}
		})
	}
}

func TestLoadWalletRejectsTamperedKDFMetadata(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWalletStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := &WalletData{Version: 1, Name: "bound", Network: "testnet"}
	password := []byte("bound-test-password")
	if err := s.SaveWallet("bound", data, string(password)); err != nil {
		t.Fatal(err)
	}
	path, _ := s.walletPath("bound")
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var enc EncryptedWalletFile
	if err := json.Unmarshal(raw, &enc); err != nil {
		t.Fatal(err)
	}
	// KDF metadata is authenticated as associated data, so a change that
	// still passes the parameter bounds must make decryption fail.
	enc.KDFTime++
	writeEncryptedWalletFile(t, path, enc, 0o600)
	if _, _, err := s.LoadWalletKeyInfo("bound", password); err == nil {
		t.Fatal("wallet opened after its KDF metadata was changed")
	}
}

func TestWalletFilesLiveInPrivateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wallets")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWalletStorage(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("directory mode = %o, want 700", perm)
	}
}
