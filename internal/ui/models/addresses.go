package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
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
	Path    string
	Index   uint32
	Used    bool
	Balance api.Satoshi
	Label   string
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

	// Label editing
	labelCol   bool // table has a Label column
	labelColW  int
	editing    bool
	editAddr   string
	labelInput textinput.Model

	// Hardware wallet address verification
	verify AddressVerifyView

	// stealth hides address balances.
	stealth bool
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

	li := textinput.New()
	li.Placeholder = "Label for this address"
	li.CharLimit = 255
	li.Width = 40

	return AddressesModel{
		labelInput:    li,
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
	case StealthToggledMsg:
		m.stealth = msg.On
		m.updateTable()
		return m, nil

	case AddressesInitMsg:
		m.loadAddresses()
		m.loading = false
		m.updateTable()
		m.table.Focus()
		m.table.GotoTop()
		m.syncVerifyAddress()
		return m, nil

	case ScreenActiveMsg:
		m.isActive = true
		m.loadAddresses()
		m.updateTable()
		m.table.Focus()
		m.table.GotoTop()
		m.syncVerifyAddress()
		return m, nil

	case StateChangedMsg:
		m.loadAddresses()
		m.updateTable()
		m.syncVerifyAddress()
		return m, nil

	case AddressVerifyTickMsg:
		if m.verify.IsWaiting() {
			m.verify.Advance()
			cmds = append(cmds, AddressVerifyTickCmd())
		}
		return m, tea.Batch(cmds...)

	case HWIVerifyMsg:
		if m.verify.IsWaiting() && (msg.Attempt == 0 || msg.Attempt == m.verify.Attempt()) {
			m.verify.Complete(msg.Err)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeColumnsForWidth()
		// Keep table height stable across scroll/selection states.
		tableHeight := m.height - 9 // leaves a line for the label
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
		if m.editing {
			return m.handleLabelKey(msg)
		}
		if m.verify.Active() {
			switch msg.String() {
			case "esc":
				m.verify.Cancel()
				return m, nil
			case "v":
				if a, ok := m.selected(); ok {
					m.beginVerify(a)
					cmds = append(cmds, AddressVerifyTickCmd(), m.verifyAddressOnDevice(a))
				}
			}
			return m, tea.Batch(cmds...)
		}
		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, Navigate(StateMainDashboard)

		case "tab":
			// Toggle between receive and change addresses
			m.viewMode = (m.viewMode + 1) % 2
			m.updateTable()
			m.table.GotoTop()
			m.syncVerifyAddress()

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
				if err := copyToClipboard(addr); err == nil {
					m.showCopied = true
					m.copyMsg = "ADDRESS COPIED!"
					m.copiedTime = time.Now()
					cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
						return TickMsg(t)
					}))
				}
			}

		case "e":
			if a, ok := m.selected(); ok {
				m.editing = true
				m.editAddr = a.Address
				m.err = nil
				m.labelInput.SetValue(a.Label)
				m.labelInput.CursorEnd()
				return m, m.labelInput.Focus()
			}

		case "v":
			if m.walletManager.IsHardwareWallet() && !m.walletManager.IsMultisig() {
				if a, ok := m.selected(); ok && a.Address != "" && a.Path != "" {
					m.beginVerify(a)
					cmds = append(cmds, AddressVerifyTickCmd(), m.verifyAddressOnDevice(a))
					return m, tea.Batch(cmds...)
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
		m.syncVerifyAddress()
		if tableCmd != nil {
			cmds = append(cmds, tableCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// syncVerifyAddress resets the verification result when the selected address
// changes. Results stay visible while the same address is selected.
func (m *AddressesModel) syncVerifyAddress() {
	a, ok := m.selected()
	if !ok || !m.walletManager.IsHardwareWallet() || m.walletManager.IsMultisig() {
		if m.verify.Address != "" || m.verify.State != AddressVerifyIdle {
			m.verify.Reset("", "")
		}
		return
	}
	if m.verify.Address != a.Address || m.verify.Path != a.Path {
		m.verify.Reset(a.Address, a.Path)
	}
}

// beginVerify points the panel at a and starts a new device confirmation.
func (m *AddressesModel) beginVerify(a AddrDisplayInfo) {
	if m.verify.Address != a.Address || m.verify.Path != a.Path {
		m.verify.Reset(a.Address, a.Path)
	}
	m.verify.Begin()
}

// verifyAddressOnDevice asks the hardware wallet to display the selected
// address so the user can compare it.
func (m AddressesModel) verifyAddressOnDevice(a AddrDisplayInfo) tea.Cmd {
	walletMgr := m.walletManager
	address := a.Address
	path := a.Path
	attempt := m.verify.Attempt()
	return func() tea.Msg {
		return HWIVerifyMsg{
			Err:     walletMgr.DisplayAddressOnDevice(address, path),
			Attempt: attempt,
		}
	}
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

	cols := []table.Column{
		{Title: "#", Width: indexW},
		{Title: "Address", Width: addressW},
		{Title: "Status", Width: statusW},
		{Title: "Balance", Width: balanceW},
	}
	// Wide terminals get a Label column; narrow ones show the selected
	// row's label under the table.
	if w := available - indexW - addressW - statusW - balanceW - overhead - 2; w >= addressLabelColMin {
		if w > 30 {
			w = 30
		}
		cols = append(cols, table.Column{Title: "Label", Width: w})
	}
	m.labelCol = len(cols) == 5
	m.labelColW = 0
	if m.labelCol {
		m.labelColW = cols[4].Width
	}
	m.table.SetColumns(cols)
}

// addressLabelColMin is the narrowest Label column worth showing.
const addressLabelColMin = 12

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
				Path:    addr.Path,
				Index:   addr.Index,
				Used:    addr.Used,
				Balance: balance,
				Label:   acc.Labels[addr.Address],
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
		balanceStr := maskAmount(wallet.FormatAmountCompact(addr.Balance, displayUnit), m.stealth)

		row := table.Row{
			indexStr,
			addrDisplay,
			status,
			balanceStr,
		}
		if m.labelCol {
			row = append(row, truncateRunes(sanitizeLine(addr.Label), m.labelColW, "…"))
		}
		rows = append(rows, row)
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

	header := styles.ScreenHeaderStyle.Render(modeLabel) + stealthBadge(m.stealth)

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
	balanceStr := maskAmount(wallet.FormatAmountCompact(totalBalance, displayUnit), m.stealth)

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

	// Table content — wrap in border for visual consistency with history screen.
	// In verification mode the panel replaces the list detail area.
	verifyActive := m.verify.Active()
	var content string
	if verifyActive {
		verify := m.verify
		verify.Width = m.width
		content = verify.View()
	} else if m.loading {
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
	helpItems := []styles.HelpItem{
		styles.HK("↑/↓", "navigate"),
		styles.HK("PgUp/PgDn", "scroll"),
		styles.HK("tab", "recv/change"),
		styles.HK("c", "copy"),
		styles.HK("e", "label"),
	}
	if m.walletManager.IsHardwareWallet() && !m.walletManager.IsMultisig() {
		helpItems = append(helpItems, styles.HK("v", "verify on device"))
	}
	helpItems = append(helpItems, styles.HK("esc", "back"))
	help := styles.RenderHelpWrapped(m.width, helpItems...)
	if m.editing {
		help = styles.RenderHelp(
			styles.HK("enter", "save"),
			styles.HK("esc", "cancel"),
		)
	}

	// Build layout
	parts := []string{header + copyStatus, tabs, statsLine, "", content}
	if !verifyActive {
		if l := m.labelLine(); l != "" {
			parts = append(parts, l)
		}
	}
	output := strings.Join(append(parts, help), "\n")

	return output
}

// CapturesTextInput reports whether the label editor owns the keyboard.
func (m AddressesModel) CapturesTextInput() bool {
	return m.editing
}

// selected returns the address under the cursor.
func (m AddressesModel) selected() (AddrDisplayInfo, bool) {
	addrs := m.receiveAddresses
	if m.viewMode == 1 {
		addrs = m.changeAddresses
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(addrs) {
		return AddrDisplayInfo{}, false
	}
	return addrs[idx], true
}

// handleLabelKey edits the selected address's label.
func (m AddressesModel) handleLabelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = false
		m.labelInput.Blur()
		return m, nil
	case "enter":
		if err := m.walletManager.SetAddressLabel(m.editAddr, m.labelInput.Value()); err != nil {
			m.err = fmt.Errorf("label not saved: %w", err)
		}
		m.editing = false
		m.labelInput.Blur()
		m.loadAddresses()
		m.updateTable()
		return m, nil
	}
	var cmd tea.Cmd
	m.labelInput, cmd = m.labelInput.Update(msg)
	return m, cmd
}

// labelLine shows the label editor, a save error, or the selected
// address's label.
func (m AddressesModel) labelLine() string {
	if m.editing {
		return styles.DimText.Render("Label: ") + m.labelInput.View()
	}
	if m.err != nil && !m.loading {
		return styles.ErrorText.Render(truncateRunes(m.err.Error(), 76, "…"))
	}
	a, ok := m.selected()
	if !ok {
		return ""
	}
	if a.Label == "" {
		return styles.DimText.Render("No label · press e to add one")
	}
	return styles.DimText.Render("Label: ") + styles.NormalText.Render(truncateRunes(sanitizeLine(a.Label), 68, "…"))
}
