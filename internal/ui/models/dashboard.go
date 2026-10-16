package models

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

type MainDashboardModel struct {
	width  int
	height int

	walletManager WalletService
	configManager *config.ConfigManager
	balance       api.Satoshi
	balanceLoaded bool
	err           error
	loading       bool
	spinner       spinner.Model
	cursor        int // Track selected menu item

	// Animation state for balance display
	balanceSpring harmonica.Spring
	balanceValue  float64
	balanceTarget float64
	balanceVel    float64

	// Animation state for menu cursor highlight
	cursorSpring harmonica.Spring
	cursorY      float64 // animated position (slides between items)
	cursorVel    float64

	// Animation state for status breathing
	pulseFrame int
	isActive   bool

	// Chain state for confirmation calculation
	blockHeight int64

	// Re-trust flow for an Electrum server whose TLS key changed.
	retrustPrompt *backend.PinMismatchError
	retrustErr    error
}

// BalanceMsg is sent when balance is fetched
type BalanceMsg struct {
	Balance api.Satoshi
	Err     error
}

// RefreshBalanceMsg triggers a balance refresh with sync
type RefreshBalanceMsg struct{}

func NewMainDashboardModel(walletManager WalletService, configManager *config.ConfigManager) MainDashboardModel {
	return MainDashboardModel{
		walletManager: walletManager,
		configManager: configManager,
		loading:       true,
		spinner:       styles.NewStandardSpinner(),
		balanceSpring: harmonica.NewSpring(harmonica.FPS(60), 4.0, 0.7),
		// Menu cursor spring: low stiffness + low damping = visible, bouncy glide
		cursorSpring: harmonica.NewSpring(harmonica.FPS(30), 3.5, 0.5),
		isActive:     true,
	}
}

func (m MainDashboardModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchBalance(m.walletManager), m.breatheTick())
}

// animateTick drives the cursor slide animation.
func (m MainDashboardModel) animateTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

type breatheTickMsg time.Time

// breatheTick drives the status pulse.
func (m MainDashboardModel) breatheTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return breatheTickMsg(t)
	})
}

func (m MainDashboardModel) refreshCacheCmd() tea.Cmd {
	return func() tea.Msg {
		m.walletManager.RefreshCache()
		return StateChangedMsg{State: m.walletManager.GetState()}
	}
}

// fetchBalance prefers cached balance data and falls back to the backend.
func fetchBalance(wm WalletService) tea.Cmd {
	return func() tea.Msg {
		if !wm.IsUnlocked() {
			return BalanceMsg{
				Balance: 0,
				Err:     fmt.Errorf("wallet is not unlocked"),
			}
		}

		if cached, ok := wm.GetCachedBalance(); ok {
			return BalanceMsg{
				Balance: cached,
				Err:     nil,
			}
		}

		balance, err := wm.GetBalance(0)
		if err != nil {
			return BalanceMsg{
				Balance: 0,
				Err:     fmt.Errorf("could not fetch balance: %v", err),
			}
		}
		return BalanceMsg{
			Balance: balance,
			Err:     nil,
		}
	}
}

func (m MainDashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ScreenActiveMsg:
		m.isActive = true
		state := m.walletManager.GetState()
		animCmds := []tea.Cmd{m.breatheTick()}
		needsSpinner := m.loading || state.Status == wallet.SyncStatusConnecting || state.Status == wallet.SyncStatusSyncing
		if needsSpinner {
			animCmds = append(animCmds, m.spinner.Tick)
		}
		return m, tea.Batch(animCmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case RefreshBalanceMsg:
		m.loading = true
		if cached, ok := m.walletManager.GetCachedBalance(); ok {
			m.balance = cached
			m.balanceLoaded = true
		}
		return m, tea.Batch(m.spinner.Tick, m.refreshCacheCmd())

	case BalanceMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.balance = msg.Balance
			m.balanceLoaded = true
			m.balanceTarget = float64(msg.Balance)
		}

	case StateChangedMsg:
		m.balance = msg.State.Balance
		m.balanceLoaded = true
		m.balanceTarget = float64(msg.State.Balance)
		if msg.State.BlockHeight > 0 {
			m.blockHeight = msg.State.BlockHeight
		}
		currentState := m.walletManager.GetState()
		if currentState.Status == wallet.SyncStatusSynced {
			m.loading = false
		}
		cmds = append(cmds, m.spinner.Tick)
		cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}))

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		state := m.walletManager.GetState()
		needsSpinner := m.loading || state.Status == wallet.SyncStatusConnecting || state.Status == wallet.SyncStatusSyncing
		if needsSpinner && cmd != nil {
			cmds = append(cmds, cmd)
		}

	case breatheTickMsg:
		if !m.isActive {
			return m, nil
		}
		state := m.walletManager.GetState()
		if state.Status == wallet.SyncStatusSynced || state.Status == wallet.SyncStatusConnected {
			m.pulseFrame++
		}
		return m, m.breatheTick()

	case TickMsg:
		if !m.isActive {
			return m, nil
		}
		cursorTarget := float64(m.cursor)
		cursorDiff := math.Abs(m.cursorY - cursorTarget)
		if cursorDiff > 0.01 {
			m.cursorY, m.cursorVel = m.cursorSpring.Update(m.cursorY, m.cursorVel, cursorTarget)
			cmds = append(cmds, m.animateTick())
		} else {
			m.cursorY = cursorTarget
			m.cursorVel = 0
		}

	case tea.KeyMsg:
		if m.retrustPrompt != nil {
			return m.handleRetrustKey(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "T":
			if pe := pinMismatch(m.walletManager.GetState()); pe != nil {
				m.retrustPrompt = pe
				m.retrustErr = nil
				return m, nil
			}

		// Arrow key navigation
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				// Snap highlight immediately to avoid perceived input lag.
				m.cursorY = float64(m.cursor)
				m.cursorVel = 0
			}
		case "down", "j":
			if m.cursor < 7 { // 8 menu items (0-7)
				m.cursor++
				// Snap highlight immediately to avoid perceived input lag.
				m.cursorY = float64(m.cursor)
				m.cursorVel = 0
			}

		// Enter selects current cursor position
		case "enter":
			switch m.cursor {
			case 0: // Receive
				return m, Navigate(StateReceive)
			case 1: // Send
				return m, Navigate(StateSend)
			case 2: // History
				return m, Navigate(StateTransactionHistory)
			case 3: // Addresses
				return m, Navigate(StateAddresses)
			case 4: // UTXOs
				return m, Navigate(StateUTXOs)
			case 5: // Settings
				return m, Navigate(StateSettings)
			case 6: // Help
				return m, Navigate(StateHelp)
			case 7: // Quit
				return m, tea.Quit
			}

		// Letter shortcuts still work
		case "r":
			return m, Navigate(StateReceive)
		case "s":
			return m, Navigate(StateSend)
		case "h":
			return m, Navigate(StateTransactionHistory)
		case "a":
			return m, Navigate(StateAddresses)
		case "u":
			return m, Navigate(StateUTXOs)
		case "t":
			return m, Navigate(StateSettings)
		case "?":
			return m, Navigate(StateHelp)
		}
	}

	// Update balance animation - use tolerance to prevent infinite loop
	// Floating point comparison needs epsilon to avoid CPU spin
	const epsilon = 0.01
	diff := m.balanceTarget - m.balanceValue
	if diff < 0 {
		diff = -diff
	}
	if diff > epsilon {
		m.balanceValue, m.balanceVel = m.balanceSpring.Update(
			m.balanceValue,
			m.balanceVel,
			m.balanceTarget,
		)
		cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}))
	} else {
		// Snap to target when close enough to stop animation
		m.balanceValue = m.balanceTarget
		m.balanceVel = 0
	}

	return m, tea.Batch(cmds...)
}

func (m MainDashboardModel) View() string {
	grid := m.viewGrid()
	state := m.walletManager.GetState()
	if panel := m.renderRetrustPanel(state); panel != "" {
		return lipgloss.JoinVertical(lipgloss.Left, grid, panel)
	}
	return grid
}

func (m MainDashboardModel) viewGrid() string {
	// Get unified state for status display
	state := m.walletManager.GetState()
	displayUnit := m.getDisplayUnit()

	// Determine layout mode based on terminal width
	singleColumn := m.width < 80

	var colWidth int
	if singleColumn {
		// Single column: use full width minus borders
		colWidth = m.width - 2
		if colWidth < 30 {
			colWidth = 30
		}
	} else {
		// Two columns: subtract gap (2) + borders (2 per column = 4)
		colWidth = (m.width - 6) / 2
		if colWidth < 30 {
			colWidth = 30
		}
		if colWidth > 60 {
			colWidth = 60
		}
	}

	// --- Wallet Info Box ---
	walletName := m.walletManager.GetWalletName()
	walletTitle := styles.HighlightText.Render(walletName)

	// Add multisig indicator if applicable
	if m.walletManager.IsMultisig() {
		config := m.walletManager.GetMultisigConfig()
		if config != nil {
			walletTitle += " " + styles.DimText.Render(fmt.Sprintf("(%d-of-%d multisig)", config.M, config.N))
		}
	}

	// Status indicator
	var statusIndicator string
	switch state.Status {
	case wallet.SyncStatusDisconnected:
		statusIndicator = styles.DimText.Render("○ Disconnected")
	case wallet.SyncStatusConnecting:
		msg := "Connecting..."
		if state.StatusMsg != "" {
			msg = state.StatusMsg
		}
		statusIndicator = m.spinner.View() + " " + styles.StatusConnectingStyle.Render(msg)
	case wallet.SyncStatusConnected:
		dotStyle := m.breathingDotStyle(styles.PastelBlue)
		statusIndicator = dotStyle.Render("●") + " " + styles.StatusConnectedStyle.Render("Connected")
	case wallet.SyncStatusSyncing:
		msg := "Syncing..."
		if state.StatusMsg != "" {
			msg = state.StatusMsg
		}
		statusIndicator = m.spinner.View() + " " + styles.StatusSyncingStyle.Render(msg)
	case wallet.SyncStatusSynced:
		dotStyle := m.breathingDotStyle(styles.PastelGreen)
		statusIndicator = dotStyle.Render("●") + " " + dotStyle.Render("Synced") +
			styles.DimText.Render(fmt.Sprintf(" · block %d", state.BlockHeight))
	case wallet.SyncStatusError:
		errMsg := "Error"
		if state.LastError != nil {
			errMsg = state.LastError.Error()
			errMsg = truncateRunes(errMsg, 33, "...")
		}
		if pinMismatch(state) != nil {
			errMsg = "Server key changed"
		}
		statusIndicator = styles.StatusErrorStyle.Render("● " + errMsg)
	}

	// Balance display
	displayBalance := state.Balance
	if state.Balance == 0 && m.balance > 0 {
		displayBalance = m.balance
	}
	var balanceDisplay string
	if m.err != nil {
		balanceDisplay = styles.ErrorText.Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		balanceFormatted := wallet.FormatAmount(displayBalance, displayUnit)
		balanceDisplay = styles.DimText.Render("Balance: ") + styles.BalanceValueStyle.Render(balanceFormatted)
	}

	// Script type and UTXO count
	scriptType, _, _ := m.walletManager.GetScriptType()
	scriptName := scriptTypeDisplayName(scriptType)
	isLoading := state.Status == wallet.SyncStatusConnecting || state.Status == wallet.SyncStatusSyncing
	var walletMeta string
	if isLoading {
		walletMeta = styles.DimText.Render(scriptName)
	} else {
		utxoCount := len(state.UTXOs)
		utxoLabel := "UTXOs"
		if utxoCount == 1 {
			utxoLabel = "UTXO"
		}
		walletMeta = styles.DimText.Render(fmt.Sprintf("%s · %d %s", scriptName, utxoCount, utxoLabel))
	}

	// Build wallet info box content
	walletInfoContent := walletTitle + "\n" +
		statusIndicator + "\n\n" +
		balanceDisplay + "\n\n" +
		withBadge(walletMeta, walletNetwork(m.walletManager), "\n")

	// --- Recent Activity Box ---
	var recentActivityContent string
	if isLoading {
		recentActivityContent = styles.DimText.Render("Loading...")
	} else {
		recentActivityContent = m.renderRecentTransactions(state.Transactions, displayUnit, state.BlockHeight)
	}
	recentActivityText := styles.BoldText.Render("Recent Activity") + "\n\n" + recentActivityContent

	// --- Fee Estimates Box ---
	feeContent := m.renderFeeEstimates(state.FeeEstimates)
	feeText := styles.BoldText.Render("Network Fees") + "\n" + feeContent

	// --- Menu Box ---
	menuOptions := []struct {
		key  string
		desc string
	}{
		{"r", "Receive"},
		{"s", "Send"},
		{"h", "History"},
		{"a", "Addresses"},
		{"u", "UTXOs"},
		{"t", "Settings"},
		{"?", "Help"},
		{"q", "Quit"},
	}

	var menuList string
	for i, opt := range menuOptions {
		line := "  " + opt.key + "  " + opt.desc
		if i == m.cursor {
			menuList += styles.SelectedItemStyle.Render("▶ "+opt.key+"  "+opt.desc) + "\n"
		} else {
			menuList += styles.UnselectedItemStyle.Render(line) + "\n"
		}
	}
	menuList = strings.TrimSuffix(menuList, "\n")

	menuText := styles.BoldText.Render("Menu") + "\n" + menuList

	// Normalize card heights based on content so borders stay the same size in all states.
	uniformContentHeight := max(
		lipgloss.Height(walletInfoContent),
		lipgloss.Height(recentActivityText),
		lipgloss.Height(feeText),
		lipgloss.Height(menuText),
	)
	walletInfoBox := styles.BoxStyle.MarginBottom(0).Width(colWidth).Height(uniformContentHeight).Render(walletInfoContent)
	recentActivityBox := styles.InfoBoxStyle.MarginBottom(0).Width(colWidth).Height(uniformContentHeight).Render(recentActivityText)
	feeBox := styles.FeeBoxStyle.Width(colWidth).Height(uniformContentHeight).Render(feeText)
	menuBox := styles.MenuBoxStyle.Width(colWidth).Height(uniformContentHeight).Render(menuText)

	// --- Layout ---
	if singleColumn {
		// Narrow terminal: stack all boxes vertically
		return lipgloss.JoinVertical(lipgloss.Left,
			walletInfoBox,
			recentActivityBox,
			feeBox,
			menuBox,
		)
	}

	// Two-column layout
	leftColumn := lipgloss.JoinVertical(lipgloss.Left,
		walletInfoBox,
		feeBox,
	)

	rightColumn := lipgloss.JoinVertical(lipgloss.Left,
		recentActivityBox,
		menuBox,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		leftColumn,
		"  ",
		rightColumn,
	)
}

// renderRecentTransactions renders a compact list of recent transactions
func (m MainDashboardModel) renderRecentTransactions(txs []api.Transaction, unit wallet.DisplayUnit, blockHeight int64) string {
	if len(txs) == 0 {
		return styles.DimText.Render("No transactions yet")
	}

	// Get our addresses for transaction analysis
	ourAddresses := make(map[string]bool)
	accounts := m.walletManager.GetAccounts()
	for _, acc := range accounts {
		for _, addr := range acc.Addresses {
			ourAddresses[addr.Address] = true
		}
	}

	// Sort transactions by time (newest first)
	// Make a copy to avoid modifying the original slice
	sortedTxs := make([]api.Transaction, len(txs))
	copy(sortedTxs, txs)
	sort.Slice(sortedTxs, func(i, j int) bool {
		// Pending transactions (no block time) should appear first
		if !sortedTxs[i].Status.Confirmed && sortedTxs[j].Status.Confirmed {
			return true
		}
		if sortedTxs[i].Status.Confirmed && !sortedTxs[j].Status.Confirmed {
			return false
		}
		// Both confirmed or both pending - sort by block time descending
		if sortedTxs[i].Status.BlockTime != sortedTxs[j].Status.BlockTime {
			return sortedTxs[i].Status.BlockTime > sortedTxs[j].Status.BlockTime
		}
		// Same block time - use TxID for deterministic ordering
		return sortedTxs[i].TxID > sortedTxs[j].TxID
	})

	var lines []string
	maxTxs := 4
	if m.height > 0 && m.height < 30 {
		maxTxs = 3
	}
	if m.height > 0 && m.height < 22 {
		maxTxs = 2
	}
	if len(sortedTxs) < maxTxs {
		maxTxs = len(sortedTxs)
	}

	for i := 0; i < maxTxs; i++ {
		tx := sortedTxs[i]

		// Analyze transaction
		analysis := m.analyzeTransactionSimple(tx, ourAddresses)

		// Format time - include year if not current year
		timeStr := "pending"
		if tx.Status.Confirmed && tx.Status.BlockTime > 0 {
			t := time.Unix(tx.Status.BlockTime, 0)
			now := time.Now()
			if t.Year() == now.Year() {
				timeStr = t.Format("Jan 2")
			} else {
				timeStr = t.Format("Jan 02 '06")
			}
		}

		// Format amount
		absAmount := analysis.NetAmount
		if absAmount < 0 {
			absAmount = -absAmount
		}
		amountStr := wallet.FormatAmountCompact(absAmount, unit)

		// Direction indicator
		direction := styles.TxDirectionInStyle.Render("↓")
		if analysis.Type == "↑ OUT" {
			direction = styles.TxDirectionOutStyle.Render("↑")
		} else if analysis.Type == "⟲ SELF" {
			direction = styles.DimText.Render("⟲")
		}

		// Compact confirmation indicator with progress symbols
		// ◔ = 25% (1 conf), ◑ = 50% (2-3 conf), ◕ = 75% (4-5 conf)
		confIndicator := ""
		if !tx.Status.Confirmed {
			confIndicator = styles.WarningText.Render(" ⏳")
		} else if blockHeight > 0 && tx.Status.BlockHeight > 0 {
			confirmations := blockHeight - tx.Status.BlockHeight + 1
			if confirmations >= 1 && confirmations < 6 {
				symbol := "◔" // 1 confirmation
				if confirmations >= 2 && confirmations <= 3 {
					symbol = "◑" // 2-3 confirmations
				} else if confirmations >= 4 {
					symbol = "◕" // 4-5 confirmations
				}
				confIndicator = styles.DimText.Render(fmt.Sprintf(" %s %d/6", symbol, confirmations))
			}
			// For 6+ confirmations, no indicator needed (fully confirmed)
		}

		line := fmt.Sprintf("%s %s %s%s",
			direction,
			styles.DimText.Render(timeStr),
			amountStr,
			confIndicator,
		)
		lines = append(lines, line)
	}

	if len(txs) > maxTxs {
		lines = append(lines, styles.DimText.Render(fmt.Sprintf("  +%d more", len(txs)-maxTxs)))
	}

	return strings.Join(lines, "\n")
}

// analyzeTransactionSimple is a simplified version of transaction analysis for dashboard
func (m MainDashboardModel) analyzeTransactionSimple(tx api.Transaction, ourAddresses map[string]bool) struct {
	Type      string
	NetAmount api.Satoshi
} {
	var totalIn, totalOut api.Satoshi

	// Sum our inputs
	for _, vin := range tx.Vin {
		if vin.Prevout != nil && ourAddresses[vin.Prevout.ScriptpubkeyAddress] {
			totalIn += vin.Prevout.Value
		}
	}

	// Sum our outputs
	for _, vout := range tx.Vout {
		if ourAddresses[vout.ScriptpubkeyAddress] {
			totalOut += vout.Value
		}
	}

	netAmount := totalOut - totalIn
	txType := "↓ IN"
	if netAmount < 0 {
		txType = "↑ OUT"
	} else if totalIn > 0 && totalOut > 0 && netAmount <= 0 {
		// Self-send: we funded it and received change
		txType = "⟲ SELF"
	}

	return struct {
		Type      string
		NetAmount api.Satoshi
	}{txType, netAmount}
}

// renderFeeEstimates renders network fee estimates from the connected backend
// (Electrum blockchain.estimatefee or Core estimatesmartfee). Missing rates
// are shown as unavailable — never invented.
func (m MainDashboardModel) renderFeeEstimates(fees *api.FeeEstimates) string {
	if fees == nil {
		return styles.DimText.Render("Unavailable")
	}

	format := func(rate float64) string {
		if rate <= 0 {
			return "unavailable"
		}
		return fmt.Sprintf("%s sat/vB", formatFeeRate(rate))
	}

	lines := []string{
		fmt.Sprintf("%s %s",
			styles.DimText.Render("High:"),
			styles.HighlightText.Render(format(fees.FastestFee))),
		styles.DimText.Render("  Next block"),
		"",
		fmt.Sprintf("%s %s",
			styles.DimText.Render("Med: "),
			styles.HighlightText.Render(format(fees.HalfHourFee))),
		styles.DimText.Render("  Within 30 min"),
		"",
		fmt.Sprintf("%s %s",
			styles.DimText.Render("Low: "),
			styles.HighlightText.Render(format(fees.EconomyFee))),
		styles.DimText.Render("  Within 1 hour"),
	}

	return strings.Join(lines, "\n")
}

// scriptTypeDisplayName returns a human-readable name for a script type
func scriptTypeDisplayName(st wallet.ScriptType) string {
	switch st {
	case wallet.ScriptTypeP2PKH:
		return "Legacy"
	case wallet.ScriptTypeP2SHP2WPKH:
		return "Nested SegWit"
	case wallet.ScriptTypeP2WPKH:
		return "Native SegWit"
	case wallet.ScriptTypeP2TR:
		return "Taproot"
	case wallet.ScriptTypeP2SH:
		return "P2SH Multisig"
	case wallet.ScriptTypeP2SHP2WSH:
		return "Nested Multisig"
	case wallet.ScriptTypeP2WSH:
		return "Native Multisig"
	default:
		return "SegWit"
	}
}

// breathingDotStyle returns a lipgloss style with a pulsing brightness for status dots.
// Uses a slow sine wave (3-second cycle) to blend between a dim and bright variant.
func (m MainDashboardModel) breathingDotStyle(baseColor lipgloss.Color) lipgloss.Style {
	// 5 FPS tick with a 30-frame cycle => ~6 seconds per pulse.
	// Keep it smooth but visibly noticeable.
	phase := 2 * math.Pi * float64(m.pulseFrame%30) / 30.0
	raw := 0.5 + 0.5*math.Sin(phase)
	brightness := 0.25 + 0.75*math.Pow(raw, 0.65)
	brightColor, _ := colorful.Hex(string(baseColor))
	mutedColor, _ := colorful.Hex(string(styles.TextMuted))
	mutedBase := brightColor.BlendLab(mutedColor, 0.55)
	blended := mutedBase.BlendLab(brightColor, brightness)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(blended.Hex()))
}

// getDisplayUnit returns the configured display unit (BIP-177)
func (m MainDashboardModel) getDisplayUnit() wallet.DisplayUnit {
	if m.configManager == nil {
		return wallet.DisplayUnitBTC
	}
	cfg, err := m.configManager.Load()
	if err != nil || cfg == nil {
		return wallet.DisplayUnitBTC
	}
	// Convert config.DisplayUnit to wallet.DisplayUnit
	switch cfg.DisplayUnit {
	case config.DisplayUnitBitcoin:
		return wallet.DisplayUnitBitcoin
	default:
		return wallet.DisplayUnitBTC
	}
}

// renderNetworkBadge returns a TESTNET4 tag for test-network wallets and ""
// for mainnet, which is the normal, unlabeled state.
func renderNetworkBadge(network string) string {
	if network == wallet.NetworkMainnet {
		return ""
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.PrimaryBg).
		Background(styles.Amber).
		Padding(0, 1).
		Render("TESTNET4")
}

// withBadge appends the network badge after s, if there is one.
func withBadge(s, network, sep string) string {
	if b := renderNetworkBadge(network); b != "" {
		return s + sep + b
	}
	return s
}

// walletNetwork returns the active wallet's network, defaulting to testnet4
// when no wallet is loaded.
func walletNetwork(w WalletService) string {
	if w == nil {
		return wallet.NetworkTestnet
	}
	if n := w.GetNetwork(); n != "" {
		return n
	}
	return wallet.NetworkTestnet
}
