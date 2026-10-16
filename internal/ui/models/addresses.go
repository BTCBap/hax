package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// AddrDisplayInfo holds info about an address for display
type AddrDisplayInfo struct {
	Address string
	Index   uint32
	Used    bool
	Balance api.Satoshi
}

type addressVisualState int

const (
	addressVisualUnused addressVisualState = iota
	addressVisualUsedNoBalance
	addressVisualUsedWithBalance
)

func getAddressVisualState(addr AddrDisplayInfo) addressVisualState {
	if !addr.Used {
		return addressVisualUnused
	}
	if addr.Balance > 0 {
		return addressVisualUsedWithBalance
	}
	return addressVisualUsedNoBalance
}

func renderAddressCell(addr AddrDisplayInfo, addressText string) string {
	// Keep address column plain to avoid ANSI clipping artifacts in wide table cells.
	_ = addr
	return addressText
}

// AddressesModel displays wallet addresses like Sparrow's Addresses tab
type AddressesModel struct {
	width  int
	height int

	walletManager WalletService
	configManager *config.ConfigManager

	// Data - loaded from cached account data only
	receiveAddresses []AddrDisplayInfo
	changeAddresses  []AddrDisplayInfo
	loading          bool
	err              error

	// View mode: 0=receive, 1=change
	viewMode int

	// Table
	table table.Model

	// Copy notification
	showCopied bool
	copiedTime time.Time
	copyMsg    string

	// Screen visibility
	isActive bool
}

// AddressesInitMsg triggers initial load
type AddressesInitMsg struct{}

func NewAddressesModel(walletManager WalletService, configManager *config.ConfigManager) AddressesModel {
	columns := []table.Column{
		{Title: "#", Width: 4},
		{Title: "Address", Width: 45},
		{Title: "Status", Width: 8},
		{Title: "Balance", Width: 14},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	tableStyles := styles.NewTableStyles()
	// Keep selected-row styling consistent with the app-wide orange highlight.
	tableStyles.Selected = tableStyles.Selected.
		Foreground(styles.BitcoinOrange).
		Bold(true).
		Background(lipgloss.Color(""))
	t.SetStyles(tableStyles)

	return AddressesModel{
		walletManager: walletManager,
		configManager: configManager,
		loading:       false,
		table:         t,
		viewMode:      0,
		isActive:      true,
	}
}

func (m AddressesModel) Init() tea.Cmd {
	return func() tea.Msg {
		return AddressesInitMsg{}
	}
}

func (m AddressesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case AddressesInitMsg:
		m.loadAddresses()
		m.loading = false
		m.updateTable()
		m.table.Focus()
		m.table.GotoTop()
		return m, nil

	case ScreenActiveMsg:
		m.isActive = true
		m.loadAddresses()
		m.updateTable()
		m.table.Focus()
		m.table.GotoTop()
		return m, nil

	case StateChangedMsg:
		m.loadAddresses()
		m.updateTable()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeColumnsForWidth()
		// Keep table height stable across scroll/selection states.
		tableHeight := m.height - 8
		if tableHeight < 3 {
			tableHeight = 3
		}
		m.table.SetHeight(tableHeight)
		m.updateTable()

	case TickMsg:
		if m.showCopied && time.Since(m.copiedTime) > 2*time.Second {
			m.showCopied = false
		}
		if m.showCopied {
			cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
				return TickMsg(t)
			}))
		}

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.table.MoveUp(1)
			m.clampCursor()
			return m, nil
		case tea.MouseButtonWheelDown:
			m.table.MoveDown(1)
			m.clampCursor()
			return m, nil
		}

	case tea.KeyMsg:
		// Any keyboard interaction should cancel queued wheel momentum globally.
		cmds = append(cmds, SuppressWheelFor(150*time.Millisecond))
		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, Navigate(StateMainDashboard)

		case "tab":
			// Toggle between receive and change addresses
			m.viewMode = (m.viewMode + 1) % 2
			m.updateTable()
			m.table.GotoTop()

		case "c":
			// Copy selected address
			var addrs []AddrDisplayInfo
			if m.viewMode == 0 {
				addrs = m.receiveAddresses
			} else {
				addrs = m.changeAddresses
			}
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(addrs) {
				addr := addrs[idx].Address
				if err := clipboard.WriteAll(addr); err == nil {
					m.showCopied = true
					m.copyMsg = "ADDRESS COPIED!"
					m.copiedTime = time.Now()
					cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
						return TickMsg(t)
					}))
				}
			}

		case "r":
			// Refresh
			m.loadAddresses()
			m.updateTable()
		}

		// Let table handle navigation keys (up/down/pgup/pgdown/j/k).
		var tableCmd tea.Cmd
		m.table, tableCmd = m.table.Update(msg)
		m.clampCursor()
		if tableCmd != nil {
			cmds = append(cmds, tableCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *AddressesModel) resizeColumnsForWidth() {
	available := m.width - 8 // account for border/padding margins
	if available < 60 {
		available = 60
	}

	indexW := 4
	statusW := 8
	balanceW := 14
	// Reserve room for table separators/spacing between columns.
	overhead := 10
	addressW := available - indexW - statusW - balanceW - overhead
	if addressW < 32 {
		addressW = 32
	}
	// Rendered addresses are truncated to 20 + "..." + 20 = 43 chars.
	// Keep column close to that width to avoid a large visual gutter.
	if addressW > 43 {
		addressW = 43
	}

	m.table.SetColumns([]table.Column{
		{Title: "#", Width: indexW},
		{Title: "Address", Width: addressW},
		{Title: "Status", Width: statusW},
		{Title: "Balance", Width: balanceW},
	})
}

func (m *AddressesModel) loadAddresses() {
	// Use only cached data - NO network calls
	accounts := m.walletManager.GetAccounts()
	state := m.walletManager.GetState()

	// Build address to balance map from cached UTXOs
	// We need to figure out which address each UTXO belongs to
	// by checking transaction outputs
	addrBalances := make(map[string]api.Satoshi)

	// Build a map of our addresses for quick lookup
	ourAddresses := make(map[string]bool)
	for _, acc := range accounts {
		for _, addr := range acc.Addresses {
			ourAddresses[addr.Address] = true
		}
	}

	// Look through transactions to find unspent outputs belonging to our addresses
	// This matches UTXOs to addresses via transaction outputs
	utxoSet := make(map[string]bool) // txid:vout -> exists
	for _, utxo := range state.UTXOs {
		key := fmt.Sprintf("%s:%d", utxo.TxID, utxo.Vout)
		utxoSet[key] = true
	}

	// For each transaction, check outputs that are still unspent
	for _, tx := range state.Transactions {
		for voutIdx, vout := range tx.Vout {
			if ourAddresses[vout.ScriptpubkeyAddress] {
				key := fmt.Sprintf("%s:%d", tx.TxID, voutIdx)
				if utxoSet[key] {
					addrBalances[vout.ScriptpubkeyAddress] += vout.Value
				}
			}
		}
	}

	m.receiveAddresses = nil
	m.changeAddresses = nil
	seenReceive := make(map[string]struct{})
	seenChange := make(map[string]struct{})

	// Get gap limit from config (default 20)
	gapLimit := 20
	if m.configManager != nil {
		cfg, err := m.configManager.Load()
		if err == nil && cfg != nil && cfg.GapLimit > 0 {
			gapLimit = cfg.GapLimit
		}
	}

	for _, acc := range accounts {
		// Track last used index for gap limit calculation
		lastUsedReceive := -1
		lastUsedChange := -1

		// First pass: find last used addresses
		for _, addr := range acc.Addresses {
			if addr.Used {
				if addr.Change {
					if int(addr.Index) > lastUsedChange {
						lastUsedChange = int(addr.Index)
					}
				} else {
					if int(addr.Index) > lastUsedReceive {
						lastUsedReceive = int(addr.Index)
					}
				}
			}
		}

		// Second pass: collect addresses up to gap limit
		for _, addr := range acc.Addresses {
			balance := addrBalances[addr.Address]

			info := AddrDisplayInfo{
				Address: addr.Address,
				Index:   addr.Index,
				Used:    addr.Used,
				Balance: balance,
			}

			if addr.Change {
				// Show up to lastUsedChange + gapLimit
				if int(addr.Index) <= lastUsedChange+gapLimit {
					if _, exists := seenChange[addr.Address]; exists {
						continue
					}
					seenChange[addr.Address] = struct{}{}
					m.changeAddresses = append(m.changeAddresses, info)
				}
			} else {
				// Show up to lastUsedReceive + gapLimit
				if int(addr.Index) <= lastUsedReceive+gapLimit {
					if _, exists := seenReceive[addr.Address]; exists {
						continue
					}
					seenReceive[addr.Address] = struct{}{}
					m.receiveAddresses = append(m.receiveAddresses, info)
				}
			}
		}
	}
}

func (m *AddressesModel) updateTable() {
	var addrs []AddrDisplayInfo
	if m.viewMode == 0 {
		addrs = m.receiveAddresses
	} else {
		addrs = m.changeAddresses
	}

	displayUnit := m.getDisplayUnit()

	var rows []table.Row
	for _, addr := range addrs {
		// Index
		indexStr := fmt.Sprintf("%d", addr.Index)

		// Address (truncated to fit)
		addrDisplay := addr.Address
		if len(addrDisplay) > 43 {
			addrDisplay = addrDisplay[:20] + "..." + addrDisplay[len(addrDisplay)-20:]
		}
		addrDisplay = renderAddressCell(addr, addrDisplay)

		// Status as plain text to avoid ANSI truncation artifacts in table cells.
		status := "UNUSED"
		if addr.Used {
			status = "USED"
		}

		// Balance
		balanceStr := wallet.FormatAmountCompact(addr.Balance, displayUnit)

		rows = append(rows, table.Row{
			indexStr,
			addrDisplay,
			status,
			balanceStr,
		})
	}

	m.table.SetRows(rows)
	if len(rows) == 0 {
		m.table.SetCursor(0)
		return
	}
	if m.table.Cursor() >= len(rows) {
		m.table.SetCursor(len(rows) - 1)
	}
}

func (m *AddressesModel) visibleRowsCount() int {
	if m.viewMode == 0 {
		return len(m.receiveAddresses)
	}
	return len(m.changeAddresses)
}

func (m *AddressesModel) clampCursor() {
	rowCount := m.visibleRowsCount()
	if rowCount <= 0 {
		m.table.SetCursor(0)
		return
	}
	if m.table.Cursor() >= rowCount {
		m.table.SetCursor(rowCount - 1)
	}
	if m.table.Cursor() < 0 {
		m.table.SetCursor(0)
	}
}

func (m AddressesModel) getDisplayUnit() wallet.DisplayUnit {
	if m.configManager == nil {
		return wallet.DisplayUnitBTC
	}
	cfg, err := m.configManager.Load()
	if err != nil || cfg == nil {
		return wallet.DisplayUnitBTC
	}
	switch cfg.DisplayUnit {
	case config.DisplayUnitBitcoin:
		return wallet.DisplayUnitBitcoin
	default:
		return wallet.DisplayUnitBTC
	}
}

func (m AddressesModel) View() string {
	// Header with mode indicator
	modeLabel := "Receive Addresses"
	if m.viewMode == 1 {
		modeLabel = "Change Addresses"
	}

	header := styles.ScreenHeaderStyle.Render(modeLabel)

	// Copy status badge
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ "+m.copyMsg)
	}

	// Stats
	var addrs []AddrDisplayInfo
	if m.viewMode == 0 {
		addrs = m.receiveAddresses
	} else {
		addrs = m.changeAddresses
	}

	usedCount := 0
	var totalBalance api.Satoshi
	for _, addr := range addrs {
		if addr.Used {
			usedCount++
		}
		totalBalance += addr.Balance
	}

	displayUnit := m.getDisplayUnit()
	balanceStr := wallet.FormatAmountCompact(totalBalance, displayUnit)

	statsLine := styles.DimText.Render(fmt.Sprintf(
		"%d used • %s total", usedCount, balanceStr))

	// Tab indicator
	receiveTab := styles.DimText.Render("Receive")
	changeTab := styles.DimText.Render("Change")
	if m.viewMode == 0 {
		receiveTab = styles.HighlightText.Render("[Receive]")
	} else {
		changeTab = styles.HighlightText.Render("[Change]")
	}
	tabs := receiveTab + "  " + changeTab

	// Table content — wrap in border for visual consistency with history screen
	var content string
	if m.loading {
		content = styles.DimText.Render("Loading addresses...")
	} else if m.err != nil {
		content = styles.ErrorText.Render(fmt.Sprintf("Error: %v", m.err))
	} else if len(addrs) == 0 {
		content = styles.InfoBoxStyle.Render("No addresses found\n\n" +
			styles.DimText.Render("Addresses will appear after wallet sync completes."))
	} else {
		tableView := strings.TrimSuffix(m.table.View(), "\n")
		// Bubble table can expose one extra body row when cursor moves near top.
		// Clamp rendered body rows to the viewport height for stable row count.
		lines := strings.Split(tableView, "\n")
		if len(lines) > 2 {
			headerLines := lines[:2]
			bodyLines := lines[2:]
			maxBody := m.table.Height()
			if maxBody > 0 && len(bodyLines) > maxBody {
				bodyLines = bodyLines[:maxBody]
			}
			tableView = strings.Join(append(headerLines, bodyLines...), "\n")
		}
		content = styles.TableBorderStyle.Render(tableView)
	}

	// Help footer
	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("PgUp/PgDn", "scroll"),
		styles.HK("tab", "recv/change"),
		styles.HK("c", "copy"),
		styles.HK("esc", "back"),
	)

	// Build layout
	output := strings.Join([]string{
		header + copyStatus,
		tabs,
		statsLine,
		"",
		content,
		help,
	}, "\n")

	return output
}
