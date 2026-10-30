package models

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
	Label   string // from a BIP21 payment request; becomes the tx label
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
	recipients       []Recipient        // List of all recipients
	recipientAddress string             // Current recipient being edited
	pendingRequest   *wallet.PaymentURI // BIP21 request that filled the current recipient
	amountSats       api.Satoshi        // Current amount being edited
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
	utxos        []api.UTXO // spendable coins; frozen ones are left out
	frozenCount  int
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
	psbtBase64     string                 // Exported PSBT in base64 format
	showPSBTExport bool                   // Whether to show PSBT export view
	psbtExportErr  error                  // Error during PSBT export
	psbtSigsAdded  int                    // Signatures added locally
	psbtSigsTotal  int                    // Total signatures needed (M for multisig)
	psbtResult     *wallet.PSBTResult     // The actual PSBT for further signing
	inspection     *wallet.PSBTInspection // What psbtResult does, for review before signing
	signConfirmed  bool                   // dangerous PSBT inspection acknowledged once
	psbtImported   bool                   // PSBT arrived from outside hax's send flow
	bump           *feeBumpState          // Set when speeding up a pending transaction

	// Transaction visualizer overlay for the review and PSBT signing steps.
	txFlow *TxFlowView

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
	loading bool
	spinner spinner.Model
	err     error
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
	addrInput.Placeholder = "Address or bitcoin: URI"
	addrInput.Width = 60
	addrInput.CharLimit = 2048 // room for a pasted BIP21 URI
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
	spendable, frozen, frozenValue := withoutFrozen(walletManager, cachedUTXOs)
	if cachedUTXOs != nil {
		cachedUTXOs = spendable
		cachedBalance -= frozenValue
	}

	return SendModel{
		walletManager:  walletManager,
		step:           SendStepAddress,
		addressInput:   addrInput,
		amountInput:    amtInput,
		customFeeInput: customFeeInput,
		psbtFileInput:  fileInput,
		spinner:        s,
		scrollViewport: vp,
		usesBTC:        true,
		feeLevel:       FeeLevelMedium,
		balance:        cachedBalance,
		utxos:          cachedUTXOs,
		frozenCount:    frozen,
		loading:        loadingBalance, // Show loading until fresh data arrives
	}
}

// withoutFrozen drops frozen coins, which send never offers or spends,
// and returns how many there were and their value.
func withoutFrozen(w WalletService, utxos []api.UTXO) ([]api.UTXO, int, api.Satoshi) {
	if w == nil || utxos == nil {
		return utxos, 0, 0
	}
	out := make([]api.UTXO, 0, len(utxos))
	var n int
	var value api.Satoshi
	for _, u := range utxos {
		if w.IsUTXOFrozen(u.TxID, u.Vout) {
			n++
			value += u.Value
			continue
		}
		out = append(out, u)
	}
	return out, n, value
}

// estimateTxVSize estimates the transaction vSize using wallet-aware script type detection
// This provides accurate fee estimation for all wallet types (single-sig, multisig, different address types)
func (m *SendModel) estimateTxVSize(numInputs, numOutputs int) int {
	scriptType, multisigM, multisigN := m.walletManager.GetScriptType()
	return wallet.EstimateVSize(numInputs, numOutputs, scriptType, scriptType, multisigM, multisigN)
}

func (m SendModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textinput.Blink,
		m.spinner.Tick,
		m.fetchFeeEstimates(),
		m.fetchUTXOs(),
	}
	if m.bump != nil {
		cmds = append(cmds, m.analyzeFeeBump())
	}
	return tea.Batch(cmds...)
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
	if m.bump != nil {
		return m.previewFeeBump(feeRate)
	}

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

// CapturesTextInput reports whether the current send step owns the keyboard.
// The address input stays focused after its step, so the step matters.
func (m SendModel) CapturesTextInput() bool {
	if m.step == SendStepAddress || m.step == SendStepAmount {
		return true
	}
	return m.txFlow != nil || m.editingCustom || m.showFileInput || m.showURInput
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
			m.utxos, m.frozenCount, _ = withoutFrozen(m.walletManager, msg.UTXOs)
			m.balance = 0
			for _, utxo := range m.utxos {
				m.balance += utxo.Value
			}
			if len(m.utxoSelected) != len(m.utxos) {
				m.utxoSelected = make([]bool, len(m.utxos))
				for i := range m.utxoSelected {
					m.utxoSelected[i] = true
				}
			}
		}

	case StateChangedMsg:
		m.balance = msg.State.Balance
		if len(msg.State.UTXOs) > 0 {
			var frozenValue api.Satoshi
			m.utxos, m.frozenCount, frozenValue = withoutFrozen(m.walletManager, msg.State.UTXOs)
			m.balance -= frozenValue
			if len(m.utxoSelected) != len(m.utxos) {
				m.utxoSelected = make([]bool, len(m.utxos))
				for i := range m.utxoSelected {
					m.utxoSelected[i] = true
				}
			}
		}
		if msg.State.FeeEstimates != nil {
			m.feeEstimates = msg.State.FeeEstimates
			m.updateFeeEstimate()
		}

	case FeeBumpInfoMsg:
		m.handleFeeBumpInfo(msg)
		if m.bump != nil && m.bump.presetRate > 0 && m.bump.info != nil {
			if cmd := m.startPresetFeeBump(); cmd != nil {
				cmds = append(cmds, cmd)
			}
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
			m.setPSBTResult(msg.Result)
			m.psbtImported = false
			if m.bump != nil {
				m.recipients = m.bumpRecipients()
			}
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
			m.labelFromRequest()
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
			m.setPSBTResult(msg.Result)
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
			m.setPSBTResult(msg.Result)
			m.psbtImported = true
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
				m.setPSBTResult(result)
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
		// The visualizer owns every key while it is open.
		if m.txFlow != nil {
			return m.handleTxFlowKey(msg)
		}

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
	m.pendingRequest = nil
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
	m.setPSBTResult(nil)

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
	// Never construct a transaction from a non-finite or absurd rate.
	if math.IsNaN(feeRate) || math.IsInf(feeRate, 0) || feeRate > feeHardCap {
		return 0
	}
	// Floor extremely small positive estimates at the common min-relay rate.
	if feeRate < 0.1 {
		return 0.1
	}
	return feeRate
}

// requestLabel is the label carried by the recipients' payment requests.
func (m SendModel) requestLabel() string {
	var labels []string
	seen := map[string]bool{}
	for _, r := range m.recipients {
		if r.Label != "" && !seen[r.Label] {
			seen[r.Label] = true
			labels = append(labels, r.Label)
		}
	}
	return strings.Join(labels, ", ")
}

// labelFromRequest labels a just-broadcast transaction with its payment
// request label, unless the user already labelled it. Best effort: a
// failed save is shown but doesn't affect the broadcast.
func (m *SendModel) labelFromRequest() {
	label := m.requestLabel()
	if label == "" && m.psbtResult != nil && m.psbtResult.Replaces != "" &&
		m.walletManager != nil {
		// A replacement keeps the label of the transaction it replaces.
		label = m.walletManager.GetTransactionLabel(m.psbtResult.Replaces)
	}
	if label == "" || m.txID == "" || m.walletManager == nil {
		return
	}
	if m.walletManager.GetTransactionLabel(m.txID) != "" {
		return
	}
	if err := m.walletManager.SetTransactionLabel(m.txID, label); err != nil {
		m.err = fmt.Errorf("sent, but the label could not be saved: %v", err)
	}
}
