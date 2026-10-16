package wallet

import "testing"

func TestTrimHardened(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"48h", "48"},
		{"48H", "48"},
		{"48'", "48"},
		{"48", "48"},
		{"", ""},
		{"0", "0"},
	}
	for _, tt := range tests {
		if got := trimHardened(tt.in); got != tt.want {
			t.Errorf("trimHardened(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsHardened(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"48h", true},
		{"48H", true},
		{"48'", true},
		{"48", false},
		{"0", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isHardened(tt.in); got != tt.want {
			t.Errorf("isHardened(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestKeyOriginToPath(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   string
	}{
		{"BIP48 standard", "0f056943/48h/1h/0h/2h", "m/48'/1'/0'/2'"},
		{"BIP84 single-sig", "0f056943/84h/1h/0h", "m/84'/1'/0'"},
		{"BIP49 nested", "aabbccdd/49h/1h/0h", "m/49'/1'/0'"},
		{"BIP44 legacy", "aabbccdd/44h/0h/0h", "m/44'/0'/0'"},
		{"apostrophe notation", "0f056943/48'/1'/0'/2'", "m/48'/1'/0'/2'"},
		{"mixed notation", "0f056943/48h/1'/0H/2h", "m/48'/1'/0'/2'"},
		{"with unhardened components", "0f056943/48h/1h/0h/2h/0/5", "m/48'/1'/0'/2'/0/5"},
		{"fingerprint only", "0f056943", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keyOriginToPath(tt.origin)
			if got != tt.want {
				t.Errorf("keyOriginToPath(%q) = %q, want %q", tt.origin, got, tt.want)
			}
		})
	}
}

func TestParseAccountPath(t *testing.T) {
	tests := []struct {
		name                            string
		path                            string
		wantPurpose, wantCoin, wantAcct uint32
		wantErr                         bool
	}{
		{"BIP84 testnet account 0", "m/84'/1'/0'", 84, 1, 0, false},
		{"BIP44 mainnet account 0", "m/44'/0'/0'", 44, 0, 0, false},
		{"BIP49 testnet account 3", "m/49'/1'/3'", 49, 1, 3, false},
		{"BIP86 mainnet account 5", "m/86'/0'/5'", 86, 0, 5, false},
		{"h notation", "m/84h/1h/0h", 84, 1, 0, false},
		{"H notation", "m/84H/1H/0H", 84, 1, 0, false},
		{"without m/ prefix", "84'/1'/0'", 84, 1, 0, false},
		{"too few components", "m/84'/1'", 0, 0, 0, true},
		{"empty", "", 0, 0, 0, true},
		{"invalid number", "m/abc/1'/0'", 0, 0, 0, true},
		{"extra components ok", "m/48'/1'/0'/2'", 48, 1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			purpose, coin, acct, err := ParseAccountPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAccountPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if err == nil {
				if purpose != tt.wantPurpose || coin != tt.wantCoin || acct != tt.wantAcct {
					t.Errorf("ParseAccountPath(%q) = (%d, %d, %d), want (%d, %d, %d)",
						tt.path, purpose, coin, acct, tt.wantPurpose, tt.wantCoin, tt.wantAcct)
				}
			}
		})
	}
}

func TestAccountPathForAddressType(t *testing.T) {
	tests := []struct {
		addrType  AddressType
		isTestnet bool
		account   uint32
		want      string
	}{
		{AddressTypeSegWitNative, true, 0, "m/84'/1'/0'"},
		{AddressTypeLegacy, true, 0, "m/44'/1'/0'"},
		{AddressTypeSegWitNested, false, 0, "m/49'/0'/0'"},
		{AddressTypeTaproot, true, 3, "m/86'/1'/3'"},
	}
	for _, tt := range tests {
		got := AccountPathForAddressType(tt.addrType, tt.isTestnet, tt.account)
		if got != tt.want {
			t.Errorf("AccountPathForAddressType(%v, %v, %d) = %q, want %q",
				tt.addrType, tt.isTestnet, tt.account, got, tt.want)
		}
	}
}

func TestBuildFullPath(t *testing.T) {
	tests := []struct {
		accountPath string
		change      uint32
		index       uint32
		want        string
	}{
		{"m/84'/1'/0'", 0, 5, "m/84'/1'/0'/0/5"},
		{"m/44'/0'/3'", 1, 12, "m/44'/0'/3'/1/12"},
		{"m/86'/1'/0'", 0, 0, "m/86'/1'/0'/0/0"},
	}
	for _, tt := range tests {
		got := BuildFullPath(tt.accountPath, tt.change, tt.index)
		if got != tt.want {
			t.Errorf("BuildFullPath(%q, %d, %d) = %q, want %q",
				tt.accountPath, tt.change, tt.index, got, tt.want)
		}
	}
}

func TestValidateAccountPath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"m/84'/1'/0'", false},
		{"m/44'/0'/0'", false},
		{"m/48'/1'/0'/2'", false},
		{"", true},
		{"84'/1'/0'", true},   // Missing m/ prefix
		{"m/84'/1'", true},    // Too few components
		{"m/abc/1'/0'", true}, // Invalid number
	}
	for _, tt := range tests {
		err := ValidateAccountPath(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateAccountPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
		}
	}
}

func TestResolveAccountPath(t *testing.T) {
	tests := []struct {
		name       string
		customPath string
		addrType   AddressType
		isTestnet  bool
		account    uint32
		want       string
	}{
		{"custom path takes priority", "m/84'/0'/5'", AddressTypeSegWitNative, true, 0, "m/84'/0'/5'"},
		{"empty falls back to standard", "", AddressTypeSegWitNative, true, 0, "m/84'/1'/0'"},
		{"empty legacy", "", AddressTypeLegacy, true, 0, "m/44'/1'/0'"},
		{"empty taproot", "", AddressTypeTaproot, true, 0, "m/86'/1'/0'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAccountPath(tt.customPath, tt.addrType, tt.isTestnet, tt.account)
			if got != tt.want {
				t.Errorf("ResolveAccountPath(%q, ...) = %q, want %q", tt.customPath, got, tt.want)
			}
		})
	}
}

func TestGetExtendedPublicKeyAtPath(t *testing.T) {
	// Generate a test wallet from a known seed
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	hdw, err := NewHDWallet(seed, true)
	if err != nil {
		t.Fatal(err)
	}

	// Standard BIP84 path should work
	xpub84, err := hdw.GetExtendedPublicKeyAtPath("m/84'/1'/0'")
	if err != nil {
		t.Fatalf("GetExtendedPublicKeyAtPath BIP84 failed: %v", err)
	}
	if xpub84 == "" {
		t.Error("expected non-empty xpub")
	}

	// Same path via standard method should match
	xpubStd, err := hdw.GetExtendedPublicKey(0, AddressTypeSegWitNative)
	if err != nil {
		t.Fatalf("GetExtendedPublicKey failed: %v", err)
	}
	if xpub84 != xpubStd {
		t.Errorf("xpub mismatch: custom path %q != standard %q", xpub84, xpubStd)
	}

	// Different account should produce different xpub
	xpubAcct3, err := hdw.GetExtendedPublicKeyAtPath("m/84'/1'/3'")
	if err != nil {
		t.Fatalf("GetExtendedPublicKeyAtPath account 3 failed: %v", err)
	}
	if xpubAcct3 == xpub84 {
		t.Error("different account paths should produce different xpubs")
	}

	// BIP44 path
	xpub44, err := hdw.GetExtendedPublicKeyAtPath("m/44'/1'/0'")
	if err != nil {
		t.Fatalf("GetExtendedPublicKeyAtPath BIP44 failed: %v", err)
	}
	xpub44Std, err := hdw.GetExtendedPublicKey(0, AddressTypeLegacy)
	if err != nil {
		t.Fatalf("GetExtendedPublicKey legacy failed: %v", err)
	}
	if xpub44 != xpub44Std {
		t.Errorf("BIP44 xpub mismatch: custom path %q != standard %q", xpub44, xpub44Std)
	}
}

func TestResolvePathPrefix(t *testing.T) {
	tests := []struct {
		name    string
		account AccountData
		wallet  WalletData
		want    string
	}{
		{
			"multisig always returns m/48'",
			AccountData{AddressType: "segwit_native"},
			WalletData{Multisig: &MultisigWalletConfig{}},
			"m/48'",
		},
		{
			"custom path takes priority",
			AccountData{AddressType: "segwit_native", CustomDerivationPath: "m/84'/0'/5'"},
			WalletData{},
			"m/84'/0'/5'",
		},
		{
			"hardware wallet uses KeyOrigin",
			AccountData{AddressType: "segwit_native"},
			WalletData{HardwareWallet: &HardwareWalletConfig{KeyOrigin: "abcd1234/84h/1h/0h"}},
			"m/84'/1'/0'",
		},
		{
			"standard segwit_native",
			AccountData{AddressType: "segwit_native"},
			WalletData{},
			"m/84'",
		},
		{
			"standard legacy",
			AccountData{AddressType: "legacy"},
			WalletData{},
			"m/44'",
		},
		{
			"standard taproot",
			AccountData{AddressType: "taproot"},
			WalletData{},
			"m/86'",
		},
		{
			"standard nested segwit",
			AccountData{AddressType: "segwit_nested"},
			WalletData{},
			"m/49'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePathPrefix(&tt.account, &tt.wallet)
			if got != tt.want {
				t.Errorf("resolvePathPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCosignerDerivationPath(t *testing.T) {
	tests := []struct {
		name       string
		cs         MultisigCosignerConfig
		coinType   uint32
		scriptType MultisigScriptType
		want       string
	}{
		{
			name:       "ActualDerivation takes priority",
			cs:         MultisigCosignerConfig{ActualDerivation: "m/84'/1'/0'", KeyOrigin: "fp/48h/1h/0h/2h"},
			coinType:   1,
			scriptType: MultisigNativeSegwit,
			want:       "m/84'/1'/0'",
		},
		{
			name:       "KeyOrigin fallback",
			cs:         MultisigCosignerConfig{KeyOrigin: "0f056943/48h/1h/0h/2h"},
			coinType:   1,
			scriptType: MultisigNativeSegwit,
			want:       "m/48'/1'/0'/2'",
		},
		{
			name:       "KeyOrigin with non-BIP48 path",
			cs:         MultisigCosignerConfig{KeyOrigin: "0f056943/84h/1h/0h"},
			coinType:   1,
			scriptType: MultisigNativeSegwit,
			want:       "m/84'/1'/0'",
		},
		{
			name:       "BIP48 default when nothing set",
			cs:         MultisigCosignerConfig{},
			coinType:   1,
			scriptType: MultisigNativeSegwit,
			want:       "m/48'/1'/0'/2'",
		},
		{
			name:       "BIP48 default respects scriptType",
			cs:         MultisigCosignerConfig{},
			coinType:   1,
			scriptType: MultisigLegacy,
			want:       "m/48'/1'/0'/0'",
		},
		{
			name:       "BIP48 default respects coinType",
			cs:         MultisigCosignerConfig{},
			coinType:   0,
			scriptType: MultisigNativeSegwit,
			want:       "m/48'/0'/0'/2'",
		},
		{
			name:       "fingerprint-only KeyOrigin falls through to default",
			cs:         MultisigCosignerConfig{KeyOrigin: "0f056943"},
			coinType:   1,
			scriptType: MultisigNativeSegwit,
			want:       "m/48'/1'/0'/2'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosignerDerivationPath(tt.cs, tt.coinType, tt.scriptType)
			if got != tt.want {
				t.Errorf("cosignerDerivationPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultMultisigAccountPath(t *testing.T) {
	tests := []struct {
		name       string
		scriptType MultisigScriptType
		isTestnet  bool
		want       string
	}{
		{"Legacy P2SH uses BIP45", MultisigLegacy, true, "m/45'"},
		{"Legacy P2SH mainnet uses BIP45", MultisigLegacy, false, "m/45'"},
		{"Nested SegWit testnet", MultisigNestedSegwit, true, "m/48'/1'/0'/1'"},
		{"Nested SegWit mainnet", MultisigNestedSegwit, false, "m/48'/0'/0'/1'"},
		{"Native SegWit testnet", MultisigNativeSegwit, true, "m/48'/1'/0'/2'"},
		{"Native SegWit mainnet", MultisigNativeSegwit, false, "m/48'/0'/0'/2'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultMultisigAccountPath(tt.scriptType, tt.isTestnet)
			if got != tt.want {
				t.Errorf("DefaultMultisigAccountPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
