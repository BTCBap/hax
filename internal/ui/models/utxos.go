package models

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"hax/internal/api"
	"hax/internal/config"
	"hax/internal/ui/styles"
	"hax/internal/wallet"
)

// UTXOsModel displays unspent transaction outputs like Sparrow's UTXOs tab
type UTXOsModel struct {
	width  int
	height int

	walletManager WalletService
	configManager *config.ConfigManager

	// Data - using cached state only (no network calls)
	utxos   []api.UTXO
	loading bool
	err     error

	// Table
	table       table.Model
	selectedIdx int

	// Pagination
	currentPage  int
	itemsPerPage int

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
}

// UTXOsInitMsg triggers initial load
type UTXOsInitMsg struct{}

func NewUTXOsModel(walletManager WalletService, configManager *config.ConfigManager) UTXOsModel {
	columns := []table.Column{
		{Title: "Date", Width: 14},
		{Title: "Output", Width: 20},
		{Title: "Label", Width: 15},
		{Title: "Value", Width: 14},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	t.SetStyles(styles.NewTableStyles())

	labelInput := textinput.New()
	labelInput.Placeholder = "Enter label for UTXO..."
	labelInput.CharLimit = 255
	labelInput.Width = 60

	return UTXOsModel{
		walletManager: walletManager,
		configManager: configManager,
		loading:       false,
		table:         t,
		itemsPerPage:  15,
		labelInput:    labelInput,
		isActive:      true,
	}
}

func (m UTXOsModel) Init() tea.Cmd {
	return func() tea.Msg {
		return UTXOsInitMsg{}
	}
}

func (m UTXOsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
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
		newHeight := m.height - 12
		if newHeight < 5 {
			newHeight = 5
		}
		m.itemsPerPage = newHeight
		m.table.SetHeight(newHeight)
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

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.table.MoveUp(1)
			m.selectedIdx = m.table.Cursor()
			return m, nil
		case tea.MouseButtonWheelDown:
			m.table.MoveDown(1)
			m.selectedIdx = m.table.Cursor()
			return m, nil
		}

	case tea.KeyMsg:
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
				if err := clipboard.WriteAll(outpoint); err == nil {
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

		case "r":
			// Refresh from cached state
			state := m.walletManager.GetState()
			m.utxos = state.UTXOs
			m.sortAndUpdateTable()

		case "up", "k":
			m.table.MoveUp(1)
			m.selectedIdx = m.table.Cursor()
		case "down", "j":
			m.table.MoveDown(1)
			m.selectedIdx = m.table.Cursor()

		case "left", "h":
			// Previous page
			if m.currentPage > 0 {
				m.currentPage--
				m.sortAndUpdateTable()
				m.table.GotoTop()
				m.selectedIdx = 0
			}
		case "right", "l":
			// Next page
			maxPage := (len(m.utxos) - 1) / m.itemsPerPage
			if m.currentPage < maxPage {
				m.currentPage++
				m.sortAndUpdateTable()
				m.table.GotoTop()
				m.selectedIdx = 0
			}

		case "pgup":
			m.table.MoveUp(10)
			m.selectedIdx = m.table.Cursor()
		case "pgdown":
			m.table.MoveDown(10)
			m.selectedIdx = m.table.Cursor()
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *UTXOsModel) getAbsoluteIndex() int {
	return m.currentPage*m.itemsPerPage + m.selectedIdx
}

func (m *UTXOsModel) sortAndUpdateTable() {
	displayUnit := m.getDisplayUnit()

	// Build txid -> blocktime map from transactions for date lookup
	txBlockTimes := make(map[string]int64)
	state := m.walletManager.GetState()
	for _, tx := range state.Transactions {
		if tx.Status.Confirmed && tx.Status.BlockTime > 0 {
			txBlockTimes[tx.TxID] = tx.Status.BlockTime
		}
	}

	// Sort UTXOs by value descending (largest first, like Sparrow)
	sortedUTXOs := make([]api.UTXO, len(m.utxos))
	copy(sortedUTXOs, m.utxos)
	sort.Slice(sortedUTXOs, func(i, j int) bool {
		return sortedUTXOs[i].Value > sortedUTXOs[j].Value
	})
	m.utxos = sortedUTXOs

	// Calculate pagination
	startIdx := m.currentPage * m.itemsPerPage
	endIdx := startIdx + m.itemsPerPage
	if endIdx > len(m.utxos) {
		endIdx = len(m.utxos)
	}
	if startIdx > len(m.utxos) {
		startIdx = len(m.utxos)
	}

	pageUTXOs := m.utxos[startIdx:endIdx]

	var rows []table.Row
	for _, utxo := range pageUTXOs {
		// Date - look up from transactions cache, color-code pending
		var dateStr string
		blockTime := txBlockTimes[utxo.TxID]
		if blockTime > 0 {
			t := time.Unix(blockTime, 0)
			dateStr = t.Format("01-02 15:04")
		} else if utxo.Status.Confirmed {
			dateStr = "confirmed"
		} else {
			dateStr = styles.StatusPendingStyle.Render("pending")
		}

		// Output (txid:vout truncated)
		outpoint := fmt.Sprintf("%s..:%d", utxo.TxID[:8], utxo.Vout)

		// Label (from transaction labels)
		label := m.walletManager.GetTransactionLabel(utxo.TxID)
		if len(label) > 15 {
			label = label[:14] + "…"
		}
		if label == "" {
			label = "-"
		}

		// Value
		valueStr := wallet.FormatAmountCompact(utxo.Value, displayUnit)

		rows = append(rows, table.Row{
			dateStr,
			outpoint,
			label,
			valueStr,
		})
	}

	m.table.SetRows(rows)
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

func (m UTXOsModel) View() string {
	header := styles.ScreenHeaderStyle.Render("Unspent Transaction Outputs")

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

	// Stats box — responsive width
	displayUnit := m.getDisplayUnit()
	var confirmedBalance, pendingBalance api.Satoshi
	for _, utxo := range m.utxos {
		if utxo.Status.Confirmed {
			confirmedBalance += utxo.Value
		} else {
			pendingBalance += utxo.Value
		}
	}

	balanceStr := wallet.FormatAmountCompact(confirmedBalance, displayUnit)
	mempoolStr := wallet.FormatAmountCompact(pendingBalance, displayUnit)

	statsContent := fmt.Sprintf("%s %s\n%s %s\n%s %d",
		styles.DimText.Render("Balance:"),
		styles.HighlightText.Render(balanceStr),
		styles.DimText.Render("Mempool:"),
		styles.HighlightText.Render(mempoolStr),
		styles.DimText.Render("UTXOs:"),
		len(m.utxos),
	)

	statsWidth := styles.ClampWidth(30, m.width)
	statsBox := styles.InfoBoxStyle.
		Width(statsWidth).
		Padding(0, 2).
		MarginBottom(0).
		Render(statsContent)

	// Table content — wrap in border for visual consistency
	var content string
	if m.loading {
		content = styles.DimText.Render("Loading UTXOs...")
	} else if m.err != nil {
		content = styles.ErrorText.Render(fmt.Sprintf("Error: %v", m.err))
	} else if len(m.utxos) == 0 {
		content = styles.InfoBoxStyle.Render("No UTXOs found\n\n" +
			styles.DimText.Render("UTXOs will appear after receiving Bitcoin."))
	} else {
		tableWidth := styles.ClampWidth(72, m.width)
		content = styles.TableBorderStyle.Width(tableWidth).Render(m.table.View())
	}

	// Pagination indicator
	totalPages := (len(m.utxos) + m.itemsPerPage - 1) / m.itemsPerPage
	if totalPages == 0 {
		totalPages = 1
	}
	pageInfo := ""
	if totalPages > 1 {
		pageInfo = styles.Badge(styles.PastelPurple).
			Render(fmt.Sprintf("Page %d/%d", m.currentPage+1, totalPages))
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("←/→", "page"),
		styles.HK("e", "edit label"),
		styles.HK("c", "copy"),
		styles.HK("esc", "back"),
	)

	contentOut := strings.Join([]string{
		header + copyStatus,
		"",
		statsBox,
		"",
		content,
		pageInfo,
		help,
	}, "\n")

	return contentOut
}
