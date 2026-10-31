package models

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/bbqr"
	"github.com/BTCBap/hax/internal/fsutil"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/ur"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
		if err := fsutil.WriteFileAtomic(expandedPath, psbtBinary, 0o600); err != nil {
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
		m.fileInputFromBrowser = true
		m.fileInputMode = "import"
		m.psbtFileInput.SetValue("")
		m.psbtFileInput.Focus()
		return m, textinput.Blink

	case "p", "P":
		// Manual path entry
		m.showFileBrowser = false
		m.showFileInput = true
		m.fileInputFromBrowser = true
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

// psbtExportHeader is the Sign & Broadcast title and status frame.
func (m SendModel) psbtExportHeader() (header, statusText string, statusStyle lipgloss.Style) {
	// Guard against nil wallet manager in tests or incomplete state.
	if m.walletManager == nil {
		title := styles.TitleStyle.Render("🔐 Sign & Broadcast")
		subtitle := m.stepHeader(7, "Sign to complete transaction")
		return title + "\n" + subtitle, "", styles.DimText
	}

	// Determine header based on signing status
	headerText := "🔐 Sign & Broadcast"
	if m.psbtSigsAdded >= m.psbtSigsTotal {
		headerText = "✓ Sign & Broadcast"
	}
	title := styles.TitleStyle.Render(headerText)

	// Determine status based on signatures
	var subtitleText string
	if m.psbtSigsAdded >= m.psbtSigsTotal {
		subtitleText = "Ready to broadcast"
		statusText = fmt.Sprintf("✓ %d of %d signatures", m.psbtSigsAdded, m.psbtSigsTotal)
		statusStyle = styles.SignatureCompleteStyle
	} else if m.psbtSigsAdded > 0 {
		subtitleText = "Partially signed - need more signatures"
		statusText = fmt.Sprintf("%d of %d signatures (need %d more)", m.psbtSigsAdded, m.psbtSigsTotal, m.psbtSigsTotal-m.psbtSigsAdded)
		statusStyle = styles.SignaturePartialStyle
	} else {
		subtitleText = "Sign to complete transaction"
		statusText = fmt.Sprintf("0 of %d signatures", m.psbtSigsTotal)
		statusStyle = styles.DimText
	}
	subtitle := m.stepHeader(7, subtitleText)
	return title + "\n" + subtitle, statusText, statusStyle
}

func (m SendModel) buildPSBTExportScrollable() (string, string, string) {
	// Guard against nil wallet manager in tests or incomplete state.
	if m.walletManager == nil {
		header, _, _ := m.psbtExportHeader()
		help := styles.RenderHelp(styles.HK("esc", "back"))
		return header, styles.DimText.Render("Loading..."), help
	}
	header, statusText, statusStyle := m.psbtExportHeader()

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
		styles.HK("h", "HWI"),
		styles.HK("q", "QR"),
		styles.HK("f", "save"),
		styles.HK("i", "import"),
		styles.HK("u", "UR"),
		styles.HK("v", "flow"),
	)
	if m.psbtSigsAdded >= m.psbtSigsTotal {
		items = append(items, styles.HK("b", "broadcast"))
	}

	items = append(items, styles.HK("esc", "back"))

	help := styles.RenderHelp(items...)

	// Split into fixed header and scrollable content
	fixedHeader := header

	var contentSections []string
	if inspection := m.renderInspection(psbtBoxWidth); inspection != "" {
		contentSections = append(contentSections, inspection)
	}
	contentSections = append(contentSections, psbtBox, infoSection, usageHints)
	if statusIndicator != "" {
		contentSections = append(contentSections, statusIndicator)
	}
	scrollContent := strings.Join(contentSections, "\n")

	return fixedHeader, scrollContent, help
}

func (m SendModel) viewPSBTExport() string {
	if m.txFlow != nil {
		header, _, _ := m.psbtExportHeader()
		return m.renderTxFlowUnder(header)
	}
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

		options := make([]choiceOption, 0, maxShow)
		for i := 0; i < maxShow; i++ {
			file := m.psbtFiles[i]
			// Get just the filename for display
			filename := filepath.Base(file)
			// Get parent directory name
			parentDir := filepath.Base(filepath.Dir(file))

			options = append(options, choiceOption{name: filename, desc: parentDir})
		}
		content.WriteString(renderChoiceMenu(options, m.fileBrowserCursor))

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
