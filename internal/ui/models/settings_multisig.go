package models

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/ui/styles"
)

// viewCosignerList displays the list of cosigners with their signing status
func (m SettingsModel) viewCosignerList() string {
	title := styles.ScreenHeaderStyle.Render("🔐 Multisig Cosigners")

	config := m.walletManager.GetMultisigConfig()
	if config == nil {
		return title + "\n\nNo multisig configuration found."
	}

	subtitle := styles.DimText.Render(fmt.Sprintf("%d-of-%d multisig • Manage signing keys for each cosigner", config.M, config.N))

	// Build cosigner list
	var rows []string
	cosigners := m.walletManager.GetCosigners()

	labelStyle := styles.DimText.Width(14)
	valueStyle := lipgloss.NewStyle().Foreground(styles.PastelOrange)

	for i, cs := range cosigners {
		// Status indicator and text
		var statusIcon, statusText string
		var statusStyle lipgloss.Style

		switch {
		case cs.IsMine && cs.Mnemonic != "":
			statusIcon = "🟢"
			statusText = "Local (can sign)"
			statusStyle = lipgloss.NewStyle().Foreground(styles.PastelGreen)
		case cs.DeviceType == "coldcard" || cs.DeviceType == "trezor" || cs.DeviceType == "ledger":
			statusIcon = "🔷"
			statusText = fmt.Sprintf("Hardware (%s)", cs.DeviceType)
			statusStyle = lipgloss.NewStyle().Foreground(styles.ElectricBlue)
		default:
			statusIcon = "⚪"
			statusText = "External (watch-only)"
			statusStyle = styles.DimText
		}

		// Name/label
		name := cs.Name
		if name == "" {
			name = fmt.Sprintf("Keystore %d", i+1)
		}

		// Fingerprint
		fingerprint := cs.Fingerprint
		if fingerprint == "" {
			fingerprint = "00000000"
		}

		// Derivation path - prefer ActualDerivation if set (shows real path used)
		derivation := "m/48'/1'/0'/2'"
		if cs.ActualDerivation != "" {
			derivation = cs.ActualDerivation + " (actual)"
		} else if cs.KeyOrigin != "" {
			// Extract just the path part (after fingerprint/)
			parts := strings.SplitN(cs.KeyOrigin, "/", 2)
			if len(parts) > 1 {
				derivation = "m/" + parts[1]
			}
		}

		// Truncate xpub for display
		xpubDisplay := cs.Xpub
		if len(xpubDisplay) > 50 {
			xpubDisplay = xpubDisplay[:25] + "..." + xpubDisplay[len(xpubDisplay)-22:]
		}

		// Build the row like Sparrow's keystore view
		row := fmt.Sprintf("  %s  %s\n", statusIcon, lipgloss.NewStyle().Bold(true).Render(name))
		row += fmt.Sprintf("      %s %s\n", labelStyle.Render("Fingerprint:"), lipgloss.NewStyle().Foreground(styles.NeonPurple).Render(fingerprint))
		row += fmt.Sprintf("      %s %s\n", labelStyle.Render("Derivation:"), styles.DimText.Render(derivation))
		row += fmt.Sprintf("      %s %s\n", labelStyle.Render("xpub:"), valueStyle.Render(xpubDisplay))
		row += fmt.Sprintf("      %s %s", labelStyle.Render("Status:"), statusStyle.Render(statusText))

		rows = append(rows, row)
	}

	cosignerList := styles.BoxStyle.
		Width(styles.ClampWidth(78, m.width)+2).
		BorderForeground(styles.ElectricBlue).
		Padding(1, 2).
		Render(strings.Join(rows, "\n\n"))

	// Signing summary (get this first for use in potential matches logic)
	localCount, hardware, external := m.walletManager.GetMultisigSigningStatus()
	summaryText := fmt.Sprintf("Signing: %d local, %d hardware, %d external",
		localCount, hardware, external)
	if localCount >= config.M {
		summaryText += " • " + lipgloss.NewStyle().
			Foreground(styles.PastelGreen).
			Render("Can sign locally!")
	}
	summary := styles.DimText.Render(summaryText)

	// Success/error messages
	statusMsg := ""
	if m.cosignerImportSuccess != "" {
		statusMsg = styles.CopyStatusStyle.Render("✓ " + m.cosignerImportSuccess)
	}
	if m.cosignerImportError != "" {
		statusMsg = lipgloss.NewStyle().
			Foreground(styles.PastelPink).
			Bold(true).
			Render("⚠ " + m.cosignerImportError)
	}

	help := styles.RenderHelp(
		styles.HK("1-"+fmt.Sprintf("%d", len(cosigners)), "edit"),
		styles.HK("i", "import seed"),
		styles.HK("s", "scan local"),
		styles.HK("esc", "back"),
	)

	fixedHeader := "\n" + title + "\n" + subtitle

	scrollContent := cosignerList + "\n" +
		summary + "\n" +
		statusMsg

	return renderScrollableView(m.height, m.scrollOffset, fixedHeader, scrollContent, help)
}

// viewCosignerEdit displays the edit screen for a single cosigner
func (m SettingsModel) viewCosignerEdit() string {
	title := styles.ScreenHeaderStyle.Render(fmt.Sprintf("✏️  Edit Cosigner %d", m.editingCosignerIndex+1))

	cosigners := m.walletManager.GetCosigners()
	if m.editingCosignerIndex >= len(cosigners) {
		return title + "\n\nInvalid cosigner index"
	}

	cs := cosigners[m.editingCosignerIndex]

	subtitle := styles.DimText.Render("Update name and fingerprint for this cosigner")

	// Show current xpub (read-only)
	xpubDisplay := cs.Xpub
	if len(xpubDisplay) > 50 {
		xpubDisplay = xpubDisplay[:25] + "..." + xpubDisplay[len(xpubDisplay)-22:]
	}
	xpubInfo := styles.DimText.Render("xpub: ") + lipgloss.NewStyle().Foreground(styles.PastelOrange).Render(xpubDisplay)

	// Input fields
	labelStyle := lipgloss.NewStyle().Width(14).Foreground(styles.ElectricBlue)

	nameField := labelStyle.Render("Name:") + " " + m.cosignerNameInput.View()
	fpField := labelStyle.Render("Fingerprint:") + " " + m.cosignerFingerprintInput.View()

	// Field indicator
	indicator := ""
	if m.cosignerEditField == 0 {
		nameField = "▶ " + nameField
		fpField = "  " + fpField
	} else {
		nameField = "  " + nameField
		fpField = "▶ " + fpField
	}

	inputBox := styles.BoxStyle.
		Width(styles.ClampWidth(70, m.width)+2).
		BorderForeground(styles.ElectricBlue).
		Padding(1, 2).
		Render(xpubInfo + "\n\n" + nameField + "\n\n" + fpField + indicator)

	// Error message
	errorMsg := ""
	if m.cosignerImportError != "" {
		errorMsg = lipgloss.NewStyle().
			Foreground(styles.PastelPink).
			Bold(true).
			Render("⚠ " + m.cosignerImportError)
	}

	help := styles.RenderHelp(
		styles.HK("tab", "switch field"),
		styles.HK("enter", "save"),
		styles.HK("esc", "cancel"),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		title,
		subtitle,
		"",
		inputBox,
		"",
		errorMsg,
		"",
		help,
	)
}

// viewCosignerSeedInput displays the seed phrase input for importing a cosigner key
func (m SettingsModel) viewCosignerSeedInput() string {
	title := styles.ScreenHeaderStyle.Render("🔐 Import Cosigner Seed")

	subtitle := styles.DimText.Render("Enter the seed phrase for one of your cosigners")

	// Security warning
	warning := styles.StatusErrorStyle.Render(
		"⚠ This seed will be stored encrypted with your wallet.\n" +
			"  Only import seeds you control and trust this device with.",
	)

	// Show fingerprints we're looking for
	config := m.walletManager.GetMultisigConfig()
	fingerprintInfo := ""
	if config != nil {
		var fingerprints []string
		for _, cs := range config.Cosigners {
			if cs.Fingerprint != "" && cs.Mnemonic == "" {
				fingerprints = append(fingerprints, cs.Fingerprint)
			}
		}
		if len(fingerprints) > 0 {
			fingerprintInfo = styles.DimText.Render(
				"Looking for fingerprints: " + strings.Join(fingerprints, ", "),
			)
		}
	}

	// Input box
	inputBox := styles.BoxStyle.
		Width(styles.ClampWidth(70, m.width)+2).
		BorderForeground(styles.ElectricBlue).
		Padding(1, 2).
		Render(m.cosignerSeedInput.View())

	// Error message
	errorMsg := ""
	if m.cosignerImportError != "" {
		errorMsg = lipgloss.NewStyle().
			Foreground(styles.PastelPink).
			Bold(true).
			Render("⚠ " + m.cosignerImportError)
	}

	help := styles.RenderHelp(
		styles.HK("enter", "import"),
		styles.HK("esc", "cancel"),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		title,
		subtitle,
		"",
		warning,
		"",
		fingerprintInfo,
		"",
		inputBox,
		"",
		errorMsg,
		"",
		help,
	)
}

// viewSingleSigSeedInput displays the seed import screen for watch-only single-sig wallets
func (m SettingsModel) viewSingleSigSeedInput() string {
	title := styles.ScreenHeaderStyle.Render("🔐 Import Seed Phrase")

	subtitle := styles.DimText.Render("Enter the seed phrase that matches this wallet's xpub")

	// Security warning
	warning := styles.StatusErrorStyle.Render(
		"⚠ This will convert your watch-only wallet to a full signing wallet.\n" +
			"  The seed will be stored encrypted with your wallet password.",
	)

	// Show the xpub we're matching against
	xpubInfo := ""
	if m.walletManager.IsHardwareWallet() {
		hwConfig := m.walletManager.GetHardwareWalletConfig()
		if hwConfig != nil {
			xpubShort := hwConfig.Xpub
			if len(xpubShort) > 30 {
				xpubShort = xpubShort[:15] + "..." + xpubShort[len(xpubShort)-10:]
			}
			xpubInfo = styles.DimText.Render("Matching xpub: " + xpubShort)
		}
	}

	// Input box
	inputBox := styles.BoxStyle.
		Width(styles.ClampWidth(70, m.width)+2).
		BorderForeground(styles.ElectricBlue).
		Padding(1, 2).
		Render(m.cosignerSeedInput.View())

	// Error message
	errorMsg := ""
	if m.singleSigImportError != "" {
		errorMsg = lipgloss.NewStyle().
			Foreground(styles.PastelPink).
			Bold(true).
			Render("⚠ " + m.singleSigImportError)
	}

	help := styles.RenderHelp(
		styles.HK("enter", "import"),
		styles.HK("esc", "cancel"),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		title,
		subtitle,
		"",
		warning,
		"",
		xpubInfo,
		"",
		inputBox,
		"",
		errorMsg,
		"",
		help,
	)
}

// scanAndImportCosigners scans other local wallets (decrypted with password)
// for seeds matching this multisig's cosigners and imports them.
func (m *SettingsModel) scanAndImportCosigners(password string) {
	if password == "" {
		m.cosignerImportError = "Password required to scan local wallets"
		return
	}
	matches, err := m.walletManager.ScanLocalWalletsForCosigners(password)
	if err != nil {
		m.cosignerImportError = fmt.Sprintf("Scan failed: %v", err)
		return
	}
	if len(matches) == 0 {
		m.cosignerImportError = "No matching seeds found in local wallets with that password"
		return
	}
	imported := 0
	for _, match := range matches {
		if _, err := m.walletManager.ImportCosignerFromLocalWallet(match.WalletName, password); err == nil {
			imported++
		}
	}
	if imported > 0 {
		m.cosignerImportSuccess = fmt.Sprintf("Imported seeds from %d local wallet(s)!", imported)
		m.cosignerImportError = ""
	} else {
		m.cosignerImportError = "Found matches but failed to import"
	}
}

func (m SettingsModel) viewCosignerScanPassword() string {
	title := styles.TitleStyle.Render("Scan Local Wallets")
	info := styles.DimText.Render("Enter the password of the local wallets to scan for matching cosigner seeds.\nWallets with a different password are skipped.")
	box := styles.BoxStyle.
		Width(styles.ClampWidth(70, m.width)+2).
		BorderForeground(styles.ElectricBlue).
		Padding(1, 2).
		Render(m.cosignerScanPassword.View())
	help := styles.RenderHelp(styles.HK("enter", "scan"), styles.HK("esc", "cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, "", title, info, "", box, help)
}
