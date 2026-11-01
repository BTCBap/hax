package models

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// sendCompactAddressWidth keeps the compact address column used by the send
// flows: two eight-character groups plus the old "..." middle.
const sendCompactAddressWidth = 19

func (m SendModel) View() string {
	_ = m.width

	if m.showFileBrowser {
		return m.viewFileBrowser()
	}

	if m.showFileInput {
		return m.viewFileInput()
	}

	if m.showQRDisplay {
		return m.viewQRDisplay()
	}

	if m.showQRFormatMenu {
		return m.viewQRFormatMenu()
	}

	// Handle UR paste input overlay
	if m.showURInput {
		return m.viewURInput()
	}

	// Handle PSBT export view overlay
	if m.showPSBTExport {
		return m.viewPSBTExport()
	}

	// Handle raw hex export view overlay
	if m.showRawHexExport {
		return m.viewRawHexExport()
	}

	var content string

	switch m.step {
	case SendStepAddress:
		content = m.viewAddressInput()
	case SendStepAmount:
		content = m.viewAmountInput()
	case SendStepRecipientList:
		content = m.viewRecipientList()
	case SendStepUTXOSelection:
		content = m.viewUTXOSelection()
	case SendStepFee:
		content = m.viewFeeSelection()
	case SendStepPreviewLoading:
		content = m.viewPreviewLoading()
	case SendStepReview:
		content = m.viewReview()
	case SendStepBroadcasting:
		content = m.viewBroadcasting()
	case SendStepComplete:
		content = m.viewComplete()
	}

	// Show error if present
	if m.err != nil {
		content += "\n\n" + styles.RenderErrorBox("⚠ "+m.err.Error(), m.width)
	}

	// Show PSBT export error if present
	if m.psbtExportErr != nil {
		content += "\n\n" + styles.RenderErrorBox("⚠ PSBT Export: "+m.psbtExportErr.Error(), m.width)
	}

	return content
}

// sendFinalHintDesc is the primary action on the last step before signing.
const sendFinalHintDesc = "confirm & sign"

// renderSendHelp draws a send hint row with the primary action first. The
// final step renders that action in the warning style.
func renderSendHelp(primary styles.HelpItem, final bool, rest ...styles.HelpItem) string {
	parts, _ := sendHelpParts(primary, final, rest...)
	return styles.FooterStyle.Render(strings.Join(parts, " • "))
}

// renderSendHelpWrapped starts a new line when the next item would pass
// width cells.
func renderSendHelpWrapped(width int, primary styles.HelpItem, final bool, rest ...styles.HelpItem) string {
	parts, widths := sendHelpParts(primary, final, rest...)
	if width <= 0 {
		return styles.FooterStyle.Render(strings.Join(parts, " • "))
	}

	var lines []string
	var line strings.Builder
	lineWidth := 0
	for i, part := range parts {
		w := widths[i]
		if lineWidth > 0 && lineWidth+3+w > width {
			lines = append(lines, line.String())
			line.Reset()
			lineWidth = 0
		}
		if lineWidth > 0 {
			line.WriteString(" • ")
			lineWidth += 3
		}
		line.WriteString(part)
		lineWidth += w
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return styles.FooterStyle.Render(strings.Join(lines, "\n"))
}

func sendHelpParts(primary styles.HelpItem, final bool, rest ...styles.HelpItem) ([]string, []int) {
	text := primary.Key + " " + primary.Desc

	var first string
	if final {
		first = styles.WarningText.Render(text)
	} else {
		first = styles.HelpKeyStyle.Render(primary.Key) + " " +
			styles.HelpDescStyle.Render(primary.Desc)
	}
	parts := []string{first}
	widths := []int{lipgloss.Width(text)}
	for _, item := range rest {
		parts = append(parts, styles.HelpKeyStyle.Render(item.Key)+" "+styles.HelpDescStyle.Render(item.Desc))
		widths = append(widths, lipgloss.Width(item.Key)+1+lipgloss.Width(item.Desc))
	}
	return parts, widths
}

func (m SendModel) viewAddressInput() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := m.stepHeader(1, "Enter recipient address")

	// Format balance with pastel orange (matching dashboard style)
	var balanceBox string
	if m.loading {
		balanceInfo := m.spinner.View() + " " + styles.DimText.Render("Loading balance...")
		balanceBox = styles.InfoBoxStyle.Render(balanceInfo)
	} else {
		btcBalance := m.balance.ToBTC()
		balanceInfo := styles.DimText.Render("Available: ") +
			styles.BalanceValueStyle.Render(fmt.Sprintf("%.8f BTC", btcBalance)) +
			styles.DimText.Render(fmt.Sprintf(" (%d sats)", m.balance))
		balanceBox = styles.InfoBoxStyle.Render(balanceInfo)
	}

	instruction := styles.NormalText.Render("Enter the recipient's address or a bitcoin: payment URI:")

	// Hint about import option
	importHint := styles.DimText.Render("Or press 'i' to import a PSBT, or paste one")

	help := renderSendHelp(
		styles.HK("enter", "continue"), false,
		styles.HK("i", "import PSBT"),
		styles.HK("ctrl+v", "paste"),
		styles.HK("esc", "cancel"),
	)

	return header + "\n" + subtitle + "\n\n" +
		balanceBox + "\n\n" +
		instruction + "\n" +
		importHint + "\n\n" +
		m.addressInput.View() + "\n\n" +
		help
}

func (m SendModel) viewAmountInput() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := m.stepHeader(2, "Enter amount")

	// Format the address with the shared pastel layout.
	formattedAddr := ui.FormatAddress(m.recipientAddress, 0)

	// Keep recipient details left-aligned and size the box to content.
	addrContent := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.DimText.Render("To:"),
		formattedAddr,
	)

	desiredWidth := lipgloss.Width(formattedAddr) + 8 // border/padding + breathing room
	addrBoxWidth := styles.ClampWidth(desiredWidth, m.width)
	if addrBoxWidth < 40 {
		addrBoxWidth = 40
	}
	addrDisplay := styles.BoxStyle.Width(addrBoxWidth + 2).Render(addrContent)

	unitLabel := "BTC"
	if !m.usesBTC {
		unitLabel = "satoshis"
	}

	instruction := styles.NormalText.Render(fmt.Sprintf("Enter amount in %s:", unitLabel))
	toggleHint := styles.DimText.Render("(Press TAB to toggle BTC/satoshis)")
	if req := m.pendingRequest; req != nil {
		var lines []string
		if req.Amount > 0 {
			lines = append(lines, "Amount from payment request. Check it, then press enter.")
		}
		if req.Label != "" {
			lines = append(lines, "Label: "+truncateRunes(req.Label, 60, "…"))
		}
		if req.Message != "" {
			lines = append(lines, "Message: "+truncateRunes(req.Message, 60, "…"))
		}
		if len(lines) > 0 {
			toggleHint += "\n" + styles.HighlightText.Render(strings.Join(lines, "\n"))
		}
	}

	help := renderSendHelp(
		styles.HK("enter", "continue"), false,
		styles.HK("m", "send all"),
		styles.HK("tab", "unit"),
		styles.HK("esc", "back"),
	)

	return header + "\n" + subtitle + "\n\n" +
		addrDisplay + "\n\n" +
		instruction + "\n" +
		toggleHint + "\n\n" +
		m.amountInput.View() + "\n\n" +
		help
}

func (m SendModel) viewRecipientList() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := m.stepHeader(3, "Review Recipients")

	// Calculate totals
	var totalAmount api.Satoshi
	for _, r := range m.recipients {
		totalAmount += r.Amount
	}

	// Summary info
	summaryInfo := fmt.Sprintf("Recipients: %d • Total: %d sats (%.8f BTC)",
		len(m.recipients), totalAmount, totalAmount.ToBTC())
	summaryBox := styles.InfoBoxStyle.Render(summaryInfo)

	// List recipients
	recipientList := "\n"
	for i, r := range m.recipients {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}

		// Format the address in the same compact column as before.
		shortAddr := ui.FormatAddress(r.Address, sendCompactAddressWidth)

		itemStyle := styles.UnselectedItemStyle
		if i == m.cursor {
			itemStyle = styles.SelectedItemStyle
		}
		recipientList += itemStyle.Render(fmt.Sprintf("%s#%d: ", cursor, i+1)) +
			shortAddr +
			itemStyle.Render(fmt.Sprintf(" • %d sats", r.Amount)) + "\n"
	}

	help := renderSendHelp(
		styles.HK("enter", "continue"), false,
		styles.HK("↑/↓", "navigate"),
		styles.HK("a/+", "add recipient"),
		styles.HK("d", "delete"),
		styles.HK("esc", "edit last"),
	)

	return header + "\n" + subtitle + "\n\n" +
		summaryBox + "\n" +
		recipientList + "\n" +
		help
}

func (m SendModel) viewUTXOSelection() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := m.stepHeader(4, "Select UTXOs")

	// Calculate total amount for all recipients
	var totalSending api.Satoshi
	for _, r := range m.recipients {
		totalSending += r.Amount
	}

	// Show amount being sent
	amountInfo := fmt.Sprintf("Sending: %s BTC (%d sats) to %d recipient%s",
		formatBTC(totalSending.ToBTC()), totalSending,
		len(m.recipients), pluralize(len(m.recipients)))
	if m.sendAll {
		amountInfo = "Sending: MAX spend (final recipient amount derived after fee + preview)"
	}
	amountBox := styles.InfoBoxStyle.Render(amountInfo)

	// Calculate total from selected UTXOs
	var totalSelected api.Satoshi
	var selectedCount int
	for i, selected := range m.utxoSelected {
		if selected && i < len(m.utxos) {
			totalSelected += m.utxos[i].Value
			selectedCount++
		}
	}

	selectionInfo := fmt.Sprintf("Selected: %d UTXO%s • %d sats",
		selectedCount, pluralize(selectedCount), totalSelected)
	if m.sendAll {
		selectionInfo = selectionInfo + " • MAX spend locks all inputs"
	}
	if m.frozenCount > 0 {
		selectionInfo += fmt.Sprintf("\n%d frozen coin%s not shown",
			m.frozenCount, pluralize(m.frozenCount))
	}

	// List UTXOs. The confirmation state is helper text on the selected
	// row, so the rows keep one width as the cursor moves.
	utxoOptions := make([]choiceOption, len(m.utxos))
	for i, utxo := range m.utxos {
		checkbox := "[ ]"
		if i < len(m.utxoSelected) && m.utxoSelected[i] {
			checkbox = "[✓]"
		}

		// Short txid
		shortTxid := utxo.TxID
		if len(shortTxid) > 16 {
			shortTxid = shortTxid[:8] + "..." + shortTxid[len(shortTxid)-8:]
		}

		status := "✓"
		statusDesc := "confirmed"
		if !utxo.Status.Confirmed {
			status = "○"
			statusDesc = "unconfirmed"
		}

		utxoOptions[i] = choiceOption{
			name: fmt.Sprintf("%s %s %s:%d • %d sats",
				checkbox, status, shortTxid, utxo.Vout, utxo.Value),
			desc: statusDesc,
		}
	}
	utxoList := "\n" + renderChoiceMenu(utxoOptions, m.cursor)

	help := renderSendHelpWrapped(m.width,
		styles.HK("tab", "continue"), false,
		styles.HK("↑/↓", "navigate"),
		styles.HK("space/enter", "toggle"),
		styles.HK("a", "all"),
		styles.HK("n", "none"),
		styles.HK("esc", "back"),
	)
	if m.sendAll {
		help = renderSendHelpWrapped(m.width,
			styles.HK("tab", "continue"), false,
			styles.HK("↑/↓", "navigate"),
			styles.HK("esc", "back"),
		)
	}

	return header + "\n" + subtitle + "\n\n" +
		amountBox + "\n\n" +
		styles.DimText.Render(selectionInfo) + "\n" +
		utxoList + "\n" +
		help
}

func (m SendModel) viewFeeSelection() string {
	header := m.sendTitle()
	subtitle := m.stepHeader(5, "Choose fee priority")
	if m.bump != nil {
		subtitle += "\n" + m.renderBumpSummary(styles.ClampWidth(70, m.width-4))
	}

	feeOptions := []struct {
		name  string
		level FeeLevel
		eta   string
	}{
		{"🚀 High Priority", FeeLevelHigh, "~10 min"},
		{"⚡ Medium Priority", FeeLevelMedium, "~30 min"},
		{"🐢 Low Priority", FeeLevelLow, "~1 hour"},
	}

	const customLabel = "✏️  Custom Fee"
	labelWidth := lipgloss.Width(customLabel)
	for _, opt := range feeOptions {
		if w := lipgloss.Width(opt.name); w > labelWidth {
			labelWidth = w
		}
	}

	optionLine := func(index int, label, helper string) string {
		marker := "  "
		markerStyle := styles.DimText
		labelStyle := styles.UnselectedItemStyle
		if index == m.cursor {
			marker = "▶ "
			markerStyle = styles.HighlightText
			labelStyle = styles.SelectedItemStyle
		}
		return markerStyle.Render(marker) +
			labelStyle.Render(padCell(label, labelWidth)) +
			"  " + styles.DimText.Render(helper)
	}

	menuLines := make([]string, 0, 4)
	for i, opt := range feeOptions {
		var feeRate float64
		if m.feeEstimates != nil {
			switch opt.level {
			case FeeLevelHigh:
				feeRate = m.feeEstimates.FastestFee
			case FeeLevelMedium:
				feeRate = m.feeEstimates.HalfHourFee
			case FeeLevelLow:
				feeRate = m.feeEstimates.HourFee
			}
		}

		var helper string
		switch {
		case feeRate <= 0:
			helper = opt.eta + " · unavailable"
		case m.bump != nil && m.bumpReady() && feeRate < m.bumpMinRate():
			helper = fmt.Sprintf("%s · %s sat/vB · below minimum",
				opt.eta, formatFeeRate(feeRate))
		default:
			helper = fmt.Sprintf("%s · %s sat/vB · ~%d sats",
				opt.eta, formatFeeRate(feeRate), m.optionFee(feeRate))
		}
		menuLines = append(menuLines, optionLine(i, opt.name, helper))
	}

	customHelper := "enter a rate"
	customDetail := ""
	switch {
	case m.highFeePending > 0:
		customHelper = "unusually high"
		customDetail = styles.WarningText.Render(fmt.Sprintf(
			"    %s sat/vB is unusually high (~%d sats).\n    Type %q to confirm: %s",
			formatFeeRate(m.highFeePending), m.optionFee(m.highFeePending),
			highFeeConfirmWord, m.highFeeInput.View()))
	case m.editingCustom:
		customHelper = "enter a rate"
		customDetail = styles.DimText.Render(fmt.Sprintf("    Enter rate: %s sat/vB",
			m.customFeeInput.View()))
	case m.customFeeRate > 0:
		customHelper = fmt.Sprintf("%s sat/vB · ~%d sats",
			formatFeeRate(m.customFeeRate), m.optionFee(m.customFeeRate))
	}
	customLine := optionLine(3, customLabel, customHelper)
	if customDetail != "" {
		customLine += "\n" + customDetail
	}
	menuLines = append(menuLines, customLine)

	menu := "\n" + strings.Join(menuLines, "\n\n") + "\n"

	var help string
	if m.highFeePending > 0 {
		help = styles.RenderHelp(
			styles.HK("enter", "confirm high fee"),
			styles.HK("esc", "edit rate"),
		)
	} else if m.editingCustom {
		help = styles.RenderHelp(
			styles.HK("enter", "confirm"),
			styles.HK("esc", "cancel"),
		)
	} else if m.bump != nil {
		items := []styles.HelpItem{
			styles.HK("↑/↓", "navigate"),
			styles.HK("c", "custom"),
		}
		if m.bump.info != nil && m.bump.info.CanRBF && m.bump.info.CanCPFP {
			items = append(items, styles.HK("m", "method"))
		}
		help = renderSendHelp(styles.HK("enter", "continue"), false,
			append(items, styles.HK("esc", "cancel"))...)
	} else {
		help = renderSendHelp(
			styles.HK("enter", "continue"), false,
			styles.HK("↑/↓", "navigate"),
			styles.HK("c", "custom"),
			styles.HK("esc", "back"),
		)
	}

	return header + "\n" + subtitle + "\n" + menu + "\n" + help
}

// optionFee is the fee shown next to a feerate in the fee menu.
func (m SendModel) optionFee(rate float64) int64 {
	if m.bump != nil {
		return int64(m.bumpFeeAt(rate))
	}
	return int64(math.Ceil(float64(m.estimatedSize) * rate))
}

func (m SendModel) viewPreviewLoading() string {
	header := m.sendTitle()

	progress := m.spinner.View() + " " + styles.NormalText.Render("Selecting UTXOs and calculating fees...")

	return header + "\n\n" + progress
}

func (m SendModel) reviewTxVSize(numInputs, numOutputs int) int {
	if m.psbtResult != nil && m.psbtResult.VSize > 0 {
		return m.psbtResult.VSize
	}
	if m.walletManager != nil {
		return m.estimateTxVSize(numInputs, numOutputs)
	}
	return 0
}

func (m SendModel) buildReviewWarnings(totalRecipientAmount, displayFee api.Satoshi, txVSize int) []string {
	var warnings []string

	unconfirmedInputs := 0
	for _, utxo := range m.psbtResult.SelectedUTXOs {
		if !utxo.Status.Confirmed {
			unconfirmedInputs++
		}
	}
	if unconfirmedInputs > 0 {
		warnings = append(warnings,
			fmt.Sprintf("%d selected input%s %s still unconfirmed. Replacement, delay, or rejection risk is higher until confirmation.",
				unconfirmedInputs,
				pluralize(unconfirmedInputs),
				pluralVerb(unconfirmedInputs, "is", "are")))
	}

	if !m.sendAll && len(m.recipients) > 0 && !m.psbtResult.HasChange && m.psbtResult.DustAbsorbed == 0 {
		warnings = append(warnings,
			"No change output is being created. Review every output carefully because this spend does not return coins back to the wallet.")
	}

	if totalRecipientAmount > 0 && displayFee*10 >= totalRecipientAmount {
		feePct := float64(displayFee) / float64(totalRecipientAmount) * 100
		warnings = append(warnings,
			fmt.Sprintf("Network fee is consuming %.1f%% of the amount being sent. That is unusually high for a normal spend.",
				feePct))
	}

	if txVSize > 0 {
		effectiveFeeRate := float64(displayFee) / float64(txVSize)
		selectedFeeRate := m.getFeeRate()
		if selectedFeeRate > 0 && effectiveFeeRate > selectedFeeRate*1.25 {
			warnings = append(warnings,
				fmt.Sprintf("Effective fee rate is %s sat/vB, above the selected %s sat/vB. Review dust absorption or output shape before signing.",
					formatFeeRate(effectiveFeeRate),
					formatFeeRate(selectedFeeRate)))
		}
	}

	return warnings
}

func (m SendModel) buildReviewScrollable() (string, string, string) {
	header := m.sendTitle()
	subtitle := m.stepHeader(6, "Review transaction")

	// Responsive box width for all review sections
	boxWidth := m.width - 4
	if boxWidth > 70 {
		boxWidth = 70
	}
	if boxWidth < 40 {
		boxWidth = 40
	}

	// Guard against nil psbtResult (shouldn't happen but defensive)
	if m.psbtResult == nil {
		fixedHeader := header + "\n" + subtitle
		scrollContent := styles.DimText.Render("Loading transaction preview...")
		help := styles.RenderHelp(styles.HK("esc", "back"))
		return fixedHeader, scrollContent, help
	}

	// Use actual fee if we have preview data, otherwise use estimate
	displayFee := m.estimatedFee
	if m.psbtResult.Fee > 0 {
		displayFee = m.psbtResult.Fee
	}

	// === INPUTS SECTION ===
	var inputsSection string
	if len(m.psbtResult.SelectedUTXOs) > 0 {
		inputRows := []string{
			styles.HighlightText.Render("Inputs:"),
		}

		for i, utxo := range m.psbtResult.SelectedUTXOs {
			// Show first 8 and last 8 chars of txid
			shortTxid := utxo.TxID
			if len(shortTxid) > 16 {
				shortTxid = shortTxid[:8] + "..." + shortTxid[len(shortTxid)-8:]
			}

			addr := utxo.Address

			// Compact one-line address formatting to avoid hard wraps in review boxes.
			formattedAddr := ""
			if addr != "" {
				formattedAddr = ui.FormatAddress(addr, sendCompactAddressWidth)
			}

			// Combine input info on fewer lines with aligned amounts
			if formattedAddr != "" {
				inputRows = append(inputRows,
					fmt.Sprintf("  %s %s", styles.DimText.Render(fmt.Sprintf("Input #%d:", i+1)), formattedAddr),
					fmt.Sprintf("    %s", styles.DimText.Render(fmt.Sprintf("%s:%d", shortTxid, utxo.Vout))),
					renderRow("  Amount:", formatBTCWithSats(utxo.Value)),
				)
			}

			// Limit display to first 3 UTXOs to save space
			if i >= 2 && len(m.psbtResult.SelectedUTXOs) > 3 {
				inputRows = append(inputRows, fmt.Sprintf("  ... and %d more input%s", len(m.psbtResult.SelectedUTXOs)-3, pluralize(len(m.psbtResult.SelectedUTXOs)-3)))
				break
			}
		}

		inputRows = append(inputRows, renderRow("Total Input:", formatBTCWithSats(m.psbtResult.TotalInput())))

		inputsSection = styles.InfoBoxStyle.Width(boxWidth + 2).Render(strings.Join(inputRows, "\n"))
	}

	// === OUTPUTS SECTION ===
	outputRows := []string{
		styles.HighlightText.Render("Outputs:"),
	}

	// Calculate total amount sent to recipients
	var totalRecipientAmount api.Satoshi

	// Show all recipients
	for i, r := range m.recipients {
		formattedRecipient := ui.FormatAddress(r.Address, sendCompactAddressWidth)
		outputRows = append(outputRows,
			"  "+styles.DimText.Render(fmt.Sprintf("Output #%d (Recipient):", i+1))+" "+formattedRecipient,
			renderRow("  Amount:", formatBTCWithSats(r.Amount)),
		)
		if r.Label != "" {
			outputRows = append(outputRows, renderRow("  Label:", truncateRunes(r.Label, 40, "…")))
		}
		totalRecipientAmount += r.Amount

		// Limit display to first 3 recipients to save space
		if i >= 2 && len(m.recipients) > 3 {
			remaining := len(m.recipients) - 3
			var remainingAmount api.Satoshi
			for j := 3; j < len(m.recipients); j++ {
				remainingAmount += m.recipients[j].Amount
			}
			outputRows = append(outputRows,
				fmt.Sprintf("  ... and %d more recipient%s (%d sats)", remaining, pluralize(remaining), remainingAmount))
			for j := 3; j < len(m.recipients); j++ {
				totalRecipientAmount += m.recipients[j].Amount
			}
			break
		}
	}

	// Change output (if applicable)
	if m.psbtResult.HasChange {
		formattedChange := ui.FormatAddress(m.psbtResult.ChangeAddress, sendCompactAddressWidth)
		// Add extra spaces to align "Change):" to match length of "Recipient):"
		// "Recipient" = 9 chars, "Change" = 6 chars, difference = 3 spaces needed
		outputRows = append(outputRows,
			"  "+styles.DimText.Render(fmt.Sprintf("Output #%d (Change):   ", len(m.recipients)+1))+" "+formattedChange,
			renderRow("  Amount:", formatBTCWithSats(m.psbtResult.Change)),
		)
	}

	outputRows = append(outputRows, renderRow("Total Output:", formatBTCWithSats(totalRecipientAmount+m.psbtResult.Change)))

	outputsSection := styles.InfoBoxStyle.Width(boxWidth + 2).Render(strings.Join(outputRows, "\n"))

	// === FEE & SIZE BREAKDOWN ===
	// Calculate actual transaction size breakdown
	numInputs := len(m.psbtResult.SelectedUTXOs)
	numOutputs := len(m.recipients)
	if numOutputs == 0 {
		numOutputs = 1
	}
	if m.psbtResult.HasChange {
		numOutputs++
	}
	txVSize := m.reviewTxVSize(numInputs, numOutputs)

	sizeSummary := "unknown"
	if txVSize > 0 {
		sizeSummary = fmt.Sprintf("%d vB (%d input%s, %d output%s)",
			txVSize, numInputs, pluralize(numInputs), numOutputs, pluralize(numOutputs))
	}

	feeRows := []string{
		styles.HighlightText.Render("Fee & Size Breakdown:"),
		renderRow("Tx Size:", sizeSummary),
		renderRow("Fee Rate:", fmt.Sprintf("%s sat/vB · %s", formatFeeRate(m.getFeeRate()), m.feeChoiceSummary())),
		renderRow("Network Fee:", formatBTCWithSats(displayFee)),
		renderRow("Total Cost:", formatBTCWithSats(totalRecipientAmount+displayFee)),
	}

	feeSection := styles.BoxStyle.Width(boxWidth + 2).Render(strings.Join(feeRows, "\n"))

	reviewWarnings := m.buildReviewWarnings(totalRecipientAmount, displayFee, txVSize)
	warningSection := ""
	if len(reviewWarnings) > 0 {
		warningLines := []string{"⚠ Safety Checks", ""}
		for _, warning := range reviewWarnings {
			warningLines = append(warningLines, "• "+warning)
		}
		warningSection = styles.WarningBoxStyle.Width(boxWidth + 2).Render(strings.Join(warningLines, "\n"))
	}

	// Dust warning - show if dust was absorbed into fee
	dustWarning := ""
	if m.psbtResult.DustAbsorbed > 0 {
		// Calculate how much the fee increased due to dust absorption
		feeIncrease := displayFee - m.psbtResult.IntendedFee
		dustWarning = styles.WarningBoxStyle.Render(
			fmt.Sprintf("⚠ Dust absorbed into fee\n\n"+
				"Change of %d sats is below dust limit and cannot be\n"+
				"created as an output. It has been added to the fee.\n\n"+
				"Intended fee: %d sats → Actual fee: %d sats (+%d sats)",
				m.psbtResult.DustAbsorbed, m.psbtResult.IntendedFee, displayFee, feeIncrease))
	}

	// Show error if broadcast failed
	var errorSection string
	if m.broadcastErr != nil {
		errorSection = styles.RenderErrorBox(
			fmt.Sprintf("❌ Transaction failed:\n\n%v", m.broadcastErr), m.width)
	}

	// Unified help - all wallet types go to Sign & Broadcast
	help := renderSendHelp(
		styles.HK("enter", sendFinalHintDesc), true,
		styles.HK("v", "flow"),
		styles.HK("esc", "change fee"),
	)

	// Split into fixed header and scrollable content
	fixedHeader := header + "\n" + subtitle

	var sections []string
	if inputsSection != "" {
		sections = append(sections, inputsSection)
	}
	sections = append(sections, outputsSection, feeSection)
	if warningSection != "" {
		sections = append(sections, warningSection)
	}
	if dustWarning != "" {
		sections = append(sections, dustWarning)
	}
	if errorSection != "" {
		sections = append(sections, errorSection)
	}
	scrollContent := strings.Join(sections, "\n")

	return fixedHeader, scrollContent, help
}

func (m SendModel) viewReview() string {
	if m.txFlow != nil {
		return m.viewReviewTxFlow()
	}
	fixedHeader, scrollContent, help := m.buildReviewScrollable()
	return m.renderScrollableViewport(fixedHeader, scrollContent, help)
}

// viewReviewTxFlow replaces the review body with the visualizer while the
// screen title and step header stay in place.
func (m SendModel) viewReviewTxFlow() string {
	header := m.sendTitle() + "\n" + m.stepHeader(6, "Review transaction")
	return m.renderTxFlowUnder(header)
}

// renderTxFlowUnder draws the visualizer over the rows below header. The
// flow block is opaque, so the screen behind it keeps its size and does not
// show through the diagram's blank rows.
func (m SendModel) renderTxFlowUnder(header string) string {
	headerHeight := lipgloss.Height(header)
	flow := *m.txFlow
	flow.Width = m.width
	flow.Height = m.height - headerHeight - 1
	if flow.Height < 1 {
		flow.Height = 1
	}
	base := m
	base.txFlow = nil
	return ui.Composite(base.View(), m.width, headerHeight+flow.Height,
		ui.Overlay{Content: flowBlock(flow), X: 0, Y: headerHeight}.TopLeft())
}

func (m SendModel) viewBroadcasting() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")

	progress := m.spinner.View() + " " + styles.NormalText.Render("Broadcasting transaction...")

	return header + "\n\n" + progress
}

func (m SendModel) viewComplete() string {
	if m.txID == "" {
		// Error state
		header := styles.TitleStyle.Render("❌ Transaction Failed")
		errorBox := styles.RenderErrorBox(
			fmt.Sprintf("Failed to broadcast transaction:\n\n%v", m.broadcastErr), m.width)

		help := styles.RenderHelp(styles.HK("enter", "back to dashboard"))

		return header + "\n\n" + errorBox + "\n\n" + help
	}

	// Success state
	header := styles.TitleStyle.Render("✅ Transaction Sent!")

	// Calculate totals - prefer recipients if available, else extract from PSBT
	var totalAmount api.Satoshi
	var addrSection string
	var numRecipients int

	completeBoxWidth := m.width - 4
	if completeBoxWidth > 70 {
		completeBoxWidth = 70
	}
	if completeBoxWidth < 40 {
		completeBoxWidth = 40
	}
	explorerURL := wallet.ExplorerTxURL(walletNetwork(m.walletManager), m.txID)
	// On wide terminals let the card grow enough to show the whole URL.
	if needed := lipgloss.Width(explorerURL) + 4; needed > completeBoxWidth && needed <= m.width-4 {
		completeBoxWidth = needed
	}
	// Outer width minus border and horizontal padding.
	contentWidth := completeBoxWidth - 4

	if len(m.recipients) > 0 {
		// Normal flow: use recipients
		for _, r := range m.recipients {
			totalAmount += r.Amount
		}
		numRecipients = len(m.recipients)

		if numRecipients == 1 {
			formattedAddr := ui.FormatAddress(m.recipients[0].Address, contentWidth-16)
			addrSection = renderRow("To:", formattedAddr)
		} else {
			addrSection = renderRow("To:", fmt.Sprintf("%d recipients", numRecipients))
		}
	} else if m.psbtResult != nil && m.psbtResult.Packet != nil {
		// Imported PSBT: extract from PSBT outputs
		tx := m.psbtResult.Packet.UnsignedTx
		if tx != nil {
			// Sum outputs (excluding change if we can identify it)
			for _, txOut := range tx.TxOut {
				totalAmount += api.Satoshi(txOut.Value)
			}
			numRecipients = len(tx.TxOut)
			addrSection = renderRow("To:", fmt.Sprintf("%d output(s)", numRecipients))
		}
	} else {
		addrSection = renderRow("To:", "imported transaction")
	}

	summaryRows := []string{
		renderRow("Amount:", fmt.Sprintf("%s BTC", formatBTC(totalAmount.ToBTC()))),
		"",
		addrSection,
		"",
		renderRow("Fee:", fmt.Sprintf("%d sats", m.estimatedFee)),
		"",
		renderRow("Transaction ID:", ""),
		styles.HighlightText.Render(m.txID),
	}

	successBox := styles.SuccessBoxStyle.Width(completeBoxWidth + 2).Render(strings.Join(summaryRows, "\n"))

	// Show "copied" indicator if transaction ID was just copied
	copiedIndicator := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		if m.lastAction == "link" {
			copiedIndicator = "\n" + styles.SuccessText.Render("✓ link copied")
		} else {
			copiedIndicator = "\n" + styles.SuccessBoxStyle.Render("✓ Copied to clipboard!")
		}
	}

	// Keep the URL on one line. The visible text is shortened when it does not
	// fit, but the hyperlink target and the u copy payload stay the full URL.
	link := styles.Hyperlink(explorerURL, styles.FitURL(explorerURL, contentWidth))
	info := styles.InfoBoxStyle.Width(completeBoxWidth + 2).Render(
		"🔍 Track your transaction:\n\n" + link)

	help := styles.RenderHelpWrapped(m.width,
		styles.HK("c", "copy txid"),
		styles.HK("u", "copy link"),
		styles.HK("enter", "open in browser"),
		styles.HK("n", "new transaction"),
		styles.HK("q/esc", "dashboard"),
	)

	return header + "\n\n" +
		successBox + "\n\n" +
		info + "\n\n" +
		help +
		copiedIndicator
}

// isViewportScrollActive returns true when Step 6/7 should use viewport scrolling.
func (m SendModel) isViewportScrollActive() bool {
	isScrollableScreen := m.step == SendStepReview || m.showPSBTExport
	return isScrollableScreen &&
		!m.showFileBrowser &&
		!m.showRawHexExport &&
		!m.showQRDisplay &&
		!m.showQRFormatMenu &&
		!m.showURInput &&
		!m.showFileInput &&
		!m.showHWIDevices
}

// syncScrollViewport updates Step 6/7 viewport content and size from model state.
func (m *SendModel) syncScrollViewport() {
	if !m.isViewportScrollActive() {
		return
	}
	if m.walletManager == nil {
		return
	}

	var header, content, footer string
	if m.showPSBTExport {
		header, content, footer = m.buildPSBTExportScrollable()
	} else {
		header, content, footer = m.buildReviewScrollable()
	}

	width := m.width
	if width < 1 {
		width = 1
	}
	availableHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if availableHeight < 1 {
		availableHeight = 1
	}

	m.scrollViewport.SetWidth(width)
	m.scrollViewport.SetHeight(availableHeight)
	m.scrollViewport.SetContent(content)
}

// renderScrollableViewport renders fixed header/footer with a smooth Bubble Tea viewport body.
func (m SendModel) renderScrollableViewport(header, content, footer string) string {
	if m.height <= 0 {
		return header + "\n" + content + "\n" + footer
	}

	view := m.scrollViewport.View()
	if view == "" {
		view = content
	}

	indicator := ""
	if m.scrollViewport.TotalLineCount() > m.scrollViewport.Height() {
		scrollPct := int(m.scrollViewport.ScrollPercent() * 100)
		indicator = styles.ScrollIndicatorStyle.Render(fmt.Sprintf("  %d%%", scrollPct))
	}

	return header + indicator + "\n" + view + "\n" + footer
}

// feeChoiceSummary names the fee the user picked, so the review shows
// the choice and not just a number: "Medium priority, ~30 min".
func (m SendModel) feeChoiceSummary() string {
	switch m.feeLevel {
	case FeeLevelHigh:
		return "High priority, next block"
	case FeeLevelLow:
		return "Low priority, ~1 hour"
	case FeeLevelCustom:
		if m.customFeeRate > 0 {
			return "Custom rate"
		}
	}
	return "Medium priority, ~30 min"
}
