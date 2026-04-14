package models

import (
	"strings"
	"testing"

	"hax/internal/api"
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

func TestRenderAddressCell_ColorModes(t *testing.T) {
	address := "tb1qexampleaddress0000000000000000000000"

	unused := renderAddressCell(AddrDisplayInfo{Used: false, Balance: 0}, address)
	usedNoBalance := renderAddressCell(AddrDisplayInfo{Used: true, Balance: 0}, address)
	usedWithBalance := renderAddressCell(AddrDisplayInfo{Used: true, Balance: api.Satoshi(1000)}, address)

	if !strings.Contains(unused, address) || !strings.Contains(usedNoBalance, address) || !strings.Contains(usedWithBalance, address) {
		t.Fatal("expected rendered strings to contain original address text")
	}
}
