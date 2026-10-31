package models

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/BTCBap/hax/internal/ui/styles"
)

// sendStepNames are the breadcrumb labels for the seven send steps.
var sendStepNames = []string{"Address", "Amount", "Recipients", "Coins", "Fee", "Review", "Sign"}

// stepHeader renders the send-flow breadcrumb with step (1-based) highlighted,
// followed by the step's title. Narrow terminals get a compact "Step n/7".
func (m SendModel) stepHeader(step int, title string) string {
	if m.bump != nil {
		// A speed-up skips the recipient steps; the bar would mislead.
		return styles.SubtitleStyle.Render(title)
	}
	return renderStepBar(step, m.width) + "\n" + styles.SubtitleStyle.Render(title)
}

func renderStepBar(step, width int) string {
	total := len(sendStepNames)
	if step < 1 || step > total {
		return ""
	}
	pad := lipgloss.NewStyle().Padding(0, 2)
	sep := styles.DimText.Render(" › ")
	parts := make([]string, total)
	plain := 0
	for i, name := range sendStepNames {
		plain += len(name)
		switch {
		case i+1 < step:
			parts[i] = styles.SuccessText.Render(name)
		case i+1 == step:
			parts[i] = styles.HighlightText.Bold(true).Render(name)
		default:
			parts[i] = styles.DimText.Render(name)
		}
	}
	plain += 3*(total-1) + 4 // separators and padding
	if width > 0 && plain > width {
		return pad.Render(styles.HighlightText.Render(
			fmt.Sprintf("Step %d/%d · %s", step, total, sendStepNames[step-1])))
	}
	return pad.Render(strings.Join(parts, sep))
}
