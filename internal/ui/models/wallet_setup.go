package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// WalletSetupStep represents the current step in wallet setup
type WalletSetupStep int

const (
	StepChooseAction     WalletSetupStep = iota
	StepChooseWalletType                 // NEW: Single-sig or Multisig
	StepEnterName
	StepEnterPassword
	StepConfirmPassword
	StepEnterPassphrase   // optional BIP39 passphrase (create, software seed)
	StepConfirmPassphrase // confirm BIP39 passphrase
	StepChooseScriptType
	StepChooseWordCount
	StepShowMnemonic
	StepConfirmMnemonic
	StepComplete
	StepChooseNetwork // mainnet / testnet4 (create + restore, software seed)

	// Restore flow steps
	StepRestoreChooseMethod
	StepRestoreEnterName
	StepRestoreEnterMnemonic
	StepRestoreChooseScriptType
	StepRestoreEnterPassword
	StepRestoreConfirmPassword
	StepRestoreEnterPassphrase   // optional BIP39 passphrase (restore from seed)
	StepRestoreConfirmPassphrase // confirm BIP39 passphrase
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
	// Seed backup check after creating a wallet (see wallet_setup_backup.go)
	backupWords       []int // 0-based word positions to ask for, ascending
	backupPos         int   // index into backupWords of the word being asked
	backupInput       textinput.Model
	backupSkipConfirm bool // "skip the check?" prompt is open

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
	progressBar            progress.Model
	nameInput              textinput.Model
	passwordInput          textinput.Model
	confirmPasswordInput   textinput.Model
	passphraseInput        textinput.Model
	confirmPassphraseInput textinput.Model
	mnemonicInput          textinput.Model

	// Data
	walletName           string
	password             string
	passphrase           string // BIP39 passphrase (empty = none); wiped after use
	wordCount            int
	network              string // wallet.NetworkTestnet or wallet.NetworkMainnet
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

	passphraseInput := textinput.New()
	passphraseInput.Placeholder = "optional"
	passphraseInput.EchoMode = textinput.EchoPassword
	passphraseInput.CharLimit = 256

	confirmPassphraseInput := textinput.New()
	confirmPassphraseInput.Placeholder = "Confirm passphrase"
	confirmPassphraseInput.EchoMode = textinput.EchoPassword
	confirmPassphraseInput.CharLimit = 256

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
	var startStep = StepChooseNetwork

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
		step:                   startStep,
		cursor:                 0,
		walletManager:          walletManager,
		configManager:          configManager,
		progressBar:            prog,
		nameInput:              nameInput,
		passwordInput:          passwordInput,
		confirmPasswordInput:   confirmPasswordInput,
		passphraseInput:        passphraseInput,
		confirmPassphraseInput: confirmPassphraseInput,
		mnemonicInput:          mnemonicInput,
		wordCount:              12, // Default to 12 words
		network:                wallet.NetworkMainnet,
		hasLastWallet:          hasLastWallet,
		lastWalletName:         lastWalletName,
		selectedWallet:         lastWalletName, // Pre-select the last wallet
		existingWallets:        existingWallets,
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

			switch m.step {
			case StepHardwareWalletConnect:
				// Single-sig hardware wallet: advance to review
				m.step = StepHardwareWalletReview
				m.cursor = 0
			case StepMultisigEnterCosigners:
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
			if m.step == StepChooseNetwork {
				return m, tea.Quit
			}
			if m.step == StepConfirmMnemonic && m.backupSkipConfirm {
				m.backupSkipConfirm = false
				return m, nil
			}
			// Clear errors when navigating back
			m.hwiError = nil
			m.err = nil
			prev := m.getPreviousStep()
			// Wipe BIP39 passphrase when leaving / re-entering passphrase steps
			switch m.step {
			case StepEnterPassphrase, StepRestoreEnterPassphrase:
				m.wipePassphrase()
			case StepConfirmPassphrase, StepRestoreConfirmPassphrase:
				m.confirmPassphraseInput.SetValue("")
				if prev != StepEnterPassphrase && prev != StepRestoreEnterPassphrase {
					m.wipePassphrase()
				}
			case StepChooseWordCount:
				if prev == StepEnterPassphrase || prev == StepConfirmPassphrase {
					m.wipePassphrase()
				}
			}
			m.step = prev
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

		case StepEnterPassphrase:
			m.passphraseInput, cmd = m.passphraseInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" {
				m.err = nil
				m.passphrase = m.passphraseInput.Value()
				m.confirmPassphraseInput.SetValue("")
				m.step = StepConfirmPassphrase
				cmds = append(cmds, m.confirmPassphraseInput.Focus())
			}

		case StepConfirmPassphrase:
			m.confirmPassphraseInput, cmd = m.confirmPassphraseInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" {
				if m.confirmPassphraseInput.Value() == m.passphrase {
					m.err = nil
					m.confirmPassphraseInput.SetValue("")
					m.passphraseInput.SetValue("")
					m.step = StepChooseWordCount
					m.cursor = 0
				} else {
					m.err = fmt.Errorf("passphrases do not match")
					m.confirmPassphraseInput.SetValue("")
				}
			}

		case StepChooseWordCount:
			return m.handleChooseWordCount(msg)

		case StepChooseNetwork:
			return m.handleChooseNetwork(msg)

		case StepConfirmMnemonic:
			return m.handleConfirmMnemonic(msg)

		case StepShowMnemonic:
			switch msg.String() {
			case "enter":
				// Ask for a few words back before opening the wallet.
				m.startBackupCheck()
				m.step = StepConfirmMnemonic
				return m, textinput.Blink
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
						// Normal multisig flow - network was already chosen
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
				m.wipePassphrase()
				m.step = StepRestoreEnterPassphrase
				cmds = append(cmds, m.passphraseInput.Focus())
			}

		case StepRestoreEnterPassphrase:
			m.passphraseInput, cmd = m.passphraseInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" {
				m.err = nil
				m.passphrase = m.passphraseInput.Value()
				m.confirmPassphraseInput.SetValue("")
				m.step = StepRestoreConfirmPassphrase
				cmds = append(cmds, m.confirmPassphraseInput.Focus())
			}

		case StepRestoreConfirmPassphrase:
			m.confirmPassphraseInput, cmd = m.confirmPassphraseInput.Update(msg)
			cmds = append(cmds, cmd)
			if msg.String() == "enter" {
				if m.confirmPassphraseInput.Value() == m.passphrase {
					m.err = nil
					m.confirmPassphraseInput.SetValue("")
					m.passphraseInput.SetValue("")
					m.step = StepRestoreChooseScriptType
					m.cursor = 2 // Default to Native SegWit (recommended)
				} else {
					m.err = fmt.Errorf("passphrases do not match")
					m.confirmPassphraseInput.SetValue("")
				}
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
					// Restore the wallet (BIP39 passphrase already collected; empty = none)
					err := m.walletManager.RestoreWalletOnNetwork(m.walletName, m.password, m.restoreMnemonic, m.passphrase, m.network)
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
					m.wipePassphrase()

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
		switch m.cursor {
		case 0:
			// Create new wallet - go to wallet type selection
			m.isRestore = false
			m.cursor = 0
			m.step = StepChooseWalletType
		case 1:
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
	// Restore flow: network was already chosen before the seed was entered.
	m.step = StepRestoreEnterPassword
	m.cursor = 0
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

		// Generate wallet (BIP39 passphrase already collected; empty = none)
		mnemonic, err := m.walletManager.CreateWalletOnNetwork(m.walletName, m.password, m.wordCount, m.network, m.passphrase)
		if err != nil {
			m.err = err
			return m, nil
		}

		m.generatedMnemonic = mnemonic
		m.wipePassphrase()
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

// saveLastWalletName saves the wallet name to config for auto-login and
// records that onboarding has completed.
func (m *WalletSetupModel) saveLastWalletName(walletName string) {
	if m.configManager == nil {
		return
	}

	cfg, err := m.configManager.Load()
	if err != nil {
		return
	}
	if cfg == nil {
		cfg = &config.AppConfig{Version: 1}
	}

	cfg.FirstRunComplete = true
	cfg.LastWalletName = walletName
	_ = m.configManager.Save(cfg) // Ignore error - non-critical
}

// getPreviousStep returns the correct previous step for ESC navigation
func (m WalletSetupModel) getPreviousStep() WalletSetupStep {
	switch m.step {
	// Network is chosen before anything else.
	case StepChooseAction:
		return StepChooseNetwork
	case StepChooseNetwork:
		return StepChooseNetwork

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
	case StepEnterPassphrase:
		return StepChooseKeySource
	case StepConfirmPassphrase:
		return StepEnterPassphrase
	case StepChooseWordCount:
		return StepEnterPassphrase
	case StepShowMnemonic:
		return StepChooseWordCount
	case StepConfirmMnemonic:
		return StepShowMnemonic

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
	case StepRestoreEnterPassphrase:
		return StepRestoreEnterMnemonic
	case StepRestoreConfirmPassphrase:
		return StepRestoreEnterPassphrase
	case StepRestoreChooseScriptType:
		return StepRestoreConfirmPassphrase
	case StepRestoreEnterPassword:
		return StepRestoreChooseScriptType
	case StepRestoreConfirmPassword:
		return StepRestoreEnterPassword

	// Restore from file flow
	case StepRestoreChooseFile:
		return StepRestoreChooseMethod
	case StepRestoreFilePassword:
		// If auto-login mode, go back to the start of onboarding.
		if m.hasLastWallet {
			return StepChooseNetwork
		}
		return StepRestoreChooseFile

	default:
		return StepChooseAction
	}
}

// wipePassphrase clears BIP39 passphrase material from the UI model (best-effort).
func (m *WalletSetupModel) wipePassphrase() {
	if m.passphrase != "" {
		b := []byte(m.passphrase)
		wallet.Wipe(b)
		m.passphrase = ""
	}
	m.passphraseInput.SetValue("")
	m.confirmPassphraseInput.SetValue("")
}
