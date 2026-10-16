package wallet

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-smith/go-bip39"
)

// Official BIP39 vectors (trezor/python-mnemonic vectors.json), passphrase "TREZOR".
var bip39Vectors = []struct{ entropy, mnemonic, seed string }{
	{"00000000000000000000000000000000",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		"c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"},
	{"7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f",
		"legal winner thank year wave sausage worth useful legal winner thank yellow",
		"2e8905819b8723fe2c1d161860e5ee1830318dbf49a83bd451cfb8440c28bd6fa457fe1296106559a3c80937a1c1069be3a3a5bd381ee6260e8d9739fce1f607"},
	{"ffffffffffffffffffffffffffffffff",
		"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
		"ac27495480225222079d7be181583751e86f571027b0497b5b5d11218e0a8a13332572917f0f8e5a589620c6f15b11c61dee327651a14c34e18231052e48c069"},
	{"0000000000000000000000000000000000000000000000000000000000000000",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art",
		"bda85446c68413707090a52022edd26a1c9462295029f2e60cd7c4f2bbd3097170af7a4d73245cafa9c3cca8d561a7c3de6f5d4a10be8ed2a5e608d68f92fcc8"},
	{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo vote",
		"dd48c104698c30cfe2b6142103248622fb7bb0ff692eebb00089b32d22484e1613912f0a5b694407be899ffd31ed3992c456cdf60f5d4564b8ba3f05a69890ad"},
}

func TestBIP39_KnownVectors(t *testing.T) {
	for _, v := range bip39Vectors {
		ent, _ := hex.DecodeString(v.entropy)
		m, err := bip39.NewMnemonic(ent)
		if err != nil || m != v.mnemonic {
			t.Fatalf("entropy %s: got %q err %v", v.entropy, m, err)
		}
		if !ValidateMnemonic(m) {
			t.Fatalf("vector mnemonic rejected: %s", m)
		}
		seed, err := MnemonicToSeed(m, "TREZOR")
		if err != nil || hex.EncodeToString(seed) != v.seed {
			t.Fatalf("seed mismatch for %s", v.entropy)
		}
	}
}

func TestGenerateMnemonic_WordCountsAndChecksum(t *testing.T) {
	for strength, words := range map[MnemonicStrength]int{Mnemonic12Words: 12, Mnemonic24Words: 24} {
		seen := map[string]bool{}
		for i := 0; i < 20; i++ {
			m, err := GenerateMnemonic(strength)
			if err != nil {
				t.Fatal(err)
			}
			if n := len(strings.Fields(m)); n != words {
				t.Fatalf("strength %d: %d words", strength, n)
			}
			ent, err := bip39.EntropyFromMnemonic(m)
			if err != nil || len(ent)*8 != int(strength) {
				t.Fatalf("entropy len %d err %v", len(ent)*8, err)
			}
			if seen[m] {
				t.Fatal("duplicate mnemonic: RNG broken")
			}
			seen[m] = true
		}
	}
}

func TestGenerateEntropy_RejectsBadSizes(t *testing.T) {
	for _, bits := range []int{0, 64, 96, 127, 129, 288} {
		if _, err := generateEntropy(bits); err == nil {
			t.Fatalf("bit size %d accepted", bits)
		}
	}
}

func TestValidateMnemonic_RejectsBadChecksum(t *testing.T) {
	bad := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon"
	if ValidateMnemonic(bad) {
		t.Fatal("bad checksum accepted")
	}
	if _, err := MnemonicToSeed(bad, ""); err == nil {
		t.Fatal("seed derived from bad checksum mnemonic")
	}
}

// Guard: no math/rand in non-test code anywhere in the module.
func TestNoMathRandInProductionCode(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // walking our own source tree
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "\"math/rand") {
			t.Errorf("%s imports math/rand", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
