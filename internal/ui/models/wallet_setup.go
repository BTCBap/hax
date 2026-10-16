package models

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// WalletSetupStep represents the current step in wallet setup
type WalletSetupStep int

const (
	StepChooseAction     WalletSetupStep = iota
	StepChooseWalletType                 // NEW: Single-sig or Multisig
	StepEnterName
	StepEnterPassword
	StepConfirmPassword
	StepChooseScriptType
	StepChooseWordCount
	StepShowMnemonic
	StepConfirmMnemonic
	StepComplete

	// Restore flow steps
	StepRestoreChooseMethod
	StepRestoreEnterName
	StepRestoreEnterMnemonic
	StepRestoreChooseScriptType
	StepRestoreEnterPassword
	StepRestoreConfirmPassword
	StepRestoreChooseFile
	StepRestoreFilePassword

	// Multisig flow steps (after StepChooseWalletType → Multisig)
	StepMultisigChooseType     // M-of-N and script type
	StepMultisigConfirm        // Confirm configuration before cosigner entry
	StepMultisigEnterCosigners // Enter xpubs
	StepMultisigReview         // Review and confirm

	// Single-sig hardware wallet flow (after StepChooseScriptType)
	StepChooseKeySource       // Generate seed vs Import from hardware wallet
	StepHWISetup              // Guided HWI installation flow
	StepHardwareWalletConnect // Detect devices, select, get xpub
	StepHardwareWalletReview  // Review and confirm hardware wallet setup

	// Multisig import from descriptor
	StepMultisigImportDescriptor // Paste output descriptor
	StepMultisigImportReview     // Review parsed descriptor and confirm
)

// WalletSetupModel handles wallet creation or restoration
type WalletSetupModel struct {
	width        int
	height       int
	scrollOffset int // For scrollable views

	// Navigation
	step   WalletSetupStep
	cursor int

	// Wallet manager
	walletManager WalletService
	configManager *config.ConfigManager

	// UI components
	progressBar          progress.Model
	nameInput            textinput.Model
	passwordInput        textinput.Model
	confirmPasswordInput textinput.Model
	mnemonicInput        textinput.Model

	// Data
	walletName           string
	password             string
	wordCount            int
	generatedMnemonic    string
	selectedScriptType   wallet.AddressType
	customDerivationPath string // Optional custom derivation path (e.g. "m/84'/0'/3'")
	showCustomPath       bool   // Whether custom path input is visible
	customPathInput      textinput.Model
	err                  error

	// Restore mode
	isRestore       bool
	restoreMnemonic string
	existingWallets []string
	selectedWallet  string

	// Auto-login state
	hasLastWallet  bool
	lastWalletName string

	// Multisig mode
	isMultisig              bool
	multisigM               int                       // Required signatures
	multisigN               int                       // Total cosigners
	multisigScriptType      wallet.MultisigScriptType // 0=Legacy, 1=Nested, 2=Native
	multisigCosigners       []wallet.MultisigCosignerConfig
	multisigCurrentXpub     textinput.Model // Input for current xpub
	multisigCosignerNames   []string        // Names for each cosigner
	multisigInputMode       int             // 0=xpub, 1=mnemonic, 2=hardware wallet
	multisigCosignerSeed    textinput.Model // Input for cosigner mnemonic
	multisigWarningAcked    string          // Tracks if user acknowledged warning for current input
	multisigCustomDerivPath string          // Optional custom derivation path for multisig
	showMultisigCustomPath  bool            // Whether multisig custom path input is visible

	// Hardware wallet support
	hwiDevices       []wallet.HardwareDevice // Detected hardware wallets
	selectedHWDevice int                     // Selected device index
	showHWIDevices   bool                    // Whether showing device selection
	hwiLoading       bool                    // Loading devices
	spinner          spinner.Model
	hwiError         error  // Error from HWI operations
	hwDerivedXpub    string // Xpub derived from hardware wallet
	hwFingerprint    string // Master fingerprint from device
	hwiVerifying     bool   // Currently verifying HWI installation
	hwiVerifyOK      bool   // Whether HWI verification succeeded
	hwiVerifyMsg     string // Verification result message

	// Descriptor import support
	descriptorInput      textinput.Model                   // Text input for pasting descriptor
	parsedDescriptor     *wallet.MultisigWalletConfig      // Parsed multisig descriptor config
	parsedSingleSig      *wallet.SingleSigDescriptorConfig // Parsed single-sig descriptor config
	descriptorIsMultisig bool                              // Whether parsed descriptor is multisig
	descriptorWarnings   []string                          // Non-fatal warnings from parsing
}

// HWIDevicesMsg carries list of detected hardware wallets (for wallet setup)
type HWIDevicesMsg struct {
	Devices []wallet.HardwareDevice
	Err     error
}

// HWIXpubMsg carries xpub derived from hardware wallet
type HWIXpubMsg struct {
	Xpub           string
	Fingerprint    string
	DerivationPath string
	DeviceType     string
	Err            error
}

// HWIInstallCheckMsg carries the result of an HWI installation verification
type HWIInstallCheckMsg struct {
	Version string
	Err     error
}

func NewWalletSetupModel(walletManager WalletService, configManager *config.ConfigManager) WalletSetupModel {

	// Create progress bar with neon orange gradient
	prog := progress.New(
		progress.WithScaledGradient(string(styles.NeonOrange), string(styles.BitcoinGold)),
		progress.WithWidth(50),
	)

	// Create input fields
	nameInput := textinput.New()
	nameInput.Placeholder = "My Bitcoin Wallet"
	nameInput.CharLimit = 50

	passwordInput := textinput.New()
	passwordInput.Placeholder = ""
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.CharLimit = 100

	confirmPasswordInput := textinput.New()
	confirmPasswordInput.Placeholder = "Confirm password"
	confirmPasswordInput.EchoMode = textinput.EchoPassword
	confirmPasswordInput.CharLimit = 100

	mnemonicInput := textinput.New()
	mnemonicInput.Placeholder = "word1 word2 word3 ..."
	mnemonicInput.Width = 80
	mnemonicInput.CharLimit = 300

	// Multisig xpub input
	multisigXpubInput := textinput.New()
	multisigXpubInput.Placeholder = "tpub... (BIP48 xpub from cosigner)"
	multisigXpubInput.Width = 80
	multisigXpubInput.CharLimit = 200

	// Multisig cosigner seed input (for deriving BIP48 xpub)
	multisigSeedInput := textinput.New()
	multisigSeedInput.Placeholder = "word1 word2 word3 ... (12 or 24 words)"
	multisigSeedInput.Width = 80
	multisigSeedInput.CharLimit = 300

	// Custom derivation path input
	customPathInput := textinput.New()
	customPathInput.Placeholder = "m/84'/1'/0'"
	customPathInput.Width = 30
	customPathInput.CharLimit = 50

	// Descriptor import text input (single line - descriptors don't have newlines)
	descriptorInput := textinput.New()
	descriptorInput.Placeholder = "wsh(sortedmulti(2,[fp/48h/1h/0h/2h]tpub...))#checksum"
	descriptorInput.Width = 70
	descriptorInput.CharLimit = 2000

	// Check for last used wallet (auto-login feature)
	var hasLastWallet bool
	var lastWalletName string
	var startStep WalletSetupStep = StepChooseAction

	if configManager != nil {
		cfg, err := configManager.Load()
		if err == nil && cfg != nil && cfg.LastWalletName != "" {
			// Verify the wallet still exists
			if walletManager.WalletExists(cfg.LastWalletName) {
				hasLastWallet = true
				lastWalletName = cfg.LastWalletName
				// Skip directly to password entry for last used wallet
				startStep = StepRestoreFilePassword
			}
		}
	}

	// Also load existing wallets list
	existingWallets, _ := walletManager.ListWallets()

	// Focus the correct input based on starting step
	if hasLastWallet && startStep == StepRestoreFilePassword {
		passwordInput.Focus()
	} else {
		nameInput.Focus()
	}

	return WalletSetupModel{
		step:                 startStep,
		cursor:               0,
		walletManager:        walletManager,
		configManager:        configManager,
		progressBar:          prog,
		nameInput:            nameInput,
		passwordInput:        passwordInput,
		confirmPasswordInput: confirmPasswordInput,
		mnemonicInput:        mnemonicInput,
		wordCount:            12, // Default to 12 words
		hasLastWallet:        hasLastWallet,
		lastWalletName:       lastWalletName,
		selectedWallet:       lastWalletName, // Pre-select the last wallet
		existingWallets:      existingWallets,
		// Multisig defaults
		multisigM:             2,
		multisigN:             3,
		multisigScriptType:    wallet.MultisigNativeSegwit, // P2WSH recommended
		multisigCurrentXpub:   multisigXpubInput,
		multisigCosigners:     make([]wallet.MultisigCosignerConfig, 0),
		multisigCosignerNames: make([]string, 0),
		multisigInputMode:     0, // Start with xpub mode
		multisigCosignerSeed:  multisigSeedInput,
		customPathInput:       customPathInput,
		descriptorInput:       descriptorInput,
		spinner:               styles.NewStandardSpinner(),
	}
}

func (m WalletSetupModel) Init() tea.Cmd {
	// If starting in auto-login mode, focus password input
	if m.hasLastWallet && m.step == StepRestoreFilePassword {
		m.passwordInput.SetValue("")
		return tea.Batch(textinput.Blink, m.passwordInput.Focus())
	}
	return textinput.Blink
}

func (m WalletSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		if m.hwiLoading || m.hwiVerifying {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case HWIInstallCheckMsg:
		m.hwiVerifying = false
		if msg.Err != nil {
			m.hwiVerifyOK = false
			m.hwiVerifyMsg = "HWI not found. Check the steps above and try again."
		} else {
			m.hwiVerifyOK = true
			m.hwiVerifyMsg = "HWI " + msg.Version + " detected!"
		}

	case HWIDevicesMsg:
		m.hwiLoading = false
		if msg.Err != nil {
			// Redirect to guided setup if HWI is not installed (single-sig flow)
			if hwiErr, ok := msg.Err.(*wallet.HWIError); ok && hwiErr.Code == wallet.HWIErrNotInstalled {
				if m.step == StepHardwareWalletConnect {
					m.step = StepHWISetup
					m.hwiVerifyOK = false
					m.hwiVerifyMsg = ""
					m.hwiVerifying = false
					return m, nil
				}
			}
			m.hwiError = msg.Err
			m.showHWIDevices = false
		} else if len(msg.Devices) == 0 {
			m.hwiError = &wallet.HWIError{Code: wallet.HWIErrNoDevice, RawMsg: "no devices found"}
			m.showHWIDevices = false
		} else {
			m.hwiDevices = msg.Devices
			m.hwiError = nil
			m.showHWIDevices = true
			m.selectedHWDevice = 0
		}

	case HWIXpubMsg:
		m.hwiLoading = false
		m.showHWIDevices = false
		if msg.Err != nil {
			m.hwiError = msg.Err
		} else {
			// Successfully got xpub from hardware wallet
			m.hwDerivedXpub = msg.Xpub
			m.hwFingerprint = msg.Fingerprint

			if m.step == StepHardwareWalletConnect {
				// Single-sig hardware wallet: advance to review
				m.step = StepHardwareWalletReview
				m.cursor = 0
			} else if m.step == StepMultisigEnterCosigners {
				// Check for duplicate xpub
				if m.hasDuplicateCosignerXpub(msg.Xpub) {
					m.hwiError = fmt.Errorf("this key is already added as a cosigner")
					return m, nil
				}
				keyOrigin := ""
				if msg.Fingerprint != "" && msg.DerivationPath != "" {
					pathNoM := strings.TrimPrefix(msg.DerivationPath, "m/")
					pathNoM = strings.ReplaceAll(pathNoM, "'", "h")
					keyOrigin = fmt.Sprintf("%s/%s", msg.Fingerprint, pathNoM)
				}
				// Add as cosigner for multisig setup
				cs := wallet.MultisigCosignerConfig{
					Xpub:             msg.Xpub,
					Fingerprint:      msg.Fingerprint,
					KeyOrigin:        keyOrigin,
					ActualDerivation: msg.DerivationPath,
					DeviceType:       msg.DeviceType,
					Name:             fmt.Sprintf("HW Cosigner %d", len(m.multisigCosigners)+1),
					IsMine:           false, // Hardware wallet key - not stored locally
				}
				m.multisigCosigners = append(m.multisigCosigners, cs)
				m.hwiError = nil
				m.err = nil

				// Check if we have all cosigners
				if len(m.multisigCosigners) >= m.multisigN {
					m.step = StepMultisigReview
					m.cursor = 0
				}
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			// Don't handle globally when a sub-input is active (e.g. custom path editing)
			if m.showMultisigCustomPath || m.showCustomPath {
				break
			}
			if m.step == StepChooseAction {
				return m, tea.Quit
			}
			// Clear errors when navigating back
			m.hwiError = nil
			m.err = nil
			// Go back to previous step with proper navigation
			m.step = m.getPreviousStep()
			return m, nil
		}

		// Handle input based on current step
		switch m.step {
		case StepChooseAction:
			return m.handleChooseAction(msg)

		case StepChooseWalletType:
			return m.handleChooseWalletType(msg)

		case StepEnterName:
			m.nameInput, cmd = m.nameInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" && m.nameInput.Value() != "" {
				walletName := m.nameInput.Value()
				if err := wallet.ValidateWalletName(walletName); err != nil {
					m.err = err
					return m, tea.Batch(cmds...)
				}

				// Check if wallet already exists
				if m.walletManager.WalletExists(walletName) {
					m.err = fmt.Errorf("wallet '%s' already exists. Try '%s_2'?", walletName, walletName)
					return m, tea.Batch(cmds...)
				}

				m.walletName = walletName
				m.err = nil
				m.step = StepEnterPassword
				cmds = append(cmds, m.passwordInput.Focus())
			}

		case StepChooseWordCount:
			return m.handleChooseWordCount(msg)

		case StepShowMnemonic:
			switch msg.String() {
			case "enter":
				// Wallet creation complete! Navigate to dashboard
				// First, unlock the wallet we just created
				err := m.walletManager.UnlockWallet(m.walletName, m.password)
				if err != nil {
					m.err = err
					return m, tea.Batch(cmds...)
				}

				// Create default account with selected script type
				_, err = m.walletManager.CreateAccount("Main Account", m.selectedScriptType, m.customDerivationPath)
				if err != nil {
					m.err = err
					return m, tea.Batch(cmds...)
				}

				// Save this wallet as last used (for auto-login)
				m.saveLastWalletName(m.walletName)

				// Navigate to dashboard
				return m, func() tea.Msg {
					return NavigateMsg{Target: StateMainDashboard}
				}
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

		case StepEnterPassword:
			m.passwordInput, cmd = m.passwordInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" && m.passwordInput.Value() != "" {
				if err := wallet.ValidatePassword(m.passwordInput.Value()); err != nil {
					m.err = err
					return m, tea.Batch(cmds...)
				}
				m.err = nil
				m.password = m.passwordInput.Value()
				m.step = StepConfirmPassword
				cmds = append(cmds, m.confirmPasswordInput.Focus())
			}

		case StepConfirmPassword:
			m.confirmPasswordInput, cmd = m.confirmPasswordInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" {
				if m.confirmPasswordInput.Value() == m.password {
					// Check if we're coming from descriptor import
					if m.parsedDescriptor != nil || m.parsedSingleSig != nil {
						// Create wallet from parsed descriptor (single-sig or multisig)
						err := m.createWalletFromDescriptor()
						if err != nil {
							m.err = err
							return m, tea.Batch(cmds...)
						}

						// Unlock the wallet
						err = m.walletManager.UnlockWallet(m.walletName, m.password)
						if err != nil {
							m.err = fmt.Errorf("wallet created but failed to unlock: %w", err)
							return m, tea.Batch(cmds...)
						}

						// Save as last wallet
						m.saveLastWalletName(m.walletName)

						// Navigate to dashboard
						return m, func() tea.Msg {
							return NavigateMsg{Target: StateMainDashboard}
						}
					}

					if m.isMultisig {
						// Normal multisig flow - go to M-of-N configuration
						m.step = StepMultisigChooseType
						m.cursor = 0
					} else {
						// Single-sig flow - go to script type selection
						m.step = StepChooseScriptType
						m.cursor = 2 // Default to Native SegWit (recommended)
					}
				} else {
					m.err = fmt.Errorf("passwords do not match")
					m.confirmPasswordInput.SetValue("")
				}
			}

		case StepChooseScriptType:
			return m.handleChooseScriptType(msg)

		// Restore flow handlers
		case StepRestoreChooseMethod:
			return m.handleRestoreChooseMethod(msg)

		case StepRestoreEnterName:
			m.nameInput, cmd = m.nameInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" && m.nameInput.Value() != "" {
				walletName := m.nameInput.Value()
				if err := wallet.ValidateWalletName(walletName); err != nil {
					m.err = err
					return m, tea.Batch(cmds...)
				}

				// Check if wallet already exists
				if m.walletManager.WalletExists(walletName) {
					m.err = fmt.Errorf("wallet '%s' already exists. Try '%s_2'?", walletName, walletName)
					return m, tea.Batch(cmds...)
				}

				m.walletName = walletName
				m.err = nil
				m.step = StepRestoreEnterMnemonic
				cmds = append(cmds, m.mnemonicInput.Focus())
			}

		case StepRestoreEnterMnemonic:
			m.mnemonicInput, cmd = m.mnemonicInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" && m.mnemonicInput.Value() != "" {
				m.restoreMnemonic = m.mnemonicInput.Value()
				// Validate mnemonic
				if !wallet.ValidateMnemonic(m.restoreMnemonic) {
					m.err = fmt.Errorf("invalid mnemonic phrase")
					return m, tea.Batch(cmds...)
				}
				m.err = nil
				m.step = StepRestoreChooseScriptType
				m.cursor = 2 // Default to Native SegWit (recommended)
			}

		case StepRestoreChooseScriptType:
			return m.handleChooseScriptType(msg)

		case StepRestoreEnterPassword:
			m.passwordInput, cmd = m.passwordInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" && m.passwordInput.Value() != "" {
				if err := wallet.ValidatePassword(m.passwordInput.Value()); err != nil {
					m.err = err
					return m, tea.Batch(cmds...)
				}
				m.err = nil
				m.password = m.passwordInput.Value()
				m.step = StepRestoreConfirmPassword
				cmds = append(cmds, m.confirmPasswordInput.Focus())
			}

		case StepRestoreConfirmPassword:
			m.confirmPasswordInput, cmd = m.confirmPasswordInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" {
				if m.confirmPasswordInput.Value() == m.password {
					// Restore the wallet
					err := m.walletManager.RestoreWallet(m.walletName, m.password, m.restoreMnemonic)
					if err != nil {
						m.err = fmt.Errorf("failed to restore wallet: %w", err)
						return m, tea.Batch(cmds...)
					}

					// Unlock the restored wallet
					err = m.walletManager.UnlockWallet(m.walletName, m.password)
					if err != nil {
						m.err = fmt.Errorf("failed to unlock wallet: %w", err)
						return m, tea.Batch(cmds...)
					}

					// Create default account with selected script type
					_, err = m.walletManager.CreateAccount("Main Account", m.selectedScriptType, m.customDerivationPath)
					if err != nil {
						m.err = fmt.Errorf("failed to create account: %w", err)
						return m, tea.Batch(cmds...)
					}

					// Save this wallet as last used (for auto-login)
					m.saveLastWalletName(m.walletName)

					// Navigate to dashboard
					return m, func() tea.Msg {
						return NavigateMsg{Target: StateMainDashboard}
					}
				} else {
					m.err = fmt.Errorf("passwords do not match")
					m.confirmPasswordInput.SetValue("")
				}
			}

		case StepRestoreChooseFile:
			return m.handleRestoreChooseFile(msg)

		case StepRestoreFilePassword:
			if !m.passwordInput.Focused() {
				m.passwordInput.Focus()
				cmds = append(cmds, textinput.Blink)
			}
			m.passwordInput, cmd = m.passwordInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" && m.passwordInput.Value() != "" {
				m.password = m.passwordInput.Value()

				// Try to unlock the selected wallet
				err := m.walletManager.UnlockWallet(m.selectedWallet, m.password)
				if err != nil {
					m.err = fmt.Errorf("failed to unlock wallet: %w", err)
					m.passwordInput.SetValue("")
					return m, tea.Batch(cmds...)
				}

				// Save this wallet as last used (for auto-login)
				m.saveLastWalletName(m.selectedWallet)

				// Navigate to dashboard
				return m, func() tea.Msg {
					return NavigateMsg{Target: StateMainDashboard}
				}
			}

		// Multisig flow (after password confirmation)
		case StepMultisigChooseType:
			return m.handleMultisigChooseType(msg)

		case StepMultisigConfirm:
			return m.handleMultisigConfirm(msg)

		case StepMultisigEnterCosigners:
			return m.handleMultisigEnterCosigners(msg)

		case StepMultisigReview:
			return m.handleMultisigReview(msg)

		// Single-sig hardware wallet flow (after StepChooseScriptType)
		case StepChooseKeySource:
			return m.handleChooseKeySource(msg)

		case StepHWISetup:
			return m.handleHWISetup(msg)

		case StepHardwareWalletConnect:
			return m.handleHardwareWalletConnect(msg)

		case StepHardwareWalletReview:
			return m.handleHardwareWalletReview(msg)

		case StepMultisigImportDescriptor:
			return m.handleMultisigImportDescriptor(msg)

		case StepMultisigImportReview:
			return m.handleMultisigImportReview(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m WalletSetupModel) handleChooseAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 { // 2 options (0, 1)
			m.cursor++
		}
	case "enter":
		if m.cursor == 0 {
			// Create new wallet - go to wallet type selection
			m.isRestore = false
			m.cursor = 0
			m.step = StepChooseWalletType
		} else if m.cursor == 1 {
			// Restore wallet - show method selection
			m.isRestore = true
			m.isMultisig = false
			m.cursor = 0 // Reset cursor for next menu
			m.step = StepRestoreChooseMethod

			// Load list of existing wallets
			wallets, err := m.walletManager.ListWallets()
			if err != nil {
				m.err = fmt.Errorf("failed to list wallets: %w", err)
			} else {
				m.existingWallets = wallets
			}
		}
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m WalletSetupModel) handleChooseWalletType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			// Single-sig wallet
			m.isMultisig = false
			m.step = StepEnterName
			m.nameInput.SetValue("")
			return m, m.nameInput.Focus()
		} else {
			// Multisig wallet
			m.isMultisig = true
			m.step = StepEnterName
			m.nameInput.SetValue("")
			return m, m.nameInput.Focus()
		}
	case "esc":
		m.step = StepChooseAction
		m.cursor = 0
	}
	return m, nil
}

func (m WalletSetupModel) handleChooseScriptType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When custom path input is active, route keys to it
	if m.showCustomPath {
		switch msg.String() {
		case "esc":
			m.showCustomPath = false
			m.customDerivationPath = ""
			m.customPathInput.SetValue("")
			m.customPathInput.Blur()
			return m, nil
		case "enter":
			pathValue := strings.TrimSpace(m.customPathInput.Value())
			if pathValue != "" {
				if err := wallet.ValidateAccountPath(pathValue); err != nil {
					m.err = fmt.Errorf("invalid path: %w", err)
					return m, nil
				}
				m.customDerivationPath = pathValue
			}
			m.showCustomPath = false
			m.customPathInput.Blur()
			// Proceed to next step
			return m.proceedFromScriptType()
		default:
			var cmd tea.Cmd
			m.customPathInput, cmd = m.customPathInput.Update(msg)
			m.err = nil // Clear any previous validation error
			return m, cmd
		}
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 3 { // 4 options: Legacy, Nested, Native, Taproot
			m.cursor++
		}
	case "p":
		// Toggle custom derivation path input
		m.showCustomPath = true
		// Pre-fill with standard path for current selection
		addrType := wallet.AddressTypeSegWitNative
		switch m.cursor {
		case 0:
			addrType = wallet.AddressTypeLegacy
		case 1:
			addrType = wallet.AddressTypeSegWitNested
		case 3:
			addrType = wallet.AddressTypeTaproot
		}
		defaultPath := wallet.AccountPathForAddressType(addrType, true, 0)
		m.customPathInput.SetValue(defaultPath)
		return m, m.customPathInput.Focus()
	case "enter":
		// Set script type based on cursor
		switch m.cursor {
		case 0:
			m.selectedScriptType = wallet.AddressTypeLegacy
		case 1:
			m.selectedScriptType = wallet.AddressTypeSegWitNested
		case 2:
			m.selectedScriptType = wallet.AddressTypeSegWitNative
		case 3:
			m.selectedScriptType = wallet.AddressTypeTaproot
		}
		m.customDerivationPath = "" // Standard path
		return m.proceedFromScriptType()
	}
	return m, nil
}

// proceedFromScriptType advances to the next step after script type + optional custom path selection.
func (m WalletSetupModel) proceedFromScriptType() (tea.Model, tea.Cmd) {
	// Set script type from cursor if not already set
	if m.customDerivationPath != "" {
		// Infer address type from custom path's purpose
		purpose, _, _, err := wallet.ParseAccountPath(m.customDerivationPath)
		if err == nil {
			switch purpose {
			case 44:
				m.selectedScriptType = wallet.AddressTypeLegacy
			case 49:
				m.selectedScriptType = wallet.AddressTypeSegWitNested
			case 84:
				m.selectedScriptType = wallet.AddressTypeSegWitNative
			case 86:
				m.selectedScriptType = wallet.AddressTypeTaproot
			}
		}
	}

	if m.step == StepChooseScriptType {
		m.step = StepChooseKeySource
		m.cursor = 0
		return m, nil
	}
	// Restore flow
	m.step = StepRestoreEnterPassword
	return m, m.passwordInput.Focus()
}

func (m WalletSetupModel) handleChooseWordCount(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		// Set word count
		if m.cursor == 0 {
			m.wordCount = 12
		} else {
			m.wordCount = 24
		}

		// Generate wallet
		mnemonic, err := m.walletManager.CreateWallet(m.walletName, m.password, m.wordCount)
		if err != nil {
			m.err = err
			return m, nil
		}

		m.generatedMnemonic = mnemonic
		m.step = StepShowMnemonic
		m.cursor = 0
	}
	return m, nil
}

func (m WalletSetupModel) handleRestoreChooseMethod(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 3 { // 4 options now (0, 1, 2, 3)
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case 0:
			// Restore from seed phrase (single-sig)
			m.isMultisig = false
			m.step = StepRestoreEnterName
			m.cursor = 0
			return m, m.nameInput.Focus()
		case 1:
			// Import multisig wallet (manual xpub entry)
			m.isMultisig = true
			m.step = StepEnterName
			m.nameInput.SetValue("")
			m.cursor = 0
			return m, m.nameInput.Focus()
		case 2:
			// Import from descriptor
			m.isMultisig = true
			m.step = StepMultisigImportDescriptor
			m.descriptorInput.SetValue("")
			m.parsedDescriptor = nil
			m.descriptorWarnings = nil
			m.err = nil
			m.cursor = 0
			return m, m.descriptorInput.Focus()
		case 3:
			// Restore from wallet file
			if len(m.existingWallets) == 0 {
				m.err = fmt.Errorf("no existing wallet files found")
				return m, nil
			}
			m.step = StepRestoreChooseFile
			m.cursor = 0
		}
	}
	return m, nil
}

func (m WalletSetupModel) handleRestoreChooseFile(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.existingWallets)-1 {
			m.cursor++
		}
	case "enter":
		if m.cursor < len(m.existingWallets) {
			m.selectedWallet = m.existingWallets[m.cursor]
			m.step = StepRestoreFilePassword
			m.passwordInput.SetValue("")
			return m, m.passwordInput.Focus()
		}
	}
	return m, nil
}

// saveLastWalletName saves the wallet name to config for auto-login
func (m *WalletSetupModel) saveLastWalletName(walletName string) {
	if m.configManager == nil {
		return
	}

	cfg, err := m.configManager.Load()
	if err != nil || cfg == nil {
		return
	}

	cfg.LastWalletName = walletName
	_ = m.configManager.Save(cfg) // Ignore error - non-critical
}

// getPreviousStep returns the correct previous step for ESC navigation
func (m WalletSetupModel) getPreviousStep() WalletSetupStep {
	switch m.step {
	// Create wallet flow
	case StepChooseWalletType:
		return StepChooseAction
	case StepEnterName:
		// If importing multisig from restore flow, go back to restore method
		if m.isRestore && m.isMultisig {
			return StepRestoreChooseMethod
		}
		return StepChooseWalletType
	case StepEnterPassword:
		return StepEnterName
	case StepConfirmPassword:
		return StepEnterPassword
	case StepChooseScriptType:
		return StepConfirmPassword
	case StepChooseKeySource:
		return StepChooseScriptType
	case StepChooseWordCount:
		return StepChooseKeySource
	case StepShowMnemonic:
		return StepChooseWordCount

	// Single-sig hardware wallet flow
	case StepHWISetup:
		return StepChooseKeySource
	case StepHardwareWalletConnect:
		return StepChooseKeySource
	case StepHardwareWalletReview:
		return StepHardwareWalletConnect

	// Multisig flow (after password confirmation)
	case StepMultisigChooseType:
		return StepConfirmPassword
	case StepMultisigConfirm:
		return StepMultisigChooseType
	case StepMultisigEnterCosigners:
		return StepMultisigConfirm
	case StepMultisigReview:
		return StepMultisigEnterCosigners

	// Descriptor import flow
	case StepMultisigImportDescriptor:
		return StepRestoreChooseMethod
	case StepMultisigImportReview:
		return StepMultisigImportDescriptor

	// Restore from seed phrase flow
	case StepRestoreChooseMethod:
		return StepChooseAction
	case StepRestoreEnterName:
		return StepRestoreChooseMethod
	case StepRestoreEnterMnemonic:
		return StepRestoreEnterName
	case StepRestoreChooseScriptType:
		return StepRestoreEnterMnemonic
	case StepRestoreEnterPassword:
		return StepRestoreChooseScriptType
	case StepRestoreConfirmPassword:
		return StepRestoreEnterPassword

	// Restore from file flow
	case StepRestoreChooseFile:
		return StepRestoreChooseMethod
	case StepRestoreFilePassword:
		// If auto-login mode, go back to choose action
		if m.hasLastWallet {
			return StepChooseAction
		}
		return StepRestoreChooseFile

	default:
		return StepChooseAction
	}
}

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
	case StepChooseWordCount:
		return 0.80
	case StepShowMnemonic:
		return 1.0

	// Restore wallet flow from seed (5 steps)
	case StepRestoreChooseMethod:
		return 0.0
	case StepRestoreEnterName:
		return 0.20
	case StepRestoreEnterMnemonic:
		return 0.40
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
	case StepChooseAction, StepRestoreChooseMethod, StepChooseWalletType,
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
	case StepChooseWordCount:
		stepNum, totalSteps = 4, 5
	case StepShowMnemonic:
		stepNum, totalSteps = 5, 5

	// Restore wallet flow from seed (5 steps)
	case StepRestoreEnterName:
		stepNum, totalSteps = 1, 5
	case StepRestoreEnterMnemonic:
		stepNum, totalSteps = 2, 5
	case StepRestoreChooseScriptType:
		stepNum, totalSteps = 3, 5
	case StepRestoreEnterPassword:
		stepNum, totalSteps = 4, 5
	case StepRestoreConfirmPassword:
		stepNum, totalSteps = 5, 5

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
	if m.step != StepChooseAction && m.step != StepRestoreChooseMethod {
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
	case StepChooseScriptType:
		content.WriteString(m.viewChooseScriptType())
	case StepChooseWordCount:
		content.WriteString(m.viewChooseWordCount())
	case StepShowMnemonic:
		content.WriteString(m.viewShowMnemonic())

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

// =============================================================================
// Single-Sig Hardware Wallet Flow Views and Handlers
// =============================================================================

func (m WalletSetupModel) viewChooseKeySource() string {
	info := styles.DimText.Render("How do you want to store your keys?")

	options := []struct {
		name string
		desc string
	}{
		{"Generate New Seed", "Create a new 12 or 24 word seed phrase"},
		{"Import from Hardware Wallet", "Use a Trezor, Ledger, Coldcard, etc."},
	}

	menu := ""
	for i, opt := range options {
		cursor := " "
		if m.cursor == i {
			cursor = "▶"
			line := fmt.Sprintf("%s %s", cursor, opt.name)
			if i == 0 {
				line += " ✓" // Mark generate as recommended for most users
			}
			menu += styles.SelectedItemStyle.Render(line) + "\n"
			menu += styles.DimText.Render("   "+opt.desc) + "\n"
		} else {
			line := fmt.Sprintf("%s %s", cursor, opt.name)
			menu += styles.UnselectedItemStyle.Render(line) + "\n"
		}
	}

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
			// Generate new seed - go to word count selection
			m.step = StepChooseWordCount
			m.cursor = 0
		} else {
			// Import from hardware wallet - scan for devices
			m.step = StepHardwareWalletConnect
			m.cursor = 0
			m.hwiLoading = true
			m.hwiError = nil
			m.hwiDevices = nil
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSingleSig())
		}
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

	// Verification status
	if m.hwiVerifying {
		content.WriteString(m.spinner.View() + " " +
			styles.DimText.Render("Verifying installation...") + "\n")
	} else if m.hwiVerifyMsg != "" {
		if m.hwiVerifyOK {
			content.WriteString(styles.SuccessText.Render("  "+m.hwiVerifyMsg) + "\n")
		} else {
			content.WriteString(styles.ErrorText.Render("  "+m.hwiVerifyMsg) + "\n")
		}
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
	title := styles.TitleStyle.Render("Connect Hardware Wallet [4/5]")

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

		for i, device := range m.hwiDevices {
			deviceName := string(device.Type)
			if device.Model != "" {
				deviceName = device.Model
			}

			fp := device.Fingerprint
			if fp == "" || fp == "00000000" {
				fp = "unknown"
			} else if len(fp) > 8 {
				fp = fp[:8]
			}

			line := fmt.Sprintf("%s [%s]", deviceName, fp)

			if i == m.selectedHWDevice {
				line = "▶ " + line
				content.WriteString(styles.SelectedItemStyle.Render(line) + "\n")
			} else {
				line = "  " + line
				content.WriteString(styles.UnselectedItemStyle.Render(line) + "\n")
			}

			if device.NeedsPin {
				content.WriteString("    " + styles.WarningText.Render("⚠ needs PIN — unlock device first") + "\n")
			}
			if device.NeedsPass {
				content.WriteString(styles.DimText.Render("    (needs passphrase)") + "\n")
			}
			if device.Error != "" {
				content.WriteString("    " + styles.ErrorText.Render(device.Error) + "\n")
			}
		}

		// PIN warning for selected device
		if m.selectedHWDevice >= 0 && m.selectedHWDevice < len(m.hwiDevices) {
			if m.hwiDevices[m.selectedHWDevice].NeedsPin {
				content.WriteString("\n" + styles.WarningText.Render(
					"⚠ Enter PIN on your device first, then press Enter.") + "\n")
			}
		}
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
	title := styles.TitleStyle.Render("Review Hardware Wallet [5/5]")

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
	keyOrigin := fmt.Sprintf("%s/%d'/1'/0'", m.hwFingerprint, purpose)

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
		keyOrigin := fmt.Sprintf("%s/%d'/1'/0'", m.hwFingerprint, purpose)

		// Create the hardware wallet
		err := m.walletManager.CreateHardwareWallet(
			m.walletName,
			m.password,
			m.hwDerivedXpub,
			m.hwFingerprint,
			keyOrigin,
			m.selectedScriptType,
			deviceType,
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
	walletMgr := m.walletManager
	scriptType := m.selectedScriptType

	return func() tea.Msg {
		hwi := walletMgr.GetHWIClient()
		if hwi == nil || !hwi.IsAvailable() {
			return HWIXpubMsg{Err: fmt.Errorf("HWI not installed. %s", wallet.HWIInstallInstructions())}
		}

		// Determine derivation path based on script type
		// BIP44/49/84/86 for single-sig: m/purpose'/coin'/account'
		var path string
		switch scriptType {
		case wallet.AddressTypeLegacy:
			path = "m/44'/1'/0'" // BIP44 Legacy
		case wallet.AddressTypeSegWitNested:
			path = "m/49'/1'/0'" // BIP49 Nested SegWit
		case wallet.AddressTypeSegWitNative:
			path = "m/84'/1'/0'" // BIP84 Native SegWit
		case wallet.AddressTypeTaproot:
			path = "m/86'/1'/0'" // BIP86 Taproot
		default:
			path = "m/84'/1'/0'" // Default to native segwit
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

// =============================================================================
// Multisig Flow Views and Handlers
// =============================================================================

func (m WalletSetupModel) viewMultisigChooseType() string {
	var title string
	if m.isRestore {
		title = styles.TitleStyle.Render("Import Multisig [3/5]")
	} else {
		title = ""
	}

	var menuContent string

	// M-of-N section with inline adjustment
	menuContent += styles.BoldText.Render("Signature Scheme:") + "\n"

	// Row 0: M (required signatures)
	mLabel := fmt.Sprintf("Required signatures (M): ◄ %d ►", m.multisigM)
	if m.cursor == 0 {
		menuContent += styles.SelectedItemStyle.Render("▶ "+mLabel) + "\n"
	} else {
		menuContent += styles.UnselectedItemStyle.Render("  "+mLabel) + "\n"
	}

	// Row 1: N (total cosigners)
	nLabel := fmt.Sprintf("Total cosigners (N):     ◄ %d ►", m.multisigN)
	if m.cursor == 1 {
		menuContent += styles.SelectedItemStyle.Render("▶ "+nLabel) + "\n"
	} else {
		menuContent += styles.UnselectedItemStyle.Render("  "+nLabel) + "\n"
	}

	menuContent += styles.DimText.Render(fmt.Sprintf("  %d-of-%d multisig", m.multisigM, m.multisigN)) + "\n"

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
		check := ""
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
	customCheck := ""
	if m.multisigCustomDerivPath != "" {
		customCheck = " ✓"
	}
	menuContent += customStyle.Render(customCursor+" Custom Path"+customCheck) + "\n"

	// Derivation path box
	menuContent += "\n"
	if m.showMultisigCustomPath {
		pathBox := styles.BoldText.Render("Derivation Path") + "\n" +
			m.customPathInput.View()
		if m.err != nil {
			pathBox += "\n" + styles.ErrorText.Render(m.err.Error())
		}
		pathBox += "\n" + styles.DimText.Render("enter: confirm • esc: cancel")
		menuContent += styles.InfoBoxStyle.MarginBottom(0).Padding(0, 1).Render(pathBox) + "\n"
	} else {
		effectivePath := m.multisigCustomDerivPath
		if effectivePath == "" {
			effectivePath = wallet.DefaultMultisigAccountPath(m.multisigScriptType, true)
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
				prefill = wallet.DefaultMultisigAccountPath(m.multisigScriptType, true)
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
		effectivePath = wallet.DefaultMultisigAccountPath(m.multisigScriptType, true)
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
		titlePrefix = "Import Multisig [4/5] - "
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
		inputSection = inputLabel + " (xpub)\n" + info + "\n" + m.multisigCurrentXpub.View()
	case 1:
		// Seed mode
		derivInfo := "The BIP48 xpub will be derived automatically."
		if m.multisigCustomDerivPath != "" {
			derivInfo = fmt.Sprintf("The xpub will be derived at %s.", m.multisigCustomDerivPath)
		}
		info := styles.DimText.Render(
			"Enter the cosigner's seed phrase (12 or 24 words).\n" + derivInfo)
		inputSection = inputLabel + " (seed phrase)\n" + info + "\n" + m.multisigCosignerSeed.View()
	case 2:
		// Hardware wallet mode
		derivInfo := "The BIP48 xpub will be retrieved from the device."
		if m.multisigCustomDerivPath != "" {
			derivInfo = fmt.Sprintf("The xpub will be retrieved at %s.", m.multisigCustomDerivPath)
		}
		info := styles.DimText.Render(
			"Connect a hardware wallet (Trezor, Ledger, Coldcard, etc.)\n" + derivInfo)

		var statusLine string
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

	// Show general errors
	if m.err != nil {
		content += "\n" + styles.RenderErrorBox("⚠ "+m.err.Error(), m.width)
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

		for i, device := range m.hwiDevices {
			deviceName := string(device.Type)
			if device.Model != "" {
				deviceName = device.Model
			}

			fp := device.Fingerprint
			if fp == "" || fp == "00000000" {
				fp = "unknown"
			} else if len(fp) > 8 {
				fp = fp[:8]
			}

			line := fmt.Sprintf("  %s [%s]", deviceName, fp)

			if i == m.selectedHWDevice {
				line = styles.HighlightText.Render("► " + line)
			} else {
				line = styles.DimText.Render("  " + line)
			}

			if device.NeedsPin {
				line += " " + styles.WarningText.Render("⚠ needs PIN")
			}
			if device.NeedsPass {
				line += " " + styles.DimText.Render("(needs passphrase)")
			}
			if device.Error != "" {
				line += "\n    " + styles.ErrorText.Render(device.Error)
			}

			content.WriteString(line + "\n")
		}

		// PIN warning for selected device
		if m.selectedHWDevice >= 0 && m.selectedHWDevice < len(m.hwiDevices) {
			if m.hwiDevices[m.selectedHWDevice].NeedsPin {
				content.WriteString("\n" + styles.WarningText.Render(
					"⚠ Enter PIN on your device first, then press Enter.") + "\n")
			}
		}
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
		if m.multisigInputMode == 0 {
			m.multisigCurrentXpub.Focus()
			m.multisigCosignerSeed.Blur()
		} else if m.multisigInputMode == 1 {
			m.multisigCosignerSeed.Focus()
			m.multisigCurrentXpub.Blur()
		} else {
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
		if m.multisigInputMode == 0 {
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
		} else if m.multisigInputMode == 1 {
			// Seed mode - derive BIP48 xpub from mnemonic
			seedVal := strings.TrimSpace(m.multisigCosignerSeed.Value())
			if seedVal != "" {
				// Derive cosigner config from mnemonic
				cs, err := wallet.DeriveMultisigCosignerFromMnemonic(
					seedVal,
					m.multisigScriptType,
					true, // testnet
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
		} else if m.multisigInputMode == 2 {
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
		err := m.walletManager.CreateMultisigWallet(
			m.walletName,
			m.password,
			m.multisigM,
			m.multisigCosigners,
			m.multisigScriptType,
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
	walletMgr := m.walletManager
	scriptType := m.multisigScriptType
	customPath := m.multisigCustomDerivPath

	return func() tea.Msg {
		hwi := walletMgr.GetHWIClient()
		if hwi == nil || !hwi.IsAvailable() {
			return HWIXpubMsg{Err: fmt.Errorf("HWI not installed. %s", wallet.HWIInstallInstructions())}
		}

		// Use custom path if set, otherwise standard path for script type
		var path string
		if customPath != "" {
			path = customPath
		} else {
			path = wallet.DefaultMultisigAccountPath(scriptType, true)
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

// =============================================================================
// Descriptor Import Handlers
// =============================================================================

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
		// Create multisig wallet
		return m.walletManager.CreateMultisigWallet(
			m.walletName,
			m.password,
			m.multisigM,
			m.multisigCosigners,
			m.multisigScriptType,
		)
	}

	// Create single-sig watch-only wallet (like hardware wallet)
	cfg := m.parsedSingleSig
	keyOrigin := cfg.KeyOrigin
	if keyOrigin == "" {
		// Build a basic key origin if we only have fingerprint
		keyOrigin = cfg.Fingerprint
	}

	return m.walletManager.CreateHardwareWallet(
		m.walletName,
		m.password,
		cfg.Xpub,
		cfg.Fingerprint,
		keyOrigin,
		cfg.AddressType,
		"descriptor", // device type
	)
}
