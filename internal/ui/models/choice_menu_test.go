package models

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestChoiceMenusKeepRowsInPlace(t *testing.T) {
	cases := []struct {
		name    string
		step    WalletSetupStep
		options []choiceOption
	}{
		{
			name: "network",
			step: StepChooseNetwork,
			options: []choiceOption{
				{name: "Mainnet", desc: "Bitcoin network."},
				{name: "Testnet4", desc: "Test network; coins have no value."},
			},
		},
		{
			name: "choose action",
			step: StepChooseAction,
			options: []choiceOption{
				{name: "Create New Wallet"},
				{name: "Restore Wallet"},
			},
		},
		{
			name: "wallet type",
			step: StepChooseWalletType,
			options: []choiceOption{
				{name: "Single-Signature Wallet", desc: "Standard wallet with one key - simple and secure"},
				{name: "Multisig Wallet (M-of-N)", desc: "Requires multiple keys to spend - for shared custody"},
			},
		},
		{
			name: "key source",
			step: StepChooseKeySource,
			options: []choiceOption{
				{name: "Generate New Seed", desc: "Create a new 12 or 24 word seed phrase", check: " ✓"},
				{name: "Import from Hardware Wallet", desc: "Use a Trezor, Ledger, Coldcard, etc."},
			},
		},
		{
			name: "script type",
			step: StepChooseScriptType,
			options: []choiceOption{
				{name: "Legacy (P2PKH)", desc: "Original Bitcoin format m/44'/..."},
				{name: "Nested SegWit (P2SH-P2WPKH)", desc: "Compatible with older wallets m/49'/..."},
				{name: "Native SegWit (Bech32)", desc: "Recommended - lowest fees m/84'/...", check: " ✓"},
				{name: "Taproot (P2TR)", desc: "Latest - privacy & efficiency m/86'/..."},
			},
		},
		{
			name: "restore script type",
			step: StepRestoreChooseScriptType,
			options: []choiceOption{
				{name: "Legacy (P2PKH)", desc: "Addresses start with 1 or m/n m/44'/..."},
				{name: "Nested SegWit (P2SH-P2WPKH)", desc: "Addresses start with 3 or 2 m/49'/..."},
				{name: "Native SegWit (Bech32)", desc: "Addresses start with bc1q/tb1q m/84'/...", check: " ✓"},
				{name: "Taproot (P2TR)", desc: "Addresses start with bc1p/tb1p m/86'/..."},
			},
		},
		{
			name: "restore method",
			step: StepRestoreChooseMethod,
			options: []choiceOption{
				{name: "From Seed Phrase", desc: "Single-sig wallet from 12/24 word seed"},
				{name: "Import Multisig (Manual)", desc: "Enter cosigner xpubs one by one"},
				{name: "Import from Descriptor", desc: "Single-sig or multisig from Sparrow/Coldcard"},
				{name: "From Wallet File", desc: "Open existing .wallet file"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			type position struct{ line, col int }
			wantPositions := make(map[string]position, len(tc.options))
			wantLines := 0

			for cursor := range tc.options {
				m := NewWalletSetupModel(&MockWalletManager{}, nil)
				m.width = 100
				m.height = 40
				m.step = tc.step
				m.cursor = cursor

				lines := strings.Split(stripANSI(m.View()), "\n")
				if cursor == 0 {
					wantLines = len(lines)
				} else if len(lines) != wantLines {
					t.Fatalf("cursor %d renders %d lines, cursor 0 renders %d", cursor, len(lines), wantLines)
				}

				for _, opt := range tc.options {
					line, col, ok := findChoiceLine(lines, opt.name)
					if !ok {
						t.Fatalf("cursor %d: option %q not found", cursor, opt.name)
					}
					if cursor == 0 {
						wantPositions[opt.name] = position{line: line, col: col}
					} else if got := wantPositions[opt.name]; got != (position{line: line, col: col}) {
						t.Fatalf("cursor %d: option %q moved from line %d col %d to line %d col %d",
							cursor, opt.name, got.line, got.col, line, col)
					}
				}

				selected := tc.options[cursor]
				if selected.desc == "" {
					continue
				}
				line, _, ok := findChoiceLine(lines, selected.name)
				if !ok {
					t.Fatalf("cursor %d: selected option %q not found", cursor, selected.name)
				}
				if !strings.Contains(lines[line], selected.desc) {
					t.Fatalf("cursor %d: description %q is not on the line of %q:\n%s",
						cursor, selected.desc, selected.name, lines[line])
				}
			}
		})
	}
}

func findChoiceLine(lines []string, text string) (line, col int, ok bool) {
	for i, l := range lines {
		if at := strings.Index(l, text); at >= 0 {
			return i, lipgloss.Width(l[:at]), true
		}
	}
	return 0, 0, false
}
