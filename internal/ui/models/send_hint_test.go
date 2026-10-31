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
				// 70 columns keeps the final hint item on a wrapped line.
				m.width, m.height = 70, 24
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
			primary:   "enter confirm & sign",
			following: "v flow",
			rest:      []string{"v flow", "esc change fee"},
		},
	}
}

func TestSendHintMorphTextAndSeparators(t *testing.T) {
	for _, tc := range sendHintCases() {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.build(t).View()
			plain := stripANSI(view)
			primary := requireTextPosition(t, view, tc.primary)
			following := requireTextPosition(t, view, tc.following)

			if primary.line != following.line {
				t.Fatalf("primary hint is on line %d, following item on line %d",
					primary.line, following.line)
			}

			wantLine := tc.primary + " • " + strings.Join(tc.rest, " • ")
			// The coins step wraps its last hint at 70 columns.
			wantNextLine := ""
			if tc.name == "utxos" {
				wantLine = strings.TrimSuffix(wantLine, " • esc back")
				wantNextLine = "esc back"
			}
			lines := strings.Split(plain, "\n")
			got := strings.TrimRight(lines[primary.line], " ")
			if got != wantLine {
				t.Fatalf("hint row = %q, want %q", got, wantLine)
			}
			if strings.Contains(got, "  • ") {
				t.Fatalf("hint row has extra spaces before a separator: %q", got)
			}
			if wantNextLine != "" {
				if got := strings.TrimRight(lines[primary.line+1], " "); got != wantNextLine {
					t.Fatalf("wrapped hint row = %q, want %q", got, wantNextLine)
				}
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
	if !strings.Contains(review, styles.WarningText.Render("enter confirm & sign")) {
		t.Fatalf("final hint is not warning-colored and bold:\n%q", review)
	}

	address := cases[0].build(t).View()
	if strings.Contains(address, styles.WarningText.Render("enter continue")) {
		t.Fatalf("continue hint should not use the warning style:\n%q", address)
	}
}

// The review step starts signing; it does not broadcast straight away.
func TestSendReviewHintSaysSignNotBroadcast(t *testing.T) {
	cases := sendHintCases()
	review := cases[len(cases)-1]
	view := review.build(t).View()
	plain := stripANSI(view)

	pos := requireTextPosition(t, view, "enter confirm & sign")
	lines := strings.Split(plain, "\n")
	hintLine := lines[pos.line]
	const want = "enter confirm & sign"
	if !strings.HasPrefix(hintLine, want) {
		t.Fatalf("review hint = %q, want it to start with %q", hintLine, want)
	}
	primary := strings.TrimRight(strings.SplitN(hintLine, " • ", 2)[0], " ")
	if primary != want {
		t.Fatalf("review hint = %q, want %q", primary, want)
	}

	for _, tc := range cases {
		for _, line := range strings.Split(stripANSI(tc.build(t).View()), "\n") {
			if strings.Contains(line, "enter ") && strings.Contains(line, "broadcast") {
				t.Fatalf("%s: enter hint still says broadcast: %q", tc.name, line)
			}
		}
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
