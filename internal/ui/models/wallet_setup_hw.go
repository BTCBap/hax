package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m WalletSetupModel) viewChooseKeySource() string {
	info := styles.DimText.Render("How do you want to store your keys?")

	options := []choiceOption{
		{name: "Generate New Seed", desc: "Create a new 12 or 24 word seed phrase", check: " ✓"},
		{name: "Import from Hardware Wallet", desc: "Use a Trezor, Ledger, Coldcard, etc."},
	}
	menu := renderChoiceMenu(options, m.cursor)

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "back"),
	)

	content := info + "\n\n" +
		menu +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) handleChooseKeySource(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "enter":
		if m.cursor == 0 {
			// Generate new seed - network was already chosen
			m.step = StepEnterPassphrase
			m.cursor = 0
			return m, m.passphraseInput.Focus()
		}
		// Hardware wallet - network was already chosen, scan for devices
		m.step = StepHardwareWalletConnect
		m.cursor = 0
		m.hwiLoading = true
		m.hwiError = nil
		m.hwiDevices = nil
		return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSingleSig())
	}
	return m, nil
}

// viewHWISetup renders the guided HWI installation screen with platform-specific steps
func (m WalletSetupModel) viewHWISetup() string {
	title := styles.TitleStyle.Render("Install HWI (Hardware Wallet Interface)")

	var content strings.Builder
	content.WriteString(styles.NormalText.Render(
		"HWI is required to communicate with hardware wallets.") + "\n")
	content.WriteString(styles.NormalText.Render(
		"Follow these steps in your terminal:") + "\n\n")

	platform, steps, note := wallet.HWIPlatformSteps()
	content.WriteString(styles.BoldText.Render("Platform: "+platform) + "\n\n")

	for i, step := range steps {
		num := styles.HighlightText.Render(fmt.Sprintf("  %d.", i+1))
		desc := styles.DimText.Render(" " + step.Description)
		cmd := styles.BoldText.Render("     $ " + step.Command)
		content.WriteString(num + desc + "\n" + cmd + "\n\n")
	}

	if note != "" {
		content.WriteString(styles.WarningText.Render("Note: ") +
			styles.DimText.Render(note) + "\n\n")
	}

	content.WriteString(lipgloss.NewStyle().
		Foreground(styles.TextSecondary).
		Render("More info: https://github.com/bitcoin-core/HWI") + "\n\n")

	// Verification status, on a row that is reserved when there is no status.
	if m.hwiVerifying {
		content.WriteString(m.spinner.View() + " " +
			styles.DimText.Render("Verifying installation...") + "\n")
	} else if m.hwiVerifyMsg != "" {
		if m.hwiVerifyOK {
			content.WriteString(styles.SuccessText.Render("  "+m.hwiVerifyMsg) + "\n")
		} else {
			content.WriteString(styles.ErrorText.Render("  "+m.hwiVerifyMsg) + "\n")
		}
	} else {
		content.WriteString("\n")
	}

	enterDesc := "verify installation"
	if m.hwiVerifyOK {
		enterDesc = "continue"
	}
	help := styles.RenderHelp(styles.HK("enter", enterDesc), styles.HK("esc", "back"))

	return styles.CenterHorizontal(title+"\n\n"+content.String()+"\n"+help, m.width)
}

// handleHWISetup handles key events on the HWI installation guide screen
func (m WalletSetupModel) handleHWISetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.hwiVerifyOK {
			// Verification passed — proceed to device scanning
			m.step = StepHardwareWalletConnect
			m.cursor = 0
			m.hwiLoading = true
			m.hwiError = nil
			m.hwiDevices = nil
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSingleSig())
		}
		// Run verification
		m.hwiVerifying = true
		m.hwiVerifyMsg = ""
		return m, tea.Batch(m.spinner.Tick, m.verifyHWIInstallCmd())
	}
	return m, nil
}

// verifyHWIInstallCmd returns a tea.Cmd that checks if HWI is properly installed
func (m WalletSetupModel) verifyHWIInstallCmd() tea.Cmd {
	return func() tea.Msg {
		version, err := wallet.VerifyHWIInstallation()
		return HWIInstallCheckMsg{Version: version, Err: err}
	}
}

func (m WalletSetupModel) viewHardwareWalletConnect() string {
	title := styles.TitleStyle.Render("Connect Hardware Wallet" + m.stepCounterSuffix())

	var content strings.Builder

	if m.hwiLoading {
		content.WriteString(m.spinner.View() + " " + styles.DimText.Render("Scanning for hardware wallets...") + "\n\n")
	} else if m.hwiError != nil {
		content.WriteString(styles.ErrorText.Render("Error: "+m.hwiError.Error()) + "\n\n")
		content.WriteString(m.renderDeviceTroubleshooting())
	} else if len(m.hwiDevices) == 0 {
		content.WriteString(styles.WarningText.Render("No hardware wallets detected.") + "\n\n")
		content.WriteString(m.renderDeviceTroubleshooting())
	} else {
		content.WriteString(styles.SuccessText.Render(fmt.Sprintf("Found %d device(s):", len(m.hwiDevices))) + "\n\n")
		content.WriteString(renderDeviceMenu(hardwareDeviceChoices(m.hwiDevices), m.selectedHWDevice))
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "select"),
		styles.HK("enter", "get xpub"),
		styles.HK("r", "rescan"),
		styles.HK("esc", "back"),
	)

	return styles.CenterHorizontal(title+"\n\n"+content.String()+"\n"+help, m.width)
}

// renderDeviceTroubleshooting returns common troubleshooting hints for device connection
func (m WalletSetupModel) renderDeviceTroubleshooting() string {
	return styles.DimText.Render("Make sure your device is:") + "\n" +
		styles.DimText.Render("  • Connected via USB") + "\n" +
		styles.DimText.Render("  • Unlocked (PIN entered)") + "\n" +
		styles.DimText.Render("  • On the home screen") + "\n\n" +
		styles.NormalText.Render("Press Enter to scan again, or Esc to go back") + "\n"
}

func (m WalletSetupModel) handleHardwareWalletConnect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if len(m.hwiDevices) > 0 && m.selectedHWDevice > 0 {
			m.selectedHWDevice--
		}
	case "down", "j":
		if len(m.hwiDevices) > 0 && m.selectedHWDevice < len(m.hwiDevices)-1 {
			m.selectedHWDevice++
		}
	case "r", "R":
		// Rescan for devices
		m.hwiLoading = true
		m.hwiError = nil
		m.hwiDevices = nil
		m.selectedHWDevice = 0
		return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSingleSig())
	case "enter":
		if m.hwiError != nil || len(m.hwiDevices) == 0 {
			// Retry scan
			m.hwiLoading = true
			m.hwiError = nil
			m.hwiDevices = nil
			m.selectedHWDevice = 0
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSingleSig())
		}
		if len(m.hwiDevices) > 0 && m.selectedHWDevice < len(m.hwiDevices) {
			// Get xpub from selected device
			m.hwiLoading = true
			m.hwiError = nil
			return m, tea.Batch(m.spinner.Tick, m.getXpubFromHWIForSingleSig(m.hwiDevices[m.selectedHWDevice]))
		}
	}
	return m, nil
}

func (m WalletSetupModel) viewHardwareWalletReview() string {
	title := styles.TitleStyle.Render("Review Hardware Wallet" + m.stepCounterSuffix())

	// Determine address type name and purpose
	var addrTypeName string
	var purpose uint32
	switch m.selectedScriptType {
	case wallet.AddressTypeLegacy:
		addrTypeName = "Legacy (P2PKH)"
		purpose = 44
	case wallet.AddressTypeSegWitNested:
		addrTypeName = "Nested SegWit (P2SH-P2WPKH)"
		purpose = 49
	case wallet.AddressTypeSegWitNative:
		addrTypeName = "Native SegWit (P2WPKH)"
		purpose = 84
	case wallet.AddressTypeTaproot:
		addrTypeName = "Taproot (P2TR)"
		purpose = 86
	}

	// Show device info
	deviceInfo := "Unknown device"
	if len(m.hwiDevices) > 0 && m.selectedHWDevice < len(m.hwiDevices) {
		device := m.hwiDevices[m.selectedHWDevice]
		if device.Model != "" {
			deviceInfo = device.Model
		} else {
			deviceInfo = string(device.Type)
		}
	}

	// Format key origin
	keyOrigin := fmt.Sprintf("%s/%d'/%d'/0'", m.hwFingerprint, purpose, m.coinType())

	// Truncate xpub for display
	xpubShort := m.hwDerivedXpub
	if len(xpubShort) > 40 {
		xpubShort = xpubShort[:20] + "..." + xpubShort[len(xpubShort)-20:]
	}

	summary := styles.InfoBoxStyle.Render(
		styles.BoldText.Render("Wallet: ") + m.walletName + "\n" +
			styles.BoldText.Render("Device: ") + deviceInfo + "\n" +
			styles.BoldText.Render("Address Type: ") + addrTypeName + "\n" +
			styles.BoldText.Render("Key Origin: ") + "[" + keyOrigin + "]\n" +
			styles.BoldText.Render("Xpub: ") + xpubShort)

	warning := styles.WarningText.Render(
		"This creates a watch-only wallet. The hardware device\n" +
			"will be required to sign any transactions.")

	help := styles.RenderHelp(styles.HK("enter", "create wallet"), styles.HK("esc", "back"))

	return styles.CenterHorizontal(title+"\n\n"+summary+"\n\n"+warning+"\n\n"+help, m.width)
}

func (m WalletSetupModel) handleHardwareWalletReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Determine device type
		deviceType := "unknown"
		if len(m.hwiDevices) > 0 && m.selectedHWDevice < len(m.hwiDevices) {
			deviceType = string(m.hwiDevices[m.selectedHWDevice].Type)
		}

		// Determine purpose for key origin
		var purpose uint32
		switch m.selectedScriptType {
		case wallet.AddressTypeLegacy:
			purpose = 44
		case wallet.AddressTypeSegWitNested:
			purpose = 49
		case wallet.AddressTypeSegWitNative:
			purpose = 84
		case wallet.AddressTypeTaproot:
			purpose = 86
		}
		keyOrigin := fmt.Sprintf("%s/%d'/%d'/0'", m.hwFingerprint, purpose, m.coinType())

		// Create the hardware wallet
		err := m.walletManager.CreateHardwareWalletOnNetwork(
			m.walletName,
			m.password,
			m.hwDerivedXpub,
			m.hwFingerprint,
			keyOrigin,
			m.selectedScriptType,
			deviceType,
			m.network,
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
	}
	return m, nil
}

// enumerateHWIDevicesForSingleSig lists connected hardware wallets for single-sig setup
func (m WalletSetupModel) enumerateHWIDevicesForSingleSig() tea.Cmd {
	walletMgr := m.walletManager

	return func() tea.Msg {
		devices, err := walletMgr.ListHardwareWallets()
		return HWIDevicesMsg{Devices: devices, Err: err}
	}
}

// getXpubFromHWIForSingleSig retrieves the xpub from a hardware wallet for single-sig
func (m WalletSetupModel) getXpubFromHWIForSingleSig(device wallet.HardwareDevice) tea.Cmd {
	scriptType := m.selectedScriptType
	testnet := m.isTestnet()
	coin := m.coinType()

	return func() tea.Msg {
		hwi := wallet.NewHWIClient(testnet)
		if hwi == nil || !hwi.IsAvailable() {
			return HWIXpubMsg{Err: fmt.Errorf("HWI not installed. %s", wallet.HWIInstallInstructions())}
		}

		// Determine derivation path based on script type
		// BIP44/49/84/86 for single-sig: m/purpose'/coin'/account'
		var path string
		switch scriptType {
		case wallet.AddressTypeLegacy:
			path = fmt.Sprintf("m/44'/%d'/0'", coin) // BIP44 Legacy
		case wallet.AddressTypeSegWitNested:
			path = fmt.Sprintf("m/49'/%d'/0'", coin) // BIP49 Nested SegWit
		case wallet.AddressTypeSegWitNative:
			path = fmt.Sprintf("m/84'/%d'/0'", coin) // BIP84 Native SegWit
		case wallet.AddressTypeTaproot:
			path = fmt.Sprintf("m/86'/%d'/0'", coin) // BIP86 Taproot
		default:
			path = fmt.Sprintf("m/84'/%d'/0'", coin) // Default to native segwit
		}

		xpub, err := hwi.GetXpub(device, path)
		if err != nil {
			return HWIXpubMsg{Err: fmt.Errorf("failed to get xpub: %w", err)}
		}

		return HWIXpubMsg{
			Xpub:           xpub,
			Fingerprint:    device.Fingerprint,
			DerivationPath: path,
			DeviceType:     string(device.Type),
			Err:            nil,
		}
	}
}
