package wallet

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestDeriveAddressFromXpub tests address derivation from xpub for all address types
// This validates the hardware wallet address derivation logic
func TestDeriveAddressFromXpub(t *testing.T) {
	// Test xpub at BIP84 account level (m/84'/1'/0')
	// Generated from: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	bip84Xpub := "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M"

	tests := []struct {
		name       string
		xpub       string
		change     uint32
		index      uint32
		addrType   AddressType
		network    *chaincfg.Params
		wantPrefix string // Verify address format, not exact value
		wantError  bool
	}{
		// Native SegWit (BIP84)
		{
			name:       "BIP84 testnet receive index 0",
			xpub:       bip84Xpub,
			change:     0,
			index:      0,
			addrType:   AddressTypeSegWitNative,
			network:    &TestNet4Params,
			wantPrefix: "tb1q",
			wantError:  false,
		},
		{
			name:       "BIP84 testnet receive index 1",
			xpub:       bip84Xpub,
			change:     0,
			index:      1,
			addrType:   AddressTypeSegWitNative,
			network:    &TestNet4Params,
			wantPrefix: "tb1q",
			wantError:  false,
		},
		{
			name:       "BIP84 testnet change index 0",
			xpub:       bip84Xpub,
			change:     1,
			index:      0,
			addrType:   AddressTypeSegWitNative,
			network:    &TestNet4Params,
			wantPrefix: "tb1q",
			wantError:  false,
		},
		// Taproot (BIP86)
		{
			name:       "BIP86 testnet receive index 0",
			xpub:       bip84Xpub, // Same xpub, different address type
			change:     0,
			index:      0,
			addrType:   AddressTypeTaproot,
			network:    &TestNet4Params,
			wantPrefix: "tb1p",
			wantError:  false,
		},
		// Nested SegWit (BIP49)
		{
			name:       "BIP49 testnet receive index 0",
			xpub:       bip84Xpub,
			change:     0,
			index:      0,
			addrType:   AddressTypeSegWitNested,
			network:    &TestNet4Params,
			wantPrefix: "2", // P2SH on testnet starts with 2
			wantError:  false,
		},
		// Legacy (BIP44)
		{
			name:       "BIP44 testnet receive index 0",
			xpub:       bip84Xpub,
			change:     0,
			index:      0,
			addrType:   AddressTypeLegacy,
			network:    &TestNet4Params,
			wantPrefix: "m", // or "n" - testnet P2PKH
			wantError:  false,
		},
		// Invalid xpub
		{
			name:       "Invalid xpub",
			xpub:       "invalid_xpub",
			change:     0,
			index:      0,
			addrType:   AddressTypeSegWitNative,
			network:    &TestNet4Params,
			wantPrefix: "",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := DeriveAddressFromXpub(tt.xpub, tt.change, tt.index, tt.addrType, tt.network)

			if tt.wantError {
				if err == nil {
					t.Errorf("DeriveAddressFromXpub() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("DeriveAddressFromXpub() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// Verify address has expected format
			if tt.addrType == AddressTypeLegacy {
				// Legacy testnet addresses start with m or n
				if addr[0] != 'm' && addr[0] != 'n' {
					t.Errorf("DeriveAddressFromXpub() = %v, want prefix m or n", addr)
				}
			} else {
				if !strings.HasPrefix(addr, tt.wantPrefix) {
					t.Errorf("DeriveAddressFromXpub() = %v, want prefix %v", addr, tt.wantPrefix)
				}
			}
		})
	}
}

// TestDeriveAddressConsistency verifies that the same inputs produce the same outputs
func TestDeriveAddressConsistency(t *testing.T) {
	xpub := "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M"
	network := &TestNet4Params

	// Derive same address twice - should be identical
	addr1, err := DeriveAddressFromXpub(xpub, 0, 0, AddressTypeSegWitNative, network)
	if err != nil {
		t.Fatalf("First derivation failed: %v", err)
	}

	addr2, err := DeriveAddressFromXpub(xpub, 0, 0, AddressTypeSegWitNative, network)
	if err != nil {
		t.Fatalf("Second derivation failed: %v", err)
	}

	if addr1 != addr2 {
		t.Errorf("Address derivation not consistent: %s != %s", addr1, addr2)
	}

	// Different index should produce different address
	addr3, err := DeriveAddressFromXpub(xpub, 0, 1, AddressTypeSegWitNative, network)
	if err != nil {
		t.Fatalf("Third derivation failed: %v", err)
	}

	if addr1 == addr3 {
		t.Errorf("Different indexes produced same address: %s", addr1)
	}

	// Different change should produce different address
	addr4, err := DeriveAddressFromXpub(xpub, 1, 0, AddressTypeSegWitNative, network)
	if err != nil {
		t.Fatalf("Fourth derivation failed: %v", err)
	}

	if addr1 == addr4 {
		t.Errorf("Different change values produced same address: %s", addr1)
	}
}

// TestDeriveChildPubKey tests public key derivation from xpub
func TestDeriveChildPubKey(t *testing.T) {
	// Test that we can derive public keys from an xpub
	xpub := "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M"

	// Derive receive address 0
	pubKey, err := DeriveChildPubKey(xpub, 0, 0, &TestNet4Params)
	if err != nil {
		t.Fatalf("DeriveChildPubKey() error = %v", err)
	}

	// Public key should be 33 bytes (compressed)
	if len(pubKey) != 33 {
		t.Errorf("DeriveChildPubKey() returned pubkey of length %d, want 33", len(pubKey))
	}

	// First byte should be 0x02 or 0x03 (compressed pubkey prefix)
	if pubKey[0] != 0x02 && pubKey[0] != 0x03 {
		t.Errorf("DeriveChildPubKey() returned pubkey with invalid prefix %x", pubKey[0])
	}

	// Test error case
	_, err = DeriveChildPubKey("invalid_xpub", 0, 0, &TestNet4Params)
	if err == nil {
		t.Error("DeriveChildPubKey() expected error for invalid xpub")
	}
}

// TestHWIPlatformSteps_Darwin verifies macOS installation steps
func TestHWIPlatformSteps_Darwin(t *testing.T) {
	platform, steps, note := hwiPlatformStepsForOS("darwin")

	if platform != "macOS" {
		t.Errorf("expected platform 'macOS', got %q", platform)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps for macOS, got %d", len(steps))
	}
	if !strings.Contains(steps[0].Command, "brew install libusb") {
		t.Errorf("step 1 should install libusb via brew, got %q", steps[0].Command)
	}
	if !strings.Contains(steps[1].Command, "pipx") {
		t.Errorf("step 2 should install pipx, got %q", steps[1].Command)
	}
	if !strings.Contains(steps[2].Command, "ensurepath") {
		t.Errorf("step 3 should run pipx ensurepath, got %q", steps[2].Command)
	}
	if steps[3].Command != "pipx install hwi" {
		t.Errorf("step 4 should install hwi, got %q", steps[3].Command)
	}
	if !strings.Contains(note, "Restart") {
		t.Errorf("macOS note should mention restarting terminal, got %q", note)
	}
}

// TestHWIPlatformSteps_Linux verifies Linux installation steps
func TestHWIPlatformSteps_Linux(t *testing.T) {
	platform, steps, note := hwiPlatformStepsForOS("linux")

	if platform != "Linux" {
		t.Errorf("expected platform 'Linux', got %q", platform)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps for Linux, got %d", len(steps))
	}
	if !strings.Contains(steps[0].Command, "apt install") {
		t.Errorf("step 1 should use apt, got %q", steps[0].Command)
	}
	if !strings.Contains(steps[0].Command, "libusb") {
		t.Errorf("step 1 should install libusb dev package, got %q", steps[0].Command)
	}
	if !strings.Contains(steps[0].Command, "libudev") {
		t.Errorf("step 1 should install libudev, got %q", steps[0].Command)
	}
	if steps[2].Command != "pipx install hwi" {
		t.Errorf("step 3 should install hwi, got %q", steps[2].Command)
	}
	if !strings.Contains(steps[3].Command, "installudevrules") {
		t.Errorf("step 4 should set up udev rules, got %q", steps[3].Command)
	}
	if !strings.Contains(note, "Fedora") {
		t.Errorf("Linux note should mention Fedora/Arch alternatives, got %q", note)
	}
}

// TestHWIPlatformSteps_Unknown verifies fallback for unsupported platforms
func TestHWIPlatformSteps_Unknown(t *testing.T) {
	platform, steps, note := hwiPlatformStepsForOS("windows")

	if platform != "windows" {
		t.Errorf("expected platform 'windows', got %q", platform)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 fallback step, got %d", len(steps))
	}
	if steps[0].Command != "pip install hwi" {
		t.Errorf("fallback should use pip, got %q", steps[0].Command)
	}
	if !strings.Contains(note, "github.com") {
		t.Errorf("fallback note should link to HWI repo, got %q", note)
	}
}

// TestHWIPlatformSteps_AllHaveDescriptions verifies no empty fields across all platforms
func TestHWIPlatformSteps_AllHaveDescriptions(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "freebsd"} {
		_, steps, _ := hwiPlatformStepsForOS(goos)
		for i, step := range steps {
			if step.Command == "" {
				t.Errorf("%s step %d: empty command", goos, i+1)
			}
			if step.Description == "" {
				t.Errorf("%s step %d: empty description", goos, i+1)
			}
		}
	}
}

// TestHWIInstallInstructions verifies the formatted instruction string
func TestHWIInstallInstructions(t *testing.T) {
	instructions := HWIInstallInstructions()

	if !strings.Contains(instructions, "not installed") {
		t.Error("instructions should say 'not installed'")
	}
	if !strings.Contains(instructions, "github.com/bitcoin-core/HWI") {
		t.Error("instructions should link to HWI repo")
	}
	if !strings.Contains(instructions, "1.") {
		t.Error("instructions should contain numbered steps")
	}
}

// TestHardwareWalletAddressTypes tests all address types for hardware wallet derivation
func TestHardwareWalletAddressTypes(t *testing.T) {
	// Test that all address types produce valid addresses from the same derivation
	xpub := "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M"
	network := &TestNet4Params

	addrTypes := []struct {
		addrType AddressType
		prefix   string
	}{
		{AddressTypeLegacy, "m"},          // Legacy starts with m or n on testnet
		{AddressTypeSegWitNested, "2"},    // Nested SegWit starts with 2 on testnet
		{AddressTypeSegWitNative, "tb1q"}, // Native SegWit starts with tb1q
		{AddressTypeTaproot, "tb1p"},      // Taproot starts with tb1p
	}

	for _, tt := range addrTypes {
		t.Run(tt.prefix, func(t *testing.T) {
			addr, err := DeriveAddressFromXpub(xpub, 0, 0, tt.addrType, network)
			if err != nil {
				t.Fatalf("DeriveAddressFromXpub() error = %v", err)
			}

			// Check address has expected prefix
			hasPrefix := false
			if tt.addrType == AddressTypeLegacy {
				// Legacy testnet addresses start with m or n
				hasPrefix = addr[0] == 'm' || addr[0] == 'n'
			} else {
				hasPrefix = len(addr) >= len(tt.prefix) && addr[:len(tt.prefix)] == tt.prefix
			}

			if !hasPrefix {
				t.Errorf("Address %s doesn't have expected prefix for type %d", addr, tt.addrType)
			}
		})
	}
}

func TestClassifyHWIError(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    HWIErrorCode
		msgPart string
	}{
		{
			name:    "no device",
			raw:     "No device found",
			want:    HWIErrNoDevice,
			msgPart: "No device connected",
		},
		{
			name:    "locked device",
			raw:     "Device locked, PIN needed",
			want:    HWIErrDeviceLocked,
			msgPart: "Device is locked",
		},
		{
			name:    "cancelled",
			raw:     "Action cancelled by user",
			want:    HWIErrUserCancelled,
			msgPart: "cancelled",
		},
		{
			name:    "unknown multisig wallet",
			raw:     "Unknown multisig wallet",
			want:    HWIErrUnknownMultisigWallet,
			msgPart: "unknown multisig wallet",
		},
		{
			name:    "ledger policy not registered",
			raw:     "Wallet policy not registered",
			want:    HWIErrUnknownMultisigWallet,
			msgPart: "unknown multisig wallet",
		},
		{
			name:    "generic signing failure",
			raw:     "signing failed with status error",
			want:    HWIErrSigningFailed,
			msgPart: "Signing failed",
		},
		{
			name:    "fallback device error",
			raw:     "unexpected transport error",
			want:    HWIErrDeviceError,
			msgPart: "Device error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyHWIError(tt.raw)
			if err.Code != tt.want {
				t.Fatalf("classifyHWIError(%q) code=%v, want %v", tt.raw, err.Code, tt.want)
			}
			if !strings.Contains(strings.ToLower(err.UserMessage()), strings.ToLower(tt.msgPart)) {
				t.Fatalf("UserMessage()=%q, expected to contain %q", err.UserMessage(), tt.msgPart)
			}
		})
	}
}

func TestHWIErrorHints_UnknownMultisigWallet(t *testing.T) {
	err := &HWIError{Code: HWIErrUnknownMultisigWallet, RawMsg: "Unknown multisig wallet"}

	hints := err.TroubleshootingHints()
	if len(hints) == 0 {
		t.Fatal("expected troubleshooting hints for unknown multisig wallet")
	}

	joined := strings.Join(hints, " ")
	if !strings.Contains(joined, "Allow from PSBT") {
		t.Fatalf("expected Coldcard remediation hint, got: %q", joined)
	}
	if !strings.Contains(joined, "PSBT-2-of-3") {
		t.Fatalf("expected naming behavior hint, got: %q", joined)
	}
}
