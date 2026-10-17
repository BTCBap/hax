package models

import (
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/wallet"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
	tea "github.com/charmbracelet/bubbletea"
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
				inspectTestDest, "change ✓", "300 sats · 2 sat/vB", "Replaceable:    yes",
				"Check before signing", "claims to be this wallet's", "hw warning"} {
				if !strings.Contains(plain, want) {
					t.Errorf("missing %q:\n%s", want, plain)
				}
			}
			assertFits(t, content, 80)
		})
	}
}
