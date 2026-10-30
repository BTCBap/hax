package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m WalletSetupModel) viewMultisigChooseType() string {
	var title string
	if m.isRestore {
		title = styles.TitleStyle.Render("Import Multisig" + m.stepCounterSuffix())
	} else {
		title = ""
	}

	var menuContent string

	// M-of-N section with inline adjustment
	menuContent += styles.BoldText.Render("Signature Scheme:") + "\n"

	// Row 0: M (required signatures)
	mLabel := fmt.Sprintf("Required signatures (M): ◄ %2d ►", m.multisigM)
	if m.cursor == 0 {
		menuContent += styles.SelectedItemStyle.Render("▶ "+mLabel) + "\n"
	} else {
		menuContent += styles.UnselectedItemStyle.Render("  "+mLabel) + "\n"
	}

	// Row 1: N (total cosigners)
	nLabel := fmt.Sprintf("Total cosigners (N):     ◄ %2d ►", m.multisigN)
	if m.cursor == 1 {
		menuContent += styles.SelectedItemStyle.Render("▶ "+nLabel) + "\n"
	} else {
		menuContent += styles.UnselectedItemStyle.Render("  "+nLabel) + "\n"
	}

	menuContent += styles.DimText.Render(fmt.Sprintf("  %2d-of-%2d multisig", m.multisigM, m.multisigN)) + "\n"

	// Script type options
	scriptTypes := []struct {
		name       string
		scriptType wallet.MultisigScriptType
	}{
		{"Native SegWit (P2WSH) - Recommended", wallet.MultisigNativeSegwit},
		{"Nested SegWit (P2SH-P2WSH)", wallet.MultisigNestedSegwit},
		{"Legacy (P2SH)", wallet.MultisigLegacy},
	}

	menuContent += "\n" + styles.BoldText.Render("Script Type:") + "\n"
	for i, opt := range scriptTypes {
		idx := i + 2 // offset past M and N rows
		cursor := " "
		style := styles.UnselectedItemStyle
		if m.cursor == idx {
			cursor = "▶"
			style = styles.SelectedItemStyle
		}
		check := "  "
		if m.multisigScriptType == opt.scriptType && m.multisigCustomDerivPath == "" {
			check = " ✓"
		}
		menuContent += style.Render(cursor+" "+opt.name+check) + "\n"
	}

	// Custom Path option (cursor 5)
	customCursor := " "
	customStyle := styles.UnselectedItemStyle
	if m.cursor == 5 {
		customCursor = "▶"
		customStyle = styles.SelectedItemStyle
	}
	customCheck := "  "
	if m.multisigCustomDerivPath != "" {
		customCheck = " ✓"
	}
	menuContent += customStyle.Render(customCursor+" Custom Path"+customCheck) + "\n"

	// Derivation path box
	menuContent += "\n"
	if m.showMultisigCustomPath {
		pathBox := styles.BoldText.Render("Derivation Path") + "\n" +
			lipgloss.NewStyle().Width(33).Render(m.customPathInput.View())
		errorLine := ""
		if m.err != nil {
			errorLine = lipgloss.NewStyle().MaxWidth(lipgloss.Width(menuContent)).
				Render(styles.ErrorText.Render(m.err.Error()))
		}
		pathBox += "\n" + errorLine
		pathBox += "\n" + styles.DimText.Render("enter: confirm • esc: cancel")
		menuContent += styles.InfoBoxStyle.MarginBottom(0).Padding(0, 1).Render(pathBox) + "\n"
	} else {
		effectivePath := m.multisigCustomDerivPath
		if effectivePath == "" {
			effectivePath = wallet.DefaultMultisigAccountPath(m.multisigScriptType, m.isTestnet())
		}
		pathBox := styles.BoldText.Render("Derivation Path") + "\n" +
			styles.NormalText.Render(effectivePath)
		menuContent += styles.InfoBoxStyle.MarginBottom(0).Padding(0, 1).Render(pathBox) + "\n"
	}

	help := styles.RenderHelp(
		styles.HK("←/→", "adjust"),
		styles.HK("↑/↓", "navigate"),
		styles.HK("space", "select"),
		styles.HK("enter", "continue"),
		styles.HK("esc", "back"),
	)

	prefix := ""
	if title != "" {
		prefix = title + "\n\n"
	}
	return styles.CenterHorizontal(prefix+menuContent+"\n"+help, m.width)
}

func (m WalletSetupModel) handleMultisigChooseType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// cursor: 0=M row, 1=N row, 2=Native SegWit, 3=Nested SegWit, 4=Legacy, 5=Custom Path
	totalOptions := 6

	// Handle custom path text input mode
	if m.showMultisigCustomPath {
		switch msg.String() {
		case "esc":
			m.showMultisigCustomPath = false
			m.customPathInput.SetValue("")
			m.customPathInput.Blur()
			m.err = nil
			// If no custom path was confirmed, clear it
			if m.multisigCustomDerivPath == "" {
				return m, nil
			}
			return m, nil
		case "enter":
			pathValue := strings.TrimSpace(m.customPathInput.Value())
			if pathValue != "" {
				if err := wallet.ValidateAccountPath(pathValue); err != nil {
					m.err = fmt.Errorf("invalid path: %w", err)
					return m, nil
				}
				m.multisigCustomDerivPath = pathValue
			}
			m.showMultisigCustomPath = false
			m.customPathInput.Blur()
			m.err = nil
			return m, nil
		default:
			var cmd tea.Cmd
			m.customPathInput, cmd = m.customPathInput.Update(msg)
			m.err = nil
			return m, cmd
		}
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < totalOptions-1 {
			m.cursor++
		}
	case "left", "h":
		// Adjust M or N down
		if m.cursor == 0 && m.multisigM > 1 {
			m.multisigM--
		} else if m.cursor == 1 && m.multisigN > 2 {
			m.multisigN--
			if m.multisigM > m.multisigN {
				m.multisigM = m.multisigN
			}
		}
	case "right", "l":
		// Adjust M or N up
		if m.cursor == 0 && m.multisigM < m.multisigN {
			m.multisigM++
		} else if m.cursor == 1 && m.multisigN < 15 {
			m.multisigN++
		}
	case " ":
		// Spacebar selects
		switch m.cursor {
		case 2:
			m.multisigScriptType = wallet.MultisigNativeSegwit
			m.multisigCustomDerivPath = ""
		case 3:
			m.multisigScriptType = wallet.MultisigNestedSegwit
			m.multisigCustomDerivPath = ""
		case 4:
			m.multisigScriptType = wallet.MultisigLegacy
			m.multisigCustomDerivPath = ""
		case 5:
			// Open custom path input
			m.showMultisigCustomPath = true
			prefill := m.multisigCustomDerivPath
			if prefill == "" {
				prefill = wallet.DefaultMultisigAccountPath(m.multisigScriptType, m.isTestnet())
			}
			m.customPathInput.SetValue(prefill)
			return m, m.customPathInput.Focus()
		}
	case "enter":
		m.step = StepMultisigConfirm
		m.cursor = 0
	case "esc":
		m.step = m.getPreviousStep()
		m.cursor = 0
	}
	return m, nil
}

func (m WalletSetupModel) viewMultisigConfirm() string {
	title := styles.TitleStyle.Render("Confirm Configuration")

	var scriptTypeName string
	switch m.multisigScriptType {
	case wallet.MultisigNativeSegwit:
		scriptTypeName = "Native SegWit (P2WSH)"
	case wallet.MultisigNestedSegwit:
		scriptTypeName = "Nested SegWit (P2SH-P2WSH)"
	case wallet.MultisigLegacy:
		scriptTypeName = "Legacy (P2SH)"
	}

	effectivePath := m.multisigCustomDerivPath
	if effectivePath == "" {
		effectivePath = wallet.DefaultMultisigAccountPath(m.multisigScriptType, m.isTestnet())
	}

	summary := styles.InfoBoxStyle.Render(
		styles.BoldText.Render("Threshold: ") + fmt.Sprintf("%d-of-%d", m.multisigM, m.multisigN) + "\n" +
			styles.BoldText.Render("Script:    ") + scriptTypeName + "\n" +
			styles.BoldText.Render("Path:      ") + effectivePath)

	nextStep := styles.DimText.Render(fmt.Sprintf("You will enter %d cosigner xpubs next.", m.multisigN))

	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "go back"))

	return styles.CenterHorizontal(title+"\n\n"+summary+"\n\n"+nextStep+"\n\n"+help, m.width)
}

func (m WalletSetupModel) handleMultisigConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.step = StepMultisigEnterCosigners
		m.cursor = 0
		m.multisigCosigners = make([]wallet.MultisigCosignerConfig, 0)
		m.multisigCurrentXpub.SetValue("")
		return m, m.multisigCurrentXpub.Focus()
	case "esc":
		m.step = m.getPreviousStep()
		m.cursor = 0
	}
	return m, nil
}

func (m WalletSetupModel) viewMultisigEnterCosigners() string {
	// Handle HWI device selection overlay
	if m.showHWIDevices {
		return m.viewHWIDeviceSelection()
	}

	var titlePrefix string
	if m.isRestore {
		titlePrefix = "Import Multisig" + m.stepCounterSuffix() + " - "
	} else {
		titlePrefix = ""
	}
	title := styles.TitleStyle.Render(fmt.Sprintf("%sCosigner Keys (%d/%d)",
		titlePrefix, len(m.multisigCosigners)+1, m.multisigN))

	// Mode selector - 3 modes: xpub, seed, hardware
	var modeIndicator string
	switch m.multisigInputMode {
	case 0:
		modeIndicator = styles.HighlightText.Render("[xpub]") + " " +
			styles.DimText.Render("seed") + " " +
			styles.DimText.Render("hardware")
	case 1:
		modeIndicator = styles.DimText.Render("xpub") + " " +
			styles.HighlightText.Render("[seed]") + " " +
			styles.DimText.Render("hardware")
	case 2:
		modeIndicator = styles.DimText.Render("xpub") + " " +
			styles.DimText.Render("seed") + " " +
			styles.HighlightText.Render("[hardware]")
	}
	modeSelector := "Input mode: " + modeIndicator + "  " + styles.DimText.Render("(tab to switch, h for hardware)")

	// Show custom derivation path if set
	if m.multisigCustomDerivPath != "" {
		modeSelector += "\n" + styles.DimText.Render("Custom path: "+m.multisigCustomDerivPath)
	}

	// Show already added cosigners
	var cosignerList string
	for i, cs := range m.multisigCosigners {
		name := cs.Name
		if name == "" {
			name = fmt.Sprintf("Cosigner %d", i+1)
		}
		// Show fingerprint and indicate if we have signing capability
		var keyInfo string
		if cs.KeyOrigin != "" {
			keyInfo = fmt.Sprintf("[%s]", cs.Fingerprint)
		} else if cs.Fingerprint != "" {
			keyInfo = fmt.Sprintf("[%s]", cs.Fingerprint)
		} else {
			keyInfo = "(no key origin)"
		}
		if cs.IsMine {
			keyInfo += " ✓can sign"
		}
		// Show truncated xpub
		xpubShort := cs.Xpub
		if len(xpubShort) > 16 {
			xpubShort = xpubShort[:8] + "..." + xpubShort[len(xpubShort)-8:]
		}
		cosignerList += styles.SuccessText.Render(fmt.Sprintf("✓ %s %s %s", name, keyInfo, xpubShort)) + "\n"
	}

	// Current input based on mode
	var inputSection string
	inputLabel := styles.NormalText.Render(fmt.Sprintf("Cosigner %d:", len(m.multisigCosigners)+1))

	switch m.multisigInputMode {
	case 0:
		// xpub mode
		info := styles.DimText.Render(
			"Paste the BIP48 xpub from the cosigner's wallet.\n" +
				"Just the tpub is fine, or with key origin: [fingerprint/48h/...]tpub...")
		inputSection = inputLabel + " (xpub)\n" + info + "\n" +
			lipgloss.NewStyle().Width(63).Render(m.multisigCurrentXpub.View())
	case 1:
		// Seed mode
		derivInfo := "The BIP48 xpub will be derived automatically."
		if m.multisigCustomDerivPath != "" {
			derivInfo = fmt.Sprintf("The xpub will be derived at %s.", m.multisigCustomDerivPath)
		}
		info := styles.DimText.Render(
			"Enter the cosigner's seed phrase (12 or 24 words).\n" + derivInfo)
		inputSection = inputLabel + " (seed phrase)\n" + info + "\n" +
			lipgloss.NewStyle().Width(63).Render(m.multisigCosignerSeed.View())
	case 2:
		// Hardware wallet mode
		derivInfo := "The BIP48 xpub will be retrieved from the device."
		if m.multisigCustomDerivPath != "" {
			derivInfo = fmt.Sprintf("The xpub will be retrieved at %s.", m.multisigCustomDerivPath)
		}
		info := styles.DimText.Render(
			"Connect a hardware wallet (Trezor, Ledger, Coldcard, etc.)\n" + derivInfo)

		// Keep one line for the detection status so it can appear or clear
		// without moving the scan prompt and help below it.
		statusLine := "\n"
		if m.hwiLoading {
			statusLine = "\n" + m.spinner.View() + " Detecting hardware wallets..."
		} else if m.hwiError != nil {
			statusLine = "\n" + styles.ErrorText.Render("⚠ "+m.hwiError.Error())
		}

		inputSection = inputLabel + " (hardware wallet)\n" + info + statusLine +
			"\n\n" + styles.NormalText.Render("Press Enter to scan for devices")
	}

	help := styles.RenderHelp(
		styles.HK("enter", "add cosigner"),
		styles.HK("tab", "switch mode"),
		styles.HK("h", "hardware"),
		styles.HK("esc", "back"),
	)

	content := title + "\n" + modeSelector + "\n\n"
	if cosignerList != "" {
		content += cosignerList + "\n"
	}
	content += inputSection + "\n\n" + help

	// Show general errors without widening the centered block.
	if m.err != nil {
		content += "\n" + styles.RenderErrorBox("⚠ "+m.err.Error(), lipgloss.Width(content)+4)
	}

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewHWIDeviceSelection() string {
	title := styles.TitleStyle.Render("🔌 Select Hardware Wallet")
	subtitle := styles.SubtitleStyle.Render("Choose device to get xpub from")

	var content strings.Builder

	if m.hwiLoading {
		content.WriteString(m.spinner.View() + " Detecting hardware wallets...\n")
	} else if m.hwiError != nil {
		content.WriteString(styles.RenderErrorBox("⚠ "+m.hwiError.Error(), m.width) + "\n\n")
		content.WriteString(renderHWITroubleshootingHints(hwiErrorHints(m.hwiError)))
		content.WriteString(styles.DimText.Render("Press 'r' to retry or 'esc' to go back.") + "\n")
	} else if len(m.hwiDevices) == 0 {
		content.WriteString(styles.DimText.Render("No hardware wallets detected.") + "\n\n")
		content.WriteString(renderHWITroubleshootingHints(wallet.DeviceConnectionHints()))
		content.WriteString(styles.DimText.Render("Press 'r' to retry or 'esc' to go back.") + "\n")
	} else {
		content.WriteString(styles.NormalText.Render("Connected devices:") + "\n\n")
		content.WriteString(renderDeviceMenu(hardwareDeviceChoices(m.hwiDevices), m.selectedHWDevice))
	}

	instructions := styles.DimText.Render(
		"\nThe device will display the xpub derivation.\n" +
			"Confirm on your hardware wallet if prompted.")

	help := styles.RenderHelp(
		styles.HK("↑/↓", "select"),
		styles.HK("enter", "get xpub"),
		styles.HK("r", "retry"),
		styles.HK("esc", "back"),
	)

	return title + "\n" + subtitle + "\n\n" +
		content.String() +
		instructions + "\n\n" +
		help
}

func (m WalletSetupModel) handleMultisigEnterCosigners(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Handle HWI device selection mode
	if m.showHWIDevices {
		switch msg.String() {
		case "up", "k":
			if m.selectedHWDevice > 0 {
				m.selectedHWDevice--
			}
			return m, nil
		case "down", "j":
			if m.selectedHWDevice < len(m.hwiDevices)-1 {
				m.selectedHWDevice++
			}
			return m, nil
		case "enter":
			// Get xpub from selected device
			if m.selectedHWDevice >= 0 && m.selectedHWDevice < len(m.hwiDevices) {
				m.hwiLoading = true
				m.hwiError = nil
				return m, tea.Batch(m.spinner.Tick, m.getXpubFromHWI(m.hwiDevices[m.selectedHWDevice]))
			}
			return m, nil
		case "r", "R":
			// Retry device enumeration
			m.hwiLoading = true
			m.hwiError = nil
			m.hwiDevices = nil
			m.selectedHWDevice = 0
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSetup())
		case "esc":
			m.showHWIDevices = false
			m.hwiError = nil
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "tab":
		// Switch input mode: 0=xpub, 1=seed, 2=hardware wallet
		m.multisigInputMode = (m.multisigInputMode + 1) % 3
		m.err = nil // Clear any errors when switching modes
		// Focus the appropriate input
		switch m.multisigInputMode {
		case 0:
			m.multisigCurrentXpub.Focus()
			m.multisigCosignerSeed.Blur()
		case 1:
			m.multisigCosignerSeed.Focus()
			m.multisigCurrentXpub.Blur()
		default:
			// Hardware wallet mode - no text input focused
			m.multisigCurrentXpub.Blur()
			m.multisigCosignerSeed.Blur()
		}
		return m, nil

	case "h", "H":
		// Shortcut to switch to hardware wallet mode and enumerate devices
		if m.multisigInputMode != 2 {
			m.multisigInputMode = 2
			m.multisigCurrentXpub.Blur()
			m.multisigCosignerSeed.Blur()
		}
		m.hwiLoading = true
		m.hwiError = nil
		return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSetup())

	case "enter":
		switch m.multisigInputMode {
		case 0:
			// xpub mode
			xpubVal := m.multisigCurrentXpub.Value()
			if xpubVal != "" {
				// Parse and validate the cosigner xpub with detailed validation
				result := wallet.ValidateCosignerInput(xpubVal)
				if result.Error != nil {
					m.err = result.Error
					return m, nil
				}

				// Show warning but allow continuation (user can acknowledge)
				if result.Warning != "" && m.multisigWarningAcked != xpubVal {
					// First time seeing this warning - show it and wait for user to press enter again
					m.err = fmt.Errorf("%s\n\nPress Enter again to add anyway, or fix the xpub", result.Warning)
					m.multisigWarningAcked = xpubVal // Remember we showed warning for this input
					return m, nil
				}

				// Check for duplicate xpub
				if m.hasDuplicateCosignerXpub(result.Config.Xpub) {
					m.err = fmt.Errorf("this key is already added as a cosigner")
					return m, nil
				}
				cs := result.Config
				cs.Name = fmt.Sprintf("Cosigner %d", len(m.multisigCosigners)+1)
				m.multisigCosigners = append(m.multisigCosigners, cs)
				m.multisigCurrentXpub.SetValue("")
				m.multisigWarningAcked = "" // Reset for next cosigner
				m.err = nil
			}
		case 1:
			// Seed mode - derive BIP48 xpub from mnemonic
			seedVal := strings.TrimSpace(m.multisigCosignerSeed.Value())
			if seedVal != "" {
				// Derive cosigner config from mnemonic
				cs, err := wallet.DeriveMultisigCosignerFromMnemonic(
					seedVal,
					m.multisigScriptType,
					m.isTestnet(),
					m.multisigCustomDerivPath,
				)
				if err != nil {
					m.err = err
					return m, nil
				}

				// Check for duplicate xpub
				if m.hasDuplicateCosignerXpub(cs.Xpub) {
					m.err = fmt.Errorf("this key is already added as a cosigner")
					return m, nil
				}
				cs.Name = fmt.Sprintf("Cosigner %d", len(m.multisigCosigners)+1)
				m.multisigCosigners = append(m.multisigCosigners, cs)
				m.multisigCosignerSeed.SetValue("")
				m.err = nil
			}
		case 2:
			// Hardware wallet mode - enumerate devices
			m.hwiLoading = true
			m.hwiError = nil
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSetup())
		}

		// Check if we have all cosigners
		if len(m.multisigCosigners) >= m.multisigN {
			m.step = StepMultisigReview
			m.cursor = 0
			return m, nil
		}

	case "esc":
		m.hwiError = nil
		m.err = nil
		if len(m.multisigCosigners) > 0 {
			// Remove last cosigner
			m.multisigCosigners = m.multisigCosigners[:len(m.multisigCosigners)-1]
		} else {
			m.step = m.getPreviousStep()
			m.cursor = 0
		}
	default:
		// Update the active input
		if m.multisigInputMode == 0 {
			m.multisigCurrentXpub, cmd = m.multisigCurrentXpub.Update(msg)
		} else {
			m.multisigCosignerSeed, cmd = m.multisigCosignerSeed.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m WalletSetupModel) viewMultisigReview() string {
	title := styles.TitleStyle.Render("Review Multisig Wallet")

	// Summary
	var scriptTypeName string
	switch m.multisigScriptType {
	case wallet.MultisigNativeSegwit:
		scriptTypeName = "Native SegWit (P2WSH)"
	case wallet.MultisigNestedSegwit:
		scriptTypeName = "Nested SegWit (P2SH-P2WSH)"
	case wallet.MultisigLegacy:
		scriptTypeName = "Legacy (P2SH)"
	}

	summaryText := styles.BoldText.Render("Wallet: ") + m.walletName + "\n" +
		styles.BoldText.Render("Type: ") + fmt.Sprintf("%d-of-%d Multisig", m.multisigM, m.multisigN) + "\n" +
		styles.BoldText.Render("Script: ") + scriptTypeName
	if m.multisigCustomDerivPath != "" {
		summaryText += "\n" + styles.BoldText.Render("Path: ") + m.multisigCustomDerivPath
	}
	summary := styles.InfoBoxStyle.Render(summaryText)

	// Cosigner list
	var cosignerList string
	for i, cs := range m.multisigCosigners {
		xpubShort := cs.Xpub
		if len(xpubShort) > 30 {
			xpubShort = xpubShort[:15] + "..." + xpubShort[len(xpubShort)-15:]
		}
		cosignerList += fmt.Sprintf("%d. %s\n   %s\n", i+1, cs.Name, xpubShort)
	}

	warning := styles.WarningText.Render("⚠ This creates a watch-only wallet. You'll need " +
		fmt.Sprintf("%d signatures to spend.", m.multisigM))

	help := styles.RenderHelp(styles.HK("enter", "create wallet"), styles.HK("esc", "back"))

	scrollContent := summary + "\n\n" +
		styles.BoldText.Render("Cosigners:") + "\n" + cosignerList + "\n" +
		warning

	return styles.CenterHorizontal(
		renderScrollableView(m.height, m.scrollOffset, title, scrollContent, help),
		m.width,
	)
}

func (m WalletSetupModel) handleMultisigReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Create the multisig wallet
		m.scrollOffset = 0
		err := m.walletManager.CreateMultisigWalletWithOrderOnNetwork(
			m.walletName,
			m.password,
			m.multisigM,
			m.multisigCosigners,
			m.multisigScriptType,
			m.network,
			m.parsedDescriptor != nil && m.parsedDescriptor.Unsorted,
		)
		if err != nil {
			m.err = err
			return m, nil
		}

		// Unlock the wallet
		err = m.walletManager.UnlockWallet(m.walletName, m.password)
		if err != nil {
			m.err = fmt.Errorf("wallet created but failed to unlock: %w", err)
			return m, nil
		}

		// Save as last wallet
		m.saveLastWalletName(m.walletName)
		m.flowComplete = true

		// Navigate to dashboard
		return m, func() tea.Msg {
			return NavigateMsg{Target: StateMainDashboard}
		}

	case "esc":
		m.scrollOffset = 0
		m.step = m.getPreviousStep()
	case "up", "k":
		m.scrollOffset -= 3
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	case "down", "j":
		m.scrollOffset += 3
	case "pgup":
		m.scrollOffset -= m.height / 2
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	case "pgdown":
		m.scrollOffset += m.height / 2
	}
	return m, nil
}

// hasDuplicateCosignerXpub checks if an xpub is already in the cosigner list
func (m WalletSetupModel) hasDuplicateCosignerXpub(xpub string) bool {
	for _, cs := range m.multisigCosigners {
		if cs.Xpub == xpub {
			return true
		}
	}
	return false
}

// enumerateHWIDevicesForSetup lists connected hardware wallets for wallet setup
func (m WalletSetupModel) enumerateHWIDevicesForSetup() tea.Cmd {
	walletMgr := m.walletManager

	return func() tea.Msg {
		devices, err := walletMgr.ListHardwareWallets()
		return HWIDevicesMsg{Devices: devices, Err: err}
	}
}

// getXpubFromHWI retrieves the xpub from a hardware wallet at the appropriate path
func (m WalletSetupModel) getXpubFromHWI(device wallet.HardwareDevice) tea.Cmd {
	scriptType := m.multisigScriptType
	customPath := m.multisigCustomDerivPath
	testnet := m.isTestnet()

	return func() tea.Msg {
		hwi := wallet.NewHWIClient(testnet)
		if hwi == nil || !hwi.IsAvailable() {
			return HWIXpubMsg{Err: fmt.Errorf("HWI not installed. %s", wallet.HWIInstallInstructions())}
		}

		// Use custom path if set, otherwise standard path for script type
		var path string
		if customPath != "" {
			path = customPath
		} else {
			path = wallet.DefaultMultisigAccountPath(scriptType, testnet)
		}

		xpub, err := hwi.GetXpub(device, path)
		if err != nil {
			return HWIXpubMsg{Err: fmt.Errorf("failed to get xpub: %w", err)}
		}

		return HWIXpubMsg{
			Xpub:        xpub,
			Fingerprint: device.Fingerprint,
			Err:         nil,
		}
	}
}

func (m WalletSetupModel) handleMultisigImportDescriptor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "enter":
		// Parse the descriptor
		descriptorText := strings.TrimSpace(m.descriptorInput.Value())
		if descriptorText == "" {
			m.err = fmt.Errorf("please paste a descriptor")
			return m, nil
		}

		result, err := wallet.ParseDescriptor(descriptorText)
		if err != nil {
			m.err = err
			return m, nil
		}

		// Store parsed data
		m.descriptorWarnings = result.Warnings
		m.descriptorIsMultisig = result.IsMultisig
		m.err = nil

		if result.IsMultisig {
			// Multisig descriptor
			m.isMultisig = true
			m.parsedDescriptor = result.MultisigConfig
			m.parsedSingleSig = nil
			m.multisigM = result.MultisigConfig.M
			m.multisigN = result.MultisigConfig.N
			m.multisigScriptType = wallet.MultisigScriptType(result.MultisigConfig.ScriptType)
			m.multisigCosigners = result.MultisigConfig.Cosigners
		} else {
			// Single-sig descriptor
			m.isMultisig = false
			m.parsedSingleSig = result.SingleSigConfig
			m.parsedDescriptor = nil
			m.selectedScriptType = result.SingleSigConfig.AddressType
			m.hwDerivedXpub = result.SingleSigConfig.Xpub
			m.hwFingerprint = result.SingleSigConfig.Fingerprint
		}

		// Move to review step
		m.step = StepMultisigImportReview
		m.nameInput.SetValue("")
		return m, m.nameInput.Focus()

	case "esc":
		m.step = StepRestoreChooseMethod
		m.cursor = 2 // Stay on "Import from Descriptor" option
		m.parsedDescriptor = nil
		m.parsedSingleSig = nil
		m.descriptorWarnings = nil
		m.err = nil
		return m, nil

	default:
		// Update text input
		m.descriptorInput, cmd = m.descriptorInput.Update(msg)
		return m, cmd
	}
}

func (m WalletSetupModel) handleMultisigImportReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "enter":
		walletName := strings.TrimSpace(m.nameInput.Value())
		if walletName == "" {
			m.err = fmt.Errorf("please enter a wallet name")
			return m, nil
		}
		if err := wallet.ValidateWalletName(walletName); err != nil {
			m.err = err
			return m, nil
		}

		// Check if wallet already exists
		if m.walletManager.WalletExists(walletName) {
			m.err = fmt.Errorf("wallet '%s' already exists", walletName)
			return m, nil
		}

		m.walletName = walletName

		// Go to password entry
		m.step = StepEnterPassword
		m.passwordInput.SetValue("")
		return m, m.passwordInput.Focus()

	case "esc":
		m.step = StepMultisigImportDescriptor
		m.err = nil
		cmd := m.descriptorInput.Focus()
		return m, cmd

	default:
		// Update name input
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
}

// createWalletFromDescriptor creates either a single-sig or multisig wallet from parsed descriptor
func (m *WalletSetupModel) createWalletFromDescriptor() error {
	if m.descriptorIsMultisig {
		// Create multisig wallet on the network chosen during onboarding
		return m.walletManager.CreateMultisigWalletWithOrderOnNetwork(
			m.walletName,
			m.password,
			m.multisigM,
			m.multisigCosigners,
			m.multisigScriptType,
			m.network,
			m.parsedDescriptor != nil && m.parsedDescriptor.Unsorted,
		)
	}

	// Create single-sig watch-only wallet (like hardware wallet)
	cfg := m.parsedSingleSig
	keyOrigin := cfg.KeyOrigin
	if keyOrigin == "" {
		// Build a basic key origin if we only have fingerprint
		keyOrigin = cfg.Fingerprint
	}

	return m.walletManager.CreateHardwareWalletOnNetwork(
		m.walletName,
		m.password,
		cfg.Xpub,
		cfg.Fingerprint,
		keyOrigin,
		cfg.AddressType,
		"descriptor", // device type
		m.network,
	)
}
