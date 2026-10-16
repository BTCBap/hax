package models

import (
	"fmt"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// Restore flow view functions

func (m WalletSetupModel) viewRestoreChooseMethod() string {
	title := styles.TitleStyle.Render("Restore Wallet")

	options := []struct {
		name string
		desc string
	}{
		{"From Seed Phrase", "Single-sig wallet from 12/24 word seed"},
		{"Import Multisig (Manual)", "Enter cosigner xpubs one by one"},
		{"Import from Descriptor", "Single-sig or multisig from Sparrow/Coldcard"},
		{"From Wallet File", "Open existing .wallet file"},
	}

	menu := ""
	for i, opt := range options {
		cursor := "  "
		if m.cursor == i {
			cursor = "▶ "
			menu += styles.SelectedItemStyle.Render(cursor+opt.name) + "\n"
			menu += styles.DimText.Render("    "+opt.desc) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(cursor+opt.name) + "\n"
			menu += styles.DimText.Render("    "+opt.desc) + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "back"),
	)
	content := title + "\n\n" + menu + "\n" + help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreEnterName() string {
	title := styles.TitleStyle.Render("Restore from Seed [1/5]")
	instruction := styles.NormalText.Render("Wallet name:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	content := title + "\n" +
		instruction + "\n" +
		m.nameInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreEnterMnemonic() string {
	title := styles.TitleStyle.Render("Restore from Seed [2/5]")
	info := styles.DimText.Render("12 or 24 words, space separated")
	instruction := styles.NormalText.Render("Seed phrase:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	content := title + "\n" +
		info + "\n" +
		instruction + "\n" +
		m.mnemonicInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreChooseScriptType() string {
	title := styles.TitleStyle.Render("Restore from Seed [3/5]")
	info := styles.DimText.Render("Select the address format used by your original wallet")
	warning := styles.WarningText.Render("⚠ Must match your original wallet's format")

	options := []struct {
		name string
		desc string
		path string
	}{
		{"Legacy (P2PKH)", "Addresses start with 1 or m/n", "m/44'/..."},
		{"Nested SegWit (P2SH-P2WPKH)", "Addresses start with 3 or 2", "m/49'/..."},
		{"Native SegWit (Bech32)", "Addresses start with bc1q/tb1q", "m/84'/..."},
		{"Taproot (P2TR)", "Addresses start with bc1p/tb1p", "m/86'/..."},
	}

	menu := ""
	for i, opt := range options {
		cursor := " "
		if m.cursor == i {
			cursor = "▶"
			line := fmt.Sprintf("%s %s", cursor, opt.name)
			if i == 2 {
				line += " ✓" // Mark Native SegWit as recommended
			}
			menu += styles.SelectedItemStyle.Render(line) + "\n"
			menu += styles.DimText.Render("   "+opt.desc+" "+opt.path) + "\n"
		} else {
			line := fmt.Sprintf("%s %s", cursor, opt.name)
			menu += styles.UnselectedItemStyle.Render(line) + "\n"
		}
	}

	// Show custom path input if active
	customPathSection := ""
	if m.showCustomPath {
		customPathSection = "\n" + styles.BoldText.Render("Custom derivation path:") + "\n" +
			m.customPathInput.View() + "\n"
		if m.err != nil {
			customPathSection += styles.ErrorText.Render(m.err.Error()) + "\n"
		}
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
	title := styles.TitleStyle.Render("Restore from Seed [4/5]")
	warning := styles.WarningText.Render("⚠ Password encrypts your wallet")
	instruction := styles.NormalText.Render("Password:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	content := title + "\n" +
		warning + "\n" +
		instruction + "\n" +
		m.passwordInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewRestoreConfirmPassword() string {
	title := styles.TitleStyle.Render("Restore from Seed [5/5]")
	instruction := styles.NormalText.Render("Confirm password:")
	help := styles.RenderHelp(styles.HK("enter", "restore"), styles.HK("esc", "back"))

	content := title + "\n" +
		instruction + "\n" +
		m.confirmPasswordInput.View() + "\n" +
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
		instruction + "\n" +
		m.passwordInput.View() + "\n" +
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
			"Supports single-sig (wpkh, tr) and multisig (wsh(sortedmulti)).")

	help := styles.RenderHelp(styles.HK("enter", "import"), styles.HK("esc", "back"))

	content := title + "\n\n" +
		info + "\n\n" +
		"Descriptor: " + m.descriptorInput.View() + "\n\n" +
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
		instructions + "\n" +
		m.nameInput.View() + "\n\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}
