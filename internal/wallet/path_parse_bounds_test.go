package wallet

import "testing"

func TestParseBip32Path_RejectsHardenedOverflow(t *testing.T) {
	if _, err := parseBip32Path("m/84'/1'/2147483648'"); err == nil {
		t.Fatal("hardened index >= 2^31 must be rejected, not wrapped")
	}
	got, err := parseBip32Path("m/84'/1'/2147483647'")
	if err != nil || got[2] != 0xFFFFFFFF {
		t.Fatalf("max hardened index: %v %x", err, got)
	}
}

func TestParseMultisigPath_RejectsNegativeIndex(t *testing.T) {
	if _, _, err := parseMultisigPath("m/48'/1'/0'/2'/0/-1"); err == nil {
		t.Fatal("negative index must be rejected")
	}
	change, idx, err := parseMultisigPath("m/48'/1'/0'/2'/1/7")
	if err != nil || !change || idx != 7 {
		t.Fatalf("change=%v idx=%d err=%v", change, idx, err)
	}
}
