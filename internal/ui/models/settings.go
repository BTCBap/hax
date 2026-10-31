package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/wallet"
)

type SettingsModel struct {
	width         int
	height        int
	scrollOffset  int // For scrollable views (main settings, cosigner list)
	walletManager WalletService
	configManager *config.ConfigManager
	showCopied    bool
	copiedTime    time.Time
	copyMessage   string // Message to show (COPIED!, IMPORTED!, etc.)

	// Mnemonic viewing state
	showMnemonicWarning bool   // Show security warning before revealing
	showMnemonic        bool   // Actually showing the mnemonic
	mnemonic            string // Cached mnemonic when viewing

	// Display unit (BIP-177)
	displayUnit config.DisplayUnit

	// Address gap limit
	gapLimit int

	// Multisig cosigner management
	showCosignerList      bool            // Show list of cosigners with status
	showCosignerSeedInput bool            // Show seed input for importing
	cosignerSeedInput     textinput.Model // Text input for seed phrase

	// Password prompt for scanning other local wallets for cosigner seeds.
	// The current wallet's password is never retained, so we ask for it.
	showCosignerScanPassword bool
	cosignerScanPassword     textinput.Model
	cosignerImportError      string // Error message from import attempt
	cosignerImportSuccess    string // Success message after import

	// Cosigner editing
	showCosignerEdit         bool            // Show edit screen for a cosigner
	editingCosignerIndex     int             // Which cosigner is being edited
	cosignerNameInput        textinput.Model // Input for cosigner name
	cosignerFingerprintInput textinput.Model // Input for fingerprint
	cosignerEditField        int             // 0=name, 1=fingerprint

	// Descriptor detail view
	showDescriptorMenu bool
	descriptorText     string
	descriptorUR       string // UR-encoded descriptor for QR code

	// Single-sig seed import (for watch-only wallets)
	showSingleSigSeedInput bool   // Show seed input for watch-only single-sig
	singleSigImportError   string // Error from seed import

	// BIP329 label import
	showLabelImport    bool
	labelImportStage   labelImportStage
	labelImportPath    textinput.Model
	labelImportData    string
	labelImportPreview bip329Preview
	labelImportResult  string
	labelImportError   string

	// Unified backup export
	showBackupExport   bool
	backupExportStage  backupExportStage
	backupTray         Tray
	backupExportPath   string
	backupExportResult string
	backupExportError  string

	// HWI status (cached on screen activation, not computed per render)
	hwiStatusCached string

	// Screen visibility - controls whether tickers run
	isActive bool
}

func NewSettingsModel(walletManager WalletService, configManager *config.ConfigManager) SettingsModel {
	// Load display unit and gap limit from config
	displayUnit := config.DisplayUnitBTC // Default
	gapLimit := wallet.AddressGapLimit   // Default
	if configManager != nil {
		cfg, err := configManager.Load()
		if err == nil && cfg != nil {
			if cfg.DisplayUnit != "" {
				displayUnit = cfg.DisplayUnit
			}
			if cfg.GapLimit > 0 {
				gapLimit = cfg.GapLimit
			}
		}
	}

	// Initialize cosigner seed input
	seedInput := newTextInput()
	seedInput.Placeholder = "Enter 12 or 24 word seed phrase..."
	seedInput.CharLimit = 500
	seedInput.SetWidth(60)

	// Initialize cosigner edit inputs
	nameInput := newTextInput()
	nameInput.Placeholder = "Cosigner name..."
	nameInput.CharLimit = 50
	nameInput.SetWidth(40)

	fingerprintInput := newTextInput()
	fingerprintInput.Placeholder = "8 hex characters (e.g., 0f056943)"
	fingerprintInput.CharLimit = 8
	fingerprintInput.SetWidth(20)

	scanPw := newTextInput()
	scanPw.Placeholder = "Password for other local wallets"
	scanPw.EchoMode = textinput.EchoPassword
	scanPw.EchoCharacter = '•'
	scanPw.CharLimit = 100
	scanPw.SetWidth(40)

	labelPath := newTextInput()
	labelPath.Placeholder = "~/.hax/<wallet>_labels_bip329.jsonl"
	labelPath.CharLimit = 1024
	labelPath.SetWidth(60)
	if walletManager != nil {
		labelPath.Placeholder = "~/.hax/" + walletManager.GetWalletName() + "_labels_bip329.jsonl"
	}

	return SettingsModel{
		cosignerScanPassword:     scanPw,
		walletManager:            walletManager,
		configManager:            configManager,
		displayUnit:              displayUnit,
		gapLimit:                 gapLimit,
		cosignerSeedInput:        seedInput,
		cosignerNameInput:        nameInput,
		cosignerFingerprintInput: fingerprintInput,
		labelImportPath:          labelPath,
	}
}

func (m SettingsModel) Init() tea.Cmd {
	m.isActive = true
	// Only start ticker if we have something to animate (copy message visible)
	if m.showCopied {
		return m.animateTick()
	}
	return nil
}

// CapturesTextInput reports whether a seed, password, or cosigner edit form is
// open, so root-level shortcuts stay out of it.
func (m SettingsModel) CapturesTextInput() bool {
	return m.showLabelImport || m.showBackupExport ||
		m.showCosignerSeedInput || m.showSingleSigSeedInput ||
		m.showCosignerScanPassword || m.showCosignerEdit ||
		m.cosignerSeedInput.Focused() || m.cosignerScanPassword.Focused() ||
		m.cosignerNameInput.Focused() || m.cosignerFingerprintInput.Focused()
}

// checkHWIStatus probes HWI availability and version (called once on screen activation)
func (m SettingsModel) checkHWIStatus() string {
	hwi := m.walletManager.GetHWIClient()
	if hwi == nil || !hwi.IsAvailable() {
		return "🔴 Not installed — pip install hwi"
	}
	version, err := hwi.GetVersion()
	if err != nil {
		return "🟢 Installed (version unknown)"
	}
	return "🟢 " + version
}

func (m SettingsModel) animateTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m SettingsModel) Update(msg tea.Msg) (screenModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ScreenActiveMsg:
		m.isActive = true
		// Cache HWI status on screen activation (avoids subprocess per render)
		if m.walletManager.IsHardwareWallet() {
			m.hwiStatusCached = m.checkHWIStatus()
		}
		// Restart ticker if copy message is still visible
		if m.showCopied {
			return m, m.animateTick()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.showBackupExport {
			m.backupTray = m.backupTray.WithWidth(msg.Width)
		}

	case TickMsg:
		// Skip tick processing if screen is not active
		if !m.isActive {
			return m, nil
		}

		// Check if copy message should be hidden (after 2 seconds)
		if m.showCopied && time.Since(m.copiedTime) > 2*time.Second {
			m.showCopied = false
		}

		// Continue animation if needed
		if m.showCopied {
			cmds = append(cmds, m.animateTick())
		}

	case tea.PasteMsg:
		var cmd tea.Cmd
		switch {
		case m.showLabelImport && m.labelImportStage == labelImportStagePath:
			m.labelImportPath, cmd = m.labelImportPath.Update(msg)
		case m.showCosignerEdit:
			if m.cosignerEditField == 0 {
				m.cosignerNameInput, cmd = m.cosignerNameInput.Update(msg)
			} else {
				m.cosignerFingerprintInput, cmd = m.cosignerFingerprintInput.Update(msg)
			}
		case m.showCosignerScanPassword:
			m.cosignerScanPassword, cmd = m.cosignerScanPassword.Update(msg)
		case m.showCosignerSeedInput, m.showSingleSigSeedInput:
			m.cosignerSeedInput, cmd = m.cosignerSeedInput.Update(msg)
		}
		if cmd != nil {
			return m, cmd
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.showLabelImport {
			return m.updateLabelImport(msg)
		}
		if m.showBackupExport {
			return m.updateBackupExport(msg)
		}

		// Handle mnemonic warning state
		if m.showMnemonicWarning {
			switch msg.String() {
			case "y", "Y":
				// User confirmed - show mnemonic
				mnemonic, err := m.walletManager.GetMnemonic()
				if err == nil {
					m.mnemonic = mnemonic
					m.showMnemonic = true
				}
				m.showMnemonicWarning = false
				return m, nil
			case "n", "N", "esc":
				// User cancelled
				m.showMnemonicWarning = false
				return m, nil
			}
			return m, nil
		}

		// Handle mnemonic viewing state
		if m.showMnemonic {
			switch msg.String() {
			case "esc", "enter", "q":
				// Hide mnemonic
				m.showMnemonic = false
				m.mnemonic = ""
				return m, nil
			}
			// Copying the seed phrase to the clipboard is intentionally not
			// supported: clipboards are readable by other apps and often synced.
			return m, nil
		}

		// Handle cosigner list view
		if m.showCosignerList {
			switch msg.String() {
			case "esc", "q":
				m.showCosignerList = false
				m.scrollOffset = 0
				m.cosignerImportError = ""
				m.cosignerImportSuccess = ""
				return m, nil
			case "i", "I":
				// Open seed input for importing
				m.showCosignerList = false
				m.showCosignerSeedInput = true
				m.cosignerSeedInput.SetValue("")
				m.cosignerSeedInput.Focus()
				m.cosignerImportError = ""
				return m, nil
			case "s", "S":
				// Ask for the password of the other local wallets to scan.
				m.showCosignerList = false
				m.showCosignerScanPassword = true
				m.cosignerScanPassword.SetValue("")
				m.cosignerImportError = ""
				m.cosignerImportSuccess = ""
				return m, m.cosignerScanPassword.Focus()
			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				// Edit cosigner by number
				index := int(msg.String()[0] - '1')
				cosigners := m.walletManager.GetCosigners()
				if index >= 0 && index < len(cosigners) {
					m.showCosignerList = false
					m.scrollOffset = 0
					m.showCosignerEdit = true
					m.editingCosignerIndex = index
					m.cosignerEditField = 0
					// Pre-fill with current values
					m.cosignerNameInput.SetValue(cosigners[index].Name)
					m.cosignerFingerprintInput.SetValue(cosigners[index].Fingerprint)
					m.cosignerNameInput.Focus()
					m.cosignerImportError = ""
				}
				return m, nil
			case "up", "k":
				m.scrollOffset -= 3
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
				return m, nil
			case "down", "j":
				m.scrollOffset += 3
				return m, nil
			case "pgup":
				m.scrollOffset -= m.height / 2
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
				return m, nil
			case "pgdown":
				m.scrollOffset += m.height / 2
				return m, nil
			}
			return m, nil
		}

		// Handle cosigner edit view
		if m.showCosignerEdit {
			switch msg.String() {
			case "esc":
				// Cancel and go back to cosigner list
				m.showCosignerEdit = false
				m.showCosignerList = true
				m.cosignerImportError = ""
				return m, nil
			case "tab":
				// Switch between fields
				if m.cosignerEditField == 0 {
					m.cosignerEditField = 1
					m.cosignerNameInput.Blur()
					m.cosignerFingerprintInput.Focus()
				} else {
					m.cosignerEditField = 0
					m.cosignerFingerprintInput.Blur()
					m.cosignerNameInput.Focus()
				}
				return m, nil
			case "enter":
				// Save changes
				name := strings.TrimSpace(m.cosignerNameInput.Value())
				fingerprint := strings.TrimSpace(m.cosignerFingerprintInput.Value())

				err := m.walletManager.UpdateCosigner(m.editingCosignerIndex, name, fingerprint)
				if err != nil {
					m.cosignerImportError = err.Error()
					return m, nil
				}

				// Success
				m.showCosignerEdit = false
				m.showCosignerList = true
				m.cosignerImportSuccess = fmt.Sprintf("Cosigner %d updated!", m.editingCosignerIndex+1)
				m.cosignerImportError = ""
				return m, nil
			default:
				// Update the focused input
				var cmd tea.Cmd
				if m.cosignerEditField == 0 {
					m.cosignerNameInput, cmd = m.cosignerNameInput.Update(msg)
				} else {
					m.cosignerFingerprintInput, cmd = m.cosignerFingerprintInput.Update(msg)
				}
				return m, cmd
			}
		}

		// Handle descriptor detail menu
		if m.showDescriptorMenu {
			switch msg.String() {
			case "esc", "q":
				m.showDescriptorMenu = false
				m.descriptorText = ""
				m.descriptorUR = ""
				return m, nil
			case "c", "C":
				// Copy descriptor to clipboard
				if m.descriptorText != "" {
					err := copyToClipboard(m.descriptorText)
					if err == nil {
						m.showCopied = true
						m.copyMessage = "DESCRIPTOR COPIED!"
						m.copiedTime = time.Now()
						return m, m.animateTick()
					}
				}
				return m, nil
			case "e", "E":
				// Export to file: multisig config JSON for multisig, raw descriptor for single-sig
				homeDir, err := os.UserHomeDir()
				if err != nil {
					m.showCopied = true
					m.copyMessage = "Failed to get home directory"
					m.copiedTime = time.Now()
					return m, m.animateTick()
				}

				if m.walletManager.IsMultisig() {
					// Multisig: export config JSON (Sparrow-compatible)
					data, err := m.walletManager.ExportMultisigConfig()
					if errors.Is(err, wallet.ErrUnsortedMultisigFormat) {
						// multi() key order cannot be represented in a sortedmulti
						// config, so export the descriptor instead.
						accounts := m.walletManager.GetAccounts()
						if len(accounts) == 0 {
							m.showCopied = true
							m.copyMessage = "Export failed: no accounts"
							m.copiedTime = time.Now()
							return m, m.animateTick()
						}
						descriptor, descErr := m.walletManager.ExportDescriptor(accounts[0].Index)
						if descErr != nil {
							m.showCopied = true
							m.copyMessage = "Export failed: " + descErr.Error()
							m.copiedTime = time.Now()
							return m, m.animateTick()
						}
						if descriptor == "" {
							m.showCopied = true
							m.copyMessage = "Export failed: no descriptor"
							m.copiedTime = time.Now()
							return m, m.animateTick()
						}
						filename, err := writePrivateExport(filepath.Join(homeDir, ".hax"),
							m.walletManager.GetWalletName()+"_descriptor.txt", []byte(descriptor))
						if err != nil {
							m.showCopied = true
							m.copyMessage = "Failed to write file"
							m.copiedTime = time.Now()
							return m, m.animateTick()
						}
						m.showCopied = true
						m.copyMessage = "SAVED: " + filename
						m.copiedTime = time.Now()
						return m, m.animateTick()
					}
					if err != nil {
						m.showCopied = true
						m.copyMessage = "Export failed: " + err.Error()
						m.copiedTime = time.Now()
						return m, m.animateTick()
					}
					filename, err := writePrivateExport(filepath.Join(homeDir, ".hax"),
						m.walletManager.GetWalletName()+"_multisig.json", data)
					if err != nil {
						m.showCopied = true
						m.copyMessage = "Failed to write file"
						m.copiedTime = time.Now()
						return m, m.animateTick()
					}
					m.showCopied = true
					m.copyMessage = "EXPORTED: " + filename
					m.copiedTime = time.Now()
					return m, m.animateTick()
				}

				// Single-sig: export raw descriptor .txt
				if m.descriptorText != "" {
					filename, err := writePrivateExport(filepath.Join(homeDir, ".hax"),
						m.walletManager.GetWalletName()+"_descriptor.txt", []byte(m.descriptorText))
					if err != nil {
						m.showCopied = true
						m.copyMessage = "Failed to write file"
						m.copiedTime = time.Now()
						return m, m.animateTick()
					}
					m.showCopied = true
					m.copyMessage = "EXPORTED: " + filename
					m.copiedTime = time.Now()
					return m, m.animateTick()
				}
				return m, nil
			}
			return m, nil
		}

		// Handle password prompt for scanning local wallets
		if m.showCosignerScanPassword {
			switch msg.String() {
			case "esc":
				m.showCosignerScanPassword = false
				m.showCosignerList = true
				m.cosignerScanPassword.SetValue("")
				return m, nil
			case "enter":
				pw := m.cosignerScanPassword.Value()
				m.cosignerScanPassword.SetValue("")
				m.cosignerScanPassword.Blur()
				m.showCosignerScanPassword = false
				m.showCosignerList = true
				m.scanAndImportCosigners(pw)
				return m, nil
			default:
				var cmd tea.Cmd
				m.cosignerScanPassword, cmd = m.cosignerScanPassword.Update(msg)
				return m, cmd
			}
		}

		// Handle cosigner seed input
		if m.showCosignerSeedInput {
			switch msg.String() {
			case "esc":
				// Cancel and go back to cosigner list
				m.showCosignerSeedInput = false
				m.showCosignerList = true
				m.cosignerSeedInput.SetValue("")
				m.cosignerImportError = ""
				return m, nil
			case "enter":
				// Try to import the seed
				seed := strings.TrimSpace(m.cosignerSeedInput.Value())
				if seed == "" {
					m.cosignerImportError = "Please enter a seed phrase"
					return m, nil
				}

				// Attempt import
				cosignerIndex, err := m.walletManager.ImportCosignerSeed(seed)
				if err != nil {
					m.cosignerImportError = err.Error()
					return m, nil
				}

				// Success! Show which derivation matched
				m.showCosignerSeedInput = false
				m.showCosignerList = true
				m.cosignerSeedInput.SetValue("") // Clear seed from memory
				m.cosignerImportError = ""

				// Get the actual derivation path that matched
				cosigners := m.walletManager.GetCosigners()
				derivation := ""
				if cosignerIndex >= 0 && cosignerIndex < len(cosigners) {
					derivation = cosigners[cosignerIndex].ActualDerivation
				}
				if derivation != "" {
					m.cosignerImportSuccess = fmt.Sprintf("Seed matched cosigner #%d at %s!", cosignerIndex+1, derivation)
				} else {
					m.cosignerImportSuccess = fmt.Sprintf("Seed imported for cosigner #%d!", cosignerIndex+1)
				}
				return m, nil
			default:
				// Update text input
				var cmd tea.Cmd
				m.cosignerSeedInput, cmd = m.cosignerSeedInput.Update(msg)
				return m, cmd
			}
		}

		// Handle single-sig seed import (for watch-only wallets)
		if m.showSingleSigSeedInput {
			switch msg.String() {
			case "esc":
				m.showSingleSigSeedInput = false
				m.cosignerSeedInput.SetValue("")
				m.singleSigImportError = ""
				return m, nil
			case "enter":
				seed := strings.TrimSpace(m.cosignerSeedInput.Value())
				if seed == "" {
					m.singleSigImportError = "Please enter a seed phrase"
					return m, nil
				}

				err := m.walletManager.ImportSeedToWatchOnlyWallet(seed)
				if err != nil {
					m.singleSigImportError = err.Error()
					return m, nil
				}

				// Success!
				m.showSingleSigSeedInput = false
				m.cosignerSeedInput.SetValue("")
				m.singleSigImportError = ""
				m.showCopied = true
				m.copyMessage = "Seed imported! Full signing enabled"
				m.copiedTime = time.Now()
				return m, m.animateTick()
			default:
				var cmd tea.Cmd
				m.cosignerSeedInput, cmd = m.cosignerSeedInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, func() tea.Msg {
				return NavigateMsg{Target: StateMainDashboard}
			}
		case "b":
			// Open connection/backend settings
			m.isActive = false
			return m, func() tea.Msg {
				return NavigateMsg{Target: StateConnectionSetup}
			}
		case "c":
			// Copy xpub to clipboard (only for single-sig wallets)
			if !m.walletManager.IsMultisig() {
				var xpub string
				// For hardware/descriptor wallets, get xpub from config
				if m.walletManager.IsHardwareWallet() {
					hwConfig := m.walletManager.GetHardwareWalletConfig()
					if hwConfig != nil {
						xpub = hwConfig.Xpub
					}
				} else {
					// For mnemonic-based wallets, derive xpub
					accounts := m.walletManager.GetAccounts()
					if len(accounts) > 0 {
						xpub, _ = m.walletManager.GetExtendedPublicKey(accounts[0].Index)
					}
				}
				if xpub != "" {
					err := copyToClipboard(xpub)
					if err == nil {
						m.showCopied = true
						m.copyMessage = "COPIED!"
						m.copiedTime = time.Now()
						cmds = append(cmds, m.animateTick())
					}
				}
			}
		case "d", "D":
			// Open descriptor detail menu
			accounts := m.walletManager.GetAccounts()
			if len(accounts) > 0 {
				descriptor, err := m.walletManager.ExportDescriptor(accounts[0].Index)
				if err == nil && descriptor != "" {
					m.descriptorText = descriptor
					// Generate UR-encoded descriptor for QR code
					urStr, urErr := m.walletManager.ExportDescriptorUR()
					if urErr == nil {
						m.descriptorUR = urStr
					} else {
						m.descriptorUR = "" // Fall back to raw descriptor for QR
					}
					m.showDescriptorMenu = true
					return m, nil
				}
			}
		case "m", "M":
			// Check if wallet is watch-only (no mnemonic)
			if m.walletManager.IsWatchOnly() {
				m.showCopied = true
				m.copyMessage = "Watch-only - no seed phrase"
				m.copiedTime = time.Now()
				return m, m.animateTick()
			}
			// Show mnemonic warning first (only for single-sig wallets with mnemonic)
			if !m.walletManager.IsMultisig() {
				m.showMnemonicWarning = true
			}
			return m, nil
		case "i", "I":
			// For multisig: open cosigner management
			// For watch-only single-sig: open seed import
			if m.walletManager.IsMultisig() {
				m.scrollOffset = 0
				m.showCosignerList = true
				m.cosignerImportError = ""
				m.cosignerImportSuccess = ""
			} else if m.walletManager.IsWatchOnly() {
				// Watch-only single-sig - allow seed import
				m.showSingleSigSeedInput = true
				m.cosignerSeedInput.SetValue("")
				m.cosignerSeedInput.Focus()
				m.singleSigImportError = ""
			}
			return m, nil
		case "l":
			// Import BIP329 labels from a JSON Lines file.
			return m, m.openLabelImport()
		case "B":
			// Export descriptors, xpubs and labels without private keys.
			m.openBackupExport()
			return m, nil
		case "u", "U":
			// Cycle display unit: BTC <-> Bitcoin (BIP-177)
			switch m.displayUnit {
			case config.DisplayUnitBTC:
				m.displayUnit = config.DisplayUnitBitcoin
			case config.DisplayUnitBitcoin:
				m.displayUnit = config.DisplayUnitBTC
			default:
				m.displayUnit = config.DisplayUnitBTC
			}
			// Save to config
			var saveErr error
			if m.configManager != nil {
				cfg, err := m.configManager.Load()
				if err == nil && cfg != nil {
					cfg.DisplayUnit = m.displayUnit
					saveErr = m.configManager.Save(cfg)
				} else {
					saveErr = err
				}
			}
			m.showCopied = true
			m.copyMessage = "UNIT: " + strings.ToUpper(string(m.displayUnit))
			if saveErr != nil {
				debugLog("[SETTINGS] saving display unit failed: %v", saveErr)
				m.copyMessage = saveFailedMessage(saveErr)
			}
			m.copiedTime = time.Now()
			cmds = append(cmds, m.animateTick())
		case "w", "W":
			// Export Coldcard multisig wallet config
			if m.walletManager.IsMultisig() {
				configData, err := m.walletManager.ExportColdcardMultisig()
				if err != nil {
					m.showCopied = true
					if errors.Is(err, wallet.ErrUnsortedMultisigFormat) {
						m.copyMessage = "Coldcard file needs sortedmulti; use descriptor export"
					} else {
						m.copyMessage = "Export failed: " + err.Error()
					}
					m.copiedTime = time.Now()
					return m, m.animateTick()
				}
				homedir, _ := os.UserHomeDir()
				configPath, err := writePrivateExport(filepath.Join(homedir, ".hax"),
					"coldcard-multisig.txt", []byte(configData))
				if err != nil {
					m.showCopied = true
					m.copyMessage = "Save failed: " + err.Error()
					m.copiedTime = time.Now()
					return m, m.animateTick()
				}
				m.showCopied = true
				m.copyMessage = "SAVED: " + configPath
				m.copiedTime = time.Now()
				return m, m.animateTick()
			}
		case "g", "G":
			// Cycle gap limit: 20 -> 50 -> 100 -> 20
			switch m.gapLimit {
			case 20:
				m.gapLimit = 50
			case 50:
				m.gapLimit = 100
			default:
				m.gapLimit = 20
			}
			m.walletManager.SetGapLimit(m.gapLimit)
			var saveErr error
			if m.configManager != nil {
				cfg, err := m.configManager.Load()
				if err == nil && cfg != nil {
					cfg.GapLimit = m.gapLimit
					saveErr = m.configManager.Save(cfg)
				} else {
					saveErr = err
				}
			}
			m.showCopied = true
			m.copyMessage = fmt.Sprintf("GAP LIMIT: %d", m.gapLimit)
			if saveErr != nil {
				debugLog("[SETTINGS] saving gap limit failed: %v", saveErr)
				m.copyMessage = saveFailedMessage(saveErr)
			}
			m.copiedTime = time.Now()
			cmds = append(cmds, m.animateTick())
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
	}
	return m, tea.Batch(cmds...)
}
