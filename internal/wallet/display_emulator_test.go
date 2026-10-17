//go:build emulator

package wallet

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmulatorDisplayAddress: the device derives and shows the receive
// address at the path hax passes, and hax confirms it matches.
func TestEmulatorDisplayAddress(t *testing.T) {
	h, dev := emulatorDevice(t)
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			origin := fmt.Sprintf("m/%dh/1h/0h", tt.purpose)
			xpub, err := h.GetXpub(dev, origin)
			if err != nil {
				t.Fatal(err)
			}
			m, err := NewWalletManager(t.TempDir(), newFundedBackend(&TestNet4Params))
			if err != nil {
				t.Fatal(err)
			}
			keyOrigin := signerTestFingerprint + strings.TrimPrefix(origin, "m")
			if err := m.CreateHardwareWalletOnNetwork("hw", "emulator-pw", xpub, signerTestFingerprint,
				keyOrigin, tt.addr, string(dev.Type), NetworkTestnet); err != nil {
				t.Fatal(err)
			}
			if err := m.UnlockWallet("hw", "emulator-pw"); err != nil {
				t.Fatal(err)
			}
			m.StopBackgroundSync()
			t.Cleanup(m.LockWallet)

			for _, i := range []uint32{0, 3} {
				addr, path, err := m.GetAddressByIndex(0, i)
				if err != nil {
					t.Fatal(err)
				}
				if err := m.verifyAddressOnDevice(h, dev, addr, path); err != nil {
					t.Fatalf("index %d (%s at %s): %v", i, addr, path, err)
				}
			}
			// Negative control: a different address for the same path.
			other, _, _ := m.GetAddressByIndex(0, 1)
			_, path, _ := m.GetAddressByIndex(0, 0)
			err = m.verifyAddressOnDevice(h, dev, other, path)
			if err == nil || !strings.Contains(err.Error(), "mismatch") {
				t.Fatalf("wrong address accepted: %v", err)
			}
		})
	}
}
