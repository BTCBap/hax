package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

// renderProgressBar renders the step indicator and the progress bar from the
// same ordered step list. The percent field is fixed width so digits never
// move the bar or the text around it.
func (m WalletSetupModel) renderProgressBar() string {
	if !m.showProgressBar() {
		return ""
	}
	stepNum, totalSteps, ok := m.flowStepNumber()
	if !ok || totalSteps == 0 {
		return ""
	}

	progress := m.getStepProgress()
	progressLine := m.progressBar.ViewAs(progress) + " " +
		styles.DimText.Render(formatProgressPercent(progress))

	stepIndicator := styles.ProgressStepStyle.Render(fmt.Sprintf("Step %d/%d", stepNum, totalSteps))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.NeonOrange).
		Padding(1, 2).
		MarginBottom(2).
		Render(stepIndicator + "\n" + progressLine)
}

func (m WalletSetupModel) View() string {
	var content strings.Builder

	// Add the progress bar for screens that belong to an ordered flow.
	if progress := m.renderProgressBar(); progress != "" {
		content.WriteString(progress)
		content.WriteString("\n")
	}

	switch m.step {
	case StepChooseAction:
		content.WriteString(m.viewChooseAction())
	case StepChooseWalletType:
		content.WriteString(m.viewChooseWalletType())
	case StepEnterName:
		content.WriteString(m.viewEnterName())
	case StepEnterPassword:
		content.WriteString(m.viewEnterPassword())
	case StepConfirmPassword:
		content.WriteString(m.viewConfirmPassword())
	case StepEnterPassphrase:
		content.WriteString(m.viewEnterPassphrase(false))
	case StepConfirmPassphrase:
		content.WriteString(m.viewConfirmPassphrase(false))
	case StepChooseScriptType:
		content.WriteString(m.viewChooseScriptType())
	case StepChooseWordCount:
		content.WriteString(m.viewChooseWordCount())
	case StepChooseNetwork:
		content.WriteString(m.viewChooseNetwork())
	case StepShowMnemonic:
		content.WriteString(m.viewShowMnemonic())
	case StepConfirmMnemonic:
		content.WriteString(m.viewConfirmMnemonic())

	// Multisig flow views
	case StepMultisigChooseType:
		content.WriteString(m.viewMultisigChooseType())
	case StepMultisigConfirm:
		content.WriteString(m.viewMultisigConfirm())
	case StepMultisigEnterCosigners:
		content.WriteString(m.viewMultisigEnterCosigners())
	case StepMultisigReview:
		content.WriteString(m.viewMultisigReview())

	// Single-sig hardware wallet flow views
	case StepChooseKeySource:
		content.WriteString(m.viewChooseKeySource())
	case StepHWISetup:
		content.WriteString(m.viewHWISetup())
	case StepHardwareWalletConnect:
		content.WriteString(m.viewHardwareWalletConnect())
	case StepHardwareWalletReview:
		content.WriteString(m.viewHardwareWalletReview())

	// Multisig descriptor import views
	case StepMultisigImportDescriptor:
		content.WriteString(m.viewMultisigImportDescriptor())
	case StepMultisigImportReview:
		content.WriteString(m.viewMultisigImportReview())

	// Restore flow views
	case StepRestoreChooseMethod:
		content.WriteString(m.viewRestoreChooseMethod())
	case StepRestoreEnterName:
		content.WriteString(m.viewRestoreEnterName())
	case StepRestoreEnterMnemonic:
		content.WriteString(m.viewRestoreEnterMnemonic())
	case StepRestoreEnterPassphrase:
		content.WriteString(m.viewEnterPassphrase(true))
	case StepRestoreConfirmPassphrase:
		content.WriteString(m.viewConfirmPassphrase(true))
	case StepRestoreChooseScriptType:
		content.WriteString(m.viewRestoreChooseScriptType())
	case StepRestoreEnterPassword:
		content.WriteString(m.viewRestoreEnterPassword())
	case StepRestoreConfirmPassword:
		content.WriteString(m.viewRestoreConfirmPassword())
	case StepRestoreChooseFile:
		content.WriteString(m.viewRestoreChooseFile())
	case StepRestoreFilePassword:
		content.WriteString(m.viewRestoreFilePassword())

	default:
		content.WriteString("Unknown step")
	}

	// Show error if present. The backup check reserves its own error rows.
	if m.err != nil && m.step != StepConfirmMnemonic {
		content.WriteString("\n\n")
		content.WriteString(styles.RenderErrorBox("Error: "+m.err.Error(), m.width))
	}

	return content.String()
}

func (m WalletSetupModel) viewChooseAction() string {
	title := styles.TitleStyle.Render("₿ Wallet Setup")

	options := []string{
		"Create New Wallet",
		"Restore Wallet",
	}

	menu := ""
	for i, option := range options {
		cursor := " "
		if m.cursor == i {
			cursor = "▶"
			menu += styles.SelectedItemStyle.Render(cursor+" "+option) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(cursor+" "+option) + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("q", "quit"),
	)

	return styles.CenterHorizontal(title+"\n"+menu+help, m.width)
}

func (m WalletSetupModel) viewChooseWalletType() string {
	title := styles.TitleStyle.Render("Create New Wallet")
	subtitle := styles.SubtitleStyle.Render("Choose wallet type:")

	options := []choiceOption{
		{name: "Single-Signature Wallet", desc: "Standard wallet with one key - simple and secure"},
		{name: "Multisig Wallet (M-of-N)", desc: "Requires multiple keys to spend - for shared custody"},
	}

	menu := renderChoiceMenu(options, m.cursor)

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "back"),
	)

	return styles.CenterHorizontal(title+"\n"+subtitle+"\n\n"+menu+"\n"+help, m.width)
}

func (m WalletSetupModel) viewEnterName() string {
	var title string
	if m.isRestore && m.isMultisig {
		title = styles.TitleStyle.Render("Import Multisig" + m.stepCounterSuffix())
	}
	instruction := styles.NormalText.Render("Wallet name:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.nameInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewEnterPassword() string {
	var title string
	if m.isRestore && m.isMultisig {
		title = styles.TitleStyle.Render("Import Multisig" + m.stepCounterSuffix())
	}
	warning := styles.WarningText.Render("⚠ Password encrypts your wallet")
	instruction := styles.NormalText.Render("Password:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += warning + "\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.passwordInput.View()) + "\n" +
		lipgloss.NewStyle().Width(14).Render(renderPasswordStrength(m.passwordInput.Value())) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewConfirmPassword() string {
	var title string
	if m.isRestore && m.isMultisig {
		title = styles.TitleStyle.Render("Import Multisig" + m.stepCounterSuffix())
	}
	instruction := styles.NormalText.Render("Confirm password:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.confirmPasswordInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

// viewEnterPassphrase renders the optional BIP39 passphrase step.
// restore=true uses restore-flow step numbering in the title.
func (m WalletSetupModel) viewEnterPassphrase(restore bool) string {
	var title string
	if restore {
		title = styles.TitleStyle.Render("Restore from Seed" + m.stepCounterSuffix())
	}
	info := styles.DimText.Render(
		"Optional BIP39 passphrase. Leave empty for none.\n" +
			"A wrong passphrase unlocks a different wallet.\n" +
			"Stored only inside the encrypted wallet file.")
	instruction := styles.NormalText.Render("BIP39 passphrase (optional):")
	help := styles.RenderHelp(
		styles.HK("enter", "continue"),
		styles.HK("esc", "back"),
	)

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += info + "\n\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.passphraseInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewConfirmPassphrase(restore bool) string {
	var title string
	if restore {
		title = styles.TitleStyle.Render("Restore from Seed" + m.stepCounterSuffix())
	}
	instruction := styles.NormalText.Render("Confirm BIP39 passphrase:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.confirmPassphraseInput.View()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewChooseScriptType() string {
	info := styles.DimText.Render("Address format for your wallet")

	options := []choiceOption{
		{name: "Legacy (P2PKH)", desc: "Original Bitcoin format m/44'/..."},
		{name: "Nested SegWit (P2SH-P2WPKH)", desc: "Compatible with older wallets m/49'/..."},
		{name: "Native SegWit (Bech32)", desc: "Recommended - lowest fees m/84'/...", check: " ✓"}, // Mark Native SegWit as recommended
		{name: "Taproot (P2TR)", desc: "Latest - privacy & efficiency m/86'/..."},
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

	content := info + "\n\n" +
		menu +
		pathNote +
		customPathSection +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewChooseWordCount() string {
	info := styles.DimText.Render("More words = more security")

	options := []string{
		"12 words",
		"24 words",
	}

	menu := ""
	for i, option := range options {
		cursor := " "
		if m.cursor == i {
			cursor = "▶"
			menu += styles.SelectedItemStyle.Render(cursor+" "+option) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(cursor+" "+option) + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "generate"),
		styles.HK("esc", "back"),
	)

	content := info + "\n" +
		menu +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewShowMnemonic() string {
	header := styles.ErrorText.Render("⚠ Write this down!")

	// Format mnemonic in a compact grid
	words := strings.Split(m.generatedMnemonic, " ")
	mnemonicDisplay := ""
	for i, word := range words {
		mnemonicDisplay += fmt.Sprintf("%2d.%-10s", i+1, word)
		if (i+1)%4 == 0 {
			mnemonicDisplay += "\n"
		} else {
			mnemonicDisplay += " "
		}
	}

	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	return styles.CenterHorizontal(
		renderScrollableView(m.height, m.scrollOffset, header, mnemonicDisplay, help),
		m.width,
	)
}
