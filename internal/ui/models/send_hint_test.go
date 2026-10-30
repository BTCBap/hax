package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

type sendHintCase struct {
	name      string
	build     func(t *testing.T) SendModel
	primary   string
	following string
	rest      []string
}

// sendHintCases returns each send step with its primary hint and the hint
// item that follows it in the row.
func sendHintCases() []sendHintCase {
	return []sendHintCase{
		{
			name: "address",
			build: func(t *testing.T) SendModel {
				m := newTestSend(t)
				m.width, m.height = 80, 24
				return m
			},
			primary:   "enter continue",
			following: "i import PSBT",
			rest:      []string{"i import PSBT", "ctrl+v paste", "esc cancel"},
		},
		{
			name: "amount",
			build: func(t *testing.T) SendModel {
				m := newTestSend(t)
				advanceToAmount(&m)
				m.width, m.height = 80, 24
				return m
			},
			primary:   "enter continue",
			following: "m send all",
			rest:      []string{"m send all", "tab unit", "esc back"},
		},
		{
			name: "recipients",
			build: func(t *testing.T) SendModel {
				m := newTestSend(t)
				advanceToRecipientList(&m)
				m.width, m.height = 80, 24
				return m
			},
			primary:   "enter continue",
			following: "↑/↓ navigate",
			rest:      []string{"↑/↓ navigate", "a/+ add recipient", "d delete", "esc edit last"},
		},
		{
			name: "utxos",
			build: func(t *testing.T) SendModel {
				m := newTestSend(t)
				advanceToUTXOSelection(&m)
				m.width, m.height = 80, 24
				return m
			},
			primary:   "tab continue",
			following: "↑/↓ navigate",
			rest:      []string{"↑/↓ navigate", "space/enter toggle", "a all", "n none", "esc back"},
		},
		{
			name: "fee",
			build: func(t *testing.T) SendModel {
				m := newTestSend(t)
				advanceToFee(&m)
				m.feeEstimates = testFeeEstimates()
				m.estimatedSize = 141
				m.width, m.height = 80, 24
				return m
			},
			primary:   "enter continue",
			following: "↑/↓ navigate",
			rest:      []string{"↑/↓ navigate", "c custom", "esc back"},
		},
		{
			name: "review",
			build: func(t *testing.T) SendModel {
				m := newTestSend(t)
				m.walletManager = NewMockWalletManager()
				advanceToReview(&m, &wallet.PSBTResult{Fee: 100})
				m.width, m.height = 80, 24
				m.syncScrollViewport()
				return m
			},
			primary:   "enter confirm & broadcast",
			following: "v flow",
			rest:      []string{"v flow", "esc change fee"},
		},
	}
}

func TestSendHintMorphTextAndAlignment(t *testing.T) {
	var wantFollow viewPosition
	for i, tc := range sendHintCases() {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.build(t).View()
			plain := stripANSI(view)
			primary := requireTextPosition(t, view, tc.primary)
			following := requireTextPosition(t, view, tc.following)

			if primary.line != following.line {
				t.Fatalf("primary hint is on line %d, following item on line %d",
					primary.line, following.line)
			}
			if i == 0 {
				wantFollow = following
			} else if following.col != wantFollow.col {
				t.Fatalf("following hint starts at x=%d, want %d:\n%s",
					following.col, wantFollow.col, plain)
			}

			pad := sendPrimaryHintWidth - lipgloss.Width(tc.primary)
			if pad < 0 {
				pad = 0
			}
			wantLine := tc.primary + strings.Repeat(" ", pad) + " • " + strings.Join(tc.rest, " • ")
			if got := strings.TrimRight(strings.Split(plain, "\n")[primary.line], " "); got != wantLine {
				t.Fatalf("hint row = %q, want %q", got, wantLine)
			}
		})
	}
}

func TestSendHintMorphFinalUsesWarningStyle(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	cases := sendHintCases()
	review := cases[len(cases)-1].build(t).View()
	if !strings.Contains(review, styles.WarningText.Render("enter confirm & broadcast")) {
		t.Fatalf("final hint is not warning-colored and bold:\n%q", review)
	}

	address := cases[0].build(t).View()
	if strings.Contains(address, styles.WarningText.Render("enter continue")) {
		t.Fatalf("continue hint should not use the warning style:\n%q", address)
	}
}

// Pressing enter on the review step still opens the signing screen; the
// confirm label reflects the flow, it does not add a shortcut.
func TestSendReviewEnterStillOpensSigning(t *testing.T) {
	m := newTestSend(t)
	mock := NewMockWalletManager()
	mock.ExportPSBTBase64Fn = func(_ *wallet.PSBTResult) (string, error) {
		return "preview_psbt_b64", nil
	}
	m.walletManager = mock
	m.step = SendStepReview
	m.psbtResult = &wallet.PSBTResult{Fee: 100}
	m.width, m.height = 80, 24

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := asSendModel(t, model)
	if cmd == nil || !updated.loading {
		t.Fatal("enter should still export the previewed PSBT")
	}
}
