package models

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ur"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// handleTxFlowKey drives the transaction visualizer; esc/q returns to the
// screen that opened it without touching that screen's state.
func (m *SendModel) handleTxFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handleTxFlowKey(m.txFlow, msg.String()) {
		m.txFlow = nil
	}
	return m, nil
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
		// 'i' is also a valid address/URI character, so it only opens
		// the PSBT import when nothing has been typed yet.
		if m.addressInput.Value() != "" {
			return false, nil
		}
		// Import PSBT - open file browser
		m.psbtFiles = discoverPSBTFiles()
		m.fileBrowserCursor = 0
		m.fileBrowserForStep = "step1"
		m.showFileBrowser = true
		m.err = nil
		return true, nil

	case "enter":
		addr := strings.TrimSpace(m.addressInput.Value())
		if addr == "" {
			m.err = fmt.Errorf("please enter a recipient address")
			return true, nil
		}

		if wallet.IsBIP21(addr) {
			return true, m.applyPaymentRequest(addr)
		}

		// Validate address
		if err := wallet.ValidateAddressForNetwork(addr, walletNetwork(m.walletManager)); err != nil {
			m.err = err
			return true, nil
		}

		m.recipientAddress = addr
		m.pendingRequest = nil
		m.err = nil
		m.step = SendStepAmount
		return true, m.amountInput.Focus()

	case "esc":
		// Handled globally, but still mark as handled
		return true, Navigate(StateMainDashboard)
	}
	return false, nil // Not handled - let input handle it
}

// applyPaymentRequest fills the recipient from a BIP21 URI. The amount, if
// any, is prefilled on the amount step for the user to confirm; it is
// never sent without that step.
func (m *SendModel) applyPaymentRequest(uri string) tea.Cmd {
	req, err := wallet.ParseBIP21(uri, walletNetwork(m.walletManager))
	if err != nil {
		m.err = fmt.Errorf("payment request: %w", err)
		return nil
	}
	m.recipientAddress = req.Address
	m.pendingRequest = req
	m.err = nil
	m.amountInput.SetValue("")
	if req.Amount > 0 {
		if m.usesBTC {
			m.amountInput.SetValue(formatBTCExact(req.Amount))
		} else {
			m.amountInput.SetValue(strconv.FormatInt(int64(req.Amount), 10))
		}
		m.amountInput.CursorEnd()
	}
	m.step = SendStepAmount
	return m.amountInput.Focus()
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
			Label:   m.pendingRequestLabel(),
		})
		m.pendingRequest = nil

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
			Label:   m.pendingRequestLabel(),
		})
		m.pendingRequest = nil

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
	case "m", "M":
		m.toggleBumpMethod()
		return m, nil
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

		rate := m.getFeeRateForLevel(m.feeLevel)
		if rate <= 0 {
			m.err = fmt.Errorf("fee estimates unavailable — enter a custom fee rate (c) or retry")
			return m, nil
		}
		// A backend estimate above the soft cap needs the same typed
		// confirmation as a custom rate; above the hard cap it is never used.
		if rate > feeHardCap {
			m.err = fmt.Errorf("fee estimate too high (maximum %.0f sat/vB); enter a custom fee rate (c)", feeHardCap)
			return m, nil
		}
		if rate > feeSoftCap {
			m.highFeePending = rate
			m.highFeeInput = textinput.New()
			m.highFeeInput.Placeholder = highFeeConfirmWord
			m.highFeeInput.CharLimit = 20
			m.highFeeInput.Width = 20
			m.err = nil
			return m, m.highFeeInput.Focus()
		}

		if m.bump != nil {
			if err := m.checkBumpRate(m.getFeeRateForLevel(m.feeLevel)); err != nil {
				m.err = err
				return m, nil
			}
			m.err = nil
			m.step = SendStepPreviewLoading
			return m, tea.Batch(m.previewTransaction(), m.spinner.Tick)
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
		if m.bump != nil {
			return m, Navigate(StateTransactionHistory)
		}
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
			m.setImportedPSBTResult(result)
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
				err := copyToClipboard(m.psbtBase64)
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

			if blocked, danger := m.signingGate(); blocked != "" {
				m.psbtExportErr = fmt.Errorf("%s", blocked)
				return m, nil
			} else if danger && !m.signConfirmed {
				m.signConfirmed = true
				m.psbtExportErr = fmt.Errorf("review the warnings above, then press s again to sign")
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
			m.refreshInspection()
			// Count unique signers in PSBT (not per-input signatures)
			m.psbtSigsAdded = wallet.CountPSBTSigners(m.psbtResult.Packet)
			m.showCopied = true
			m.copiedTime = time.Now()
			m.lastAction = "signed"
			return m, m.animateTick()
		case "b", "B":
			// Broadcast (only if fully signed)
			if m.psbtSigsAdded >= m.psbtSigsTotal && m.psbtResult != nil {
				if blocked, danger := m.signingGate(); blocked != "" {
					m.psbtExportErr = fmt.Errorf("can't broadcast: %s", blocked)
					return m, nil
				} else if danger && !m.signConfirmed {
					m.signConfirmed = true
					m.psbtExportErr = fmt.Errorf("review the warnings above, then press b again to broadcast")
					return m, nil
				}
				txID, err := m.walletManager.BroadcastPSBT(m.psbtResult)
				if err != nil {
					m.psbtExportErr = fmt.Errorf("broadcast failed: %v", err)
					return m, nil
				}
				// Success - go to complete step and trigger lightweight refresh
				m.txID = txID
				m.labelFromRequest()
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
		case "v":
			// Show the current PSBT as a transaction flow diagram.
			m.openInspectionFlow()
			return m, nil
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
				err := copyToClipboard(m.rawHex)
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
	case "v":
		// Inspect inputs and outputs as a flow diagram before signing.
		m.openInspectionFlow()
		return m, nil
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
			err := copyToClipboard(m.txID)
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

func (m SendModel) pendingRequestLabel() string {
	if m.pendingRequest == nil {
		return ""
	}
	return m.pendingRequest.Label
}

// formatBTCExact renders sats as a BTC decimal without float rounding.
func formatBTCExact(sats api.Satoshi) string {
	return fmt.Sprintf("%d.%08d", int64(sats)/100_000_000, int64(sats)%100_000_000)
}
