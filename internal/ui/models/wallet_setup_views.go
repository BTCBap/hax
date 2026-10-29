package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

// getStepProgress returns the progress percentage for the current step
func (m WalletSetupModel) getStepProgress() float64 {
	switch m.step {
	// New wallet flow (5 steps)
	case StepChooseAction:
		return 0.0
	case StepEnterName:
		return 0.20
	case StepEnterPassword, StepConfirmPassword:
		return 0.40
	case StepChooseScriptType:
		return 0.60
	case StepChooseNetwork:
		return 0.75
	case StepEnterPassphrase, StepConfirmPassphrase:
		return 0.78
	case StepChooseWordCount:
		return 0.80
	case StepShowMnemonic, StepConfirmMnemonic:
		return 1.0

	// Restore wallet flow from seed
	case StepRestoreChooseMethod:
		return 0.0
	case StepRestoreEnterName:
		return 0.20
	case StepRestoreEnterMnemonic:
		return 0.35
	case StepRestoreEnterPassphrase, StepRestoreConfirmPassphrase:
		return 0.45
	case StepRestoreChooseScriptType:
		return 0.60
	case StepRestoreEnterPassword, StepRestoreConfirmPassword:
		return 0.80

	// Multisig flow (5 steps: name, password, config, confirm, cosigners)
	case StepMultisigChooseType:
		return 0.50
	case StepMultisigConfirm:
		return 0.65
	case StepMultisigEnterCosigners:
		return 0.80
	case StepMultisigReview:
		return 1.0

	// Restore wallet flow (from file)
	case StepRestoreChooseFile:
		return 0.50
	case StepRestoreFilePassword:
		return 1.0

	default:
		return 0.0
	}
}

// renderProgressBar renders a beautiful progress indicator
func (m WalletSetupModel) renderProgressBar() string {
	// No progress bar on menu/selection screens
	switch m.step {
	case StepChooseNetwork, StepChooseAction, StepRestoreChooseMethod, StepChooseWalletType,
		StepMultisigImportDescriptor, StepMultisigImportReview:
		return ""
	}

	progress := m.getStepProgress()
	progressStr := m.progressBar.ViewAs(progress)

	// Step indicator
	var stepNum, totalSteps int
	switch m.step {
	// New wallet flow (5 steps)
	case StepEnterName:
		stepNum, totalSteps = 1, 5
	case StepEnterPassword, StepConfirmPassword:
		stepNum, totalSteps = 2, 5
	case StepChooseScriptType:
		stepNum, totalSteps = 3, 5
	case StepChooseNetwork:
		stepNum, totalSteps = 4, 5
		if m.isRestore {
			stepNum = 3
		}
	case StepEnterPassphrase, StepConfirmPassphrase:
		stepNum, totalSteps = 4, 5
	case StepChooseWordCount:
		stepNum, totalSteps = 4, 5
	case StepShowMnemonic, StepConfirmMnemonic:
		stepNum, totalSteps = 5, 5

	// Restore wallet flow from seed
	case StepRestoreEnterName:
		stepNum, totalSteps = 1, 6
	case StepRestoreEnterMnemonic:
		stepNum, totalSteps = 2, 6
	case StepRestoreEnterPassphrase, StepRestoreConfirmPassphrase:
		stepNum, totalSteps = 3, 6
	case StepRestoreChooseScriptType:
		stepNum, totalSteps = 4, 6
	case StepRestoreEnterPassword:
		stepNum, totalSteps = 5, 6
	case StepRestoreConfirmPassword:
		stepNum, totalSteps = 6, 6

	// Multisig flow (5 steps)
	case StepMultisigChooseType:
		stepNum, totalSteps = 3, 5
	case StepMultisigConfirm:
		stepNum, totalSteps = 3, 5
	case StepMultisigEnterCosigners:
		stepNum, totalSteps = 4, 5
	case StepMultisigReview:
		stepNum, totalSteps = 5, 5

	// Restore wallet flow (from file)
	case StepRestoreChooseFile:
		stepNum, totalSteps = 1, 2
	case StepRestoreFilePassword:
		stepNum, totalSteps = 2, 2
	}

	stepIndicator := styles.ProgressStepStyle.Render(fmt.Sprintf("Step %d/%d", stepNum, totalSteps))

	progressBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.NeonOrange).
		Padding(1, 2).
		MarginBottom(2).
		Render(stepIndicator + "\n" + progressStr)

	return progressBox
}

func (m WalletSetupModel) View() string {
	var content strings.Builder

	// Add progress bar for all steps except initial/choose screens
	if m.step != StepChooseNetwork && m.step != StepChooseAction && m.step != StepRestoreChooseMethod {
		content.WriteString(m.renderProgressBar())
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

	// Show error if present
	if m.err != nil {
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

	options := []string{
		"Single-Signature Wallet",
		"Multisig Wallet (M-of-N)",
	}

	descriptions := []string{
		"Standard wallet with one key - simple and secure",
		"Requires multiple keys to spend - for shared custody",
	}

	menu := ""
	for i, option := range options {
		cursor := " "
		if m.cursor == i {
			cursor = "▶"
			menu += styles.SelectedItemStyle.Render(cursor+" "+option) + "\n"
			menu += styles.DimText.Render("   "+descriptions[i]) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(cursor+" "+option) + "\n"
		}
	}

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
		title = styles.TitleStyle.Render("Import Multisig [1/5]")
	}
	instruction := styles.NormalText.Render("Wallet name:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += instruction + "\n" +
		m.nameInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewEnterPassword() string {
	var title string
	if m.isRestore && m.isMultisig {
		title = styles.TitleStyle.Render("Import Multisig [2/5]")
	}
	warning := styles.WarningText.Render("⚠ Password encrypts your wallet")
	instruction := styles.NormalText.Render("Password:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += warning + "\n" +
		instruction + "\n" +
		m.passwordInput.View() + "\n" +
		renderPasswordStrength(m.passwordInput.Value()) + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewConfirmPassword() string {
	var title string
	if m.isRestore && m.isMultisig {
		title = styles.TitleStyle.Render("Import Multisig [2/5]")
	}
	instruction := styles.NormalText.Render("Confirm password:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += instruction + "\n" +
		m.confirmPasswordInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

// viewEnterPassphrase renders the optional BIP39 passphrase step.
// restore=true uses restore-flow step numbering in the title.
func (m WalletSetupModel) viewEnterPassphrase(restore bool) string {
	var title string
	if restore {
		title = styles.TitleStyle.Render("Restore from Seed [3/6]")
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
		instruction + "\n" +
		m.passphraseInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewConfirmPassphrase(restore bool) string {
	var title string
	if restore {
		title = styles.TitleStyle.Render("Restore from Seed [3/6]")
	}
	instruction := styles.NormalText.Render("Confirm BIP39 passphrase:")
	help := styles.RenderHelp(styles.HK("enter", "continue"), styles.HK("esc", "back"))

	var content string
	if title != "" {
		content = title + "\n"
	}
	content += instruction + "\n" +
		m.confirmPassphraseInput.View() + "\n" +
		help

	return styles.CenterHorizontal(content, m.width)
}

func (m WalletSetupModel) viewChooseScriptType() string {
	info := styles.DimText.Render("Address format for your wallet")

	options := []struct {
		name string
		desc string
		path string
	}{
		{"Legacy (P2PKH)", "Original Bitcoin format", "m/44'/..."},
		{"Nested SegWit (P2SH-P2WPKH)", "Compatible with older wallets", "m/49'/..."},
		{"Native SegWit (Bech32)", "Recommended - lowest fees", "m/84'/..."},
		{"Taproot (P2TR)", "Latest - privacy & efficiency", "m/86'/..."},
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
