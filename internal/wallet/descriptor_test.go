package wallet

import (
	"fmt"
	"testing"
)

// deriveTestXpub generates a valid testnet xpub from the standard test mnemonic
func deriveTestXpub(t *testing.T) string {
	t.Helper()
	testMnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	seed, err := MnemonicToSeed(testMnemonic, "")
	if err != nil {
		t.Fatalf("Failed to create seed: %v", err)
	}
	hdWallet, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("Failed to create HD wallet: %v", err)
	}
	bip48Path := BIP48Path{
		CoinType:   1,
		Account:    0,
		ScriptType: MultisigNativeSegwit,
	}
	xpub, err := hdWallet.GetMultisigAccountXpub(bip48Path)
	if err != nil {
		t.Fatalf("Failed to derive xpub: %v", err)
	}
	return xpub
}

func TestParseMultisigDescriptor(t *testing.T) {
	// Get a valid xpub for testing
	validXpub := deriveTestXpub(t)

	tests := []struct {
		name           string
		descriptor     string
		wantM          int
		wantN          int
		wantScriptType MultisigScriptType
		wantErr        bool
		wantWarnings   bool // expect at least one warning
	}{
		{
			name:           "Native SegWit 2-of-3 with checksum",
			descriptor:     withDescriptorChecksum(fmt.Sprintf("wsh(sortedmulti(2,[73c5da0a/48h/1h/0h/2h]%s,[a1b2c3d4/48h/1h/0h/2h]%s,[deadbeef/48h/1h/0h/2h]%s))", validXpub, validXpub, validXpub)),
			wantM:          2,
			wantN:          3,
			wantScriptType: MultisigNativeSegwit,
			wantErr:        false,
		},
		{
			name:           "Native SegWit 2-of-2 bare xpubs",
			descriptor:     fmt.Sprintf("wsh(sortedmulti(2,%s,%s))", validXpub, validXpub),
			wantM:          2,
			wantN:          2,
			wantScriptType: MultisigNativeSegwit,
			wantErr:        false,
			wantWarnings:   true, // no checksum + bare xpubs
		},
		{
			name:           "Nested SegWit 2-of-3",
			descriptor:     fmt.Sprintf("sh(wsh(sortedmulti(2,[73c5da0a/48h/1h/0h/1h]%s,[a1b2c3d4/48h/1h/0h/1h]%s,[deadbeef/48h/1h/0h/1h]%s)))", validXpub, validXpub, validXpub),
			wantM:          2,
			wantN:          3,
			wantScriptType: MultisigNestedSegwit,
			wantErr:        false,
			wantWarnings:   true, // no checksum
		},
		{
			name:           "Legacy 2-of-3",
			descriptor:     fmt.Sprintf("sh(sortedmulti(2,[73c5da0a/48h/1h/0h/0h]%s,[a1b2c3d4/48h/1h/0h/0h]%s,[deadbeef/48h/1h/0h/0h]%s))", validXpub, validXpub, validXpub),
			wantM:          2,
			wantN:          3,
			wantScriptType: MultisigLegacy,
			wantErr:        false,
			wantWarnings:   true, // no checksum
		},
		{
			name:           "With derivation path suffixes",
			descriptor:     fmt.Sprintf("wsh(sortedmulti(2,[73c5da0a/48h/1h/0h/2h]%s/<0;1>/*,[a1b2c3d4/48h/1h/0h/2h]%s/<0;1>/*))", validXpub, validXpub),
			wantM:          2,
			wantN:          2,
			wantScriptType: MultisigNativeSegwit,
			wantErr:        false,
			wantWarnings:   true, // no checksum
		},
		{
			name:       "Invalid format",
			descriptor: fmt.Sprintf("wpkh([73c5da0a/84h/1h/0h]%s)", validXpub),
			wantErr:    true,
		},
		{
			name:       "M greater than N",
			descriptor: fmt.Sprintf("wsh(sortedmulti(3,%s,%s))", validXpub, validXpub),
			wantErr:    true,
		},
		{
			name:       "Empty descriptor",
			descriptor: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, warnings, err := ParseMultisigDescriptor(tt.descriptor)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMultisigDescriptor() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseMultisigDescriptor() unexpected error: %v", err)
				return
			}

			if config.M != tt.wantM {
				t.Errorf("ParseMultisigDescriptor() M = %d, want %d", config.M, tt.wantM)
			}

			if config.N != tt.wantN {
				t.Errorf("ParseMultisigDescriptor() N = %d, want %d", config.N, tt.wantN)
			}

			if MultisigScriptType(config.ScriptType) != tt.wantScriptType {
				t.Errorf("ParseMultisigDescriptor() ScriptType = %d, want %d", config.ScriptType, tt.wantScriptType)
			}

			if len(config.Cosigners) != tt.wantN {
				t.Errorf("ParseMultisigDescriptor() len(Cosigners) = %d, want %d", len(config.Cosigners), tt.wantN)
			}

			if tt.wantWarnings && len(warnings) == 0 {
				t.Errorf("ParseMultisigDescriptor() expected warnings, got none")
			}
		})
	}
}

func TestSplitDescriptorKeys(t *testing.T) {
	tests := []struct {
		name     string
		inner    string
		expected []string
	}{
		{
			name:     "Simple M and two keys",
			inner:    "2,key1,key2",
			expected: []string{"2", "key1", "key2"},
		},
		{
			name:     "Keys with brackets",
			inner:    "2,[fp/path]xpub1,[fp/path]xpub2",
			expected: []string{"2", "[fp/path]xpub1", "[fp/path]xpub2"},
		},
		{
			name:     "Keys with nested brackets in path",
			inner:    "2,[fp/48h/1h/0h/2h]xpub1,[fp/48h/1h/0h/2h]xpub2",
			expected: []string{"2", "[fp/48h/1h/0h/2h]xpub1", "[fp/48h/1h/0h/2h]xpub2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitDescriptorKeys(tt.inner)
			if len(result) != len(tt.expected) {
				t.Errorf("splitDescriptorKeys() got %d parts, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("splitDescriptorKeys()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParseSingleSigDescriptor(t *testing.T) {
	validXpub := deriveTestXpub(t)

	tests := []struct {
		name         string
		descriptor   string
		wantAddrType AddressType
		wantErr      bool
	}{
		{
			name:         "Taproot descriptor",
			descriptor:   withDescriptorChecksum(fmt.Sprintf("tr([04120286/86h/1h/0h]%s/<0;1>/*)", validXpub)),
			wantAddrType: AddressTypeTaproot,
			wantErr:      false,
		},
		{
			name:         "Native SegWit descriptor",
			descriptor:   fmt.Sprintf("wpkh([04120286/84h/1h/0h]%s)", validXpub),
			wantAddrType: AddressTypeSegWitNative,
			wantErr:      false,
		},
		{
			name:         "Nested SegWit descriptor",
			descriptor:   fmt.Sprintf("sh(wpkh([04120286/49h/1h/0h]%s))", validXpub),
			wantAddrType: AddressTypeSegWitNested,
			wantErr:      false,
		},
		{
			name:         "Legacy descriptor",
			descriptor:   fmt.Sprintf("pkh([04120286/44h/1h/0h]%s)", validXpub),
			wantAddrType: AddressTypeLegacy,
			wantErr:      false,
		},
		{
			name:         "Bare xpub (no key origin)",
			descriptor:   fmt.Sprintf("wpkh(%s)", validXpub),
			wantAddrType: AddressTypeSegWitNative,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDescriptor(tt.descriptor)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDescriptor() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDescriptor() unexpected error: %v", err)
				return
			}

			if result.IsMultisig {
				t.Errorf("ParseDescriptor() expected single-sig, got multisig")
				return
			}

			if result.SingleSigConfig.AddressType != tt.wantAddrType {
				t.Errorf("ParseDescriptor() AddressType = %v, want %v", result.SingleSigConfig.AddressType, tt.wantAddrType)
			}
		})
	}
}

func TestStripDerivationSuffix(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "No suffix",
			key:      "tpubABC123",
			expected: "tpubABC123",
		},
		{
			name:     "Multipath suffix",
			key:      "tpubABC123/<0;1>/*",
			expected: "tpubABC123",
		},
		{
			name:     "Simple receive path",
			key:      "tpubABC123/0/*",
			expected: "tpubABC123",
		},
		{
			name:     "Simple change path",
			key:      "tpubABC123/1/*",
			expected: "tpubABC123",
		},
		{
			name:     "With key origin and multipath",
			key:      "[fp/48h/1h/0h/2h]tpubABC123/<0;1>/*",
			expected: "[fp/48h/1h/0h/2h]tpubABC123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripDerivationSuffix(tt.key)
			if result != tt.expected {
				t.Errorf("stripDerivationSuffix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// withDescriptorChecksum appends the correct BIP380 checksum to a descriptor body.
func withDescriptorChecksum(body string) string {
	return body + "#" + DescriptorChecksum(body)
}

func TestParseDescriptor_RejectsBadChecksum(t *testing.T) {
	body := fmt.Sprintf("wpkh(%s/0/*)", deriveTestXpub(t))
	cases := []struct {
		name string
		in   string
	}{
		{"mismatch", body + "#abcd1234"},
		{"empty", body + "#"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseDescriptor(tc.in); err == nil {
				t.Fatal("expected checksum error")
			}
		})
	}
	if _, err := ParseDescriptor(withDescriptorChecksum(body)); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
}
