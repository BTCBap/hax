package models

import (
	"fmt"
	"strings"

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

// setPSBTResult replaces the current PSBT and its inspection.
func (m *SendModel) setPSBTResult(r *wallet.PSBTResult) {
	m.psbtResult = r
	m.refreshInspection()
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
			addr = "(no address)"
		}
		// Full addresses: a shortened one is easy to spoof. Indent less
		// when the longest (P2TR, P2WSH) wouldn't fit the box.
		indent := "      "
		if len(indent)+len(addr) > boxWidth-4 {
			indent = "  "
		}
		rows = append(rows,
			fmt.Sprintf("  #%-2d %s %s", i+1, who, formatBTCWithSats(out.Value)),
			indent+styles.NormalText.Render(addr))
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

	section := styles.InfoBoxStyle.Width(boxWidth).Render(strings.Join(rows, "\n"))
	if warnings := m.inspectionWarnings(); len(warnings) > 0 {
		lines := []string{"⚠ Check before signing", ""}
		for _, w := range warnings {
			lines = append(lines, "• "+w)
		}
		section += "\n" + styles.WarningBoxStyle.Width(boxWidth).Render(strings.Join(lines, "\n"))
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
