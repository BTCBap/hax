package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
	"github.com/charmbracelet/x/ansi"
)

const (
	inspectTestTxID = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	inspectTestDest = "tb1pqqqsyqcyq5rqwzqfpg9scrgwpugpzysnzs23v9ccrydpk8qarc0slpgr8c"
)

func updateSend(t *testing.T, m SendModel, msg tea.Msg) SendModel {
	t.Helper()
	model, _ := m.Update(msg)
	return asSendModel(t, model)
}

func TestSigningGate(t *testing.T) {
	m := NewSendModel(NewMockWalletManager(), 0, nil)
	m.inspection = &wallet.PSBTInspection{
		SignBlocked: true,
		Warnings:    []string{"Input 1 has no amount, so the fee can't be checked."},
	}
	if blocked, danger := m.signingGate(); blocked == "" || danger {
		t.Fatalf("blocked gate = (%q, %v)", blocked, danger)
	}

	m.inspection = &wallet.PSBTInspection{SignDangerous: true}
	if blocked, danger := m.signingGate(); blocked != "" || !danger {
		t.Fatalf("dangerous gate = (%q, %v)", blocked, danger)
	}

	// An external payment is normal in hax's own send flow, but an
	// imported PSBT needs it acknowledged.
	m.inspection = &wallet.PSBTInspection{ExternalOutput: true}
	if _, danger := m.signingGate(); danger {
		t.Fatal("hax-built payment should not need extra confirmation")
	}
	m.psbtImported = true
	if _, danger := m.signingGate(); !danger {
		t.Fatal("imported external payment should need confirmation")
	}

	m.setPSBTResult(&wallet.PSBTResult{Packet: &psbt.Packet{UnsignedTx: wire.NewMsgTx(2)}})
	if m.signConfirmed {
		t.Fatal("a new PSBT must reset the acknowledgement")
	}
}

// TestRenderInspection_WrapsLongAddress: the inspector keeps the whole
// address, so one that cannot fit the box wraps by visible cells with the
// continuation indent and keeps each group's style.
func TestRenderInspection_WrapsLongAddress(t *testing.T) {
	const boxWidth = 40
	m := NewSendModel(NewMockWalletManager(), 0, nil)
	m.inspection = &wallet.PSBTInspection{
		Outputs: []wallet.InspectedOutput{{
			Address: inspectTestDest, Value: 40_000, Owner: wallet.OutputExternal,
		}},
		Fee: 300, FeeKnown: true, FeeRate: 1.95, VSize: 154,
	}

	section := m.renderInspection(boxWidth)
	plain := ansi.Strip(section)
	formatted := ui.FormatAddress(inspectTestDest, 0)

	indent := "  "
	wrapWidth := boxWidth - 4 - len(indent)
	wrapped := wrapAddressCells(styledAddressCells(formatted), wrapWidth)
	if len(wrapped) < 2 {
		t.Fatalf("long address did not wrap: %q", wrapped)
	}
	var rebuilt strings.Builder
	for _, line := range wrapped {
		if w := ansi.StringWidth(line); w > wrapWidth {
			t.Errorf("wrapped line is %d cells wide, max %d: %q", w, wrapWidth, ansi.Strip(line))
		}
		if !strings.Contains(line, "\x1b[") {
			t.Errorf("wrapped line lost its address style: %q", line)
		}
		if !strings.Contains(plain, indent+ansi.Strip(line)) {
			t.Errorf("inspection box is missing wrapped line %q:\n%s", ansi.Strip(line), plain)
		}
		rebuilt.WriteString(ansi.Strip(line))
	}
	if got, want := rebuilt.String(), ansi.Strip(formatted); got != want {
		t.Fatalf("wrapped address = %q, want %q", got, want)
	}
	assertFits(t, section, boxWidth+2)
}

// TestSignScreen_ShowsInspection: the sign screen shows what the PSBT
// does, for a hax-built PSBT and for an imported one, with full
// addresses and the inspector's warnings, inside 80 columns.
func TestSignScreen_ShowsInspection(t *testing.T) {
	mock := NewMockWalletManager()
	calls := 0
	mock.InspectPSBTFn = func(*psbt.Packet) *wallet.PSBTInspection {
		calls++
		return &wallet.PSBTInspection{
			Inputs: []wallet.InspectedInput{{OutPoint: inspectTestTxID + ":1",
				Value: 100_000, ValueKnown: true, Mine: true}},
			Outputs: []wallet.InspectedOutput{
				{Address: inspectTestDest, Value: 40_000, Owner: wallet.OutputExternal},
				{Address: "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", Value: 59_700,
					Owner: wallet.OutputChange},
			},
			Fee: 300, FeeKnown: true, VSize: 154, FeeRate: 1.95, Sent: 40_000, RBF: true,
			Warnings: []string{"Output 3 claims to be this wallet's but isn't: it pays someone else."},
		}
	}
	packet := &psbt.Packet{UnsignedTx: wire.NewMsgTx(2)}
	res := &wallet.PSBTResult{Packet: packet, Warnings: []string{"hw warning"}}

	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"hax-built", PSBTExportMsg{Base64: "cHNidP8=", SigsTotal: 1, Result: res}},
		{"imported", PSBTFileMsg{Operation: "import", Base64: "cHNidP8=", Result: res}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls = 0
			m := NewSendModel(mock, 0, nil)
			m = updateSend(t, m, tea.WindowSizeMsg{Width: 80, Height: 60})
			m = updateSend(t, m, tc.msg)
			if calls != 1 {
				t.Fatalf("InspectPSBT called %d times", calls)
			}
			m.showPSBTExport = true
			_, content, _ := m.buildPSBTExportScrollable()
			plain := ansi.Strip(content)
			for _, want := range []string{"What you're signing", "yours", "payment",
				ansi.Strip(ui.FormatAddress(inspectTestDest, 0)), "change ✓",
				"300 sats · 2 sat/vB", "Replaceable:    yes",
				"Check before signing", "claims to be this wallet's", "hw warning"} {
				if !strings.Contains(plain, want) {
					t.Errorf("missing %q:\n%s", want, plain)
				}
			}
			assertFits(t, content, 80)
		})
	}
}
