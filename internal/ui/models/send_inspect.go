package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// Rows shown per list before the rest are summarised.
const (
	inspectMaxInputs  = 5
	inspectMaxOutputs = 10
)

// refreshInspection re-reads the current PSBT. Call it whenever the PSBT
// changes: a new one, an import, or a signature.
func (m *SendModel) refreshInspection() {
	m.inspection = nil
	if m.walletManager == nil || m.psbtResult == nil || m.psbtResult.Packet == nil {
		return
	}
	m.inspection = m.walletManager.InspectPSBT(m.psbtResult.Packet)
}

// openInspectionFlow opens the transaction visualizer for the current
// PSBT inspection. It does nothing until there is something to show.
func (m *SendModel) openInspectionFlow() {
	if m.inspection == nil {
		return
	}
	var labels func(string) string
	if m.walletManager != nil {
		labels = m.walletManager.GetAddressLabel
	}
	m.txFlow = &TxFlowView{
		Flow:   TxFlowFromInspection(m.inspection, labels),
		Width:  m.width,
		Height: m.height,
	}
}

// setPSBTResult replaces the current PSBT and its inspection.
func (m *SendModel) setPSBTResult(r *wallet.PSBTResult) {
	m.psbtResult = r
	m.signConfirmed = false
	if r == nil {
		m.psbtImported = false
	}
	m.refreshInspection()
}

// setImportedPSBTResult replaces the current PSBT with one that came from
// outside the wallet. Ownership checks are still authoritative, but an
// external payment in an imported PSBT needs explicit confirmation because
// the destination wasn't chosen in hax's own send flow.
func (m *SendModel) setImportedPSBTResult(r *wallet.PSBTResult) {
	m.setPSBTResult(r)
	m.psbtImported = r != nil
}

// signingGate reports whether the inspected PSBT may be signed. A non-empty
// blocked message refuses signing; dangerous asks for a second keypress so
// the warnings are acknowledged. Locally built PSBTs carry the destination
// the user already confirmed, so only genuine risks need the extra press.
func (m *SendModel) signingGate() (blocked string, dangerous bool) {
	if m.inspection == nil {
		return "", false
	}
	if m.inspection.SignBlocked {
		msg := "this PSBT can't be signed safely"
		if len(m.inspection.Warnings) > 0 {
			msg += ": " + m.inspection.Warnings[0]
		}
		return msg, false
	}
	if m.inspection.SignDangerous {
		return "", true
	}
	if m.psbtImported && m.inspection.ExternalOutput {
		return "", true
	}
	return "", false
}

// inspectionWarnings are the inspector's warnings plus the ones raised
// while building the PSBT, without duplicates.
func (m SendModel) inspectionWarnings() []string {
	var out []string
	seen := map[string]bool{}
	add := func(ws []string) {
		for _, w := range ws {
			if !seen[w] {
				seen[w] = true
				out = append(out, w)
			}
		}
	}
	if m.inspection != nil {
		add(m.inspection.Warnings)
	}
	if m.psbtResult != nil {
		add(m.psbtResult.Warnings)
	}
	return out
}

// renderInspection shows what the PSBT does, as worked out from the PSBT
// and the wallet's keys, so the user checks it before signing.
func (m SendModel) renderInspection(boxWidth int) string {
	insp := m.inspection
	if insp == nil {
		return ""
	}
	rows := []string{styles.HighlightText.Render("What you're signing:")}

	rows = append(rows, styles.DimText.Render(fmt.Sprintf("Inputs (%d)", len(insp.Inputs))))
	for i, in := range insp.Inputs {
		if i == inspectMaxInputs {
			rows = append(rows, fmt.Sprintf("  … and %d more", len(insp.Inputs)-i))
			break
		}
		who := styles.SuccessText.Render("yours   ")
		if !in.Mine {
			who = styles.WarningText.Render("external")
		}
		amount := styles.WarningText.Render("amount unknown")
		if in.ValueKnown {
			amount = formatBTCWithSats(in.Value)
		}
		line := fmt.Sprintf("  #%-2d %s %s", i+1, who, amount)
		if in.Signed {
			line += " " + styles.DimText.Render("signed")
		}
		rows = append(rows, line, "      "+styles.DimText.Render(shortOutPoint(in.OutPoint)))
	}

	rows = append(rows, styles.DimText.Render(fmt.Sprintf("Outputs (%d)", len(insp.Outputs))))
	for i, out := range insp.Outputs {
		if i == inspectMaxOutputs {
			rows = append(rows, fmt.Sprintf("  … and %d more", len(insp.Outputs)-i))
			break
		}
		var who string
		switch out.Owner {
		case wallet.OutputChange:
			who = styles.SuccessText.Render("change ✓")
		case wallet.OutputReceive:
			who = styles.SuccessText.Render("yours ✓ ")
		default:
			who = styles.HighlightText.Render("payment ")
		}
		addr := out.Address
		if addr == "" {
			rows = append(rows,
				fmt.Sprintf("  #%-2d %s %s", i+1, who, formatBTCWithSats(out.Value)),
				"      "+styles.NormalText.Render("(no address)"))
			continue
		}
		// Full addresses: a shortened one is easy to spoof. Indent less
		// when the longest (P2TR, P2WSH) wouldn't fit the box, then wrap by
		// visible cells because the styled address carries ANSI.
		contentWidth := boxWidth - 4
		formatted := ui.FormatAddress(addr, 0)
		indent := "      "
		if len(indent)+ui.AddressWidth(addr, 0) > contentWidth {
			indent = "  "
		}
		wrapWidth := contentWidth - len(indent)
		if wrapWidth < 1 {
			wrapWidth = 1
		}
		rows = append(rows,
			fmt.Sprintf("  #%-2d %s %s", i+1, who, formatBTCWithSats(out.Value)))
		for _, line := range wrapAddressCells(styledAddressCells(formatted), wrapWidth) {
			rows = append(rows, indent+line)
		}
	}

	rows = append(rows, "")
	if insp.FeeKnown {
		rows = append(rows, renderRow("Fee:", fmt.Sprintf("%d sats · %s sat/vB · ~%d vB",
			insp.Fee, formatFeeRate(roundRate(insp.FeeRate)), insp.VSize)))
	} else {
		rows = append(rows, renderRow("Fee:", styles.WarningText.Render("unknown")))
	}
	rows = append(rows, renderRow("Sent:", formatBTCWithSats(insp.Sent)))
	rbf := "no"
	if insp.RBF {
		rbf = "yes"
	}
	rows = append(rows, renderRow("Replaceable:", rbf))
	if m.psbtResult != nil && m.psbtResult.Replaces != "" {
		rows = append(rows, renderRow("Replaces:", shortTxIDUI(m.psbtResult.Replaces)))
	}
	if m.psbtResult != nil && m.psbtResult.ChildOf != "" {
		rows = append(rows, renderRow("Speeds up:", shortTxIDUI(m.psbtResult.ChildOf)))
	}

	section := styles.InfoBoxStyle.Width(boxWidth + 2).Render(strings.Join(rows, "\n"))
	if warnings := m.inspectionWarnings(); len(warnings) > 0 {
		lines := []string{"⚠ Check before signing", ""}
		for _, w := range warnings {
			lines = append(lines, "• "+w)
		}
		section += "\n" + styles.WarningBoxStyle.Width(boxWidth+2).Render(strings.Join(lines, "\n"))
	}
	return section
}

func roundRate(r float64) float64 {
	return float64(int64(r*10+0.5)) / 10
}

func shortTxIDUI(txid string) string {
	if len(txid) > 16 {
		return txid[:8] + "…" + txid[len(txid)-8:]
	}
	return txid
}

// shortOutPoint shortens "txid:vout".
func shortOutPoint(op string) string {
	i := strings.LastIndex(op, ":")
	if i < 0 {
		return shortTxIDUI(op)
	}
	return shortTxIDUI(op[:i]) + op[i:]
}
