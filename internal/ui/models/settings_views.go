package models

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

func (m SettingsModel) View() string {
	_ = m.width

	if m.showLabelImport {
		return m.viewLabelImport()
	}

	if m.showBackupExport {
		return m.viewBackupExport()
	}

	// If showing mnemonic warning, show that screen
	if m.showMnemonicWarning {
		return m.viewMnemonicWarning()
	}

	// If showing mnemonic, show that screen
	if m.showMnemonic {
		return m.viewMnemonic()
	}

	if m.showCosignerScanPassword {
		return m.viewCosignerScanPassword()
	}

	// If showing cosigner list, show that screen
	if m.showCosignerList {
		return m.viewCosignerList()
	}

	// If showing cosigner seed input, show that screen
	if m.showCosignerSeedInput {
		return m.viewCosignerSeedInput()
	}

	// If showing cosigner edit, show that screen
	if m.showCosignerEdit {
		return m.viewCosignerEdit()
	}

	// If showing descriptor menu, show that screen
	if m.showDescriptorMenu {
		return m.viewDescriptorMenu()
	}

	// If showing single-sig seed import, show that screen
	if m.showSingleSigSeedInput {
		return m.viewSingleSigSeedInput()
	}

	// Compact header (no vertical padding, like receive screen)
	header := styles.ScreenHeaderStyle.Render("⚙  Wallet Settings")

	// Copy status - inline with header (pastel green, matching theme)
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		msg := m.copyMessage
		if msg == "" {
			msg = "COPIED!"
		}
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ "+msg)
	}

	subtitle := styles.SubtitleStyle.Render("Advanced wallet configuration and information")

	// Wallet Information Section
	walletName := m.walletManager.GetWalletName()
	if m.walletManager.HasPassphrase() {
		maxValueWidth := 40
		if m.width > 0 {
			if w := styles.ClampWidth(70, m.width) - 6 - 17; w > 0 {
				maxValueWidth = w
			}
		}
		walletName = walletNameWithBadge(walletName, true, maxValueWidth)
	}
	network := m.walletManager.GetNetwork()
	createdAt := m.walletManager.GetCreatedAt()
	createdDate := time.Unix(createdAt, 0).Format("2006-01-02 15:04:05")
	walletType := m.walletManager.GetWalletType()

	// Create a beautiful info box with Lipgloss table-like layout
	infoRows := []string{
		m.renderInfoRow("Wallet Name", walletName),
		m.renderInfoRow("Type", walletType),
		m.renderInfoRow("Network", strings.ToUpper(network)),
		m.renderInfoRow("Created", createdDate),
		m.renderInfoRow("Status", "🟢 Unlocked"),
	}

	// Show HWI status for hardware-backed wallets (cached on screen activation)
	if m.walletManager.IsHardwareWallet() && m.hwiStatusCached != "" {
		infoRows = append(infoRows, m.renderInfoRow("HWI", m.hwiStatusCached))
	}

	walletInfo := styles.BoxStyle.
		Width(styles.ClampWidth(70, m.width)+2).
		BorderForeground(styles.NeonPurple).
		Padding(0, 2).
		Render(
			styles.BoldText.Render("📋 Wallet Information") + "\n" +
				strings.Join(infoRows, "\n"),
		)

	// Accounts Section
	accounts := m.walletManager.GetAccounts()
	var accountSections []string

	for i, account := range accounts {
		accountSection := m.renderAccountSection(account, i)
		accountSections = append(accountSections, accountSection)
	}

	accountsDisplay := ""
	if len(accountSections) > 0 {
		accountsDisplay = "\n" + strings.Join(accountSections, "\n")
	}

	// Connection Settings Section
	connectionInfo := m.renderConnectionSection()

	// Backup and label actions
	actions := m.renderSettingsActions()

	// Help footer - includes unit toggle hotkey
	unitLabel := "BTC"
	if m.displayUnit == config.DisplayUnitBitcoin {
		unitLabel = "₿"
	}

	// Build help text based on wallet type
	items := []styles.HelpItem{
		styles.HK("u", "unit:"+unitLabel),
		styles.HK("g", fmt.Sprintf("gap:%d", m.gapLimit)),
		styles.HK("b", "backend"),
		styles.HK("l", "import labels"),
		styles.HK("B", "export backup"),
	}

	if m.walletManager.IsMultisig() {
		items = append(items, styles.HK("i", "cosigners"), styles.HK("w", "CC config"))
	} else {
		items = append(items, styles.HK("c", "copy xpub"))
		if m.walletManager.IsWatchOnly() {
			items = append(items, styles.HK("i", "import seed"))
		}
	}

	items = append(items, styles.HK("d", "descriptor"))

	if !m.walletManager.IsWatchOnly() && !m.walletManager.IsMultisig() {
		items = append(items, styles.HK("m", "mnemonic"))
	}

	items = append(items, styles.HK("esc", "back"))

	help := styles.RenderHelpWrapped(m.width, items...)

	// Split into fixed header and scrollable content
	fixedHeader := header + copyStatus + "\n" + subtitle

	scrollContent := connectionInfo + "\n" +
		actions + "\n" +
		walletInfo + accountsDisplay

	return renderScrollableView(m.height, m.scrollOffset, fixedHeader, scrollContent, help)
}

// renderConnectionSection creates a compact display for connection/backend settings
func (m SettingsModel) renderConnectionSection() string {
	backendType := "Unknown"
	backendDetails := "No configuration loaded"

	if m.configManager != nil {
		cfg, err := m.configManager.Load()
		if err == nil && cfg != nil && cfg.Backend != nil {
			switch cfg.Backend.Type {
			case backend.BackendTypeElectrum:
				backendType = "Electrum"
				if cfg.Backend.Electrum != nil {
					ssl := ""
					if cfg.Backend.Electrum.UseSSL {
						ssl = " (SSL)"
					}
					backendDetails = fmt.Sprintf("%s:%d%s", cfg.Backend.Electrum.Host, cfg.Backend.Electrum.Port, ssl)
				}
			case backend.BackendTypeRPC:
				backendType = "Bitcoin Core RPC"
				if cfg.Backend.RPC != nil {
					backendDetails = fmt.Sprintf("%s:%d", cfg.Backend.RPC.Host, cfg.Backend.RPC.Port)
				}
			}
		}
	}

	boxWidth := styles.ClampWidth(70, m.width)
	return styles.BoxStyle.
		Width(boxWidth+2).
		BorderForeground(styles.SuccessGreen).
		Padding(0, 2).
		Render(
			styles.BoldText.Render("🔌 ") + backendType + ": " +
				styles.HighlightText.Render(backendDetails),
		)
}

// renderSettingsActions lists backup and label actions with dim helper
// text in a fixed column to the right of each label.
func (m SettingsModel) renderSettingsActions() string {
	boxWidth := styles.ClampWidth(70, m.width)
	rowWidth := boxWidth - 4 // box padding
	rows := []string{
		renderSettingsActionRow("l", "Import labels (BIP329)", "JSON Lines file", rowWidth),
		renderSettingsActionRow("B", "Export backup (descriptors + labels)", "no private keys", rowWidth),
	}
	return styles.BoxStyle.
		Width(boxWidth+2).
		BorderForeground(styles.BitcoinGold).
		Padding(0, 2).
		Render(
			styles.BoldText.Render("📤 Backup & Labels") + "\n" +
				strings.Join(rows, "\n"),
		)
}

func renderSettingsActionRow(key, label, helper string, width int) string {
	const labelWidth = 36
	helperWidth := lipgloss.Width(helper)
	if room := width - 3 - labelWidth - 2; room < helperWidth {
		helperWidth = room
	}
	if helperWidth < 0 {
		helperWidth = 0
	}
	return styles.HelpKeyStyle.Render(key) + "  " +
		styles.NormalText.Render(padCell(label, labelWidth)) + "  " +
		styles.DimText.Render(padCell(helper, helperWidth))
}

// renderInfoRow creates a formatted info row with label and value
func (m SettingsModel) renderInfoRow(label, value string) string {
	labelStyle := styles.DimText.Width(15)
	valueStyle := styles.HighlightText

	return labelStyle.Render(label+":") + "  " + valueStyle.Render(value)
}

// renderAccountSection creates a detailed account display
func (m SettingsModel) renderAccountSection(account wallet.AccountData, index int) string {
	// Account header
	accountTitle := fmt.Sprintf("💼 Account #%d: %s", index, account.Name)

	isMultisig := m.walletManager.IsMultisig()

	// Address type with icon
	addressTypeDisplay := ""
	if isMultisig {
		config := m.walletManager.GetMultisigConfig()
		if config != nil {
			switch wallet.MultisigScriptType(config.ScriptType) {
			case wallet.MultisigLegacy:
				addressTypeDisplay = fmt.Sprintf("🔐 %d-of-%d Multisig (P2SH)", config.M, config.N)
			case wallet.MultisigNestedSegwit:
				addressTypeDisplay = fmt.Sprintf("🔐 %d-of-%d Multisig (P2SH-P2WSH)", config.M, config.N)
			case wallet.MultisigNativeSegwit:
				addressTypeDisplay = fmt.Sprintf("🔐 %d-of-%d Multisig (P2WSH)", config.M, config.N)
			}
		}
	} else {
		switch account.AddressType {
		case "legacy":
			addressTypeDisplay = "🔸 Legacy (P2PKH)"
		case "segwit_nested":
			addressTypeDisplay = "🔹 SegWit Nested (P2SH-P2WPKH)"
		case "segwit_native":
			addressTypeDisplay = "🔷 Native SegWit (bech32)"
		case "taproot":
			addressTypeDisplay = "🟠 Taproot (P2TR)"
		}
	}

	// Address statistics
	usedReceivingAddrs := 0
	usedChangeAddrs := 0
	for _, addr := range account.Addresses {
		if addr.Used {
			if addr.Change {
				usedChangeAddrs++
			} else {
				usedReceivingAddrs++
			}
		}
	}

	var infoRows []string

	if isMultisig {
		// Multisig: show cosigner count and descriptor info
		config := m.walletManager.GetMultisigConfig()
		derivationPath := "BIP48 (m/48'/1'/0'/script')"
		infoRows = []string{
			m.renderInfoRow("Type", addressTypeDisplay),
			m.renderInfoRow("Path", derivationPath),
			m.renderInfoRow("Cosigners", fmt.Sprintf("%d keys", config.N)),
			styles.DimText.Render("Press 'd' for descriptor details"),
		}
	} else {
		// Single-sig: show xpub
		xpub, err := m.walletManager.GetExtendedPublicKey(account.Index)
		xpubDisplay := ""
		if err == nil && xpub != "" {
			// Truncate xpub for display
			if len(xpub) > 60 {
				xpubDisplay = xpub[:30] + "..." + xpub[len(xpub)-27:]
			} else {
				xpubDisplay = xpub
			}
		} else {
			xpubDisplay = styles.ErrorText.Render("Error loading xpub")
		}

		// Derivation path - use custom path if set, otherwise determine from address type
		var derivationPath string
		if account.CustomDerivationPath != "" {
			derivationPath = account.CustomDerivationPath + " (custom)"
		} else {
			purpose := "44" // Default to BIP44
			switch account.AddressType {
			case "legacy":
				purpose = "44" // BIP44
			case "segwit_nested":
				purpose = "49" // BIP49
			case "segwit_native":
				purpose = "84" // BIP84
			case "taproot":
				purpose = "86" // BIP86
			}
			derivationPath = fmt.Sprintf("m/%s'/1'/%d'", purpose, account.Index)
		}

		infoRows = []string{
			m.renderInfoRow("Type", addressTypeDisplay),
			m.renderInfoRow("Path", derivationPath) + "  " + styles.DimText.Render(fmt.Sprintf("(%d recv, %d change)", usedReceivingAddrs, usedChangeAddrs)),
			styles.DimText.Render("xpub: ") + lipgloss.NewStyle().Foreground(styles.PastelOrange).Render(xpubDisplay),
		}
	}

	boxWidth := styles.ClampWidth(70, m.width)
	return styles.BoxStyle.
		Width(boxWidth+2).
		BorderForeground(styles.ElectricBlue).
		Padding(0, 2).
		Render(
			styles.HighlightText.Render(accountTitle) + "\n" +
				strings.Join(infoRows, "\n"),
		)
}

const (
	descriptorBoxDesiredWidth = 78
	descriptorBoxPadding      = 2
)

// descriptorBoxWidth returns the descriptor box's outer width. The box keeps
// the fixed width the other detail boxes use.
func descriptorBoxWidth(available int) int {
	return styles.ClampWidth(descriptorBoxDesiredWidth, available) + 2
}

// descriptorInnerWidth returns the content width inside the descriptor box,
// excluding its border and horizontal padding.
func descriptorInnerWidth(boxWidth int) int {
	inner := boxWidth - 2 - descriptorBoxPadding*2
	if inner < 1 {
		inner = 1
	}
	return inner
}

// descriptorSegments splits text into segments that end after each ',', so a
// multisig descriptor starts every key on its own line:
// "wsh(sortedmulti(2,[a]xpub/0/*,[b]xpub/0/*))" becomes "wsh(sortedmulti(2,",
// "[a]xpub/0/*," and "[b]xpub/0/*))". Newlines also end a segment.
func descriptorSegments(text string) []string {
	var segments []string
	start := 0
	for i, r := range text {
		switch r {
		case ',':
			segments = append(segments, text[start:i+1])
			start = i + 1
		case '\n':
			if i > start {
				segments = append(segments, text[start:i])
			}
			start = i + 1
		}
	}
	if start < len(text) {
		segments = append(segments, text[start:])
	}
	return segments
}

// wrapDescriptor splits text into lines no wider than width display cells.
// Segments are broken after each ',' so keys are never split mid-origin. A
// segment wider than width is balanced into equal-width lines, so the block
// reads as a rectangle instead of one full line plus a short stub.
func wrapDescriptor(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	lines := make([]string, 0, len(text)/width+1)
	for _, segment := range descriptorSegments(text) {
		segmentWidth := ansi.StringWidth(segment)
		if segmentWidth <= width {
			lines = append(lines, segment)
			continue
		}
		chunks := (segmentWidth + width - 1) / width
		target := (segmentWidth + chunks - 1) / chunks
		remaining := segment
		for i := 0; i < chunks-1 && remaining != ""; i++ {
			cut := cutDisplayCells(remaining, target)
			lines = append(lines, remaining[:cut])
			remaining = remaining[cut:]
		}
		if remaining != "" {
			lines = append(lines, remaining)
		}
	}
	return lines
}

// cutDisplayCells returns the byte index of the longest prefix of s that fits
// in limit display cells. It always takes at least one rune.
func cutDisplayCells(s string, limit int) int {
	used := 0
	for i, r := range s {
		w := ansi.StringWidth(string(r))
		if used+w > limit {
			if i == 0 {
				return len(string(r))
			}
			return i
		}
		used += w
	}
	return len(s)
}

// renderDescriptorBox wraps, centers, and renders the descriptor text. Every
// line is centered within the box's inner width.
func (m SettingsModel) renderDescriptorBox() string {
	boxWidth := descriptorBoxWidth(m.width)
	inner := descriptorInnerWidth(boxWidth)
	lines := wrapDescriptor(m.descriptorText, inner)
	lineStyle := lipgloss.NewStyle().
		Foreground(styles.PastelOrange).
		Width(inner).
		AlignHorizontal(lipgloss.Center)
	for i, line := range lines {
		lines[i] = lineStyle.Render(line)
	}
	return styles.BoxStyle.
		Width(boxWidth).
		BorderForeground(styles.ElectricBlue).
		Padding(0, descriptorBoxPadding).
		MarginBottom(0).
		Render(strings.Join(lines, "\n"))
}

// viewDescriptorMenu displays the descriptor detail screen with QR code and cosigner info
func (m SettingsModel) viewDescriptorMenu() string {
	title := styles.ScreenHeaderStyle.Render("⚙  Output Descriptor")

	// Copy status - inline with header
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		msg := m.copyMessage
		if msg == "" {
			msg = "COPIED!"
		}
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ "+msg)
	}

	// QR code: use UR encoding if available, uppercase for QR alphanumeric mode efficiency.
	qrSection := ""
	qrData := m.descriptorText
	qrLabel := ""
	if m.descriptorUR != "" {
		// Uppercase UR triggers QR alphanumeric mode (smaller QR code)
		qrData = strings.ToUpper(m.descriptorUR)
		qrLabel = styles.DimText.Render("  ur:crypto-output (Sparrow-compatible)") + "\n"
	}
	qrCode, err := ui.GenerateQRCodeHalfBlocks(qrData)
	if err == nil {
		qrSection = styles.BoxStyle.
			Width(styles.ClampWidth(78, m.width)+2).
			BorderForeground(styles.NeonPurple).
			Padding(0, 0).
			MarginBottom(0).
			Render(qrLabel + qrCode)
	}

	// Descriptor text - wrap and center each line inside the box.
	descriptorBox := m.renderDescriptorBox()

	// Cosigner table (only for multisig)
	cosignerSection := ""
	if m.walletManager.IsMultisig() {
		cosigners := m.walletManager.GetCosigners()
		if len(cosigners) > 0 {
			var rows []string
			for i, cs := range cosigners {
				name := cs.Name
				if name == "" {
					name = fmt.Sprintf("Cosigner %d", i+1)
				}
				fp := cs.Fingerprint
				if fp == "" {
					fp = "--------"
				}
				device := cs.DeviceType
				if device == "" && cs.IsMine {
					device = "local"
				}
				row := fmt.Sprintf("  %d. %-12s [%s]", i+1, name, fp)
				if device != "" {
					row += " " + styles.DimText.Render(device)
				}
				rows = append(rows, row)
			}

			cosignerSection = styles.BoxStyle.
				Width(styles.ClampWidth(78, m.width)+2).
				BorderForeground(styles.ElectricBlue).
				Padding(0, 2).
				MarginBottom(0).
				Render(
					styles.BoldText.Render("Cosigners") + "\n" +
						strings.Join(rows, "\n"),
				)
		}
	}

	// Help footer - show appropriate export label
	exportLabel := "export .txt"
	if m.walletManager.IsMultisig() {
		exportLabel = "export .json"
	}
	help := styles.RenderHelp(
		styles.HK("c", "copy descriptor"),
		styles.HK("e", exportLabel),
		styles.HK("esc", "back"),
	)

	parts := []string{
		"",
		title + copyStatus,
	}
	if qrSection != "" {
		parts = append(parts, "", qrSection)
	}
	parts = append(parts, "", descriptorBox)
	if cosignerSection != "" {
		parts = append(parts, "", cosignerSection)
	}
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// viewMnemonicWarning shows security warning before revealing mnemonic
func (m SettingsModel) viewMnemonicWarning() string {
	warningStyle := lipgloss.NewStyle().
		Foreground(styles.PastelPink).
		Bold(true)

	title := warningStyle.Render("⚠️  SECURITY WARNING ⚠️")

	warnings := []string{
		"Your seed phrase (mnemonic) is the MASTER KEY to your wallet.",
		"Anyone with this phrase can steal ALL your funds.",
		"",
		"NEVER share your seed phrase with anyone.",
		"NEVER enter it on websites or apps.",
		"NEVER take a screenshot or photo.",
		"",
		"Only view this in a private, secure location.",
	}

	warningText := ""
	for _, w := range warnings {
		if w == "" {
			warningText += "\n"
		} else {
			warningText += "  • " + w + "\n"
		}
	}

	warningBox := styles.BoxStyle.
		Width(styles.ClampWidth(60, m.width) + 2).
		BorderForeground(styles.PastelPink).
		Render(warningText)

	question := styles.HighlightText.Render("Do you want to reveal your seed phrase?")

	options := lipgloss.NewStyle().
		Foreground(styles.ElectricBlue).
		Render("[Y] Yes, show my seed phrase    [N] No, go back")

	return lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		"",
		warningBox,
		"",
		question,
		"",
		options,
	)
}

// viewMnemonic displays the mnemonic seed phrase
func (m SettingsModel) viewMnemonic() string {
	title := lipgloss.NewStyle().
		Foreground(styles.PastelPink).
		Bold(true).
		Render("🔐 Your Seed Phrase")

	subtitle := lipgloss.NewStyle().
		Foreground(styles.DimText.GetForeground()).
		Italic(true).
		Render("Write this down and store it securely. Never share it.")

	// Format mnemonic as numbered words
	words := strings.Split(m.mnemonic, " ")
	var rows []string
	for i := 0; i < len(words); i += 4 {
		row := ""
		for j := 0; j < 4 && i+j < len(words); j++ {
			wordNum := i + j + 1
			word := words[i+j]
			row += fmt.Sprintf("%2d. %-12s  ", wordNum, word)
		}
		rows = append(rows, row)
	}

	mnemonicDisplay := strings.Join(rows, "\n")

	mnemonicBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(styles.BitcoinOrange).
		Padding(1, 2).
		Render(mnemonicDisplay)

	// Copy status
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copyStatus = styles.CopyStatusStyle.Render("✓ " + m.copyMessage)
	}

	help := styles.RenderHelp(
		styles.HK("esc", "hide & go back"),
	)

	return lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		subtitle,
		"",
		mnemonicBox,
		"",
		help,
		"",
		copyStatus,
	)
}

// saveFailedMessage is the status line shown when a setting applies for
// this session but could not be written to config.json.
func saveFailedMessage(err error) string {
	return "Not saved (applies until restart): " + err.Error()
}
