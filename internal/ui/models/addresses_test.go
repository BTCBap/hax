package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
)

func TestGetAddressVisualState(t *testing.T) {
	tests := []struct {
		name     string
		addr     AddrDisplayInfo
		expected addressVisualState
	}{
		{
			name:     "unused address is standard",
			addr:     AddrDisplayInfo{Used: false, Balance: 0},
			expected: addressVisualUnused,
		},
		{
			name:     "used with zero balance is dim",
			addr:     AddrDisplayInfo{Used: true, Balance: 0},
			expected: addressVisualUsedNoBalance,
		},
		{
			name:     "used with balance is highlighted",
			addr:     AddrDisplayInfo{Used: true, Balance: api.Satoshi(1)},
			expected: addressVisualUsedWithBalance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAddressVisualState(tt.addr)
			if got != tt.expected {
				t.Fatalf("getAddressVisualState() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAddresses_VerifyOnDeviceMismatch(t *testing.T) {
	mock := NewMockWalletManager()
	mock.IsHardwareWalletFn = func() bool { return true }
	mock.IsMultisigFn = func() bool { return false }
	mock.DisplayAddrOnDeviceFn = func(addr, path string) error {
		if addr != verifyTestAddress || path != verifyTestPath {
			t.Fatalf("device called with %q %q", addr, path)
		}
		return &wallet.AddressMismatchError{Displayed: verifyTestDevice, Expected: addr}
	}

	m := NewAddressesModel(mock, nil)
	m.width = 80
	m.height = 24
	m.receiveAddresses = []AddrDisplayInfo{{
		Address: verifyTestAddress,
		Path:    verifyTestPath,
		Index:   0,
	}}
	m.updateTable()

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	waiting := model.(AddressesModel)
	if !waiting.verify.IsWaiting() {
		t.Fatal("v should start device verification")
	}
	if waiting.verify.Address != verifyTestAddress || waiting.verify.Path != verifyTestPath {
		t.Fatalf("verifying %q at %q", waiting.verify.Address, waiting.verify.Path)
	}

	result, ok := findMsgOfType[HWIVerifyMsg](collectMsgs(cmd))
	if !ok {
		t.Fatal("verification command did not produce HWIVerifyMsg")
	}
	model, _ = waiting.Update(result)
	updated := model.(AddressesModel)
	if updated.verify.State != AddressVerifyMismatch {
		t.Fatalf("state = %d, want mismatch", updated.verify.State)
	}
	if !strings.Contains(stripANSI(updated.View()), "✗ Device shows a different address") {
		t.Fatalf("view does not show mismatch:\n%s", stripANSI(updated.View()))
	}
}

func TestRenderAddressCell_ColorModes(t *testing.T) {
	address := "tb1qexampleaddress0000000000000000000000"

	unused := renderAddressCell(AddrDisplayInfo{Used: false, Balance: 0}, address)
	usedNoBalance := renderAddressCell(AddrDisplayInfo{Used: true, Balance: 0}, address)
	usedWithBalance := renderAddressCell(AddrDisplayInfo{Used: true, Balance: api.Satoshi(1000)}, address)

	if !strings.Contains(unused, address) || !strings.Contains(usedNoBalance, address) || !strings.Contains(usedWithBalance, address) {
		t.Fatal("expected rendered strings to contain original address text")
	}
}
