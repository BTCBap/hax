package models

import (
	"fmt"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/lipgloss"
)

// Restore flow view functions

func (m WalletSetupModel) viewRestoreChooseMethod() string {
	title := styles.TitleStyle.Render("Restore Wallet")

	options := []choiceOption{
		{name: "From Seed Phrase", desc: "Single-sig wallet from 12/24 word seed"},
		{name: "Import Multisig (Manual)", desc: "Enter cosigner xpubs one by one"},
		{name: "Import from Descriptor", desc: "Single-sig or multisig from Sparrow/Coldcard"},
		{name: "From Wallet File", desc: "Open existing .wallet file"},
	}

	menu := renderChoiceMenu(options, m.cursor)

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "back"),
	)
	content := title + "\n\n" + menu + "\n" + help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreEnterName() string {
	title := styles.TitleStyle.Render("Restore from Seed [1/6]")
	instruction := styles.NormalText.Render("Wallet name:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	content := title + "\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.nameInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreEnterMnemonic() string {
	title := styles.TitleStyle.Render("Restore from Seed [2/6]")
	info := styles.DimText.Render("12 or 24 words, space separated")
	instruction := styles.NormalText.Render("Seed phrase:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	content := title + "\n" +
		info + "\n" +
		lipgloss.NewStyle().Width(63).Render(instruction+"\n"+m.mnemonicInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreChooseScriptType() string {
	title := styles.TitleStyle.Render("Restore from Seed [4/6]")
	info := styles.DimText.Render("Select the address format used by your original wallet")
	warning := styles.WarningText.Render("⚠ Must match your original wallet's format")

	options := []choiceOption{
		{name: "Legacy (P2PKH)", desc: "Addresses start with 1 or m/n m/44'/..."},
		{name: "Nested SegWit (P2SH-P2WPKH)", desc: "Addresses start with 3 or 2 m/49'/..."},
		{name: "Native SegWit (Bech32)", desc: "Addresses start with bc1q/tb1q m/84'/...", check: " ✓"}, // Mark Native SegWit as recommended
		{name: "Taproot (P2TR)", desc: "Addresses start with bc1p/tb1p m/86'/..."},
	}

	menu := renderChoiceMenu(options, m.cursor)

	// Show custom path input if active
	customPathSection := ""
	if m.showCustomPath {
		customPathSection = "\n" + styles.BoldText.Render("Custom derivation path:") + "\n" +
			lipgloss.NewStyle().Width(33).Render(m.customPathInput.View()) + "\n"
		errorLine := ""
		if m.err != nil {
			errorLine = lipgloss.NewStyle().MaxWidth(lipgloss.Width(menu)).
				Render(styles.ErrorText.Render(m.err.Error()))
		}
		customPathSection += errorLine + "\n"
		customPathSection += styles.DimText.Render("enter: confirm • esc: cancel") + "\n"
	}

	pathNote := ""
	if m.customDerivationPath != "" {
		pathNote = styles.DimText.Render("  Custom path: "+m.customDerivationPath) + "\n"
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("p", "custom path"),
		styles.HK("esc", "back"),
	)

	content := title + "\n" +
		info + "\n" +
		warning + "\n\n" +
		menu +
		pathNote +
		customPathSection +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreEnterPassword() string {
	title := styles.TitleStyle.Render("Restore from Seed [5/6]")
	warning := styles.WarningText.Render("⚠ Password encrypts your wallet")
	instruction := styles.NormalText.Render("Password:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	content := title + "\n" +
		warning + "\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.passwordInput.View()) + "\n" +
		lipgloss.NewStyle().Width(14).Render(renderPasswordStrength(m.passwordInput.Value())) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreConfirmPassword() string {
	title := styles.TitleStyle.Render("Restore from Seed [6/6]")
	instruction := styles.NormalText.Render("Confirm password:")
	help := styles.RenderHelp(styles.HK("enter", "restore"), styles.HK("esc", "back"))

	content := title + "\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.confirmPasswordInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreChooseFile() string {
	title := styles.TitleStyle.Render("Restore from File")
	dirInfo := styles.DimText.Render("~/.hax")

	if len(m.existingWallets) == 0 {
		noWallets := styles.WarningText.Render("No wallet files found")
		help := styles.RenderHelp(styles.HK("esc", "back"))

		content := title + "\n" +
			dirInfo + "\n" +
			noWallets + "\n" +
			help

		return styles.CenterHorizontal(content, m.width)
	}

	info := styles.NormalText.Render(fmt.Sprintf("Found %d wallet(s):", len(m.existingWallets)))

	menu := ""
	for i, walletName := range m.existingWallets {
		cursor := " "
		if m.cursor == i {
			cursor = "▶"
			menu += styles.SelectedItemStyle.Render(cursor+" "+walletName) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(cursor+" "+walletName) + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "back"),
	)

	content := title + "\n" +
		dirInfo + "\n" +
		info + "\n" +
		menu +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreFilePassword() string {
	var title, subtitle string
	if m.hasLastWallet {
		// Auto-login mode - show welcome back message
		title = styles.TitleStyle.Render("Welcome ₿ack")
		subtitle = styles.DimText.Render("Unlocking: " + m.selectedWallet)
	} else {
		title = styles.TitleStyle.Render("Unlock Wallet")
		subtitle = styles.DimText.Render(m.selectedWallet)
	}

	instruction := styles.NormalText.Render("Password:")
	help := styles.RenderHelp(styles.HK("enter", "unlock"), styles.HK("esc", "back"))

	content := title + "\n" +
		subtitle + "\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.passwordInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

// =============================================================================
// Descriptor Import Views
// =============================================================================

func (m WalletSetupModel) viewMultisigImportDescriptor() string {
	title := styles.TitleStyle.Render("Import from Descriptor")

	info := styles.DimText.Render(
		"Paste output descriptor from Sparrow, Coldcard, or other wallets.\n" +
			"Supports single-sig (wpkh, tr) and multisig (wsh(sortedmulti/multi)).")

	help := styles.RenderHelp(styles.HK("enter", "import"), styles.HK("esc", "back"))

	content := title + "\n\n" +
		info + "\n\n" +
		"Descriptor: " + lipgloss.NewStyle().Width(63).Render(m.descriptorInput.View()) + "\n\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewMultisigImportReview() string {
	title := styles.TitleStyle.Render("Review Imported Descriptor")

	var summary, details string

	if m.descriptorIsMultisig && m.parsedDescriptor != nil {
		// Multisig descriptor
		config := m.parsedDescriptor

		var scriptTypeName string
		switch config.ScriptType {
		case int(wallet.MultisigNativeSegwit):
			scriptTypeName = "Native SegWit (P2WSH)"
		case int(wallet.MultisigNestedSegwit):
			scriptTypeName = "Nested SegWit (P2SH-P2WSH)"
		case int(wallet.MultisigLegacy):
			scriptTypeName = "Legacy (P2SH)"
		default:
			scriptTypeName = "Unknown"
		}

		summary = styles.InfoBoxStyle.Render(
			styles.BoldText.Render("Type: ") + fmt.Sprintf("%d-of-%d Multisig\n", config.M, config.N) +
				styles.BoldText.Render("Script: ") + scriptTypeName)

		details = styles.BoldText.Render("Cosigners:") + "\n"
		for i, cs := range config.Cosigners {
			fp := cs.Fingerprint
			if fp == "" {
				fp = "unknown"
			}
			xpubShort := cs.Xpub
			if len(xpubShort) > 30 {
				xpubShort = xpubShort[:15] + "..." + xpubShort[len(xpubShort)-15:]
			}
			details += fmt.Sprintf("  %d. [%s] %s\n", i+1, fp, xpubShort)
		}

	} else if m.parsedSingleSig != nil {
		// Single-sig descriptor
		cfg := m.parsedSingleSig

		var addrTypeName string
		switch cfg.AddressType {
		case wallet.AddressTypeTaproot:
			addrTypeName = "Taproot (P2TR)"
		case wallet.AddressTypeSegWitNative:
			addrTypeName = "Native SegWit (P2WPKH)"
		case wallet.AddressTypeSegWitNested:
			addrTypeName = "Nested SegWit (P2SH-P2WPKH)"
		case wallet.AddressTypeLegacy:
			addrTypeName = "Legacy (P2PKH)"
		default:
			addrTypeName = "Unknown"
		}

		fp := cfg.Fingerprint
		if fp == "" {
			fp = "unknown"
		}

		xpubShort := cfg.Xpub
		if len(xpubShort) > 40 {
			xpubShort = xpubShort[:20] + "..." + xpubShort[len(xpubShort)-20:]
		}

		summary = styles.InfoBoxStyle.Render(
			styles.BoldText.Render("Type: ") + "Single-sig (Watch-only)\n" +
				styles.BoldText.Render("Script: ") + addrTypeName + "\n" +
				styles.BoldText.Render("Fingerprint: ") + fp)

		details = styles.BoldText.Render("Xpub: ") + xpubShort + "\n"
		if cfg.KeyOrigin != "" {
			details += styles.BoldText.Render("Key Origin: ") + "[" + cfg.KeyOrigin + "]\n"
		}

	} else {
		return styles.CenterHorizontal(title+"\n\nNo descriptor parsed", m.width)
	}

	// Show warnings if any
	var warningsSection string
	if len(m.descriptorWarnings) > 0 {
		warningsSection = styles.WarningText.Render("Warnings:") + "\n"
		for _, w := range m.descriptorWarnings {
			warningsSection += styles.DimText.Render("  • "+w) + "\n"
		}
		warningsSection += "\n"
	}

	instructions := styles.NormalText.Render("Wallet name:")
	help := styles.RenderHelp(styles.HK("enter", "create wallet"), styles.HK("esc", "back"))

	content := title + "\n\n" +
		summary + "\n\n" +
		details + "\n" +
		warningsSection +
		lipgloss.NewStyle().Width(43).Render(instructions+"\n"+m.nameInput.View()) + "\n\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}
