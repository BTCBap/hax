package models

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/bbqr"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/ur"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SendStep represents the current step in the send flow
type SendStep int

const (
	SendStepAddress SendStep = iota
	SendStepAmount
	SendStepRecipientList // Show list of recipients, add more or continue
	SendStepUTXOSelection // Select which UTXOs to use
	SendStepFee
	SendStepPreviewLoading // Calculate final fees with selected UTXOs
	SendStepReview
	SendStepBroadcasting
	SendStepComplete
)

// Recipient represents a single output in a batch transaction
type Recipient struct {
	Address string
	Amount  api.Satoshi
}

// FeeLevel represents different fee priority levels
type FeeLevel int

const (
	FeeLevelLow FeeLevel = iota
	FeeLevelMedium
	FeeLevelHigh
	FeeLevelCustom
)

// feeLevelFromCursor maps cursor position to FeeLevel.
// Display order is High(0), Medium(1), Low(2), Custom(3).
func feeLevelFromCursor(cursor int) FeeLevel {
	switch cursor {
	case 0:
		return FeeLevelHigh
	case 1:
		return FeeLevelMedium
	case 2:
		return FeeLevelLow
	case 3:
		return FeeLevelCustom
	default:
		return FeeLevelMedium
	}
}

// cursorFromFeeLevel maps FeeLevel back to cursor position.
func cursorFromFeeLevel(level FeeLevel) int {
	switch level {
	case FeeLevelHigh:
		return 0
	case FeeLevelMedium:
		return 1
	case FeeLevelLow:
		return 2
	case FeeLevelCustom:
		return 3
	default:
		return 1
	}
}

type SendModel struct {
	width  int
	height int

	// Scroll offset for overflowing views (review, PSBT export)
	scrollOffset   int
	scrollViewport viewport.Model

	walletManager WalletService
	step          SendStep
	cursor        int

	// Input fields
	addressInput textinput.Model
	amountInput  textinput.Model

	// Data - supports batch transactions (multiple recipients)
	recipients       []Recipient // List of all recipients
	recipientAddress string      // Current recipient being edited
	amountSats       api.Satoshi // Current amount being edited
	amountBTC        float64
	usesBTC          bool // true = BTC input, false = satoshi input
	sendAll          bool // true = user selected MAX spend mode (amount derived after fee + UTXO selection)
	sendAllRetried   bool // true after one auto re-preview attempt to remove MAX spend change output

	// Fee selection
	feeLevel       FeeLevel
	feeEstimates   *api.FeeEstimates
	estimatedFee   api.Satoshi
	estimatedSize  int
	customFeeInput textinput.Model
	customFeeRate  float64 // Custom fee rate in sat/vB (supports sub-1 like 0.5)
	editingCustom  bool    // Whether custom fee input is active
	highFeePending float64 // custom rate above feeSoftCap awaiting typed confirmation
	highFeeInput   textinput.Model

	// Transaction data
	utxos        []api.UTXO
	balance      api.Satoshi
	txID         string
	broadcastErr error

	// UTXO selection (manual selection by user)
	utxoSelected []bool // Track which UTXOs are selected

	// Status feedback state (for copy, sign, import, save actions)
	showCopied bool
	copiedTime time.Time
	lastAction string // "copied", "signed", "imported", "saved"

	// PSBT export state
	psbtBase64     string             // Exported PSBT in base64 format
	showPSBTExport bool               // Whether to show PSBT export view
	psbtExportErr  error              // Error during PSBT export
	psbtSigsAdded  int                // Signatures added locally
	psbtSigsTotal  int                // Total signatures needed (M for multisig)
	psbtResult     *wallet.PSBTResult // The actual PSBT for further signing

	// Hardware wallet / file-based PSBT support
	psbtFilePath  string          // Path where PSBT was saved/loaded
	psbtFileInput textinput.Model // Input for file path
	showFileInput bool            // Whether showing file path input
	fileInputMode string          // "export" or "import"

	// HWI (Hardware Wallet Interface) support
	hwiDevices       []wallet.HardwareDevice // Detected hardware wallets
	selectedHWDevice int                     // Selected device index
	showHWIDevices   bool                    // Whether showing device selection
	hwiError         error                   // Error from HWI operations

	// Raw hex export state
	rawHex           string // Raw transaction hex
	showRawHexExport bool   // Whether to show raw hex export view

	// File browser state (for PSBT import)
	showFileBrowser    bool     // Whether showing file browser
	psbtFiles          []string // List of discovered .psbt files
	fileBrowserCursor  int      // Selected file in browser
	fileBrowserForStep string   // "step1" or "step7" - which step triggered file browser

	// QR code display state (for air-gapped hardware wallet signing)
	showQRDisplay    bool     // Whether showing QR code display
	showQRFormatMenu bool     // Whether showing format selection menu
	qrFrames         []string // QR code content strings (one per frame)
	qrFrameIndex     int      // Current animated frame index
	qrFormat         string   // "ur" or "bbqr"
	qrRendered       string   // Pre-rendered QR code for current frame

	// UR paste import state
	showURInput bool            // Whether showing UR paste input
	urInput     textinput.Model // Text input for pasting UR string

	// UI state
	loading  bool
	spinner  spinner.Model
	progress progress.Model
	err      error
}

// FeeEstimatesMsg carries fee estimates
type FeeEstimatesMsg struct {
	Estimates *api.FeeEstimates
	Err       error
}

// UTXOsMsg carries available UTXOs
type UTXOsMsg struct {
	UTXOs []api.UTXO
	Err   error
}

// BroadcastMsg carries broadcast result
type BroadcastMsg struct {
	TxID string
	Err  error
}

// TxPreviewMsg carries transaction preview details
type TxPreviewMsg struct {
	Result *wallet.PSBTResult
	Err    error
}

// PSBTExportMsg carries PSBT export result
type PSBTExportMsg struct {
	Base64    string
	SigsAdded int                // Number of signatures added locally (for multisig)
	SigsTotal int                // Total signatures needed (M of N)
	Result    *wallet.PSBTResult // The PSBT result for further operations
	Err       error
}

// RawHexExportMsg carries raw transaction hex export result
type RawHexExportMsg struct {
	Hex string
	Err error
}

// PSBTFileMsg carries result of PSBT file operations (save/load)
type PSBTFileMsg struct {
	FilePath  string
	Base64    string             // For imports: the loaded PSBT data
	Result    *wallet.PSBTResult // For imports: the parsed PSBT
	SigsAdded int                // For imports: signature count
	Operation string             // "export" or "import"
	Err       error
}

// HWIEnumerateMsg carries list of detected hardware wallets
type HWIEnumerateMsg struct {
	Devices []wallet.HardwareDevice
	Err     error
}

// HWISignMsg carries result of hardware wallet signing
type HWISignMsg struct {
	SignedPSBT string
	SigsAdded  int
	Err        error
}

// QRFrameTickMsg advances animated QR code to the next frame
type QRFrameTickMsg struct{}

func NewSendModel(walletManager WalletService, cachedBalance api.Satoshi, cachedUTXOs []api.UTXO) SendModel {
	// Create address input
	addrInput := textinput.New()
	addrInput.Placeholder = "tb1q... (testnet address)"
	addrInput.Width = 60
	addrInput.CharLimit = 100
	addrInput.Focus()

	// Create amount input with validation
	amtInput := textinput.New()
	amtInput.Placeholder = "0.00000000"
	amtInput.Width = 30
	amtInput.CharLimit = 20
	// Only allow numbers and decimal point
	amtInput.Validate = validateBTCAmount

	// Create custom fee input
	customFeeInput := textinput.New()
	customFeeInput.Placeholder = "sat/vB"
	customFeeInput.Width = 10
	customFeeInput.CharLimit = 6
	customFeeInput.Validate = validateSatPerVB

	// Create file path input for PSBT import/export
	fileInput := textinput.New()
	fileInput.Placeholder = "~/transaction.psbt"
	fileInput.Width = 50
	fileInput.CharLimit = 200

	s := styles.NewStandardSpinner()

	// Create progress bar
	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)

	// Viewport for smooth scrolling in Step 6/7 (matches Help screen behavior)
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true
	vp.KeyMap.Up = key.NewBinding(key.WithKeys("up"))
	vp.KeyMap.Down = key.NewBinding(key.WithKeys("down"))
	vp.KeyMap.PageUp = key.NewBinding(key.WithKeys("pgup"))
	vp.KeyMap.PageDown = key.NewBinding(key.WithKeys("pgdown"))
	vp.KeyMap.HalfPageUp.SetEnabled(false)
	vp.KeyMap.HalfPageDown.SetEnabled(false)

	// If no cached data provided, mark as loading
	// This forces fresh fetch from network
	loadingBalance := cachedBalance == 0 && cachedUTXOs == nil

	return SendModel{
		walletManager:  walletManager,
		step:           SendStepAddress,
		addressInput:   addrInput,
		amountInput:    amtInput,
		customFeeInput: customFeeInput,
		psbtFileInput:  fileInput,
		spinner:        s,
		progress:       prog,
		scrollViewport: vp,
		usesBTC:        true,
		feeLevel:       FeeLevelMedium,
		balance:        cachedBalance,
		utxos:          cachedUTXOs,
		loading:        loadingBalance, // Show loading until fresh data arrives
	}
}

// estimateTxVSize estimates the transaction vSize using wallet-aware script type detection
// This provides accurate fee estimation for all wallet types (single-sig, multisig, different address types)
func (m *SendModel) estimateTxVSize(numInputs, numOutputs int) int {
	scriptType, multisigM, multisigN := m.walletManager.GetScriptType()
	return wallet.EstimateVSize(numInputs, numOutputs, scriptType, scriptType, multisigM, multisigN)
}

func (m SendModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
		m.fetchFeeEstimates(),
		m.fetchUTXOs(),
	)
}

func (m SendModel) fetchFeeEstimates() tea.Cmd {
	return func() tea.Msg {
		fees, err := m.walletManager.GetFeeEstimates()
		return FeeEstimatesMsg{Estimates: fees, Err: err}
	}
}

func (m SendModel) fetchUTXOs() tea.Cmd {
	return func() tea.Msg {
		utxos, err := m.walletManager.GetUTXOs(0) // Account 0
		if err != nil {
			return UTXOsMsg{Err: err}
		}

		// Calculate balance
		var balance api.Satoshi
		for _, utxo := range utxos {
			balance += utxo.Value
		}

		return UTXOsMsg{UTXOs: utxos, Err: nil}
	}
}

func (m SendModel) previewTransaction() tea.Cmd {
	// Calculate fee rate BEFORE creating closure to avoid stale capture issues
	feeRate := m.getFeeRateForLevel(m.feeLevel)

	// Collect UTXOs before closure
	var selectedUTXOs []api.UTXO
	for i, selected := range m.utxoSelected {
		if selected && i < len(m.utxos) {
			selectedUTXOs = append(selectedUTXOs, m.utxos[i])
		}
	}

	// Convert recipients to wallet.TxRecipient
	recipients := make([]wallet.TxRecipient, len(m.recipients))
	for i, r := range m.recipients {
		recipients[i] = wallet.TxRecipient{
			Address: r.Address,
			Amount:  r.Amount,
		}
	}

	// Capture values needed for the closure
	walletMgr := m.walletManager
	sendAll := m.sendAll

	return func() tea.Msg {
		if sendAll && len(recipients) == 1 {
			// Derive MAX amount without trusting local fee guesses.
			// Retry with adjusted recipient amount when backend reports insufficient funds.
			var totalInput api.Satoshi
			for _, u := range selectedUTXOs {
				totalInput += u.Value
			}
			if totalInput == 0 {
				return TxPreviewMsg{Err: fmt.Errorf("no UTXOs selected for MAX spend")}
			}

			maxAmount := totalInput
			for attempts := 0; attempts < 6; attempts++ {
				recipients[0].Amount = maxAmount
				result, err := walletMgr.PreviewBatchTransaction(
					0,             // Account 0
					recipients,    // Single MAX recipient
					feeRate,       // Fee rate in sat/vB (supports fractional)
					selectedUTXOs, // User-selected UTXOs
				)
				if err == nil {
					return TxPreviewMsg{Result: result}
				}

				// Parse "insufficient funds: have X sats, need Y sats" and reduce by the deficit.
				have, need, ok := parseInsufficientFundsValues(err)
				if !ok || need <= have {
					return TxPreviewMsg{Err: err}
				}
				deficit := need - have
				if deficit <= 0 || maxAmount <= deficit {
					return TxPreviewMsg{Err: err}
				}
				maxAmount -= deficit
			}

			return TxPreviewMsg{Err: fmt.Errorf("failed to derive MAX spend amount after retrying fee reconciliation")}
		}

		result, err := walletMgr.PreviewBatchTransaction(
			0,             // Account 0
			recipients,    // All recipients
			feeRate,       // Fee rate in sat/vB (supports fractional)
			selectedUTXOs, // User-selected UTXOs
		)

		if err != nil {
			return TxPreviewMsg{Err: err}
		}

		return TxPreviewMsg{Result: result}
	}
}

var insufficientFundsPattern = regexp.MustCompile(`insufficient funds: have (\d+) sats, need (\d+) sats`)

func parseInsufficientFundsValues(err error) (have, need api.Satoshi, ok bool) {
	if err == nil {
		return 0, 0, false
	}
	matches := insufficientFundsPattern.FindStringSubmatch(err.Error())
	if len(matches) != 3 {
		return 0, 0, false
	}
	haveInt, haveErr := strconv.ParseInt(matches[1], 10, 64)
	needInt, needErr := strconv.ParseInt(matches[2], 10, 64)
	if haveErr != nil || needErr != nil || haveInt < 0 || needInt < 0 {
		return 0, 0, false
	}
	return api.Satoshi(haveInt), api.Satoshi(needInt), true
}

// forceSelectAllUTXOs selects every available UTXO.
// Used by MAX spend mode to ensure deterministic "send everything" behavior.
func (m *SendModel) forceSelectAllUTXOs() {
	m.utxoSelected = make([]bool, len(m.utxos))
	for i := range m.utxoSelected {
		m.utxoSelected[i] = true
	}
}

// applyMaxSpendPreviewFinalization validates and finalizes MAX-spend amount
// from the actual preview result.
func (m *SendModel) applyMaxSpendPreviewFinalization(result *wallet.PSBTResult) error {
	if result == nil {
		return fmt.Errorf("max spend preview result is missing")
	}
	if len(m.recipients) != 1 {
		return fmt.Errorf("max spend supports exactly one recipient")
	}
	if result.HasChange || result.Change > 0 {
		return fmt.Errorf("max spend preview returned a change output (%d sats); refusing hidden leftover output", result.Change)
	}

	finalAmount := result.TotalInput() - result.Fee
	if finalAmount <= 0 {
		return fmt.Errorf("max spend amount is zero after fees")
	}

	m.recipients[0].Amount = finalAmount
	m.amountSats = finalAmount
	m.amountBTC = finalAmount.ToBTC()
	if m.usesBTC {
		m.amountInput.SetValue(fmt.Sprintf("%.8f", m.amountBTC))
	} else {
		m.amountInput.SetValue(fmt.Sprintf("%d", finalAmount))
	}

	return nil
}

// exportPSBT creates an UNSIGNED PSBT and exports it to base64 format
// User must explicitly press 's' to sign with local keys (security: no auto-signing)
func (m SendModel) exportPSBT() tea.Cmd {
	// Calculate fee rate BEFORE creating closure to avoid stale capture issues
	feeRate := m.getFeeRateForLevel(m.feeLevel)

	// Collect UTXOs before closure
	var selectedUTXOs []api.UTXO
	for i, selected := range m.utxoSelected {
		if selected && i < len(m.utxos) {
			selectedUTXOs = append(selectedUTXOs, m.utxos[i])
		}
	}

	// Convert recipients to wallet.TxRecipient
	recipients := make([]wallet.TxRecipient, len(m.recipients))
	for i, r := range m.recipients {
		recipients[i] = wallet.TxRecipient{
			Address: r.Address,
			Amount:  r.Amount,
		}
	}

	// Capture values needed for the closure
	walletMgr := m.walletManager

	return func() tea.Msg {
		// Create the PSBT
		psbtResult, err := walletMgr.CreatePSBTBatchTransaction(
			0,             // Account 0 (default account)
			recipients,    // All recipients
			feeRate,       // Fee rate in sat/vB (supports fractional)
			selectedUTXOs, // User-selected UTXOs (coin control)
		)
		if err != nil {
			return PSBTExportMsg{Err: fmt.Errorf("failed to create PSBT: %w", err)}
		}

		// Determine signature requirements (but don't sign yet - user must explicitly sign)
		var sigsTotal int
		if walletMgr.IsMultisig() {
			config := walletMgr.GetMultisigConfig()
			if config != nil {
				sigsTotal = config.M
			}
		} else {
			sigsTotal = 1
		}
		// PSBT is exported UNSIGNED - user presses 's' to sign with local keys

		// Export to base64
		base64Data, err := walletMgr.ExportPSBTBase64(psbtResult)
		if err != nil {
			return PSBTExportMsg{Err: fmt.Errorf("failed to export PSBT: %w", err)}
		}

		return PSBTExportMsg{
			Base64:    base64Data,
			SigsAdded: 0, // Unsigned - user must press 's' to sign
			SigsTotal: sigsTotal,
			Result:    psbtResult,
			Err:       nil,
		}
	}
}

// exportCurrentPSBT reuses the reviewed PSBT instead of rebuilding it.
func (m SendModel) exportCurrentPSBT() tea.Cmd {
	psbtResult := m.psbtResult
	walletMgr := m.walletManager

	return func() tea.Msg {
		if psbtResult == nil {
			return PSBTExportMsg{Err: fmt.Errorf("no previewed PSBT available")}
		}

		var sigsTotal int
		if walletMgr.IsMultisig() {
			config := walletMgr.GetMultisigConfig()
			if config != nil {
				sigsTotal = config.M
			}
		} else {
			sigsTotal = 1
		}

		base64Data, err := walletMgr.ExportPSBTBase64(psbtResult)
		if err != nil {
			return PSBTExportMsg{Err: fmt.Errorf("failed to export PSBT: %w", err)}
		}

		return PSBTExportMsg{
			Base64:    base64Data,
			SigsAdded: 0,
			SigsTotal: sigsTotal,
			Result:    psbtResult,
			Err:       nil,
		}
	}
}

// expandPath expands ~ to home directory and cleans the path.
// Returns an error if home directory cannot be determined.
func expandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		path = home
	}

	// Clean and normalize the path
	path = filepath.Clean(path)

	return path, nil
}

// validatePSBTFilePath validates a file path for PSBT operations.
// For exports, it checks the parent directory exists and is writable.
// For imports, it checks the file exists and is readable.
func validatePSBTFilePath(path string, forExport bool) error {
	if path == "" {
		return fmt.Errorf("file path is required")
	}

	// Ensure .psbt extension for clarity
	if !strings.HasSuffix(strings.ToLower(path), ".psbt") {
		return fmt.Errorf("file must have .psbt extension")
	}

	if forExport {
		// Check parent directory exists
		dir := filepath.Dir(path)
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist: %s", dir)
			}
			return fmt.Errorf("cannot access directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", dir)
		}
		// An existing file is overwritten; that is expected in the PSBT workflow.
	} else {
		// For import, check file exists and is readable
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file not found: %s", path)
			}
			return fmt.Errorf("cannot access file: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file: %s", path)
		}
		// Sanity check file size (PSBTs should be reasonably small)
		const maxPSBTSize = 10 * 1024 * 1024 // 10 MB max
		if info.Size() > maxPSBTSize {
			return fmt.Errorf("file too large (%d bytes), max %d bytes", info.Size(), maxPSBTSize)
		}
	}

	return nil
}

// savePSBTToFile saves the current PSBT to a file.
// Uses 0600 permissions for security (owner read/write only).
// PSBT files are saved as raw binary (not base64) per BIP174 standard.
func (m SendModel) savePSBTToFile(path string) tea.Cmd {
	// Capture values for closure
	psbtBase64 := m.psbtBase64

	return func() tea.Msg {
		// Expand and validate path
		expandedPath, err := expandPath(path)
		if err != nil {
			return PSBTFileMsg{Operation: "export", Err: fmt.Errorf("invalid path: %w", err)}
		}

		if err := validatePSBTFilePath(expandedPath, true); err != nil {
			return PSBTFileMsg{Operation: "export", Err: err}
		}

		// Decode base64 to binary - PSBT files are raw binary, not base64
		psbtBinary, err := base64.StdEncoding.DecodeString(psbtBase64)
		if err != nil {
			return PSBTFileMsg{Operation: "export", Err: fmt.Errorf("failed to decode PSBT: %w", err)}
		}

		// Write PSBT binary to file with restricted permissions
		// 0600 = owner read/write only (secure for sensitive data)
		if err := os.WriteFile(expandedPath, psbtBinary, 0600); err != nil {
			return PSBTFileMsg{Operation: "export", Err: fmt.Errorf("failed to write file: %w", err)}
		}

		return PSBTFileMsg{
			Operation: "export",
			FilePath:  expandedPath,
			Err:       nil,
		}
	}
}

// loadPSBTFromFile loads a PSBT from a file and parses it.
func (m SendModel) loadPSBTFromFile(path string) tea.Cmd {
	// Capture wallet manager for closure
	walletMgr := m.walletManager

	return func() tea.Msg {
		// Expand and validate path
		expandedPath, err := expandPath(path)
		if err != nil {
			return PSBTFileMsg{Operation: "import", Err: fmt.Errorf("invalid path: %w", err)}
		}

		if err := validatePSBTFilePath(expandedPath, false); err != nil {
			return PSBTFileMsg{Operation: "import", Err: err}
		}

		// Read file contents
		data, err := os.ReadFile(expandedPath) // #nosec G304 -- the user chose this PSBT file to import
		if err != nil {
			return PSBTFileMsg{Operation: "import", Err: fmt.Errorf("failed to read file: %w", err)}
		}

		// Determine if file is binary PSBT or base64-encoded
		// Binary PSBT files start with the magic bytes "psbt" (0x70736274)
		var psbtBase64 string
		if len(data) >= 4 && string(data[:4]) == "psbt" {
			// Binary PSBT - encode to base64 for our internal processing
			psbtBase64 = base64.StdEncoding.EncodeToString(data)
		} else {
			// Assume base64 text - trim whitespace (files may have trailing newlines)
			psbtBase64 = strings.TrimSpace(string(data))
		}

		// Validate it's a valid PSBT by trying to parse it
		psbtResult, err := walletMgr.ImportPSBTFromBase64(psbtBase64)
		if err != nil {
			return PSBTFileMsg{Operation: "import", Err: fmt.Errorf("invalid PSBT format: %w", err)}
		}

		// Count signatures in the imported PSBT
		sigsAdded := wallet.CountPSBTSigners(psbtResult.Packet)

		return PSBTFileMsg{
			Operation: "import",
			FilePath:  expandedPath,
			Base64:    psbtBase64,
			Result:    psbtResult,
			SigsAdded: sigsAdded,
			Err:       nil,
		}
	}
}

// getDefaultPSBTPath returns a default path for saving PSBTs.
// Format: ~/Desktop/tx-{timestamp}.psbt
func getDefaultPSBTPath() string {
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("~/Desktop/tx-%s.psbt", timestamp)
}

// discoverPSBTFiles finds all .psbt files in ~/.hax
// Returns files sorted by modification time (newest first)
func discoverPSBTFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	walletDir := filepath.Join(home, ".hax")

	// Also check Desktop as common location
	desktopDir := filepath.Join(home, "Desktop")

	var files []fileInfo

	// Scan wallet directory
	if entries, err := os.ReadDir(walletDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".psbt") {
				fullPath := filepath.Join(walletDir, entry.Name())
				if info, err := entry.Info(); err == nil {
					files = append(files, fileInfo{path: fullPath, modTime: info.ModTime()})
				}
			}
		}
	}

	// Scan Desktop
	if entries, err := os.ReadDir(desktopDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".psbt") {
				fullPath := filepath.Join(desktopDir, entry.Name())
				if info, err := entry.Info(); err == nil {
					files = append(files, fileInfo{path: fullPath, modTime: info.ModTime()})
				}
			}
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	// Extract just the paths
	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.path
	}

	return result
}

// fileInfo holds file path and modification time for sorting
type fileInfo struct {
	path    string
	modTime time.Time
}

// enumerateHWIDevices lists connected hardware wallets via HWI
func (m SendModel) enumerateHWIDevices() tea.Cmd {
	walletMgr := m.walletManager

	return func() tea.Msg {
		devices, err := walletMgr.ListHardwareWallets()
		return HWIEnumerateMsg{Devices: devices, Err: err}
	}
}

// signWithHWI signs the PSBT using the selected hardware wallet
func (m SendModel) signWithHWI(device wallet.HardwareDevice) tea.Cmd {
	walletMgr := m.walletManager
	psbtBase64 := m.psbtBase64

	return func() tea.Msg {
		signedPSBT, err := walletMgr.SignPSBTWithHardwareWallet(psbtBase64, device)
		if err != nil {
			return HWISignMsg{Err: err}
		}

		// Parse the signed PSBT to count signatures
		result, err := walletMgr.ImportPSBTFromBase64(signedPSBT)
		if err != nil {
			return HWISignMsg{Err: fmt.Errorf("failed to parse signed PSBT: %w", err)}
		}

		sigsAdded := wallet.CountPSBTSigners(result.Packet)

		return HWISignMsg{
			SignedPSBT: signedPSBT,
			SigsAdded:  sigsAdded,
			Err:        nil,
		}
	}
}

// findMatchingDevice returns the index of the first enumerated device whose
// fingerprint matches the wallet's stored hardware config (single-sig) or any
// hardware cosigner (multisig). Returns 0 if no match is found.
func (m SendModel) findMatchingDevice(devices []wallet.HardwareDevice) int {
	if hwConfig := m.walletManager.GetHardwareWalletConfig(); hwConfig != nil && hwConfig.Fingerprint != "" {
		for i, dev := range devices {
			if dev.Fingerprint == hwConfig.Fingerprint {
				return i
			}
		}
	}
	if msConfig := m.walletManager.GetMultisigConfig(); msConfig != nil {
		for i, dev := range devices {
			for _, cosigner := range msConfig.Cosigners {
				if cosigner.DeviceType != "" && cosigner.DeviceType != "local" && dev.Fingerprint == cosigner.Fingerprint {
					return i
				}
			}
		}
	}
	return 0
}

func (m SendModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ScreenActiveMsg:
		if m.loading || m.step == SendStepPreviewLoading || m.step == SendStepBroadcasting {
			return m, m.spinner.Tick
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if (m.loading || m.step == SendStepPreviewLoading || m.step == SendStepBroadcasting || m.showHWIDevices) && cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case FeeEstimatesMsg:
		if msg.Err != nil {
			m.feeEstimates = nil
			m.err = fmt.Errorf("fee estimates unavailable: %w", msg.Err)
		} else if msg.Estimates == nil {
			m.feeEstimates = nil
			m.err = fmt.Errorf("fee estimates unavailable")
		} else {
			m.feeEstimates = msg.Estimates
			m.err = nil
			m.updateFeeEstimate()
		}

	case UTXOsMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.utxos = msg.UTXOs
			m.balance = 0
			for _, utxo := range msg.UTXOs {
				m.balance += utxo.Value
			}
			if len(m.utxoSelected) != len(msg.UTXOs) {
				m.utxoSelected = make([]bool, len(msg.UTXOs))
				for i := range m.utxoSelected {
					m.utxoSelected[i] = true
				}
			}
		}

	case StateChangedMsg:
		m.balance = msg.State.Balance
		if len(msg.State.UTXOs) > 0 {
			m.utxos = msg.State.UTXOs
			if len(m.utxoSelected) != len(msg.State.UTXOs) {
				m.utxoSelected = make([]bool, len(msg.State.UTXOs))
				for i := range m.utxoSelected {
					m.utxoSelected[i] = true
				}
			}
		}
		if msg.State.FeeEstimates != nil {
			m.feeEstimates = msg.State.FeeEstimates
			m.updateFeeEstimate()
		}

	case TxPreviewMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.step = SendStepFee // Go back to fee selection
			m.cursor = cursorFromFeeLevel(m.feeLevel)
		} else {
			if m.sendAll {
				if msg.Result != nil && msg.Result.HasChange && msg.Result.Change > 0 && !m.sendAllRetried && len(m.recipients) == 1 {
					// First MAX preview can occasionally include a small change output due to fee/output shape.
					// Fold that change into the recipient amount and preview once more for a strict no-change result.
					m.sendAllRetried = true
					m.recipients[0].Amount += msg.Result.Change
					m.amountSats = m.recipients[0].Amount
					m.amountBTC = m.amountSats.ToBTC()
					if m.usesBTC {
						m.amountInput.SetValue(fmt.Sprintf("%.8f", m.amountBTC))
					} else {
						m.amountInput.SetValue(fmt.Sprintf("%d", m.amountSats))
					}
					m.err = nil
					m.step = SendStepPreviewLoading
					return m, tea.Batch(m.previewTransaction(), m.spinner.Tick)
				}
				if err := m.applyMaxSpendPreviewFinalization(msg.Result); err != nil {
					m.err = err
					m.step = SendStepFee
					m.cursor = cursorFromFeeLevel(m.feeLevel)
					break
				}
			}
			m.psbtResult = msg.Result
			m.err = nil
			m.scrollOffset = 0
			m.scrollViewport.SetYOffset(0)
			m.step = SendStepReview
			cmds = append(cmds, SuppressWheelFor(300*time.Millisecond))
		}

	case BroadcastMsg:
		m.loading = false
		if msg.Err != nil {
			m.broadcastErr = msg.Err
			m.step = SendStepReview // Go back to review
		} else {
			m.txID = msg.TxID
			m.step = SendStepComplete
		}

	case PSBTExportMsg:
		m.loading = false
		if msg.Err != nil {
			m.psbtExportErr = msg.Err
			m.showPSBTExport = false
		} else {
			m.psbtBase64 = msg.Base64
			m.psbtSigsAdded = msg.SigsAdded
			m.psbtSigsTotal = msg.SigsTotal
			m.psbtResult = msg.Result
			m.showPSBTExport = true
			m.scrollOffset = 0
			m.scrollViewport.SetYOffset(0)
			cmds = append(cmds, SuppressWheelFor(300*time.Millisecond))
			m.psbtExportErr = nil
		}

	case RawHexExportMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.showRawHexExport = false
		} else {
			m.rawHex = msg.Hex
			m.showRawHexExport = true
			m.err = nil
		}

	case PSBTFileMsg:
		m.loading = false
		m.showFileInput = false
		m.showFileBrowser = false
		if msg.Err != nil {
			m.psbtExportErr = msg.Err
		} else if msg.Operation == "export" {
			// Successfully saved PSBT to file
			m.psbtFilePath = msg.FilePath
			m.psbtExportErr = nil
			m.showCopied = true
			m.copiedTime = time.Now()
			m.lastAction = "saved"
		} else if msg.Operation == "import" {
			// Successfully loaded PSBT from file
			m.psbtFilePath = msg.FilePath
			m.psbtBase64 = msg.Base64
			m.psbtResult = msg.Result
			m.psbtSigsAdded = msg.SigsAdded
			m.psbtExportErr = nil
			m.showCopied = true
			m.copiedTime = time.Now()
			m.lastAction = "imported"

			// Update fee from imported PSBT (calculated in ImportPSBTFromBase64)
			if msg.Result != nil {
				m.estimatedFee = msg.Result.Fee
			}

			// Determine if import came from step 1 or step 7
			if m.fileBrowserForStep == "step1" || m.step == SendStepAddress {
				// Import from step 1 - jump to PSBT signing view
				// Determine signature requirements
				if m.walletManager.IsMultisig() {
					config := m.walletManager.GetMultisigConfig()
					if config != nil {
						m.psbtSigsTotal = config.M
					}
				} else {
					m.psbtSigsTotal = 1
				}
				m.scrollOffset = 0
				m.step = SendStepReview
				m.showPSBTExport = true
				m.scrollViewport.SetYOffset(0)
				cmds = append(cmds, SuppressWheelFor(300*time.Millisecond))
			}
			// For step 7, we're already showing PSBT export view, just update the data
		}

	case HWIEnumerateMsg:
		m.loading = false
		if msg.Err != nil {
			m.hwiError = msg.Err
			// Keep device selection open so user can retry
			m.showHWIDevices = true
		} else {
			m.hwiDevices = msg.Devices
			m.hwiError = nil
			if len(msg.Devices) == 0 {
				m.hwiError = &wallet.HWIError{Code: wallet.HWIErrNoDevice, RawMsg: "no devices found"}
				m.showHWIDevices = true
			} else {
				m.showHWIDevices = true
				m.selectedHWDevice = m.findMatchingDevice(msg.Devices)
			}
		}

	case HWISignMsg:
		m.loading = false
		m.showHWIDevices = false
		if msg.Err != nil {
			m.psbtExportErr = msg.Err
		} else {
			// Update PSBT with signed version
			m.psbtBase64 = msg.SignedPSBT
			m.psbtSigsAdded = msg.SigsAdded
			// Parse the new PSBT result - handle error explicitly
			result, err := m.walletManager.ImportPSBTFromBase64(msg.SignedPSBT)
			if err != nil {
				m.psbtExportErr = fmt.Errorf("failed to parse signed PSBT: %w", err)
			} else {
				m.psbtResult = result
				// Update fee from PSBT (in case it wasn't set earlier)
				m.estimatedFee = result.Fee
				m.showCopied = true
				m.copiedTime = time.Now()
				m.lastAction = "signed"
				m.psbtExportErr = nil
			}
		}

	case QRFrameTickMsg:
		// Advance animated QR code frame
		if m.showQRDisplay && len(m.qrFrames) > 1 {
			m.qrFrameIndex = (m.qrFrameIndex + 1) % len(m.qrFrames)
			m.qrRendered = m.renderQRFrame()
			return m, m.qrFrameTickCmd()
		}

	case TickMsg:
		// Handle "copied" animation timeout
		if m.showCopied && time.Since(m.copiedTime) > 2*time.Second {
			m.showCopied = false
			m.lastAction = ""
		}
		if m.showCopied {
			return m, m.animateTick()
		}

	case tea.MouseMsg:
		if m.isViewportScrollActive() {
			m.syncScrollViewport()
			m.scrollViewport, cmd = m.scrollViewport.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		// Handle file browser if open
		if m.showFileBrowser {
			return m.handleFileBrowser(msg)
		}

		// Global keys
		if msg.String() == "esc" && m.step == SendStepAddress {
			return m, Navigate(StateMainDashboard)
		}

		// Handle special keys first, then let inputs handle regular typing
		var handled bool
		var handledCmd tea.Cmd

		switch m.step {
		case SendStepAddress:
			handled, handledCmd = m.handleAddressInputSpecial(msg)
			if !handled {
				m.addressInput, cmd = m.addressInput.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				cmds = append(cmds, handledCmd)
			}
		case SendStepAmount:
			handled, handledCmd = m.handleAmountInputSpecial(msg)
			if !handled {
				m.amountInput, cmd = m.amountInput.Update(msg)
				cmds = append(cmds, cmd)
				// User is manually typing amount - disable send-all mode
				m.sendAll = false
			} else {
				cmds = append(cmds, handledCmd)
			}
		case SendStepRecipientList:
			return m.handleRecipientList(msg)
		case SendStepUTXOSelection:
			return m.handleUTXOSelection(msg)
		case SendStepFee:
			return m.handleFeeSelection(msg)
		case SendStepReview:
			return m.handleReview(msg)
		case SendStepComplete:
			return m.handleComplete(msg)
		}
	}

	// Keep viewport content/dimensions in sync for smooth Step 6/7 scrolling.
	m.syncScrollViewport()

	return m, tea.Batch(cmds...)
}

// handleAddressInputSpecial handles special keys for address input
// Returns (true, cmd) if key was handled, (false, nil) if input should handle it
func (m *SendModel) handleAddressInputSpecial(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "ctrl+v":
		// Paste from clipboard
		clipboardContent, err := clipboard.ReadAll()
		if err == nil && clipboardContent != "" {
			m.addressInput.SetValue(clipboardContent)
		} else if err != nil {
			m.err = fmt.Errorf("clipboard paste failed: %v", err)
		}
		return true, nil

	case "i", "I":
		// Import PSBT - open file browser
		m.psbtFiles = discoverPSBTFiles()
		m.fileBrowserCursor = 0
		m.fileBrowserForStep = "step1"
		m.showFileBrowser = true
		m.err = nil
		return true, nil

	case "enter":
		addr := m.addressInput.Value()
		if addr == "" {
			m.err = fmt.Errorf("please enter a recipient address")
			return true, nil
		}

		// Validate address
		if err := wallet.ValidateAddressForNetwork(addr, walletNetwork(m.walletManager)); err != nil {
			m.err = err
			return true, nil
		}

		m.recipientAddress = addr
		m.err = nil
		m.step = SendStepAmount
		return true, m.amountInput.Focus()

	case "esc":
		// Handled globally, but still mark as handled
		return true, Navigate(StateMainDashboard)
	}
	return false, nil // Not handled - let input handle it
}

// handleAmountInputSpecial handles special keys for amount input
// Returns (true, cmd) if key was handled, (false, nil) if input should handle it
func (m *SendModel) handleAmountInputSpecial(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "enter":
		amtStr := m.amountInput.Value()
		if amtStr == "" {
			m.err = fmt.Errorf("please enter an amount")
			return true, nil
		}

		// Parse amount
		if m.usesBTC {
			sats, err := api.ParseBTC(amtStr)
			if err != nil {
				m.err = fmt.Errorf("invalid BTC amount: %v", err)
				return true, nil
			}
			m.amountSats = sats
			m.amountBTC = sats.ToBTC()
		} else {
			sats, err := strconv.ParseInt(strings.TrimSpace(amtStr), 10, 64)
			if err != nil || sats <= 0 || api.Satoshi(sats) > api.MaxMoney {
				m.err = fmt.Errorf("invalid satoshi amount")
				return true, nil
			}
			m.amountSats = api.Satoshi(sats)
			m.amountBTC = api.Satoshi(sats).ToBTC()
		}

		// Validate recipient address is not empty
		if m.recipientAddress == "" {
			m.err = fmt.Errorf("recipient address is missing - please go back and enter an address")
			return true, nil
		}

		// Check dust limit for recipient address type
		dustLimit := wallet.GetDustLimit(m.recipientAddress)
		if m.amountSats < dustLimit {
			m.err = fmt.Errorf("amount %d sats is below dust limit (%d sats) for this address type - network will reject",
				m.amountSats, dustLimit)
			return true, nil
		}

		// Check if we have enough balance
		if m.amountSats > m.balance {
			m.err = fmt.Errorf("insufficient balance: have %d sats, need %d sats",
				m.balance, m.amountSats)
			return true, nil
		}

		m.err = nil

		// Add recipient to the list
		m.recipients = append(m.recipients, Recipient{
			Address: m.recipientAddress,
			Amount:  m.amountSats,
		})

		// Clear inputs for next recipient
		m.addressInput.SetValue("")
		m.amountInput.SetValue("")
		m.recipientAddress = ""
		m.amountSats = 0
		m.amountBTC = 0

		// Go to recipient list step
		m.cursor = 0
		m.step = SendStepRecipientList
		return true, nil

	case "tab":
		// Toggle between BTC and satoshi input
		m.usesBTC = !m.usesBTC
		m.amountInput.SetValue("")
		if m.usesBTC {
			m.amountInput.Placeholder = "0.00000000 BTC"
		} else {
			m.amountInput.Placeholder = "satoshis"
		}
		return true, nil

	case "m", "M":
		// Send Max/All - skip manual amount entry.
		// Amount is derived only after UTXO + fee selection and finalized from preview.
		if len(m.utxos) == 0 || m.balance == 0 {
			m.err = fmt.Errorf("no UTXOs available to send")
			return true, nil
		}
		if len(m.recipients) > 0 {
			m.err = fmt.Errorf("MAX spend cannot be combined with multiple recipients")
			return true, nil
		}
		if m.recipientAddress == "" {
			m.err = fmt.Errorf("recipient address is missing - please go back and enter an address")
			return true, nil
		}

		m.sendAll = true
		m.sendAllRetried = false
		m.err = nil

		// Add MAX recipient with placeholder amount; exact amount is derived later.
		m.recipients = append(m.recipients, Recipient{
			Address: m.recipientAddress,
			Amount:  0,
		})

		// Reset per-recipient input fields and skip UTXO selection for MAX spend.
		// Inputs are locked to "all UTXOs" and still shown later in review.
		m.addressInput.SetValue("")
		m.amountInput.SetValue("")
		m.recipientAddress = ""
		m.amountSats = 0
		m.amountBTC = 0

		m.forceSelectAllUTXOs()
		m.cursor = 1 // Medium
		m.feeLevel = FeeLevelMedium
		m.step = SendStepFee
		m.updateFeeEstimate()
		return true, nil

	case "esc":
		m.step = SendStepAddress
		return true, m.addressInput.Focus()
	}
	return false, nil // Not handled - let input handle it
}

func (m *SendModel) handleRecipientList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.recipients)-1 {
			m.cursor++
		}
	case "d", "delete", "backspace":
		// Delete selected recipient
		if m.cursor < len(m.recipients) && len(m.recipients) > 1 {
			m.recipients = append(m.recipients[:m.cursor], m.recipients[m.cursor+1:]...)
			m.sendAll = false
			if m.cursor >= len(m.recipients) {
				m.cursor = len(m.recipients) - 1
			}
		} else if len(m.recipients) == 1 {
			m.err = fmt.Errorf("cannot delete last recipient")
		}
	case "a", "+":
		// Add another recipient - go back to address input
		m.sendAll = false
		m.step = SendStepAddress
		return m, m.addressInput.Focus()
	case "enter", "tab":
		// Continue to UTXO selection (or skip for MAX spend)
		if len(m.recipients) == 0 {
			m.err = fmt.Errorf("add at least one recipient")
			return m, nil
		}

		// Calculate total amount across all recipients
		var totalAmount api.Satoshi
		for _, r := range m.recipients {
			totalAmount += r.Amount
		}

		// Check if we have enough balance
		if totalAmount > m.balance {
			m.err = fmt.Errorf("insufficient balance: have %d sats, need %d sats",
				m.balance, totalAmount)
			return m, nil
		}

		m.err = nil

		// MAX spend locks all inputs, so skip manual UTXO selection step.
		if m.sendAll {
			m.forceSelectAllUTXOs()
			m.cursor = 1 // Medium
			m.feeLevel = FeeLevelMedium
			m.step = SendStepFee
			m.updateFeeEstimate()
			return m, nil
		}

		// Initialize UTXO selection - select all by default
		m.utxoSelected = make([]bool, len(m.utxos))
		for i := range m.utxoSelected {
			m.utxoSelected[i] = true
		}
		m.cursor = 0
		m.step = SendStepUTXOSelection
		return m, nil

	case "esc":
		// Go back to edit the last recipient
		if len(m.recipients) > 0 {
			// Pop last recipient and edit it
			last := m.recipients[len(m.recipients)-1]
			m.recipients = m.recipients[:len(m.recipients)-1]
			m.recipientAddress = last.Address
			m.amountSats = last.Amount
			m.amountBTC = last.Amount.ToBTC()
			m.addressInput.SetValue(last.Address)
			if m.usesBTC {
				m.amountInput.SetValue(fmt.Sprintf("%.8f", m.amountBTC))
			} else {
				m.amountInput.SetValue(fmt.Sprintf("%d", m.amountSats))
			}

			// If address is empty, go to address step; otherwise go to amount step
			if last.Address == "" {
				m.step = SendStepAddress
				return m, m.addressInput.Focus()
			}
		}
		m.step = SendStepAmount
		return m, m.amountInput.Focus()
	}
	return m, nil
}

func (m *SendModel) handleUTXOSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.utxos)-1 {
			m.cursor++
		}
	case " ", "enter":
		if m.sendAll {
			m.forceSelectAllUTXOs()
			m.err = fmt.Errorf("MAX spend uses all UTXOs; selection is locked")
			return m, nil
		}
		// Toggle selection with spacebar or enter
		if m.cursor < len(m.utxoSelected) {
			m.utxoSelected[m.cursor] = !m.utxoSelected[m.cursor]
		}
	case "a":
		if m.sendAll {
			m.forceSelectAllUTXOs()
			m.err = nil
			return m, nil
		}
		// Select all
		for i := range m.utxoSelected {
			m.utxoSelected[i] = true
		}
	case "n":
		if m.sendAll {
			m.forceSelectAllUTXOs()
			m.err = fmt.Errorf("MAX spend cannot deselect UTXOs")
			return m, nil
		}
		// Select none
		for i := range m.utxoSelected {
			m.utxoSelected[i] = false
		}
	case "tab":
		if m.sendAll {
			m.forceSelectAllUTXOs()
		}
		// Continue to fee selection
		// Validate that at least one UTXO is selected
		hasSelection := false
		var totalSelected api.Satoshi
		for i, selected := range m.utxoSelected {
			if selected {
				hasSelection = true
				totalSelected += m.utxos[i].Value
			}
		}

		if !hasSelection {
			m.err = fmt.Errorf("please select at least one UTXO")
			return m, nil
		}

		// Calculate total amount across all recipients
		var totalSending api.Satoshi
		for _, r := range m.recipients {
			totalSending += r.Amount
		}

		// Check if selected UTXOs can cover the amount (rough check)
		if totalSelected < totalSending {
			m.err = fmt.Errorf("selected UTXOs insufficient: have %d sats, need at least %d sats", totalSelected, totalSending)
			return m, nil
		}

		m.err = nil
		m.cursor = 1 // Default to Medium (index 1: High, Medium, Low)
		m.feeLevel = FeeLevelMedium
		m.step = SendStepFee
		// Update fee estimate with newly selected UTXOs
		m.updateFeeEstimate()
		return m, nil

	case "esc":
		// Go back to recipient list (step 3), not amount (step 2)
		// This way the user can see their recipients and choose to edit one
		m.cursor = 0
		m.step = SendStepRecipientList
		return m, nil
	}
	return m, nil
}

// feeSoftCap is the custom fee rate above which the user must type
// highFeeConfirmWord to proceed. feeHardCap is never accepted.
const (
	feeSoftCap         = 500.0
	feeHardCap         = 10000.0
	highFeeConfirmWord = "high fee"
)

func (m *SendModel) handleHighFeeConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if strings.TrimSpace(strings.ToLower(m.highFeeInput.Value())) != highFeeConfirmWord {
			m.err = fmt.Errorf("type %q to use %s sat/vB, or press esc", highFeeConfirmWord, formatFeeRate(m.highFeePending))
			return m, nil
		}
		m.customFeeRate = m.highFeePending
		m.highFeePending = 0
		m.editingCustom = false
		m.err = nil
		m.updateFeeEstimate()
		return m, nil
	case "esc":
		m.highFeePending = 0
		m.err = nil
		return m, m.customFeeInput.Focus()
	}
	var cmd tea.Cmd
	m.highFeeInput, cmd = m.highFeeInput.Update(msg)
	return m, cmd
}

func (m *SendModel) handleFeeSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.highFeePending > 0 {
		return m.handleHighFeeConfirm(msg)
	}
	// Handle custom fee input mode
	if m.editingCustom {
		switch msg.String() {
		case "enter":
			// Parse and validate custom fee rate
			feeStr := m.customFeeInput.Value()
			if feeStr == "" {
				m.err = fmt.Errorf("please enter a fee rate")
				return m, nil
			}
			feeRate, err := strconv.ParseFloat(feeStr, 64)
			if err != nil || math.IsNaN(feeRate) || feeRate < 0.1 {
				m.err = fmt.Errorf("invalid fee rate (minimum 0.1 sat/vB)")
				return m, nil
			}
			if feeRate > feeHardCap {
				m.err = fmt.Errorf("fee rate too high (maximum %.0f sat/vB)", feeHardCap)
				return m, nil
			}
			if feeRate > feeSoftCap {
				m.highFeePending = feeRate
				m.highFeeInput = textinput.New()
				m.highFeeInput.Placeholder = highFeeConfirmWord
				m.highFeeInput.CharLimit = 20
				m.highFeeInput.Width = 20
				m.err = nil
				return m, m.highFeeInput.Focus()
			}
			m.customFeeRate = feeRate
			m.editingCustom = false
			m.err = nil
			m.updateFeeEstimate()
			return m, nil
		case "esc":
			// Cancel custom fee editing, go back to presets
			m.editingCustom = false
			m.feeLevel = FeeLevelMedium
			m.cursor = 1
			m.updateFeeEstimate()
			return m, nil
		default:
			// Let the text input handle the key
			var cmd tea.Cmd
			m.customFeeInput, cmd = m.customFeeInput.Update(msg)
			return m, cmd
		}
	}

	// Normal fee selection mode
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.feeLevel = feeLevelFromCursor(m.cursor)
			m.updateFeeEstimate()
		}
	case "down", "j":
		if m.cursor < 3 { // Now includes custom option
			m.cursor++
			m.feeLevel = feeLevelFromCursor(m.cursor)
			m.updateFeeEstimate()
		}
	case "c", "C":
		// Activate custom fee input
		m.cursor = 3
		m.feeLevel = FeeLevelCustom
		m.editingCustom = true
		m.customFeeInput.Focus()
		return m, textinput.Blink
	case "enter":
		if m.feeLevel == FeeLevelCustom && m.customFeeRate == 0 {
			// Need to enter custom fee first
			m.editingCustom = true
			m.customFeeInput.Focus()
			return m, textinput.Blink
		}

		if m.feeLevel != FeeLevelCustom && m.getFeeRateForLevel(m.feeLevel) <= 0 {
			m.err = fmt.Errorf("fee estimates unavailable — enter a custom fee rate (c) or retry")
			return m, nil
		}

		// If sendAll is enabled, enforce deterministic MAX preconditions.
		// Final amount is derived inside previewTransaction() from backend preview results.
		if m.sendAll {
			if len(m.recipients) != 1 {
				m.err = fmt.Errorf("MAX spend supports exactly one recipient")
				return m, nil
			}
			m.forceSelectAllUTXOs()
			m.sendAllRetried = false
		}

		m.step = SendStepPreviewLoading
		// Preview the transaction to get actual UTXO selection and change details
		return m, tea.Batch(m.previewTransaction(), m.spinner.Tick)
	case "esc":
		if m.sendAll {
			m.step = SendStepRecipientList
			m.cursor = 0
		} else {
			m.step = SendStepUTXOSelection
			m.cursor = 0
		}
		return m, nil
	}
	return m, nil
}

func (m *SendModel) handleReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle file path input mode (for save/load PSBT)
	if m.showFileInput {
		switch msg.String() {
		case "enter":
			// Execute the file operation
			path := m.psbtFileInput.Value()
			if path == "" {
				path = getDefaultPSBTPath()
			}
			m.loading = true
			if m.fileInputMode == "export" {
				return m, m.savePSBTToFile(path)
			} else {
				return m, m.loadPSBTFromFile(path)
			}
		case "esc":
			// Cancel file input
			m.showFileInput = false
			m.psbtFileInput.SetValue("")
			return m, nil
		default:
			// Pass to text input
			var cmd tea.Cmd
			m.psbtFileInput, cmd = m.psbtFileInput.Update(msg)
			return m, cmd
		}
	}

	// Handle HWI device selection
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
			// Sign with selected device
			if m.selectedHWDevice >= 0 && m.selectedHWDevice < len(m.hwiDevices) {
				m.loading = true
				m.hwiError = nil
				return m, tea.Batch(m.spinner.Tick, m.signWithHWI(m.hwiDevices[m.selectedHWDevice]))
			}
			return m, nil
		case "r", "R":
			// Retry device enumeration
			m.loading = true
			m.hwiError = nil
			m.hwiDevices = nil
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevices())
		case "esc":
			m.showHWIDevices = false
			m.hwiError = nil
			return m, nil
		}
		return m, nil
	}

	// Handle QR code display overlay
	if m.showQRDisplay {
		switch msg.String() {
		case "1":
			// Switch to BC-UR format
			if m.qrFormat != "ur" {
				m.qrFormat = "ur"
				m.generateQRFrames()
				m.qrRendered = m.renderQRFrame()
				if len(m.qrFrames) > 1 {
					return m, m.qrFrameTickCmd()
				}
			}
			return m, nil
		case "2":
			// Switch to BBQr format
			if m.qrFormat != "bbqr" {
				m.qrFormat = "bbqr"
				m.generateQRFrames()
				m.qrRendered = m.renderQRFrame()
				if len(m.qrFrames) > 1 {
					return m, m.qrFrameTickCmd()
				}
			}
			return m, nil
		case "esc":
			m.showQRDisplay = false
			m.qrFrames = nil
			m.qrRendered = ""
			return m, nil
		}
		return m, nil
	}

	// Handle QR format selection menu
	if m.showQRFormatMenu {
		switch msg.String() {
		case "1":
			// BC-UR (Passport, Keystone)
			m.showQRFormatMenu = false
			m.showQRDisplay = true
			m.qrFormat = "ur"
			m.qrFrameIndex = 0
			m.generateQRFrames()
			m.qrRendered = m.renderQRFrame()
			if len(m.qrFrames) > 1 {
				return m, m.qrFrameTickCmd()
			}
			return m, nil
		case "2":
			// BBQr (Coldcard Q)
			m.showQRFormatMenu = false
			m.showQRDisplay = true
			m.qrFormat = "bbqr"
			m.qrFrameIndex = 0
			m.generateQRFrames()
			m.qrRendered = m.renderQRFrame()
			if len(m.qrFrames) > 1 {
				return m, m.qrFrameTickCmd()
			}
			return m, nil
		case "esc":
			m.showQRFormatMenu = false
			return m, nil
		}
		return m, nil
	}

	// Handle UR paste input overlay
	if m.showURInput {
		switch msg.String() {
		case "enter":
			urStr := strings.TrimSpace(m.urInput.Value())
			if urStr == "" {
				return m, nil
			}
			psbtBytes, err := ur.DecodePSBTFromUR(urStr)
			if err != nil {
				m.psbtExportErr = fmt.Errorf("invalid UR: %v", err)
				m.showURInput = false
				return m, nil
			}
			// Import the decoded PSBT
			b64 := base64.StdEncoding.EncodeToString(psbtBytes)
			result, err := m.walletManager.ImportPSBTFromBase64(b64)
			if err != nil {
				m.psbtExportErr = fmt.Errorf("PSBT import failed: %v", err)
				m.showURInput = false
				return m, nil
			}
			m.psbtBase64 = b64
			m.psbtResult = result
			m.psbtSigsAdded = wallet.CountPSBTSigners(result.Packet)
			m.estimatedFee = result.Fee
			m.showURInput = false
			m.showCopied = true
			m.copiedTime = time.Now()
			m.lastAction = "imported"
			return m, m.animateTick()
		case "esc":
			m.showURInput = false
			return m, nil
		default:
			var urCmd tea.Cmd
			m.urInput, urCmd = m.urInput.Update(msg)
			return m, urCmd
		}
	}

	// Handle PSBT export view
	if m.showPSBTExport {
		switch msg.String() {
		case "h", "H":
			// Sign with hardware wallet via HWI
			if m.psbtBase64 == "" {
				m.psbtExportErr = fmt.Errorf("no PSBT available - please try again")
				return m, nil
			}
			m.loading = true
			m.psbtExportErr = nil
			m.hwiError = nil
			return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevices())
		case "f", "F":
			// Save PSBT to file (for hardware wallet like Coldcard)
			if m.psbtBase64 == "" {
				return m, nil
			}
			m.showFileInput = true
			m.fileInputMode = "export"
			m.psbtFileInput.SetValue(getDefaultPSBTPath())
			m.psbtFileInput.Focus()
			m.psbtExportErr = nil
			return m, textinput.Blink
		case "i", "I":
			// Import signed PSBT from file (from hardware wallet)
			// Show file browser instead of manual input
			m.psbtFiles = discoverPSBTFiles()
			m.fileBrowserCursor = 0
			m.fileBrowserForStep = "step7"
			m.showFileBrowser = true
			m.psbtExportErr = nil
			return m, nil
		case "c":
			// Copy PSBT to clipboard
			if m.psbtBase64 != "" {
				err := clipboard.WriteAll(m.psbtBase64)
				if err == nil {
					m.showCopied = true
					m.copiedTime = time.Now()
					m.lastAction = "copied"
					return m, m.animateTick()
				} else {
					m.psbtExportErr = fmt.Errorf("failed to copy to clipboard: %v", err)
				}
			}
			return m, nil
		case "s", "S":
			// Sign with local keys
			if m.psbtResult == nil {
				return m, nil
			}

			// Check if already fully signed
			if m.psbtSigsAdded >= m.psbtSigsTotal {
				m.psbtExportErr = fmt.Errorf("already fully signed")
				return m, nil
			}

			// Hardware wallets cannot sign locally - direct to HWI or file export
			if m.walletManager.IsHardwareWallet() {
				m.psbtExportErr = fmt.Errorf("hardware wallet detected - use (h) for HWI or (f) for file export")
				return m, nil
			}

			var sigsAdded int
			var err error

			if m.walletManager.IsMultisig() {
				// Multisig: sign with any local cosigner keys
				if !m.walletManager.CanSignMultisigLocally() {
					m.psbtExportErr = fmt.Errorf("no local signing keys - use (h) HWI or (f) file export")
					return m, nil
				}
				sigsAdded, err = m.walletManager.SignMultisigPSBTTransaction(m.psbtResult)
			} else {
				// Single-sig software wallet: sign with wallet key
				err = m.walletManager.SignPSBTTransaction(m.psbtResult)
				if err == nil {
					sigsAdded = 1
				}
			}

			if err != nil {
				m.psbtExportErr = fmt.Errorf("signing failed: %v", err)
				return m, nil
			}
			if sigsAdded == 0 {
				m.psbtExportErr = fmt.Errorf("already signed with local keys")
				return m, nil
			}

			// Update the base64 with the newly signed PSBT
			newBase64, err := m.walletManager.ExportPSBTBase64(m.psbtResult)
			if err != nil {
				m.psbtExportErr = fmt.Errorf("failed to export signed PSBT: %v", err)
				return m, nil
			}
			m.psbtBase64 = newBase64
			// Count unique signers in PSBT (not per-input signatures)
			m.psbtSigsAdded = wallet.CountPSBTSigners(m.psbtResult.Packet)
			m.showCopied = true
			m.copiedTime = time.Now()
			m.lastAction = "signed"
			return m, m.animateTick()
		case "b", "B":
			// Broadcast (only if fully signed)
			if m.psbtSigsAdded >= m.psbtSigsTotal && m.psbtResult != nil {
				txID, err := m.walletManager.BroadcastPSBT(m.psbtResult)
				if err != nil {
					m.psbtExportErr = fmt.Errorf("broadcast failed: %v", err)
					return m, nil
				}
				// Success - go to complete step and trigger lightweight refresh
				m.txID = txID
				m.showPSBTExport = false
				m.step = SendStepComplete
				return m, m.postBroadcastRefreshCmd()
			} else {
				m.psbtExportErr = fmt.Errorf("need %d more signature(s) before broadcast",
					m.psbtSigsTotal-m.psbtSigsAdded)
			}
			return m, nil
		case "q", "Q":
			// Show QR code for air-gapped hardware wallet signing
			if m.psbtBase64 == "" {
				return m, nil
			}
			m.showQRFormatMenu = true
			m.psbtExportErr = nil
			return m, nil
		case "u", "U":
			// Paste UR string (signed PSBT from hardware wallet)
			m.showURInput = true
			m.urInput = textinput.New()
			m.urInput.Placeholder = "ur:crypto-psbt/..."
			m.urInput.Width = 60
			m.urInput.CharLimit = 10000
			m.urInput.Focus()
			m.psbtExportErr = nil
			return m, textinput.Blink
		case "esc":
			// Close PSBT export view
			m.showPSBTExport = false
			m.scrollOffset = 0
			m.scrollViewport.SetYOffset(0)
			m.psbtBase64 = ""
			// If PSBT was imported at step 1, go back to dashboard (no prior steps data)
			if m.fileBrowserForStep == "step1" {
				m.fileBrowserForStep = ""
				return m, tea.Batch(Navigate(StateMainDashboard), SuppressWheelFor(300*time.Millisecond))
			}
			return m, SuppressWheelFor(300 * time.Millisecond)
		case "up", "down", "pgup", "pgdown":
			m.syncScrollViewport()
			m.scrollViewport, _ = m.scrollViewport.Update(msg)
			return m, nil
		}
		return m, nil
	}

	// Handle raw hex export view
	if m.showRawHexExport {
		switch msg.String() {
		case "c":
			// Copy raw hex to clipboard
			if m.rawHex != "" {
				err := clipboard.WriteAll(m.rawHex)
				if err == nil {
					m.showCopied = true
					m.copiedTime = time.Now()
					return m, m.animateTick()
				} else {
					m.err = fmt.Errorf("failed to copy to clipboard: %v", err)
				}
			}
			return m, nil
		case "esc", "q":
			// Close raw hex export view
			m.showRawHexExport = false
			m.rawHex = ""
			return m, nil
		}
		return m, nil
	}

	// Normal review screen
	switch msg.String() {
	case "enter":
		// UNIFIED FLOW: All wallet types go to Sign & Broadcast view (PSBT)
		// This provides consistent UX for software, hardware, and multisig wallets
		m.scrollOffset = 0
		m.scrollViewport.SetYOffset(0)
		m.loading = true
		if m.psbtResult != nil {
			return m, m.exportCurrentPSBT()
		}
		return m, m.exportPSBT()
	case "esc":
		m.scrollOffset = 0
		m.step = SendStepFee
		m.cursor = cursorFromFeeLevel(m.feeLevel)
	case "up", "down", "pgup", "pgdown":
		m.syncScrollViewport()
		m.scrollViewport, _ = m.scrollViewport.Update(msg)
		return m, nil
	}
	return m, nil
}

func (m *SendModel) handleComplete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c":
		// Copy transaction ID to clipboard
		if m.txID != "" {
			err := clipboard.WriteAll(m.txID)
			if err == nil {
				m.showCopied = true
				m.copiedTime = time.Now()
				return m, m.animateTick()
			} else {
				m.err = fmt.Errorf("failed to copy to clipboard: %v", err)
			}
		}
		return m, nil

	case "enter":
		// Open transaction in browser on mempool.space
		if m.txID != "" {
			_ = openTransactionInBrowser(walletNetwork(m.walletManager), m.txID)
		}
		return m, nil

	case "n", "N":
		// Start a new transaction - reset state completely
		m.resetTransaction()
		m.step = SendStepAddress
		return m, m.addressInput.Focus()

	case "q", "esc":
		// Return to dashboard - data already refreshed by postBroadcastRefreshCmd
		return m, Navigate(StateMainDashboard)
	}

	return m, nil
}

func (m *SendModel) handleFileBrowser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.fileBrowserCursor > 0 {
			m.fileBrowserCursor--
		}
		return m, nil

	case "down", "j":
		if m.fileBrowserCursor < len(m.psbtFiles)-1 {
			m.fileBrowserCursor++
		}
		return m, nil

	case "enter":
		// Select file and load it
		if len(m.psbtFiles) > 0 && m.fileBrowserCursor < len(m.psbtFiles) {
			path := m.psbtFiles[m.fileBrowserCursor]
			m.showFileBrowser = false
			m.loading = true
			return m, m.loadPSBTFromFile(path)
		}
		// No files available, show manual input
		m.showFileBrowser = false
		m.showFileInput = true
		m.fileInputMode = "import"
		m.psbtFileInput.SetValue("")
		m.psbtFileInput.Focus()
		return m, textinput.Blink

	case "p", "P":
		// Manual path entry
		m.showFileBrowser = false
		m.showFileInput = true
		m.fileInputMode = "import"
		m.psbtFileInput.SetValue("")
		m.psbtFileInput.Focus()
		return m, textinput.Blink

	case "r", "R":
		// Refresh file list
		m.psbtFiles = discoverPSBTFiles()
		m.fileBrowserCursor = 0
		return m, nil

	case "esc":
		// Close file browser
		m.showFileBrowser = false
		return m, nil
	}
	return m, nil
}

// postBroadcastRefreshCmd performs an immediate cache refresh after broadcast
// so dashboard/UTXO screens do not stay stale while waiting for notifications.
func (m *SendModel) postBroadcastRefreshCmd() tea.Cmd {
	walletMgr := m.walletManager
	return func() tea.Msg {
		before := walletMgr.GetState()
		walletMgr.RefreshCacheWithOptions(wallet.RefreshOptions{
			Quiet:            true,
			SkipAddressScan:  true,
			SkipFeeEstimates: true,
		})
		after := walletMgr.GetState()

		return PostBroadcastRefreshDoneMsg{
			BalanceChange: after.Balance - before.Balance,
			State:         after,
		}
	}
}

// resetTransaction clears all transaction-related state to start fresh
func (m *SendModel) resetTransaction() {
	// Clear inputs
	m.addressInput.SetValue("")
	m.amountInput.SetValue("")

	// Reset data
	m.recipients = nil
	m.recipientAddress = ""
	m.amountSats = 0
	m.amountBTC = 0
	m.usesBTC = true
	m.sendAll = false
	m.sendAllRetried = false

	// Reset transaction state
	m.txID = ""
	m.broadcastErr = nil
	m.err = nil

	// Reset UTXO selection
	m.utxoSelected = nil
	m.psbtResult = nil

	// Reset UI state
	m.showCopied = false
	m.lastAction = ""
	m.loading = false
	m.cursor = 1
	m.feeLevel = FeeLevelMedium

	// Update fee estimate with current level
	m.updateFeeEstimate()
}

// animateTick returns a command that sends a TickMsg for animation
func (m SendModel) animateTick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// qrFrameTickCmd returns a tea.Cmd that advances the animated QR frame after a delay.
func (m SendModel) qrFrameTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return QRFrameTickMsg{}
	})
}

// generateQRFrames creates QR frame content from the current PSBT based on qrFormat.
func (m *SendModel) generateQRFrames() {
	if m.psbtBase64 == "" {
		m.qrFrames = nil
		return
	}

	psbtBytes, err := base64.StdEncoding.DecodeString(m.psbtBase64)
	if err != nil {
		m.qrFrames = nil
		m.psbtExportErr = fmt.Errorf("failed to decode PSBT: %v", err)
		return
	}

	switch m.qrFormat {
	case "ur":
		urStr := ur.EncodePSBTAsUR(psbtBytes)
		// Uppercase for QR alphanumeric mode efficiency
		m.qrFrames = []string{strings.ToUpper(urStr)}
	case "bbqr":
		frames, err := bbqr.Encode(psbtBytes, bbqr.FileTypePSBT)
		if err != nil {
			m.qrFrames = nil
			m.psbtExportErr = fmt.Errorf("BBQr encoding failed: %v", err)
			return
		}
		m.qrFrames = frames
	}

	m.qrFrameIndex = 0
}

// renderQRFrame generates the QR code string for the current frame.
func (m *SendModel) renderQRFrame() string {
	if len(m.qrFrames) == 0 || m.qrFrameIndex >= len(m.qrFrames) {
		return ""
	}

	qrStr, err := ui.GenerateQRCodeHalfBlocks(m.qrFrames[m.qrFrameIndex])
	if err != nil {
		return styles.ErrorText.Render(fmt.Sprintf("QR generation failed: %v", err))
	}
	return qrStr
}

func (m *SendModel) updateFeeEstimate() {
	// For custom fee, we don't need fee estimates from network
	if m.feeLevel == FeeLevelCustom {
		if m.customFeeRate > 0 {
			m.calculateFeeWithRate(m.customFeeRate)
		}
		return
	}

	if m.feeEstimates == nil {
		return
	}

	var feeRate float64
	switch m.feeLevel {
	case FeeLevelLow:
		feeRate = m.feeEstimates.HourFee
	case FeeLevelMedium:
		feeRate = m.feeEstimates.HalfHourFee
	case FeeLevelHigh:
		feeRate = m.feeEstimates.FastestFee
	}

	// Count selected UTXOs and their total value
	// User's explicit selection = coin control, respect their choice
	numInputs := 0
	var totalInput api.Satoshi
	for i, selected := range m.utxoSelected {
		if selected && i < len(m.utxos) {
			numInputs++
			totalInput += m.utxos[i].Value
		}
	}

	if numInputs == 0 {
		numInputs = 1 // Minimum 1 input for estimation
	}

	// Calculate total amount to send (all recipients)
	var targetAmount api.Satoshi
	numRecipientOutputs := len(m.recipients)
	if numRecipientOutputs == 0 {
		numRecipientOutputs = 1
	}
	for _, r := range m.recipients {
		targetAmount += r.Amount
	}

	// Determine number of outputs (recipients + change if applicable)
	const dustLimit api.Satoshi = 546

	// Calculate fee with change output first
	vSizeWithChange := m.estimateTxVSize(numInputs, numRecipientOutputs+1)
	feeWithChange := api.Satoshi(float64(vSizeWithChange) * feeRate)
	changeAmount := totalInput - targetAmount - feeWithChange

	numOutputs := numRecipientOutputs + 1 // recipients + change
	if changeAmount < dustLimit {
		numOutputs = numRecipientOutputs // No change - below dust threshold
	}

	// Calculate final estimate based on user's selected inputs
	m.estimatedSize = m.estimateTxVSize(numInputs, numOutputs)
	m.estimatedFee = api.Satoshi(float64(m.estimatedSize) * feeRate)
}

// calculateFeeWithRate calculates fee using a specific rate (for custom fee)
// Supports fractional sat/vB rates (e.g., 0.5 sat/vB) by rounding up the final fee
func (m *SendModel) calculateFeeWithRate(feeRate float64) {
	// Count selected UTXOs and their total value
	numInputs := 0
	var totalInput api.Satoshi
	for i, selected := range m.utxoSelected {
		if selected && i < len(m.utxos) {
			numInputs++
			totalInput += m.utxos[i].Value
		}
	}

	if numInputs == 0 {
		numInputs = 1
	}

	// Calculate total amount to send (all recipients)
	var targetAmount api.Satoshi
	numRecipientOutputs := len(m.recipients)
	if numRecipientOutputs == 0 {
		numRecipientOutputs = 1
	}
	for _, r := range m.recipients {
		targetAmount += r.Amount
	}

	const dustLimit api.Satoshi = 546

	// Calculate fee with change output first (ceiling to ensure we don't underpay)
	vSizeWithChange := m.estimateTxVSize(numInputs, numRecipientOutputs+1)
	feeWithChange := api.Satoshi(math.Ceil(float64(vSizeWithChange) * feeRate))
	changeAmount := totalInput - targetAmount - feeWithChange

	numOutputs := numRecipientOutputs + 1
	if changeAmount < dustLimit {
		numOutputs = numRecipientOutputs
	}

	m.estimatedSize = m.estimateTxVSize(numInputs, numOutputs)
	m.estimatedFee = api.Satoshi(math.Ceil(float64(m.estimatedSize) * feeRate))
}

func (m SendModel) View() string {
	_ = m.width

	if m.showFileBrowser {
		return m.viewFileBrowser()
	}

	if m.showQRDisplay {
		return m.viewQRDisplay()
	}

	if m.showQRFormatMenu {
		return m.viewQRFormatMenu()
	}

	// Handle UR paste input overlay
	if m.showURInput {
		return m.viewURInput()
	}

	// Handle PSBT export view overlay
	if m.showPSBTExport {
		return m.viewPSBTExport()
	}

	// Handle raw hex export view overlay
	if m.showRawHexExport {
		return m.viewRawHexExport()
	}

	var content string

	switch m.step {
	case SendStepAddress:
		content = m.viewAddressInput()
	case SendStepAmount:
		content = m.viewAmountInput()
	case SendStepRecipientList:
		content = m.viewRecipientList()
	case SendStepUTXOSelection:
		content = m.viewUTXOSelection()
	case SendStepFee:
		content = m.viewFeeSelection()
	case SendStepPreviewLoading:
		content = m.viewPreviewLoading()
	case SendStepReview:
		content = m.viewReview()
	case SendStepBroadcasting:
		content = m.viewBroadcasting()
	case SendStepComplete:
		content = m.viewComplete()
	}

	// Show error if present
	if m.err != nil {
		content += "\n\n" + styles.RenderErrorBox("⚠ "+m.err.Error(), m.width)
	}

	// Show PSBT export error if present
	if m.psbtExportErr != nil {
		content += "\n\n" + styles.RenderErrorBox("⚠ PSBT Export: "+m.psbtExportErr.Error(), m.width)
	}

	return content
}

func (m SendModel) viewAddressInput() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := styles.SubtitleStyle.Render("Step 1/7: Enter recipient address")

	// Format balance with pastel orange (matching dashboard style)
	var balanceBox string
	if m.loading {
		balanceInfo := m.spinner.View() + " " + styles.DimText.Render("Loading balance...")
		balanceBox = styles.InfoBoxStyle.Render(balanceInfo)
	} else {
		btcBalance := m.balance.ToBTC()
		balanceInfo := styles.DimText.Render("Available: ") +
			styles.BalanceValueStyle.Render(fmt.Sprintf("%.8f BTC", btcBalance)) +
			styles.DimText.Render(fmt.Sprintf(" (%d sats)", m.balance))
		balanceBox = styles.InfoBoxStyle.Render(balanceInfo)
	}

	instruction := styles.NormalText.Render("Enter the recipient's Bitcoin address:")

	// Hint about import option
	importHint := styles.DimText.Render("Or press 'i' to import an existing PSBT")

	help := styles.RenderHelp(
		styles.HK("enter", "continue"),
		styles.HK("i", "import PSBT"),
		styles.HK("ctrl+v", "paste"),
		styles.HK("esc", "cancel"),
	)

	return header + "\n" + subtitle + "\n\n" +
		balanceBox + "\n\n" +
		instruction + "\n" +
		importHint + "\n\n" +
		m.addressInput.View() + "\n\n" +
		help
}

func (m SendModel) viewAmountInput() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := styles.SubtitleStyle.Render("Step 2/7: Enter amount")

	// Format address with pastel colors (first/last 6 chars highlighted)
	formattedAddr := ui.FormatBitcoinAddressColored(m.recipientAddress, 6, 6)

	// Keep recipient details left-aligned and size the box to content.
	addrContent := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.DimText.Render("To:"),
		formattedAddr,
	)

	desiredWidth := lipgloss.Width(formattedAddr) + 8 // border/padding + breathing room
	addrBoxWidth := styles.ClampWidth(desiredWidth, m.width)
	if addrBoxWidth < 40 {
		addrBoxWidth = 40
	}
	addrDisplay := styles.BoxStyle.Width(addrBoxWidth).Render(addrContent)

	unitLabel := "BTC"
	if !m.usesBTC {
		unitLabel = "satoshis"
	}

	instruction := styles.NormalText.Render(fmt.Sprintf("Enter amount in %s:", unitLabel))
	toggleHint := styles.DimText.Render("(Press TAB to toggle BTC/satoshis)")

	help := styles.RenderHelp(
		styles.HK("enter", "continue"),
		styles.HK("m", "send all"),
		styles.HK("tab", "unit"),
		styles.HK("esc", "back"),
	)

	return header + "\n" + subtitle + "\n\n" +
		addrDisplay + "\n\n" +
		instruction + "\n" +
		toggleHint + "\n\n" +
		m.amountInput.View() + "\n\n" +
		help
}

func (m SendModel) viewRecipientList() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := styles.SubtitleStyle.Render("Step 3/7: Review Recipients")

	// Calculate totals
	var totalAmount api.Satoshi
	for _, r := range m.recipients {
		totalAmount += r.Amount
	}

	// Summary info
	summaryInfo := fmt.Sprintf("Recipients: %d • Total: %d sats (%.8f BTC)",
		len(m.recipients), totalAmount, totalAmount.ToBTC())
	summaryBox := styles.InfoBoxStyle.Render(summaryInfo)

	// List recipients
	recipientList := "\n"
	for i, r := range m.recipients {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}

		// Format address (first 8 and last 8 chars)
		shortAddr := r.Address
		if len(shortAddr) > 20 {
			shortAddr = shortAddr[:8] + "..." + shortAddr[len(shortAddr)-8:]
		}

		recipientText := fmt.Sprintf("%s#%d: %s • %d sats",
			cursor, i+1, shortAddr, r.Amount)

		if i == m.cursor {
			recipientList += styles.SelectedItemStyle.Render(recipientText) + "\n"
		} else {
			recipientList += styles.UnselectedItemStyle.Render(recipientText) + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("a/+", "add recipient"),
		styles.HK("d", "delete"),
		styles.HK("enter", "continue"),
		styles.HK("esc", "edit last"),
	)

	return header + "\n" + subtitle + "\n\n" +
		summaryBox + "\n" +
		recipientList + "\n" +
		help
}

func (m SendModel) viewUTXOSelection() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := styles.SubtitleStyle.Render("Step 4/7: Select UTXOs")

	// Calculate total amount for all recipients
	var totalSending api.Satoshi
	for _, r := range m.recipients {
		totalSending += r.Amount
	}

	// Show amount being sent
	amountInfo := fmt.Sprintf("Sending: %s BTC (%d sats) to %d recipient%s",
		formatBTC(totalSending.ToBTC()), totalSending,
		len(m.recipients), pluralize(len(m.recipients)))
	if m.sendAll {
		amountInfo = "Sending: MAX spend (final recipient amount derived after fee + preview)"
	}
	amountBox := styles.InfoBoxStyle.Render(amountInfo)

	// Calculate total from selected UTXOs
	var totalSelected api.Satoshi
	var selectedCount int
	for i, selected := range m.utxoSelected {
		if selected && i < len(m.utxos) {
			totalSelected += m.utxos[i].Value
			selectedCount++
		}
	}

	selectionInfo := fmt.Sprintf("Selected: %d UTXO%s • %d sats",
		selectedCount, pluralize(selectedCount), totalSelected)
	if m.sendAll {
		selectionInfo = selectionInfo + " • MAX spend locks all inputs"
	}

	// List UTXOs
	utxoList := "\n"
	for i, utxo := range m.utxos {
		cursor := "  "
		checkbox := "[ ]"

		if i == m.cursor {
			cursor = "▶ "
		}

		if i < len(m.utxoSelected) && m.utxoSelected[i] {
			checkbox = "[✓]"
		}

		// Short txid
		shortTxid := utxo.TxID
		if len(shortTxid) > 16 {
			shortTxid = shortTxid[:8] + "..." + shortTxid[len(shortTxid)-8:]
		}

		status := "✓"
		statusDesc := "confirmed"
		if !utxo.Status.Confirmed {
			status = "○"
			statusDesc = "unconfirmed"
		}

		utxoText := fmt.Sprintf("%s%s %s %s:%d • %s • %d sats",
			cursor, checkbox, status, shortTxid, utxo.Vout, statusDesc, utxo.Value)

		if i == m.cursor {
			utxoList += styles.SelectedItemStyle.Render(utxoText) + "\n"
		} else {
			utxoList += styles.UnselectedItemStyle.Render(utxoText) + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("space/enter", "toggle"),
		styles.HK("a", "all"),
		styles.HK("n", "none"),
		styles.HK("tab", "continue"),
		styles.HK("esc", "back"),
	)
	if m.sendAll {
		help = styles.RenderHelp(
			styles.HK("↑/↓", "navigate"),
			styles.HK("tab", "continue"),
			styles.HK("esc", "back"),
		)
	}

	return header + "\n" + subtitle + "\n\n" +
		amountBox + "\n\n" +
		styles.DimText.Render(selectionInfo) + "\n" +
		utxoList + "\n" +
		help
}

func (m SendModel) viewFeeSelection() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := styles.SubtitleStyle.Render("Step 5/7: Choose fee priority")

	feeOptions := []struct {
		name         string
		level        FeeLevel
		desc         string
		timeEstimate string
	}{
		{"🚀 High Priority", FeeLevelHigh, "Next block", "~10 minutes"},
		{"⚡ Medium Priority", FeeLevelMedium, "Within 30 min", "~30 minutes"},
		{"🐢 Low Priority", FeeLevelLow, "Within 1 hour", "~60 minutes"},
	}

	menu := "\n"
	for i, opt := range feeOptions {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}

		var feeRate float64
		if m.feeEstimates != nil {
			switch opt.level {
			case FeeLevelHigh:
				feeRate = m.feeEstimates.FastestFee
			case FeeLevelMedium:
				feeRate = m.feeEstimates.HalfHourFee
			case FeeLevelLow:
				feeRate = m.feeEstimates.HourFee
			}
		}

		var optionText string
		if feeRate <= 0 {
			optionText = fmt.Sprintf("%s%s\n    %s • unavailable",
				cursor, opt.name, opt.desc)
		} else {
			estimatedFee := int64(float64(m.estimatedSize) * feeRate)
			optionText = fmt.Sprintf("%s%s\n    %s • %s sat/vB • ~%d sats total",
				cursor, opt.name, opt.desc, formatFeeRate(feeRate), estimatedFee)
		}

		if i == m.cursor {
			menu += styles.SelectedItemStyle.Render(optionText) + "\n\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(optionText) + "\n\n"
		}
	}

	// Custom fee option
	customCursor := "  "
	if m.cursor == 3 {
		customCursor = "▶ "
	}

	var customOptionText string
	if m.highFeePending > 0 {
		estimatedFee := int64(math.Ceil(float64(m.estimatedSize) * m.highFeePending))
		customOptionText = fmt.Sprintf("%s✏️  Custom Fee\n    %s sat/vB is unusually high (~%d sats).\n    Type %q to confirm: %s",
			customCursor, formatFeeRate(m.highFeePending), estimatedFee, highFeeConfirmWord, m.highFeeInput.View())
	} else if m.editingCustom {
		// Show input field when editing
		customOptionText = fmt.Sprintf("%s✏️  Custom Fee\n    Enter rate: %s sat/vB",
			customCursor, m.customFeeInput.View())
	} else if m.customFeeRate > 0 {
		// Show set custom rate (format nicely for fractional rates)
		estimatedFee := int64(math.Ceil(float64(m.estimatedSize) * m.customFeeRate))
		rateStr := formatFeeRate(m.customFeeRate)
		customOptionText = fmt.Sprintf("%s✏️  Custom Fee\n    %s sat/vB • ~%d sats total",
			customCursor, rateStr, estimatedFee)
	} else {
		// Show placeholder
		customOptionText = fmt.Sprintf("%s✏️  Custom Fee\n    Press 'c' to set custom rate",
			customCursor)
	}

	if m.cursor == 3 {
		menu += styles.SelectedItemStyle.Render(customOptionText) + "\n\n"
	} else {
		menu += styles.UnselectedItemStyle.Render(customOptionText) + "\n\n"
	}

	var help string
	if m.highFeePending > 0 {
		help = styles.RenderHelp(
			styles.HK("enter", "confirm high fee"),
			styles.HK("esc", "edit rate"),
		)
	} else if m.editingCustom {
		help = styles.RenderHelp(
			styles.HK("enter", "confirm"),
			styles.HK("esc", "cancel"),
		)
	} else {
		help = styles.RenderHelp(
			styles.HK("↑/↓", "navigate"),
			styles.HK("c", "custom"),
			styles.HK("enter", "continue"),
			styles.HK("esc", "back"),
		)
	}

	return header + "\n" + subtitle + "\n" + menu + "\n" + help
}

func (m SendModel) viewPreviewLoading() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")

	progress := m.spinner.View() + " " + styles.NormalText.Render("Selecting UTXOs and calculating fees...")

	return header + "\n\n" + progress
}

func (m SendModel) reviewTxVSize(numInputs, numOutputs int) int {
	if m.psbtResult != nil && m.psbtResult.VSize > 0 {
		return m.psbtResult.VSize
	}
	if m.walletManager != nil {
		return m.estimateTxVSize(numInputs, numOutputs)
	}
	return 0
}

func (m SendModel) buildReviewWarnings(totalRecipientAmount, displayFee api.Satoshi, txVSize int) []string {
	var warnings []string

	unconfirmedInputs := 0
	for _, utxo := range m.psbtResult.SelectedUTXOs {
		if !utxo.Status.Confirmed {
			unconfirmedInputs++
		}
	}
	if unconfirmedInputs > 0 {
		warnings = append(warnings,
			fmt.Sprintf("%d selected input%s %s still unconfirmed. Replacement, delay, or rejection risk is higher until confirmation.",
				unconfirmedInputs,
				pluralize(unconfirmedInputs),
				pluralVerb(unconfirmedInputs, "is", "are")))
	}

	if !m.sendAll && len(m.recipients) > 0 && !m.psbtResult.HasChange && m.psbtResult.DustAbsorbed == 0 {
		warnings = append(warnings,
			"No change output is being created. Review every output carefully because this spend does not return coins back to the wallet.")
	}

	if totalRecipientAmount > 0 && displayFee*10 >= totalRecipientAmount {
		feePct := float64(displayFee) / float64(totalRecipientAmount) * 100
		warnings = append(warnings,
			fmt.Sprintf("Network fee is consuming %.1f%% of the amount being sent. That is unusually high for a normal spend.",
				feePct))
	}

	if txVSize > 0 {
		effectiveFeeRate := float64(displayFee) / float64(txVSize)
		selectedFeeRate := m.getFeeRate()
		if selectedFeeRate > 0 && effectiveFeeRate > selectedFeeRate*1.25 {
			warnings = append(warnings,
				fmt.Sprintf("Effective fee rate is %s sat/vB, above the selected %s sat/vB. Review dust absorption or output shape before signing.",
					formatFeeRate(effectiveFeeRate),
					formatFeeRate(selectedFeeRate)))
		}
	}

	return warnings
}

func (m SendModel) buildReviewScrollable() (string, string, string) {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")
	subtitle := styles.SubtitleStyle.Render("Step 6/7: Review transaction")

	// Responsive box width for all review sections
	boxWidth := m.width - 4
	if boxWidth > 70 {
		boxWidth = 70
	}
	if boxWidth < 40 {
		boxWidth = 40
	}

	// Guard against nil psbtResult (shouldn't happen but defensive)
	if m.psbtResult == nil {
		fixedHeader := header + "\n" + subtitle
		scrollContent := styles.DimText.Render("Loading transaction preview...")
		help := styles.RenderHelp(styles.HK("esc", "back"))
		return fixedHeader, scrollContent, help
	}

	// Use actual fee if we have preview data, otherwise use estimate
	displayFee := m.estimatedFee
	if m.psbtResult.Fee > 0 {
		displayFee = m.psbtResult.Fee
	}

	// === INPUTS SECTION ===
	var inputsSection string
	if len(m.psbtResult.SelectedUTXOs) > 0 {
		inputRows := []string{
			styles.HighlightText.Render("Inputs:"),
		}

		for i, utxo := range m.psbtResult.SelectedUTXOs {
			// Show first 8 and last 8 chars of txid
			shortTxid := utxo.TxID
			if len(shortTxid) > 16 {
				shortTxid = shortTxid[:8] + "..." + shortTxid[len(shortTxid)-8:]
			}

			addr := utxo.Address

			// Compact one-line address formatting to avoid hard wraps in review boxes.
			formattedAddr := ""
			if addr != "" {
				formattedAddr = ui.FormatBitcoinAddressCompact(addr, 8, 8)
			}

			// Combine input info on fewer lines with aligned amounts
			if formattedAddr != "" {
				inputRows = append(inputRows,
					fmt.Sprintf("  %s %s", styles.DimText.Render(fmt.Sprintf("Input #%d:", i+1)), formattedAddr),
					fmt.Sprintf("    %s", styles.DimText.Render(fmt.Sprintf("%s:%d", shortTxid, utxo.Vout))),
					renderRow("  Amount:", formatBTCWithSats(utxo.Value)),
				)
			}

			// Limit display to first 3 UTXOs to save space
			if i >= 2 && len(m.psbtResult.SelectedUTXOs) > 3 {
				inputRows = append(inputRows, fmt.Sprintf("  ... and %d more input%s", len(m.psbtResult.SelectedUTXOs)-3, pluralize(len(m.psbtResult.SelectedUTXOs)-3)))
				break
			}
		}

		inputRows = append(inputRows, renderRow("Total Input:", formatBTCWithSats(m.psbtResult.TotalInput())))

		inputsSection = styles.InfoBoxStyle.Width(boxWidth).Render(strings.Join(inputRows, "\n"))
	}

	// === OUTPUTS SECTION ===
	outputRows := []string{
		styles.HighlightText.Render("Outputs:"),
	}

	// Calculate total amount sent to recipients
	var totalRecipientAmount api.Satoshi

	// Show all recipients
	for i, r := range m.recipients {
		formattedRecipient := ui.FormatBitcoinAddressCompact(r.Address, 8, 8)
		outputRows = append(outputRows,
			"  "+styles.DimText.Render(fmt.Sprintf("Output #%d (Recipient):", i+1))+" "+formattedRecipient,
			renderRow("  Amount:", formatBTCWithSats(r.Amount)),
		)
		totalRecipientAmount += r.Amount

		// Limit display to first 3 recipients to save space
		if i >= 2 && len(m.recipients) > 3 {
			remaining := len(m.recipients) - 3
			var remainingAmount api.Satoshi
			for j := 3; j < len(m.recipients); j++ {
				remainingAmount += m.recipients[j].Amount
			}
			outputRows = append(outputRows,
				fmt.Sprintf("  ... and %d more recipient%s (%d sats)", remaining, pluralize(remaining), remainingAmount))
			for j := 3; j < len(m.recipients); j++ {
				totalRecipientAmount += m.recipients[j].Amount
			}
			break
		}
	}

	// Change output (if applicable)
	if m.psbtResult.HasChange {
		formattedChange := ui.FormatBitcoinAddressCompact(m.psbtResult.ChangeAddress, 8, 8)
		// Add extra spaces to align "Change):" to match length of "Recipient):"
		// "Recipient" = 9 chars, "Change" = 6 chars, difference = 3 spaces needed
		outputRows = append(outputRows,
			"  "+styles.DimText.Render("Output #2 (Change):   ")+" "+formattedChange,
			renderRow("  Amount:", formatBTCWithSats(m.psbtResult.Change)),
		)
	}

	outputRows = append(outputRows, renderRow("Total Output:", formatBTCWithSats(totalRecipientAmount+m.psbtResult.Change)))

	outputsSection := styles.InfoBoxStyle.Width(boxWidth).Render(strings.Join(outputRows, "\n"))

	// === FEE & SIZE BREAKDOWN ===
	// Calculate actual transaction size breakdown
	numInputs := len(m.psbtResult.SelectedUTXOs)
	numOutputs := 1 // recipient
	if m.psbtResult.HasChange {
		numOutputs = 2 // recipient + change
	}
	txVSize := m.reviewTxVSize(numInputs, numOutputs)

	sizeSummary := "unknown"
	if txVSize > 0 {
		sizeSummary = fmt.Sprintf("%d vB (%d input%s, %d output%s)",
			txVSize, numInputs, pluralize(numInputs), numOutputs, pluralize(numOutputs))
	}

	feeRows := []string{
		styles.HighlightText.Render("Fee & Size Breakdown:"),
		renderRow("Tx Size:", sizeSummary),
		renderRow("Fee Rate:", fmt.Sprintf("%s sat/vB", formatFeeRate(m.getFeeRate()))),
		renderRow("Network Fee:", formatBTCWithSats(displayFee)),
		renderRow("Total Cost:", formatBTCWithSats(totalRecipientAmount+displayFee)),
	}

	feeSection := styles.BoxStyle.Width(boxWidth).Render(strings.Join(feeRows, "\n"))

	reviewWarnings := m.buildReviewWarnings(totalRecipientAmount, displayFee, txVSize)
	warningSection := ""
	if len(reviewWarnings) > 0 {
		warningLines := []string{"⚠ Safety Checks", ""}
		for _, warning := range reviewWarnings {
			warningLines = append(warningLines, "• "+warning)
		}
		warningSection = styles.WarningBoxStyle.Width(boxWidth).Render(strings.Join(warningLines, "\n"))
	}

	// Dust warning - show if dust was absorbed into fee
	dustWarning := ""
	if m.psbtResult.DustAbsorbed > 0 {
		// Calculate how much the fee increased due to dust absorption
		feeIncrease := displayFee - m.psbtResult.IntendedFee
		dustWarning = styles.WarningBoxStyle.Render(
			fmt.Sprintf("⚠ Dust absorbed into fee\n\n"+
				"Change of %d sats is below dust limit and cannot be\n"+
				"created as an output. It has been added to the fee.\n\n"+
				"Intended fee: %d sats → Actual fee: %d sats (+%d sats)",
				m.psbtResult.DustAbsorbed, m.psbtResult.IntendedFee, displayFee, feeIncrease))
	}

	// Show error if broadcast failed
	var errorSection string
	if m.broadcastErr != nil {
		errorSection = styles.RenderErrorBox(
			fmt.Sprintf("❌ Transaction failed:\n\n%v", m.broadcastErr), m.width)
	}

	// Unified help - all wallet types go to Sign & Broadcast
	help := styles.RenderHelp(
		styles.HK("enter", "sign & broadcast"),
		styles.HK("esc", "back"),
	)

	// Split into fixed header and scrollable content
	fixedHeader := header + "\n" + subtitle

	var sections []string
	if inputsSection != "" {
		sections = append(sections, inputsSection)
	}
	sections = append(sections, outputsSection, feeSection)
	if warningSection != "" {
		sections = append(sections, warningSection)
	}
	if dustWarning != "" {
		sections = append(sections, dustWarning)
	}
	if errorSection != "" {
		sections = append(sections, errorSection)
	}
	scrollContent := strings.Join(sections, "\n")

	return fixedHeader, scrollContent, help
}

func (m SendModel) viewReview() string {
	fixedHeader, scrollContent, help := m.buildReviewScrollable()
	return m.renderScrollableViewport(fixedHeader, scrollContent, help)
}

func (m SendModel) viewBroadcasting() string {
	header := styles.TitleStyle.Render("📤 Send Bitcoin")

	progress := m.spinner.View() + " " + styles.NormalText.Render("Broadcasting transaction...")

	return header + "\n\n" + progress
}

func (m SendModel) viewComplete() string {
	if m.txID == "" {
		// Error state
		header := styles.TitleStyle.Render("❌ Transaction Failed")
		errorBox := styles.RenderErrorBox(
			fmt.Sprintf("Failed to broadcast transaction:\n\n%v", m.broadcastErr), m.width)

		help := styles.RenderHelp(styles.HK("enter", "back to dashboard"))

		return header + "\n\n" + errorBox + "\n\n" + help
	}

	// Success state
	header := styles.TitleStyle.Render("✅ Transaction Sent!")

	// Calculate totals - prefer recipients if available, else extract from PSBT
	var totalAmount api.Satoshi
	var addrSection string
	var numRecipients int

	if len(m.recipients) > 0 {
		// Normal flow: use recipients
		for _, r := range m.recipients {
			totalAmount += r.Amount
		}
		numRecipients = len(m.recipients)

		if numRecipients == 1 {
			formattedAddr := ui.FormatBitcoinAddressColored(m.recipients[0].Address, 6, 6)
			addrSection = lipgloss.JoinVertical(
				lipgloss.Center,
				styles.DimText.Render("To:"),
				formattedAddr,
			)
		} else {
			addrSection = styles.DimText.Render(fmt.Sprintf("To: %d recipients", numRecipients))
		}
	} else if m.psbtResult != nil && m.psbtResult.Packet != nil {
		// Imported PSBT: extract from PSBT outputs
		tx := m.psbtResult.Packet.UnsignedTx
		if tx != nil {
			// Sum outputs (excluding change if we can identify it)
			for _, txOut := range tx.TxOut {
				totalAmount += api.Satoshi(txOut.Value)
			}
			numRecipients = len(tx.TxOut)
			addrSection = styles.DimText.Render(fmt.Sprintf("To: %d output(s)", numRecipients))
		}
	} else {
		addrSection = styles.DimText.Render("To: imported transaction")
	}

	summaryRows := []string{
		renderRow("Amount:", fmt.Sprintf("%s BTC", formatBTC(totalAmount.ToBTC()))),
		"",
		addrSection,
		"",
		renderRow("Fee:", fmt.Sprintf("%d sats", m.estimatedFee)),
		"",
		renderRow("Transaction ID:", ""),
		styles.HighlightText.Render(m.txID),
	}

	completeBoxWidth := m.width - 4
	if completeBoxWidth > 70 {
		completeBoxWidth = 70
	}
	if completeBoxWidth < 40 {
		completeBoxWidth = 40
	}
	successBox := styles.SuccessBoxStyle.Width(completeBoxWidth).Render(strings.Join(summaryRows, "\n"))

	// Show "copied" indicator if transaction ID was just copied
	copiedIndicator := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copiedIndicator = "\n" + styles.SuccessBoxStyle.Render("✓ Copied to clipboard!")
	}

	info := styles.InfoBoxStyle.Width(completeBoxWidth).Render(
		"🔍 Track your transaction:\n\n" +
			wallet.ExplorerTxURL(walletNetwork(m.walletManager), m.txID))

	help := styles.RenderHelp(
		styles.HK("c", "copy txid"),
		styles.HK("enter", "open in browser"),
		styles.HK("n", "new transaction"),
		styles.HK("q/esc", "back to dashboard"),
	)

	return header + "\n\n" +
		successBox + "\n\n" +
		info +
		copiedIndicator + "\n\n" +
		help
}

func (m SendModel) buildPSBTExportScrollable() (string, string, string) {
	// Guard against nil wallet manager in tests or incomplete state.
	if m.walletManager == nil {
		header := styles.TitleStyle.Render("🔐 Sign & Broadcast")
		subtitle := styles.SubtitleStyle.Render("Step 7/7: Sign to complete transaction")
		help := styles.RenderHelp(styles.HK("esc", "back"))
		return header + "\n" + subtitle, styles.DimText.Render("Loading..."), help
	}

	// Determine header based on signing status
	var headerText string
	if m.psbtSigsAdded >= m.psbtSigsTotal {
		headerText = "✓ Sign & Broadcast"
	} else {
		headerText = "🔐 Sign & Broadcast"
	}
	header := styles.TitleStyle.Render(headerText)

	// Determine status based on signatures
	var subtitleText string
	var statusText string
	var statusStyle lipgloss.Style

	if m.psbtSigsAdded >= m.psbtSigsTotal {
		subtitleText = "Step 7/7: Ready to broadcast"
		statusText = fmt.Sprintf("✓ %d of %d signatures", m.psbtSigsAdded, m.psbtSigsTotal)
		statusStyle = styles.SignatureCompleteStyle
	} else if m.psbtSigsAdded > 0 {
		subtitleText = "Step 7/7: Partially signed - need more signatures"
		statusText = fmt.Sprintf("%d of %d signatures (need %d more)", m.psbtSigsAdded, m.psbtSigsTotal, m.psbtSigsTotal-m.psbtSigsAdded)
		statusStyle = styles.SignaturePartialStyle
	} else {
		subtitleText = "Step 7/7: Sign to complete transaction"
		statusText = fmt.Sprintf("0 of %d signatures", m.psbtSigsTotal)
		statusStyle = styles.DimText
	}
	subtitle := styles.SubtitleStyle.Render(subtitleText)

	// Responsive box width for PSBT display
	psbtBoxWidth := m.width - 4
	if psbtBoxWidth > 70 {
		psbtBoxWidth = 70
	}
	if psbtBoxWidth < 40 {
		psbtBoxWidth = 40
	}
	// Wrap PSBT text to fit inside box (subtract border + padding overhead)
	wrapWidth := psbtBoxWidth - 6
	if wrapWidth < 20 {
		wrapWidth = 20
	}
	psbtWrapped := ""
	for i := 0; i < len(m.psbtBase64); i += wrapWidth {
		end := i + wrapWidth
		if end > len(m.psbtBase64) {
			end = len(m.psbtBase64)
		}
		psbtWrapped += m.psbtBase64[i:end] + "\n"
	}

	// Trim trailing newline
	psbtWrapped = strings.TrimSuffix(psbtWrapped, "\n")

	// Show PSBT in a box with monospace styling
	psbtBox := styles.BoxStyle.Width(psbtBoxWidth).Render(psbtWrapped)

	// Info section with signature progress
	progressWidth := psbtBoxWidth - 30
	if progressWidth < 20 {
		progressWidth = 20
	}
	filledWidth := 0
	if m.psbtSigsTotal > 0 {
		filledWidth = (m.psbtSigsAdded * progressWidth) / m.psbtSigsTotal
	}
	if filledWidth > progressWidth {
		filledWidth = progressWidth
	}
	progressFilled := strings.Repeat("█", filledWidth)
	progressEmpty := strings.Repeat("░", progressWidth-filledWidth)
	progressBar := statusStyle.Render(progressFilled) + styles.DimText.Render(progressEmpty)

	infoRows := []string{
		styles.HighlightText.Render("Signatures:"),
		"  [" + progressBar + "]",
		"  " + statusStyle.Render(statusText),
	}

	// Show cosigner signing status for multisig
	if m.walletManager.IsMultisig() && m.psbtResult != nil {
		cosignerStatus := m.walletManager.GetCosignerSigningStatus(m.psbtResult)
		if len(cosignerStatus) > 0 {
			infoRows = append(infoRows, "")
			infoRows = append(infoRows, styles.HighlightText.Render("Cosigners:"))
			for _, cs := range cosignerStatus {
				name := cs.Name
				if name == "" {
					name = "Cosigner"
				}
				// Truncate fingerprint for display
				fp := cs.Fingerprint
				if len(fp) > 8 {
					fp = fp[:8]
				}
				if fp == "" || fp == "00000000" {
					fp = "unknown"
				}

				var statusIcon string
				var statusColor lipgloss.Style
				if cs.HasSigned {
					statusIcon = "✓"
					statusColor = styles.CosignerSignedStyle
				} else if cs.CanSign {
					statusIcon = "○"
					statusColor = styles.CosignerPendingStyle
				} else {
					statusIcon = "·"
					statusColor = styles.DimText
				}

				line := fmt.Sprintf("  %s %s [%s]",
					statusColor.Render(statusIcon),
					name,
					styles.DimText.Render(fp))
				if cs.HasSigned {
					line += statusColor.Render(" signed")
				} else if cs.CanSign {
					line += statusColor.Render(" can sign")
				} else {
					line += styles.DimText.Render(" external")
				}
				infoRows = append(infoRows, line)
			}
		}
	}

	infoRows = append(infoRows, "")
	infoRows = append(infoRows, renderRow("Format:", "Base64 (BIP174)"))
	infoRows = append(infoRows, renderRow("Length:", fmt.Sprintf("%d characters", len(m.psbtBase64))))

	infoSection := styles.InfoBoxStyle.Render(strings.Join(infoRows, "\n"))

	// Usage hints based on wallet type
	var usageHints string
	if m.walletManager.IsMultisig() {
		usageHints = styles.DimText.Render(
			"Multisig signing options:\n" +
				"  • Sign with local keys (s) if available\n" +
				"  • Sign with hardware wallet (h) via HWI\n" +
				"  • Show QR code (q) for air-gapped signing (Passport, Keystone, Coldcard Q)\n" +
				"  • Export to file (f) for SD card signing\n" +
				"  • Import signed PSBT (i) from cosigners or paste UR (u)")
	} else if m.walletManager.IsHardwareWallet() {
		usageHints = styles.DimText.Render(
			"Hardware wallet signing:\n" +
				"  • Sign directly via HWI (h) if device is connected\n" +
				"  • Show QR code (q) for air-gapped signing (Passport, Keystone, Coldcard Q)\n" +
				"  • Save to file (f) for SD card signing\n" +
				"  • Import signed PSBT (i) or paste UR (u) after signing")
	} else {
		usageHints = styles.DimText.Render(
			"Signing options:\n" +
				"  • Sign with local keys (s) - recommended\n" +
				"  • Show QR code (q) for air-gapped hardware wallet signing\n" +
				"  • Or use HWI (h) / file export (f)\n" +
				"  • Then broadcast (b) when signed")
	}

	// Show status indicator based on last action
	statusIndicator := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		switch m.lastAction {
		case "saved":
			statusIndicator = styles.SuccessBoxStyle.Render("✓ Saved to: " + m.psbtFilePath)
		case "imported":
			statusIndicator = styles.SuccessBoxStyle.Render("✓ Imported from: " + m.psbtFilePath)
		case "signed":
			statusIndicator = styles.SuccessBoxStyle.Render("✓ Signed!")
		case "copied":
			statusIndicator = styles.SuccessBoxStyle.Render("✓ Copied to clipboard!")
		default:
			statusIndicator = styles.SuccessBoxStyle.Render("✓ Done!")
		}
	}
	if m.psbtExportErr != nil {
		statusIndicator = styles.RenderErrorBox("⚠ "+m.psbtExportErr.Error(), m.width)
	}
	// Also show HWI errors (from enumeration failures)
	if m.hwiError != nil {
		statusIndicator = styles.RenderErrorBox("⚠ "+m.hwiError.Error(), m.width)
	}

	// Build help based on state
	items := []styles.HelpItem{}

	canSignLocally := false
	if m.walletManager.IsMultisig() {
		canSignLocally = m.walletManager.CanSignMultisigLocally()
	} else if !m.walletManager.IsHardwareWallet() {
		canSignLocally = true
	}
	if canSignLocally && m.psbtSigsAdded < m.psbtSigsTotal {
		items = append(items, styles.HK("s", "sign"))
	}

	items = append(items,
		styles.HK("c", "copy"),
		styles.HK("h", "HW sign"),
		styles.HK("q", "QR code"),
		styles.HK("f", "save file"),
		styles.HK("i", "import"),
		styles.HK("u", "paste UR"),
	)
	if m.psbtSigsAdded >= m.psbtSigsTotal {
		items = append(items, styles.HK("b", "broadcast"))
	}

	items = append(items, styles.HK("esc", "back"))

	help := styles.RenderHelp(items...)

	// Split into fixed header and scrollable content
	fixedHeader := header + "\n" + subtitle

	contentSections := []string{
		psbtBox,
		infoSection,
		usageHints,
	}
	if statusIndicator != "" {
		contentSections = append(contentSections, statusIndicator)
	}
	scrollContent := strings.Join(contentSections, "\n")

	return fixedHeader, scrollContent, help
}

func (m SendModel) viewPSBTExport() string {
	// Handle file input overlay
	if m.showFileInput {
		return m.viewFileInput()
	}

	// Handle HWI device selection overlay
	if m.showHWIDevices {
		return m.viewHWIDeviceSelection()
	}
	fixedHeader, scrollContent, help := m.buildPSBTExportScrollable()
	return m.renderScrollableViewport(fixedHeader, scrollContent, help)
}

func (m SendModel) viewQRDisplay() string {
	header := styles.TitleStyle.Render("📱 QR Code — PSBT")

	var formatLabel string
	switch m.qrFormat {
	case "ur":
		formatLabel = "BC-UR (Passport, Keystone)"
	case "bbqr":
		formatLabel = "BBQr (Coldcard Q)"
	}
	subtitle := styles.SubtitleStyle.Render("Format: " + formatLabel)

	// Frame counter for animated QR
	var frameInfo string
	if len(m.qrFrames) > 1 {
		frameInfo = styles.HighlightText.Render(
			fmt.Sprintf("Frame %d of %d", m.qrFrameIndex+1, len(m.qrFrames)))
	} else {
		frameInfo = styles.DimText.Render("Single frame")
	}

	// QR code with width warning
	qrCode := m.qrRendered
	if qrCode == "" {
		qrCode = styles.DimText.Render("(generating...)")
	}

	// Check if QR is wider than terminal
	var widthWarning string
	if m.qrRendered != "" && m.width > 0 {
		qrLines := strings.Split(m.qrRendered, "\n")
		if len(qrLines) > 0 {
			qrWidth := len([]rune(qrLines[0]))
			if qrWidth > m.width {
				if m.qrFormat == "ur" {
					widthWarning = "\n" + styles.ErrorText.Render(
						fmt.Sprintf("QR code is %d cols wide (terminal: %d) — try BBQr (2) for a smaller code", qrWidth, m.width))
				} else {
					widthWarning = "\n" + styles.ErrorText.Render(
						fmt.Sprintf("QR code is %d cols wide (terminal: %d) — try widening your terminal", qrWidth, m.width))
				}
			}
		}
	}

	// Data size info
	var sizeInfo string
	if len(m.qrFrames) > 0 {
		sizeInfo = styles.DimText.Render(
			fmt.Sprintf("%d chars per frame", len(m.qrFrames[m.qrFrameIndex])))
	}

	help := styles.RenderHelp(
		styles.HK("1", "BC-UR"),
		styles.HK("2", "BBQr"),
		styles.HK("esc", "back"),
	)

	return header + "\n" + subtitle + "\n\n" +
		qrCode +
		widthWarning + "\n" +
		frameInfo + "  " + sizeInfo + "\n\n" +
		help
}

func (m SendModel) viewQRFormatMenu() string {
	header := styles.TitleStyle.Render("📱 Show PSBT as QR Code")
	subtitle := styles.SubtitleStyle.Render("Select format for your hardware wallet")

	menu := strings.Join([]string{
		"",
		styles.HighlightText.Render("  1 ") + styles.NormalText.Render("BC-UR") +
			styles.DimText.Render("   — Foundation Passport, Keystone"),
		"",
		styles.HighlightText.Render("  2 ") + styles.NormalText.Render("BBQr") +
			styles.DimText.Render("    — Coldcard Q"),
		"",
	}, "\n")

	help := styles.RenderHelp(
		styles.HK("1/2", "select"),
		styles.HK("esc", "cancel"),
	)

	return header + "\n" + subtitle + "\n" +
		menu + "\n\n" +
		help
}

func (m SendModel) viewURInput() string {
	header := styles.TitleStyle.Render("📥 Import Signed PSBT via UR")
	subtitle := styles.SubtitleStyle.Render("Paste the UR string from your hardware wallet")

	hint := styles.DimText.Render("(Paste the ur:crypto-psbt/... string from your device)")

	help := styles.RenderHelp(
		styles.HK("enter", "import"),
		styles.HK("esc", "cancel"),
	)

	return header + "\n" + subtitle + "\n\n" +
		hint + "\n\n" +
		m.urInput.View() + "\n\n" +
		help
}

func (m SendModel) viewFileInput() string {
	var header, subtitle, instruction string

	if m.fileInputMode == "export" {
		header = styles.TitleStyle.Render("💾 Save PSBT to File")
		subtitle = styles.SubtitleStyle.Render("For hardware wallet signing (Coldcard, etc.)")
		instruction = styles.NormalText.Render("Enter file path to save the PSBT:")
	} else {
		header = styles.TitleStyle.Render("📂 Import Signed PSBT")
		subtitle = styles.SubtitleStyle.Render("Load PSBT signed by hardware wallet")
		instruction = styles.NormalText.Render("Enter path to the signed PSBT file:")
	}

	// Show current path
	pathHint := styles.DimText.Render("(Use ~ for home directory, e.g., ~/Desktop/tx.psbt)")

	// Show loading indicator if processing
	var statusLine string
	if m.loading {
		statusLine = "\n" + m.spinner.View() + " Processing..."
	}

	help := styles.RenderHelp(
		styles.HK("enter", "confirm"),
		styles.HK("esc", "cancel"),
	)

	return header + "\n" + subtitle + "\n\n" +
		instruction + "\n" +
		pathHint + "\n\n" +
		m.psbtFileInput.View() +
		statusLine + "\n\n" +
		help
}

func (m SendModel) viewFileBrowser() string {
	header := styles.TitleStyle.Render("📂 Import PSBT")
	subtitle := styles.SubtitleStyle.Render("Select a PSBT file to import")

	var content strings.Builder

	if len(m.psbtFiles) == 0 {
		content.WriteString(styles.DimText.Render("No .psbt files found in ~/.hax or ~/Desktop\n\n"))
		content.WriteString(styles.NormalText.Render("Press 'p' to enter a file path manually.\n"))
	} else {
		content.WriteString(styles.NormalText.Render("Found PSBT files (newest first):") + "\n\n")

		// Show up to 10 files
		maxShow := 10
		if len(m.psbtFiles) < maxShow {
			maxShow = len(m.psbtFiles)
		}

		for i := 0; i < maxShow; i++ {
			file := m.psbtFiles[i]
			// Get just the filename for display
			filename := filepath.Base(file)
			// Get parent directory name
			parentDir := filepath.Base(filepath.Dir(file))

			var line string
			if i == m.fileBrowserCursor {
				line = styles.SelectedItemStyle.Render(fmt.Sprintf("▶ %s (%s)", filename, parentDir))
			} else {
				line = styles.UnselectedItemStyle.Render(fmt.Sprintf("  %s (%s)", filename, parentDir))
			}
			content.WriteString(line + "\n")
		}

		if len(m.psbtFiles) > maxShow {
			content.WriteString(styles.DimText.Render(fmt.Sprintf("\n  ... and %d more files", len(m.psbtFiles)-maxShow)))
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("p", "manual path"),
		styles.HK("r", "refresh"),
		styles.HK("esc", "back"),
	)

	return header + "\n" + subtitle + "\n\n" +
		content.String() + "\n\n" +
		help
}

func (m SendModel) viewHWIDeviceSelection() string {
	header := styles.TitleStyle.Render("🔌 Hardware Wallet Signing")
	subtitle := styles.SubtitleStyle.Render("Select device to sign with (via HWI)")

	var content strings.Builder

	// Show loading indicator
	if m.loading {
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
			// Format device info with model and type
			deviceName := string(device.Type)
			if device.Model != "" {
				deviceName = device.Model
			}

			// Show fingerprint if available
			fp := device.Fingerprint
			if fp == "" || fp == "00000000" {
				fp = "unknown"
			} else if len(fp) > 8 {
				fp = fp[:8]
			}

			line := fmt.Sprintf("  %s [%s]", deviceName, fp)

			// Highlight selected device
			if i == m.selectedHWDevice {
				line = styles.HighlightText.Render("► " + line)
			} else {
				line = styles.DimText.Render("  " + line)
			}

			// Show status flags with warnings
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

		// Show PIN warning for selected device
		if m.selectedHWDevice >= 0 && m.selectedHWDevice < len(m.hwiDevices) {
			selected := m.hwiDevices[m.selectedHWDevice]
			if selected.NeedsPin {
				content.WriteString("\n" + styles.WarningText.Render(
					"⚠ Enter PIN on your device first, then press Enter to sign.") + "\n")
			}
		}
	}

	// Instructions
	instructions := styles.DimText.Render(
		"\nThe device will prompt you to confirm the transaction.\n" +
			"Review all details on your hardware wallet screen.")

	help := styles.RenderHelp(
		styles.HK("↑/↓", "select"),
		styles.HK("enter", "sign"),
		styles.HK("r", "retry"),
		styles.HK("esc", "back"),
	)

	return header + "\n" + subtitle + "\n\n" +
		content.String() +
		instructions + "\n\n" +
		help
}

func (m SendModel) viewRawHexExport() string {
	header := styles.TitleStyle.Render("📦 Raw Transaction Hex")
	subtitle := styles.SubtitleStyle.Render("Signed Transaction (ready to broadcast)")

	// Responsive box width for hex display
	hexBoxWidth := m.width - 4
	if hexBoxWidth > 70 {
		hexBoxWidth = 70
	}
	if hexBoxWidth < 40 {
		hexBoxWidth = 40
	}
	hexWrapWidth := hexBoxWidth - 6
	if hexWrapWidth < 20 {
		hexWrapWidth = 20
	}
	hexWrapped := ""
	for i := 0; i < len(m.rawHex); i += hexWrapWidth {
		end := i + hexWrapWidth
		if end > len(m.rawHex) {
			end = len(m.rawHex)
		}
		hexWrapped += m.rawHex[i:end] + "\n"
	}

	// Trim trailing newline
	hexWrapped = strings.TrimSuffix(hexWrapped, "\n")

	// Show hex in a box with monospace styling
	hexBox := styles.BoxStyle.Width(hexBoxWidth).Render(hexWrapped)

	// Info section
	infoRows := []string{
		styles.HighlightText.Render("Transaction Information:"),
		renderRow("Status:", "Signed (ready to broadcast)"),
		renderRow("Format:", "Raw hex (Bitcoin wire format)"),
		renderRow("Length:", fmt.Sprintf("%d characters (%d bytes)", len(m.rawHex), len(m.rawHex)/2)),
	}
	infoSection := styles.InfoBoxStyle.Render(strings.Join(infoRows, "\n"))

	// Usage hints
	usageHints := styles.DimText.Render(
		"This transaction can be:\n" +
			"  • Broadcast via Bitcoin Core: sendrawtransaction <hex>\n" +
			"  • Broadcast via Electrum server\n" +
			"  • Broadcast on mempool.space/tx/push")

	// Show "copied" indicator
	copiedIndicator := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copiedIndicator = "\n" + styles.SuccessBoxStyle.Render("✓ Copied to clipboard!")
	}

	help := styles.RenderHelp(
		styles.HK("c", "copy to clipboard"),
		styles.HK("esc/q", "close"),
	)

	return header + "\n" + subtitle + "\n\n" +
		hexBox + "\n\n" +
		infoSection + "\n\n" +
		usageHints +
		copiedIndicator + "\n\n" +
		help
}

func (m SendModel) getFeeRate() float64 {
	return m.getFeeRateForLevel(m.feeLevel)
}

// getFeeRateForLevel returns fee rate for a given level.
// Preset levels return 0 when estimates are missing or that target is
// unavailable — callers must not invent a rate. Custom rates are returned as-is.
func (m SendModel) getFeeRateForLevel(level FeeLevel) float64 {
	// Handle custom fee rate (supports sub-1 sat/vB)
	if level == FeeLevelCustom {
		if m.customFeeRate > 0 {
			return m.customFeeRate
		}
		// Fall back to medium if custom rate not set
		level = FeeLevelMedium
	}

	if m.feeEstimates == nil {
		return 0
	}

	var feeRate float64
	switch level {
	case FeeLevelHigh:
		feeRate = m.feeEstimates.FastestFee
	case FeeLevelMedium:
		feeRate = m.feeEstimates.HalfHourFee
	case FeeLevelLow:
		feeRate = m.feeEstimates.HourFee
	default:
		feeRate = m.feeEstimates.HalfHourFee
	}

	// Missing/unavailable target (left as 0 by the backend).
	if feeRate <= 0 {
		return 0
	}
	// Floor extremely small positive estimates at the common min-relay rate.
	if feeRate < 0.1 {
		return 0.1
	}
	return feeRate
}

func renderRow(label, value string) string {
	if label == "" {
		return "  " + styles.NormalText.Render(value)
	}
	return styles.DimText.Render(fmt.Sprintf("%-15s", label)) + " " +
		styles.HighlightText.Render(value)
}

func formatBTC(btc float64) string {
	return fmt.Sprintf("%.8f", btc)
}

// formatFeeRate formats a fee rate, showing decimals only if needed
// Examples: 1.0 -> "1", 0.5 -> "0.5", 1.5 -> "1.5"
func formatFeeRate(rate float64) string {
	if rate == float64(int64(rate)) {
		return fmt.Sprintf("%d", int64(rate))
	}
	// Remove trailing zeros but keep at least one decimal
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", rate), "0"), ".")
}

// formatSatsCompact formats satoshis in compact form with k suffix for thousands
// Examples: 1234 -> "1.2k", 50000 -> "50k", 999 -> "999"
func formatSatsCompact(sats api.Satoshi) string {
	if sats < 1000 {
		return fmt.Sprintf("%d", sats)
	}
	if sats < 10000 {
		// Show one decimal place for values 1k-10k
		return fmt.Sprintf("%.1fk", float64(sats)/1000.0)
	}
	// No decimal for 10k+
	return fmt.Sprintf("%.0fk", float64(sats)/1000.0)
}

// formatBTCWithSats formats amount as "0.00012345 BTC (12.3k sats)" with colored styling
// Amount: pastel blue, "BTC": pastel orange, sats: grey
func formatBTCWithSats(sats api.Satoshi) string {
	btc := sats.ToBTC()
	amountStr := styles.AmountStyle.Render(formatBTC(btc))
	btcLabel := styles.CurrencyUnitStyle.Render("BTC")
	satsStr := styles.DimText.Render(fmt.Sprintf("(%s sats)", formatSatsCompact(sats)))
	return fmt.Sprintf("%s %s %s", amountStr, btcLabel, satsStr)
}

// isViewportScrollActive returns true when Step 6/7 should use viewport scrolling.
func (m SendModel) isViewportScrollActive() bool {
	isScrollableScreen := m.step == SendStepReview || m.showPSBTExport
	return isScrollableScreen &&
		!m.showFileBrowser &&
		!m.showRawHexExport &&
		!m.showQRDisplay &&
		!m.showQRFormatMenu &&
		!m.showURInput &&
		!m.showFileInput &&
		!m.showHWIDevices
}

// syncScrollViewport updates Step 6/7 viewport content and size from model state.
func (m *SendModel) syncScrollViewport() {
	if !m.isViewportScrollActive() {
		return
	}
	if m.walletManager == nil {
		return
	}

	var header, content, footer string
	if m.showPSBTExport {
		header, content, footer = m.buildPSBTExportScrollable()
	} else {
		header, content, footer = m.buildReviewScrollable()
	}

	width := m.width
	if width < 1 {
		width = 1
	}
	availableHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if availableHeight < 1 {
		availableHeight = 1
	}

	m.scrollViewport.Width = width
	m.scrollViewport.Height = availableHeight
	m.scrollViewport.SetContent(content)
}

// renderScrollableViewport renders fixed header/footer with a smooth Bubble Tea viewport body.
func (m SendModel) renderScrollableViewport(header, content, footer string) string {
	if m.height <= 0 {
		return header + "\n" + content + "\n" + footer
	}

	view := m.scrollViewport.View()
	if view == "" {
		view = content
	}

	indicator := ""
	if m.scrollViewport.TotalLineCount() > m.scrollViewport.Height {
		scrollPct := int(m.scrollViewport.ScrollPercent() * 100)
		indicator = styles.ScrollIndicatorStyle.Render(fmt.Sprintf("  %d%%", scrollPct))
	}

	return header + indicator + "\n" + view + "\n" + footer
}

// validateBTCAmount validates Bitcoin amount input (max 8 decimal places)
func validateBTCAmount(s string) error {
	if s == "" {
		return nil // Empty is OK while typing
	}

	// Only allow numbers and one decimal point
	if !regexp.MustCompile(`^[0-9]*\.?[0-9]*$`).MatchString(s) {
		return fmt.Errorf("only numbers and decimal point allowed")
	}

	// Check decimal places (max 8 for Bitcoin)
	if strings.Contains(s, ".") {
		parts := strings.Split(s, ".")
		if len(parts) > 2 {
			return fmt.Errorf("only one decimal point allowed")
		}
		if len(parts[1]) > 8 {
			return fmt.Errorf("maximum 8 decimal places (satoshi precision)")
		}
	}

	return nil
}

// validateSatPerVB validates custom fee rate input (allows decimals for sub-1 sat/vB)
func validateSatPerVB(s string) error {
	if s == "" {
		return nil // Empty is OK while typing
	}

	// Allow integers and decimals (e.g., "1", "0.5", "0.1")
	if !regexp.MustCompile(`^[0-9]+\.?[0-9]*$`).MatchString(s) {
		return fmt.Errorf("invalid number format")
	}

	return nil
}

// pluralize returns "s" if count != 1, otherwise empty string
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func pluralVerb(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

// renderHWITroubleshootingHints formats troubleshooting hints as a bulleted list
func renderHWITroubleshootingHints(hints []string) string {
	if len(hints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.WarningText.Render("Troubleshooting:") + "\n")
	for _, hint := range hints {
		b.WriteString(styles.DimText.Render("  • "+hint) + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// hwiErrorHints extracts troubleshooting hints from an error, returning general
// device connection hints if the error is not an *wallet.HWIError
func hwiErrorHints(err error) []string {
	if hwiErr, ok := err.(*wallet.HWIError); ok {
		return hwiErr.TroubleshootingHints()
	}
	return wallet.DeviceConnectionHints()
}
