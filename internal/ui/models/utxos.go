package models

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// utxoSortKey identifies the column the UTXO list is sorted by.
type utxoSortKey int

const (
	utxoSortValue utxoSortKey = iota
	utxoSortAge
)

// Column widths for the UTXO table. They add up to 72 cells, the same content
// width the screen has always used, so every row is exactly as wide as the
// surrounding box at 80 columns.
const (
	utxoContentWidth = 72
	utxoGap          = 1
)

type utxoColumns struct {
	marker int
	value  int
	age    int
	conf   int
	addr   int
	label  int
	frozen int
	helper int
}

func newUTXOColumns(contentWidth int) utxoColumns {
	if contentWidth < 20 {
		contentWidth = 20
	}
	c := utxoColumns{
		marker: 1,
		value:  14,
		age:    5,
		conf:   7,
		addr:   12,
		label:  14,
		frozen: 1,
		helper: 11,
	}
	// Shrink the flexible columns on narrow terminals. The 80-column layout
	// above is used unchanged because its total is exactly 72.
	mins := []struct {
		width *int
		min   int
	}{
		{&c.helper, 4},
		{&c.label, 4},
		{&c.addr, 6},
		{&c.age, 3},
		{&c.conf, 5},
		{&c.value, 8},
	}
	for c.total() > contentWidth {
		changed := false
		for _, m := range mins {
			if c.total() <= contentWidth {
				break
			}
			if *m.width > m.min {
				*m.width--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return c
}

func (c utxoColumns) total() int {
	widths := []int{c.marker, c.value, c.age, c.conf, c.addr, c.label, c.frozen, c.helper}
	total, visible := 0, 0
	for _, w := range widths {
		if w <= 0 {
			continue
		}
		total += w
		visible++
	}
	if visible > 1 {
		total += (visible - 1) * utxoGap
	}
	return total
}

// utxoDisplayRow carries the values the table needs for one coin. It is
// rebuilt from cached wallet state; nothing here touches the network.
type utxoDisplayRow struct {
	utxo      api.UTXO
	address   string
	label     string
	frozen    bool
	change    bool
	blockTime int64
}

// UTXOsModel displays unspent transaction outputs like Sparrow's UTXOs tab
type UTXOsModel struct {
	width  int
	height int

	walletManager WalletService
	configManager *config.ConfigManager

	// Data - using cached state only (no network calls)
	utxos       []api.UTXO
	loading     bool
	err         error
	chainHeight int64

	// Current page
	page         []utxoDisplayRow
	selectedIdx  int
	currentPage  int
	itemsPerPage int

	// Sorting
	sortKey       utxoSortKey
	sortAscending bool

	// Label editing state
	editMode   bool
	editTxID   string
	labelInput textinput.Model

	// Copy notification
	showCopied bool
	copiedTime time.Time
	copyMsg    string

	// Screen visibility
	isActive bool

	// stealth hides coin values.
	stealth bool
}

// UTXOsInitMsg triggers initial load
type UTXOsInitMsg struct{}

func NewUTXOsModel(walletManager WalletService, configManager *config.ConfigManager) UTXOsModel {
	labelInput := newTextInput()
	labelInput.Placeholder = "Enter label for UTXO..."
	labelInput.CharLimit = 255
	labelInput.SetWidth(60)

	return UTXOsModel{
		walletManager: walletManager,
		configManager: configManager,
		loading:       false,
		itemsPerPage:  12,
		labelInput:    labelInput,
		isActive:      true,
	}
}

func (m UTXOsModel) Init() tea.Cmd {
	return func() tea.Msg {
		return UTXOsInitMsg{}
	}
}

func (m UTXOsModel) Update(msg tea.Msg) (screenModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case StealthToggledMsg:
		m.stealth = msg.On
		m.sortAndUpdateTable()
		return m, nil

	case UTXOsInitMsg:
		// Load UTXOs from cached state only - no network calls
		state := m.walletManager.GetState()
		m.utxos = state.UTXOs
		m.loading = false
		m.sortAndUpdateTable()
		return m, nil

	case ScreenActiveMsg:
		m.isActive = true
		state := m.walletManager.GetState()
		m.utxos = state.UTXOs
		m.sortAndUpdateTable()
		return m, nil

	case StateChangedMsg:
		m.utxos = msg.State.UTXOs
		m.sortAndUpdateTable()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.itemsPerPage = m.rowsPerPage()
		m.sortAndUpdateTable()

	case TickMsg:
		if m.showCopied && time.Since(m.copiedTime) > 2*time.Second {
			m.showCopied = false
		}
		if m.showCopied {
			cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
				return TickMsg(t)
			}))
		}

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.moveCursor(-1)
			return m, nil
		case tea.MouseWheelDown:
			m.moveCursor(1)
			return m, nil
		}

	case tea.PasteMsg:
		if m.editMode {
			var cmd tea.Cmd
			m.labelInput, cmd = m.labelInput.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.KeyPressMsg:
		// Handle edit mode separately
		if m.editMode {
			switch msg.String() {
			case "esc":
				m.editMode = false
				m.editTxID = ""
				m.labelInput.SetValue("")
				return m, nil
			case "enter":
				label := m.labelInput.Value()
				err := m.walletManager.SetTransactionLabel(m.editTxID, label)
				if err != nil {
					m.err = fmt.Errorf("failed to save label: %v", err)
				}
				m.editMode = false
				m.editTxID = ""
				m.labelInput.SetValue("")
				m.sortAndUpdateTable()
				return m, nil
			default:
				var cmd tea.Cmd
				m.labelInput, cmd = m.labelInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, Navigate(StateMainDashboard)

		case "c":
			// Copy selected UTXO outpoint (txid:vout)
			idx := m.getAbsoluteIndex()
			if idx >= 0 && idx < len(m.utxos) {
				utxo := m.utxos[idx]
				outpoint := fmt.Sprintf("%s:%d", utxo.TxID, utxo.Vout)
				if err := copyToClipboard(outpoint); err == nil {
					m.showCopied = true
					m.copyMsg = "OUTPOINT COPIED!"
					m.copiedTime = time.Now()
					cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
						return TickMsg(t)
					}))
				}
			}

		case "e":
			// Enter label edit mode for selected UTXO
			idx := m.getAbsoluteIndex()
			if idx >= 0 && idx < len(m.utxos) {
				utxo := m.utxos[idx]
				m.editMode = true
				m.editTxID = utxo.TxID
				existingLabel := m.walletManager.GetTransactionLabel(utxo.TxID)
				m.labelInput.SetValue(existingLabel)
				m.labelInput.Focus()
				return m, textinput.Blink
			}

		case "f":
			// Freeze or unfreeze the selected coin
			idx := m.getAbsoluteIndex()
			if idx >= 0 && idx < len(m.utxos) {
				u := m.utxos[idx]
				frozen := !m.walletManager.IsUTXOFrozen(u.TxID, u.Vout)
				if err := m.walletManager.SetUTXOFrozen(u.TxID, u.Vout, frozen); err != nil {
					m.err = fmt.Errorf("failed to freeze coin: %v", err)
					return m, nil
				}
				m.copyMsg = "COIN UNFROZEN"
				if frozen {
					m.copyMsg = "COIN FROZEN"
				}
				m.showCopied = true
				m.copiedTime = time.Now()
				m.sortAndUpdateTable()
				cmds = append(cmds, tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
					return TickMsg(t)
				}))
			}

		case "r":
			// Refresh from cached state
			state := m.walletManager.GetState()
			m.utxos = state.UTXOs
			m.sortAndUpdateTable()

		case "1":
			// Sort by value (largest first, the long-standing default).
			m.setSort(utxoSortValue)
			m.sortAndUpdateTable()
		case "2":
			// Sort by age (newest first).
			m.setSort(utxoSortAge)
			m.sortAndUpdateTable()

		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)

		case "left", "h":
			// Previous page
			if m.currentPage > 0 {
				m.currentPage--
				m.sortAndUpdateTable()
				m.selectedIdx = 0
			}
		case "right", "l":
			// Next page
			maxPage := (len(m.utxos) - 1) / m.itemsPerPage
			if m.currentPage < maxPage {
				m.currentPage++
				m.sortAndUpdateTable()
				m.selectedIdx = 0
			}

		case "pgup":
			m.moveCursor(-10)
		case "pgdown":
			m.moveCursor(10)
		}
	}

	return m, tea.Batch(cmds...)
}

// rowsPerPage keeps the table box within the terminal at 80x24 and 120x40.
func (m UTXOsModel) rowsPerPage() int {
	rows := m.height - 12
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *UTXOsModel) moveCursor(delta int) {
	if len(m.page) == 0 {
		m.selectedIdx = 0
		return
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(m.page) {
		m.selectedIdx = len(m.page) - 1
	}
}

func (m *UTXOsModel) setSort(key utxoSortKey) {
	if m.sortKey == key {
		m.sortAscending = !m.sortAscending
		return
	}
	m.sortKey = key
	m.sortAscending = false
}

func (m *UTXOsModel) getAbsoluteIndex() int {
	return m.currentPage*m.itemsPerPage + m.selectedIdx
}

// CapturesTextInput reports whether the label editor owns the keyboard.
func (m UTXOsModel) CapturesTextInput() bool {
	return m.editMode
}

func (m *UTXOsModel) sortAndUpdateTable() {
	if m.itemsPerPage < 1 {
		m.itemsPerPage = 1
	}

	state := m.walletManager.GetState()
	m.chainHeight = state.BlockHeight
	txByID := make(map[string]api.Transaction, len(state.Transactions))
	blockTimes := make(map[string]int64, len(state.Transactions))
	for _, tx := range state.Transactions {
		txByID[tx.TxID] = tx
		if tx.Status.BlockTime > 0 {
			blockTimes[tx.TxID] = tx.Status.BlockTime
		}
	}

	sortedUTXOs := make([]api.UTXO, len(m.utxos))
	copy(sortedUTXOs, m.utxos)
	sort.SliceStable(sortedUTXOs, func(i, j int) bool {
		a, b := sortedUTXOs[i], sortedUTXOs[j]
		switch m.sortKey {
		case utxoSortAge:
			at, bt := utxoBlockTime(a, blockTimes), utxoBlockTime(b, blockTimes)
			aPending, bPending := !a.Status.Confirmed, !b.Status.Confirmed
			if aPending != bPending {
				// Newest first treats pending coins as the newest; oldest
				// first keeps them at the bottom.
				return aPending == !m.sortAscending
			}
			if at != bt {
				if m.sortAscending {
					return at < bt
				}
				return at > bt
			}
		default:
			if a.Value != b.Value {
				if m.sortAscending {
					return a.Value < b.Value
				}
				return a.Value > b.Value
			}
		}
		if a.TxID != b.TxID {
			return a.TxID < b.TxID
		}
		return a.Vout < b.Vout
	})
	m.utxos = sortedUTXOs

	maxPage := 0
	if len(m.utxos) > 0 {
		maxPage = (len(m.utxos) - 1) / m.itemsPerPage
	}
	if m.currentPage > maxPage {
		m.currentPage = maxPage
	}
	if m.currentPage < 0 {
		m.currentPage = 0
	}

	startIdx := m.currentPage * m.itemsPerPage
	endIdx := startIdx + m.itemsPerPage
	if startIdx > len(m.utxos) {
		startIdx = len(m.utxos)
	}
	if endIdx > len(m.utxos) {
		endIdx = len(m.utxos)
	}

	changeAddresses := m.changeAddresses()
	page := make([]utxoDisplayRow, 0, endIdx-startIdx)
	for _, utxo := range m.utxos[startIdx:endIdx] {
		row := utxoDisplayRow{
			utxo:      utxo,
			frozen:    m.walletManager.IsUTXOFrozen(utxo.TxID, utxo.Vout),
			blockTime: utxoBlockTime(utxo, blockTimes),
		}
		if tx, ok := txByID[utxo.TxID]; ok && int(utxo.Vout) < len(tx.Vout) {
			row.address = tx.Vout[utxo.Vout].ScriptpubkeyAddress
		}
		row.change = changeAddresses[strings.ToLower(row.address)]
		row.label = sanitizeLine(m.walletManager.GetTransactionLabel(utxo.TxID))
		page = append(page, row)
	}
	m.page = page

	if m.selectedIdx >= len(m.page) {
		m.selectedIdx = len(m.page) - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
}

// utxoBlockTime resolves when a coin was confirmed, using the transaction
// cache when the UTXO itself does not carry the timestamp.
func utxoBlockTime(utxo api.UTXO, blockTimes map[string]int64) int64 {
	if utxo.Status.BlockTime > 0 {
		return utxo.Status.BlockTime
	}
	return blockTimes[utxo.TxID]
}

// changeAddresses is the cheap, in-memory change set from the wallet's
// account data.
func (m UTXOsModel) changeAddresses() map[string]bool {
	change := make(map[string]bool)
	if m.walletManager == nil {
		return change
	}
	for _, account := range m.walletManager.GetAccounts() {
		for _, addr := range account.Addresses {
			if addr.Change && addr.Address != "" {
				change[strings.ToLower(addr.Address)] = true
			}
		}
	}
	return change
}

func (m UTXOsModel) getDisplayUnit() wallet.DisplayUnit {
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

func (m UTXOsModel) headerTitle(sortKey utxoSortKey, title string) string {
	if m.sortKey != sortKey {
		return title
	}
	if m.sortAscending {
		return title + " ↑"
	}
	return title + " ↓"
}

func (m UTXOsModel) renderHeader(cols utxoColumns) string {
	headerStyle := lipgloss.NewStyle().Foreground(styles.BitcoinGold).Bold(true)
	cells := []string{
		padCell("", cols.marker),
		padLeftCell(m.headerTitle(utxoSortValue, "Value"), cols.value),
		padLeftCell(m.headerTitle(utxoSortAge, "Age"), cols.age),
		padLeftCell("Confs", cols.conf),
		padCell("Address", cols.addr),
		padCell("Label", cols.label),
		padCell("", cols.frozen),
		padCell("", cols.helper),
	}
	return headerStyle.Render(strings.Join(cells, strings.Repeat(" ", utxoGap)))
}

func (m UTXOsModel) renderRow(index int, row utxoDisplayRow, cols utxoColumns, displayUnit wallet.DisplayUnit) string {
	selected := index == m.selectedIdx
	marker := " "
	markerStyle := styles.DimText
	bodyStyle := styles.NormalText
	if selected {
		marker = "›"
		markerStyle = styles.HighlightText
		bodyStyle = styles.SelectedItemStyle
	}
	if row.frozen {
		bodyStyle = styles.DimText
	}

	value := maskAmountRight(wallet.FormatAmountCompact(row.utxo.Value, displayUnit), m.stealth)
	conf := "pending"
	if row.utxo.Status.Confirmed {
		conf = "—"
		if row.utxo.Status.BlockHeight > 0 {
			chainHeight := m.chainHeight
			if chainHeight > 0 {
				confirmations := chainHeight - row.utxo.Status.BlockHeight + 1
				if confirmations < 1 {
					confirmations = 1
				}
				conf = wallet.FormatSats(api.Satoshi(confirmations))
			}
		}
	}

	address := "—"
	if row.address != "" {
		address = ui.FormatAddress(row.address, cols.addr)
	}
	label := "-"
	if row.label != "" {
		label = truncateRunes(row.label, cols.label, "…")
	}
	frozenMark := " "
	if row.frozen {
		frozenMark = "❄"
	}

	helper := ""
	if selected {
		helper = utxoHelper(row)
	}

	cells := []string{
		padCell(marker, cols.marker),
		padLeftCell(value, cols.value),
		padLeftCell(formatUTXOAge(row), cols.age),
		padLeftCell(conf, cols.conf),
		padCell(address, cols.addr),
		padCell(label, cols.label),
		padCell(frozenMark, cols.frozen),
		padCell(helper, cols.helper),
	}

	// The marker keeps the accent color even for a frozen (dim) row.
	cells[0] = markerStyle.Render(cells[0])
	for i := 1; i < len(cells); i++ {
		cells[i] = bodyStyle.Render(cells[i])
	}
	cells[len(cells)-1] = styles.DimText.Render(padCell(helper, cols.helper))
	return strings.Join(cells, strings.Repeat(" ", utxoGap))
}

func utxoHelper(row utxoDisplayRow) string {
	switch {
	case row.frozen:
		// The frozen column already marks this coin with ❄.
		return ""
	case !row.utxo.Status.Confirmed:
		return "unconfirmed"
	case row.utxo.Value < 1000:
		return "dust"
	case row.change:
		return "change"
	default:
		return ""
	}
}

func formatUTXOAge(row utxoDisplayRow) string {
	if !row.utxo.Status.Confirmed {
		return "now"
	}
	if row.blockTime <= 0 {
		return "—"
	}
	age := time.Since(time.Unix(row.blockTime, 0))
	switch {
	case age < time.Hour:
		return "<1h"
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	case age < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	case age < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(age.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(age.Hours()/(24*365)))
	}
}

func (m UTXOsModel) summaryLine(displayUnit wallet.DisplayUnit) string {
	var total, mempool, frozen api.Satoshi
	frozenCount := 0
	for _, utxo := range m.utxos {
		total += utxo.Value
		if !utxo.Status.Confirmed {
			mempool += utxo.Value
		}
		if m.walletManager.IsUTXOFrozen(utxo.TxID, utxo.Vout) {
			frozen += utxo.Value
			frozenCount++
		}
	}

	totalStr := maskAmount(wallet.FormatAmountCompact(total, displayUnit), m.stealth)
	// Mempool and frozen already follow the total, so they drop the unit to
	// keep the line compact. Both segments are always rendered so the
	// remaining text never shifts.
	mempoolStr := maskAmount(summaryAmount(mempool, displayUnit), m.stealth)
	frozenStr := maskAmount(summaryAmount(frozen, displayUnit), m.stealth)

	return strings.Join([]string{
		styles.DimText.Render(fmt.Sprintf("%d coins", len(m.utxos))),
		styles.HighlightText.Render(totalStr),
		styles.DimText.Render("mempool ") + styles.HighlightText.Render(mempoolStr),
		styles.DimText.Render("frozen ") + styles.HighlightText.Render(frozenStr) +
			styles.DimText.Render(fmt.Sprintf(" (%d)", frozenCount)),
	}, " · ")
}

// summaryAmount formats an amount without its unit suffix for the trailing
// totals on the coins screen.
func summaryAmount(value api.Satoshi, unit wallet.DisplayUnit) string {
	amount, _ := splitAmountUnit(wallet.FormatAmountCompact(value, unit))
	return amount
}

func (m UTXOsModel) tableBox(cols utxoColumns, displayUnit wallet.DisplayUnit) string {
	lines := make([]string, 0, m.itemsPerPage+3)
	lines = append(lines, m.renderHeader(cols))
	lines = append(lines, styles.DimText.Render(strings.Repeat("─", cols.total())))
	for i, row := range m.page {
		lines = append(lines, m.renderRow(i, row, cols, displayUnit))
	}
	// Keep the frame height fixed even on a partly filled page.
	for len(lines) < m.itemsPerPage+2 {
		lines = append(lines, strings.Repeat(" ", cols.total()))
	}
	return styles.TableBorderStyle.Width(cols.total() + 4).Render(strings.Join(lines, "\n"))
}

func (m UTXOsModel) View() string {
	header := styles.ScreenHeaderStyle.Render("Unspent Transaction Outputs") + stealthBadge(m.stealth)

	// Copy status badge
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ "+m.copyMsg)
	}

	// Edit mode indicator
	if m.editMode {
		editBox := styles.EditBoxStyle.Render("✏️  Edit Label: " + m.labelInput.View())

		help := styles.RenderHelp(
			styles.HK("enter", "save"),
			styles.HK("esc", "cancel"),
		)

		return strings.Join([]string{
			header,
			"",
			editBox,
			"",
			help,
		}, "\n")
	}

	displayUnit := m.getDisplayUnit()
	cols := newUTXOColumns(m.contentWidth())

	var content string
	switch {
	case m.loading:
		content = styles.DimText.Render("Loading UTXOs...")
	case m.err != nil:
		content = styles.RenderErrorBox(fmt.Sprintf("Error: %v", m.err), m.width)
	case len(m.utxos) == 0:
		content = styles.InfoBoxStyle.Render("No UTXOs found\n\n" +
			styles.DimText.Render("UTXOs will appear after receiving Bitcoin."))
	default:
		content = m.tableBox(cols, displayUnit)
	}

	helpWidth := m.width
	if helpWidth < utxoContentWidth+4 {
		helpWidth = utxoContentWidth + 4
	}
	help := styles.RenderHelpWrapped(helpWidth,
		styles.HK("↑/↓", "navigate"),
		styles.HK("←/→", "page"),
		styles.HK("e", "edit label"),
		styles.HK("c", "copy"),
		styles.HK("f", "freeze"),
		styles.HK("1/2", "sort"),
		styles.HK("esc", "back"),
	)

	return strings.Join([]string{
		header + copyStatus,
		"",
		m.summaryLine(displayUnit),
		"",
		content,
		"",
		help,
	}, "\n")
}

func (m UTXOsModel) contentWidth() int {
	width := utxoContentWidth
	if m.width > 0 && m.width-4 < width {
		width = m.width - 4
	}
	if width < 20 {
		width = 20
	}
	return width
}

// padLeftCell pads text on the left so right-aligned columns keep a fixed
// right edge.
func padLeftCell(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = truncateRunes(text, width, "…")
	if pad := width - lipgloss.Width(text); pad > 0 {
		text = strings.Repeat(" ", pad) + text
	}
	return text
}
