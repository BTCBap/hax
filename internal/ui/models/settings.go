package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"hax/internal/backend"
	"hax/internal/config"
	"hax/internal/ui"
	"hax/internal/ui/styles"
	"hax/internal/wallet"
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
	cosignerImportError   string          // Error message from import attempt
	cosignerImportSuccess string          // Success message after import

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
	seedInput := textinput.New()
	seedInput.Placeholder = "Enter 12 or 24 word seed phrase..."
	seedInput.CharLimit = 500
	seedInput.Width = 60

	// Initialize cosigner edit inputs
	nameInput := textinput.New()
	nameInput.Placeholder = "Cosigner name..."
	nameInput.CharLimit = 50
	nameInput.Width = 40

	fingerprintInput := textinput.New()
	fingerprintInput.Placeholder = "8 hex characters (e.g., 0f056943)"
	fingerprintInput.CharLimit = 8
	fingerprintInput.Width = 20

	return SettingsModel{
		walletManager:            walletManager,
		configManager:            configManager,
		displayUnit:              displayUnit,
		gapLimit:                 gapLimit,
		cosignerSeedInput:        seedInput,
		cosignerNameInput:        nameInput,
		cosignerFingerprintInput: fingerprintInput,
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

func (m SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case tea.KeyMsg:
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
			case "c":
				// Copy mnemonic to clipboard
				err := clipboard.WriteAll(m.mnemonic)
				if err == nil {
					m.showCopied = true
					m.copyMessage = "MNEMONIC COPIED!"
					m.copiedTime = time.Now()
					cmds = append(cmds, m.animateTick())
				}
				return m, tea.Batch(cmds...)
			}
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
				// Scan local wallets for matching seeds
				// This uses the same password as the current wallet
				matches, err := m.walletManager.ScanLocalWalletsForCosigners(m.walletManager.GetPassword())
				if err != nil {
					m.cosignerImportError = fmt.Sprintf("Scan failed: %v", err)
					return m, nil
				}
				if len(matches) == 0 {
					m.cosignerImportError = "No matching seeds found in other local wallets"
					return m, nil
				}
				// Import each matched seed
				imported := 0
				for _, match := range matches {
					_, err := m.walletManager.ImportCosignerFromLocalWallet(match.WalletName, m.walletManager.GetPassword())
					if err == nil {
						imported++
					}
				}
				if imported > 0 {
					m.cosignerImportSuccess = fmt.Sprintf("Imported seeds from %d local wallet(s)!", imported)
					m.cosignerImportError = ""
				} else {
					m.cosignerImportError = "Found matches but failed to import"
				}
				return m, nil
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
					err := clipboard.WriteAll(m.descriptorText)
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
					if err != nil {
						m.showCopied = true
						m.copyMessage = "Export failed: " + err.Error()
						m.copiedTime = time.Now()
						return m, m.animateTick()
					}
					filename := fmt.Sprintf("%s/.hax/%s_multisig.json", homeDir, m.walletManager.GetWalletName())
					if err := os.WriteFile(filename, data, 0600); err != nil {
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
					filename := fmt.Sprintf("%s/.hax/%s_descriptor.txt", homeDir, m.walletManager.GetWalletName())
					if err := os.WriteFile(filename, []byte(m.descriptorText), 0600); err != nil {
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
					err := clipboard.WriteAll(xpub)
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
			if m.configManager != nil {
				cfg, err := m.configManager.Load()
				if err == nil && cfg != nil {
					cfg.DisplayUnit = m.displayUnit
					m.configManager.Save(cfg)
				}
			}
			m.showCopied = true
			m.copyMessage = "UNIT: " + strings.ToUpper(string(m.displayUnit))
			m.copiedTime = time.Now()
			cmds = append(cmds, m.animateTick())
		case "w", "W":
			// Export Coldcard multisig wallet config
			if m.walletManager.IsMultisig() {
				configData, err := m.walletManager.ExportColdcardMultisig()
				if err != nil {
					m.showCopied = true
					m.copyMessage = "Export failed: " + err.Error()
					m.copiedTime = time.Now()
					return m, m.animateTick()
				}
				homedir, _ := os.UserHomeDir()
				configPath := filepath.Join(homedir, "coldcard-multisig.txt")
				if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
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
			if m.configManager != nil {
				cfg, err := m.configManager.Load()
				if err == nil && cfg != nil {
					cfg.GapLimit = m.gapLimit
					m.configManager.Save(cfg)
				}
			}
			m.showCopied = true
			m.copyMessage = fmt.Sprintf("GAP LIMIT: %d", m.gapLimit)
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

func (m SettingsModel) View() string {
	_ = m.width

	// If showing mnemonic warning, show that screen
	if m.showMnemonicWarning {
		return m.viewMnemonicWarning()
	}

	// If showing mnemonic, show that screen
	if m.showMnemonic {
		return m.viewMnemonic()
	}

	// If showing cosigner list, show that screen
	if m.showCosignerList {
		return m.viewCosignerList()
	}

	// If showing cosigner seed input, show that screen
	if m.showCosignerSeedInput {
		return m.viewCosignerSeedInput()
	}

	// If showing cosigner edit, show that screen
	if m.showCosignerEdit {
		return m.viewCosignerEdit()
	}

	// If showing descriptor menu, show that screen
	if m.showDescriptorMenu {
		return m.viewDescriptorMenu()
	}

	// If showing single-sig seed import, show that screen
	if m.showSingleSigSeedInput {
		return m.viewSingleSigSeedInput()
	}

	// Compact header (no vertical padding, like receive screen)
	header := styles.ScreenHeaderStyle.Render("⚙  Wallet Settings")

	// Copy status - inline with header (pastel green, matching theme)
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		msg := m.copyMessage
		if msg == "" {
			msg = "COPIED!"
		}
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ "+msg)
	}

	subtitle := styles.SubtitleStyle.Render("Advanced wallet configuration and information")

	// Wallet Information Section
	walletName := m.walletManager.GetWalletName()
	network := m.walletManager.GetNetwork()
	createdAt := m.walletManager.GetCreatedAt()
	createdDate := time.Unix(createdAt, 0).Format("2006-01-02 15:04:05")
	walletType := m.walletManager.GetWalletType()

	// Create a beautiful info box with Lipgloss table-like layout
	infoRows := []string{
		m.renderInfoRow("Wallet Name", walletName),
		m.renderInfoRow("Type", walletType),
		m.renderInfoRow("Network", strings.ToUpper(network)),
		m.renderInfoRow("Created", createdDate),
		m.renderInfoRow("Status", "🟢 Unlocked"),
	}

	// Show HWI status for hardware-backed wallets (cached on screen activation)
	if m.walletManager.IsHardwareWallet() && m.hwiStatusCached != "" {
		infoRows = append(infoRows, m.renderInfoRow("HWI", m.hwiStatusCached))
	}

	walletInfo := styles.BoxStyle.
		Width(styles.ClampWidth(70, m.width)).
		BorderForeground(styles.NeonPurple).
		Padding(0, 2).
		Render(
			styles.BoldText.Render("📋 Wallet Information") + "\n" +
				strings.Join(infoRows, "\n"),
		)

	// Accounts Section
	accounts := m.walletManager.GetAccounts()
	var accountSections []string

	for i, account := range accounts {
		accountSection := m.renderAccountSection(account, i)
		accountSections = append(accountSections, accountSection)
	}

	accountsDisplay := ""
	if len(accountSections) > 0 {
		accountsDisplay = "\n" + strings.Join(accountSections, "\n")
	}

	// Connection Settings Section
	connectionInfo := m.renderConnectionSection()

	// Help footer - includes unit toggle hotkey
	unitLabel := "BTC"
	if m.displayUnit == config.DisplayUnitBitcoin {
		unitLabel = "₿"
	}

	// Build help text based on wallet type
	items := []styles.HelpItem{
		styles.HK("u", "unit:"+unitLabel),
		styles.HK("g", fmt.Sprintf("gap:%d", m.gapLimit)),
		styles.HK("b", "backend"),
	}

	if m.walletManager.IsMultisig() {
		items = append(items, styles.HK("i", "cosigners"), styles.HK("w", "CC config"))
	} else {
		items = append(items, styles.HK("c", "copy xpub"))
		if m.walletManager.IsWatchOnly() {
			items = append(items, styles.HK("i", "import seed"))
		}
	}

	items = append(items, styles.HK("d", "descriptor"))

	if !m.walletManager.IsWatchOnly() && !m.walletManager.IsMultisig() {
		items = append(items, styles.HK("m", "mnemonic"))
	}

	items = append(items, styles.HK("esc", "back"))

	help := styles.RenderHelp(items...)

	// Split into fixed header and scrollable content
	fixedHeader := header + copyStatus + "\n" + subtitle

	scrollContent := connectionInfo + "\n" +
		walletInfo + accountsDisplay

	return renderScrollableView(m.height, m.scrollOffset, fixedHeader, scrollContent, help)
}

// renderConnectionSection creates a compact display for connection/backend settings
func (m SettingsModel) renderConnectionSection() string {
	backendType := "Unknown"
	backendDetails := "No configuration loaded"

	if m.configManager != nil {
		cfg, err := m.configManager.Load()
		if err == nil && cfg != nil && cfg.Backend != nil {
			switch cfg.Backend.Type {
			case backend.BackendTypeElectrum:
				backendType = "Electrum"
				if cfg.Backend.Electrum != nil {
					ssl := ""
					if cfg.Backend.Electrum.UseSSL {
						ssl = " (SSL)"
					}
					backendDetails = fmt.Sprintf("%s:%d%s", cfg.Backend.Electrum.Host, cfg.Backend.Electrum.Port, ssl)
				}
			case backend.BackendTypeRPC:
				backendType = "Bitcoin Core RPC"
				if cfg.Backend.RPC != nil {
					backendDetails = fmt.Sprintf("%s:%d", cfg.Backend.RPC.Host, cfg.Backend.RPC.Port)
				}
			}
		}
	}

	boxWidth := styles.ClampWidth(70, m.width)
	return styles.BoxStyle.
		Width(boxWidth).
		BorderForeground(styles.SuccessGreen).
		Padding(0, 2).
		Render(
			styles.BoldText.Render("🔌 ") + backendType + ": " +
				styles.HighlightText.Render(backendDetails),
		)
}

// renderInfoRow creates a formatted info row with label and value
func (m SettingsModel) renderInfoRow(label, value string) string {
	labelStyle := styles.DimText.Width(15)
	valueStyle := styles.HighlightText

	return labelStyle.Render(label+":") + "  " + valueStyle.Render(value)
}

// renderAccountSection creates a detailed account display
func (m SettingsModel) renderAccountSection(account wallet.AccountData, index int) string {
	// Account header
	accountTitle := fmt.Sprintf("💼 Account #%d: %s", index, account.Name)

	isMultisig := m.walletManager.IsMultisig()

	// Address type with icon
	addressTypeDisplay := ""
	if isMultisig {
		config := m.walletManager.GetMultisigConfig()
		if config != nil {
			switch wallet.MultisigScriptType(config.ScriptType) {
			case wallet.MultisigLegacy:
				addressTypeDisplay = fmt.Sprintf("🔐 %d-of-%d Multisig (P2SH)", config.M, config.N)
			case wallet.MultisigNestedSegwit:
				addressTypeDisplay = fmt.Sprintf("🔐 %d-of-%d Multisig (P2SH-P2WSH)", config.M, config.N)
			case wallet.MultisigNativeSegwit:
				addressTypeDisplay = fmt.Sprintf("🔐 %d-of-%d Multisig (P2WSH)", config.M, config.N)
			}
		}
	} else {
		switch account.AddressType {
		case "legacy":
			addressTypeDisplay = "🔸 Legacy (P2PKH)"
		case "segwit_nested":
			addressTypeDisplay = "🔹 SegWit Nested (P2SH-P2WPKH)"
		case "segwit_native":
			addressTypeDisplay = "🔷 Native SegWit (bech32)"
		case "taproot":
			addressTypeDisplay = "🟠 Taproot (P2TR)"
		}
	}

	// Address statistics
	usedReceivingAddrs := 0
	usedChangeAddrs := 0
	for _, addr := range account.Addresses {
		if addr.Used {
			if addr.Change {
				usedChangeAddrs++
			} else {
				usedReceivingAddrs++
			}
		}
	}

	var infoRows []string

	if isMultisig {
		// Multisig: show cosigner count and descriptor info
		config := m.walletManager.GetMultisigConfig()
		derivationPath := "BIP48 (m/48'/1'/0'/script')"
		infoRows = []string{
			m.renderInfoRow("Type", addressTypeDisplay),
			m.renderInfoRow("Path", derivationPath),
			m.renderInfoRow("Cosigners", fmt.Sprintf("%d keys", config.N)),
			styles.DimText.Render("Press 'd' for descriptor details"),
		}
	} else {
		// Single-sig: show xpub
		xpub, err := m.walletManager.GetExtendedPublicKey(account.Index)
		xpubDisplay := ""
		if err == nil && xpub != "" {
			// Truncate xpub for display
			if len(xpub) > 60 {
				xpubDisplay = xpub[:30] + "..." + xpub[len(xpub)-27:]
			} else {
				xpubDisplay = xpub
			}
		} else {
			xpubDisplay = styles.ErrorText.Render("Error loading xpub")
		}

		// Derivation path - use custom path if set, otherwise determine from address type
		var derivationPath string
		if account.CustomDerivationPath != "" {
			derivationPath = account.CustomDerivationPath + " (custom)"
		} else {
			purpose := "44" // Default to BIP44
			switch account.AddressType {
			case "legacy":
				purpose = "44" // BIP44
			case "segwit_nested":
				purpose = "49" // BIP49
			case "segwit_native":
				purpose = "84" // BIP84
			case "taproot":
				purpose = "86" // BIP86
			}
			derivationPath = fmt.Sprintf("m/%s'/1'/%d'", purpose, account.Index)
		}

		infoRows = []string{
			m.renderInfoRow("Type", addressTypeDisplay),
			m.renderInfoRow("Path", derivationPath) + "  " + styles.DimText.Render(fmt.Sprintf("(%d recv, %d change)", usedReceivingAddrs, usedChangeAddrs)),
			styles.DimText.Render("xpub: ") + lipgloss.NewStyle().Foreground(styles.PastelOrange).Render(xpubDisplay),
		}
	}

	boxWidth := styles.ClampWidth(70, m.width)
	return styles.BoxStyle.
		Width(boxWidth).
		BorderForeground(styles.ElectricBlue).
		Padding(0, 2).
		Render(
			styles.HighlightText.Render(accountTitle) + "\n" +
				strings.Join(infoRows, "\n"),
		)
}

// viewDescriptorMenu displays the descriptor detail screen with QR code and cosigner info
func (m SettingsModel) viewDescriptorMenu() string {
	title := styles.ScreenHeaderStyle.Render("⚙  Output Descriptor")

	// Copy status - inline with header
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		msg := m.copyMessage
		if msg == "" {
			msg = "COPIED!"
		}
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ "+msg)
	}

	// QR code: use UR encoding if available, uppercase for QR alphanumeric mode efficiency.
	qrSection := ""
	qrData := m.descriptorText
	qrLabel := ""
	if m.descriptorUR != "" {
		// Uppercase UR triggers QR alphanumeric mode (smaller QR code)
		qrData = strings.ToUpper(m.descriptorUR)
		qrLabel = styles.DimText.Render("  ur:crypto-output (Sparrow-compatible)") + "\n"
	}
	qrCode, err := ui.GenerateQRCodeHalfBlocks(qrData)
	if err == nil {
		qrSection = styles.BoxStyle.
			Width(styles.ClampWidth(78, m.width)).
			BorderForeground(styles.NeonPurple).
			Padding(0, 0).
			MarginBottom(0).
			Render(qrLabel + qrCode)
	}

	// Descriptor text - wrap long lines for readability
	descriptorDisplay := m.descriptorText
	if len(descriptorDisplay) > 70 {
		var wrapped []string
		for len(descriptorDisplay) > 70 {
			wrapped = append(wrapped, descriptorDisplay[:70])
			descriptorDisplay = descriptorDisplay[70:]
		}
		if len(descriptorDisplay) > 0 {
			wrapped = append(wrapped, descriptorDisplay)
		}
		descriptorDisplay = strings.Join(wrapped, "\n")
	}

	descriptorBox := styles.BoxStyle.
		Width(styles.ClampWidth(78, m.width)).
		BorderForeground(styles.ElectricBlue).
		Padding(0, 2).
		MarginBottom(0).
		Render(lipgloss.NewStyle().Foreground(styles.PastelOrange).Render(descriptorDisplay))

	// Cosigner table (only for multisig)
	cosignerSection := ""
	if m.walletManager.IsMultisig() {
		cosigners := m.walletManager.GetCosigners()
		if len(cosigners) > 0 {
			var rows []string
			for i, cs := range cosigners {
				name := cs.Name
				if name == "" {
					name = fmt.Sprintf("Cosigner %d", i+1)
				}
				fp := cs.Fingerprint
				if fp == "" {
					fp = "--------"
				}
				device := cs.DeviceType
				if device == "" && cs.IsMine {
					device = "local"
				}
				row := fmt.Sprintf("  %d. %-12s [%s]", i+1, name, fp)
				if device != "" {
					row += " " + styles.DimText.Render(device)
				}
				rows = append(rows, row)
			}

			cosignerSection = styles.BoxStyle.
				Width(styles.ClampWidth(78, m.width)).
				BorderForeground(styles.ElectricBlue).
				Padding(0, 2).
				MarginBottom(0).
				Render(
					styles.BoldText.Render("Cosigners") + "\n" +
						strings.Join(rows, "\n"),
				)
		}
	}

	// Help footer - show appropriate export label
	exportLabel := "export .txt"
	if m.walletManager.IsMultisig() {
		exportLabel = "export .json"
	}
	help := styles.RenderHelp(
		styles.HK("c", "copy descriptor"),
		styles.HK("e", exportLabel),
		styles.HK("esc", "back"),
	)

	parts := []string{
		"",
		title + copyStatus,
	}
	if qrSection != "" {
		parts = append(parts, "", qrSection)
	}
	parts = append(parts, "", descriptorBox)
	if cosignerSection != "" {
		parts = append(parts, "", cosignerSection)
	}
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// viewMnemonicWarning shows security warning before revealing mnemonic
func (m SettingsModel) viewMnemonicWarning() string {
	warningStyle := lipgloss.NewStyle().
		Foreground(styles.PastelPink).
		Bold(true)

	title := warningStyle.Render("⚠️  SECURITY WARNING ⚠️")

	warnings := []string{
		"Your seed phrase (mnemonic) is the MASTER KEY to your wallet.",
		"Anyone with this phrase can steal ALL your funds.",
		"",
		"NEVER share your seed phrase with anyone.",
		"NEVER enter it on websites or apps.",
		"NEVER take a screenshot or photo.",
		"",
		"Only view this in a private, secure location.",
	}

	warningText := ""
	for _, w := range warnings {
		if w == "" {
			warningText += "\n"
		} else {
			warningText += "  • " + w + "\n"
		}
	}

	warningBox := styles.BoxStyle.
		Width(styles.ClampWidth(60, m.width)).
		BorderForeground(styles.PastelPink).
		Render(warningText)

	question := styles.HighlightText.Render("Do you want to reveal your seed phrase?")

	options := lipgloss.NewStyle().
		Foreground(styles.ElectricBlue).
		Render("[Y] Yes, show my seed phrase    [N] No, go back")

	return lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		"",
		warningBox,
		"",
		question,
		"",
		options,
	)
}

// viewMnemonic displays the mnemonic seed phrase
func (m SettingsModel) viewMnemonic() string {
	title := lipgloss.NewStyle().
		Foreground(styles.PastelPink).
		Bold(true).
		Render("🔐 Your Seed Phrase")

	subtitle := lipgloss.NewStyle().
		Foreground(styles.DimText.GetForeground()).
		Italic(true).
		Render("Write this down and store it securely. Never share it.")

	// Format mnemonic as numbered words
	words := strings.Split(m.mnemonic, " ")
	var rows []string
	for i := 0; i < len(words); i += 4 {
		row := ""
		for j := 0; j < 4 && i+j < len(words); j++ {
			wordNum := i + j + 1
			word := words[i+j]
			row += fmt.Sprintf("%2d. %-12s  ", wordNum, word)
		}
		rows = append(rows, row)
	}

	mnemonicDisplay := strings.Join(rows, "\n")

	mnemonicBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(styles.BitcoinOrange).
		Padding(1, 2).
		Render(mnemonicDisplay)

	// Copy status
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copyStatus = styles.CopyStatusStyle.Render("✓ " + m.copyMessage)
	}

	help := styles.RenderHelp(
		styles.HK("c", "copy"),
		styles.HK("esc", "hide & go back"),
	)

	return lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		subtitle,
		"",
		mnemonicBox,
		"",
		copyStatus,
		"",
		help,
	)
}

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
		statusStyle := styles.DimText

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
		Width(styles.ClampWidth(78, m.width)).
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

	// Scan for potential local wallet matches (only if not all cosigners have seeds)
	potentialMatches := ""
	if localCount < len(cosigners) {
		password := m.walletManager.GetPassword()
		if password != "" {
			matches, err := m.walletManager.ScanLocalWalletsForCosigners(password)
			if err == nil && len(matches) > 0 {
				var matchLines []string
				for _, match := range matches {
					line := fmt.Sprintf("  💡 '%s' can sign for Cosigner %d (path: %s)",
						match.WalletName, match.CosignerIndex+1, match.DerivationPath)
					matchLines = append(matchLines, lipgloss.NewStyle().Foreground(styles.PastelYellow).Render(line))
				}
				potentialMatches = styles.BoxStyle.
					Width(styles.ClampWidth(78, m.width)).
					BorderForeground(styles.PastelYellow).
					Padding(0, 2).
					Render(
						lipgloss.NewStyle().Bold(true).Foreground(styles.PastelYellow).Render("🔍 Found matching seeds in local wallets:") + "\n" +
							strings.Join(matchLines, "\n") + "\n" +
							styles.DimText.Render("  Press 's' to auto-import all matches"))
			}
		}
	}

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

	scrollContent := cosignerList +
		potentialMatches + "\n" +
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
		Width(styles.ClampWidth(70, m.width)).
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
		Width(styles.ClampWidth(70, m.width)).
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
		Width(styles.ClampWidth(70, m.width)).
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
