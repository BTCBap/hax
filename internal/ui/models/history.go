package models

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/wallet"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/fsutil"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/harmonica"
)

type TransactionHistoryModel struct {
	width         int
	height        int
	walletManager WalletService

	// UI components
	table       table.Model
	viewport    viewport.Model
	spinner     spinner.Model
	searchInput textinput.Model
	labelInput  textinput.Model

	// Data
	transactions         []api.Transaction
	filteredTransactions []api.Transaction // Filtered subset for display
	loading              bool
	err                  error
	selectedRow          int // Track selected transaction via table cursor

	// Transaction visualizer overlay for the selected row.
	txFlow *TxFlowView

	// Inline speed-up tray for a pending outgoing transaction.
	speedUp *speedUpTrayState

	// Copy state
	showCopied   bool
	copiedTime   time.Time
	exportedFile string // Name of exported file (for notification)

	// Error auto-dismiss state
	errorTime time.Time

	// Sorting state
	sortColumn    int // 0=Time, 1=Type, 2=Amount, 3=Status, 4=TxID, 5=Label
	sortAscending bool

	// Filtering state
	filterType   string // "all", "in", "out"
	filterStatus string // "all", "confirmed", "pending"

	// Pagination state
	currentPage  int // Current page (0-indexed)
	itemsPerPage int // Transactions per page

	// Column widths (resizable)
	columnWidths [6]int // Widths for Time, Type, Amount, Status, TxID, Label

	// Search state
	searchMode  bool   // Whether search mode is active
	searchQuery string // Current search query

	// Label editing state
	editMode bool   // Whether in label edit mode
	editTxID string // TxID being edited

	// Animation
	fadeIn             float64
	fadeVelocity       float64
	fadeSpring         harmonica.Spring
	pulseFrame         int
	highlightIntensity float64

	// Screen visibility - controls whether tickers run
	isActive bool

	// Chain state for confirmation calculation
	blockHeight int64

	// stealth hides transaction amounts.
	stealth bool
}

// TransactionsMsg carries fetched transactions
type TransactionsMsg struct {
	Transactions []api.Transaction
	Err          error
	BlockHeight  int64 // Current chain tip for confirmation calculation
}

// SpeedUpFeeEstimatesMsg carries fee estimates for the inline speed-up tray.
type SpeedUpFeeEstimatesMsg struct {
	Estimates *api.FeeEstimates
	Err       error
}

// speedUpTrayState is the inline speed-up panel opened from a pending row.
type speedUpTrayState struct {
	tx        api.Transaction
	tray      Tray
	rates     [3]float64
	loading   bool
	baseLines int
}

const speedUpCustomIndex = 3

// speedUpRBFSequence is the highest input sequence that still signals
// BIP125 opt-in replace-by-fee.
const speedUpRBFSequence uint32 = 0xfffffffd

// TransactionAnalysis contains analyzed transaction data
type TransactionAnalysis struct {
	Type      string      // "↓ IN" or "↑ OUT"
	NetAmount api.Satoshi // Net satoshis added/subtracted from wallet
}

func NewTransactionHistoryModel(walletManager WalletService) TransactionHistoryModel {
	s := styles.NewStandardSpinner()

	defaultWidths := [6]int{16, 6, 16, 11, 20, 15}

	columns := []table.Column{
		{Title: "Time", Width: defaultWidths[0]},
		{Title: "Type", Width: defaultWidths[1]},
		{Title: "Amount", Width: defaultWidths[2]},
		{Title: "Status", Width: defaultWidths[3]},
		{Title: "TxID", Width: defaultWidths[4]},
		{Title: "Label", Width: defaultWidths[5]},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	tableStyles := table.DefaultStyles()
	tableStyles.Header = tableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(styles.BitcoinOrange).
		BorderBottom(true).
		Bold(true).
		Foreground(styles.BitcoinGold)

	tableStyles.Selected = tableStyles.Selected.
		Foreground(styles.BitcoinOrange).
		Bold(true).
		Background(lipgloss.Color(""))

	t.SetStyles(tableStyles)

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))

	searchInput := newTextInput()
	searchInput.Placeholder = "Search transactions..."
	searchInput.CharLimit = 100
	searchInput.SetWidth(50)

	labelInput := newTextInput()
	labelInput.Placeholder = "Enter label for transaction..."
	labelInput.CharLimit = 255
	labelInput.SetWidth(60)

	return TransactionHistoryModel{
		walletManager: walletManager,
		table:         t,
		viewport:      vp,
		spinner:       s,
		searchInput:   searchInput,
		labelInput:    labelInput,
		loading:       true,
		fadeSpring:    harmonica.NewSpring(harmonica.FPS(60), 4.0, 0.7),
		fadeIn:        0.0,
		selectedRow:   0,
		sortColumn:    0,
		sortAscending: false,
		filterType:    "all",
		filterStatus:  "all",
		currentPage:   0,
		itemsPerPage:  10,
		columnWidths:  defaultWidths,
		searchMode:    false,
		searchQuery:   "",
		editMode:      false,
		editTxID:      "",
	}
}

func (m TransactionHistoryModel) Init() tea.Cmd {
	m.isActive = true
	return tea.Batch(
		m.spinner.Tick,
		m.fetchTransactions(),
	)
}

func (m TransactionHistoryModel) fetchTransactions() tea.Cmd {
	return func() tea.Msg {
		state := m.walletManager.GetState()
		blockHeight := state.BlockHeight

		if cached, ok := m.walletManager.GetCachedTransactions(); ok && len(cached) > 0 {
			return TransactionsMsg{
				Transactions: cached,
				Err:          nil,
				BlockHeight:  blockHeight,
			}
		}

		txs, err := m.walletManager.GetTransactions(0)
		return TransactionsMsg{
			Transactions: txs,
			Err:          err,
			BlockHeight:  blockHeight,
		}
	}
}

func (m TransactionHistoryModel) animateTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m TransactionHistoryModel) Update(msg tea.Msg) (screenModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case StealthToggledMsg:
		m.stealth = msg.On
		m.updateTableData()
		return m, nil

	case ScreenActiveMsg:
		m.isActive = true
		if m.loading {
			return m, m.spinner.Tick
		}
		if m.fadeIn < 0.99 {
			return m, m.animateTick()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading && cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width - 4)
		reservedHeight := 17
		if m.searchMode {
			reservedHeight += 2
		}
		if m.editMode {
			reservedHeight += 2
		}

		tableHeight := m.height - reservedHeight
		if tableHeight < 6 {
			tableHeight = 6
		}
		maxTableHeight := m.height - 12
		if maxTableHeight < 6 {
			maxTableHeight = 6
		}
		if tableHeight > maxTableHeight {
			tableHeight = maxTableHeight
		}
		m.table.SetHeight(tableHeight)
		m.itemsPerPage = tableHeight
		m.viewport.SetHeight(m.height - tableHeight - 14)
		if m.viewport.Height() < 5 {
			m.viewport.SetHeight(5)
		}
		m.updateTableData()
		if m.speedUp != nil {
			m.speedUp.tray = m.speedUp.tray.WithWidth(m.trayWidth())
			m.speedUp.baseLines = m.linesWithoutTray()
		}

	case TransactionsMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.errorTime = time.Now()
			cmds = append(cmds, m.animateTick())
		} else {
			m.transactions = msg.Transactions
			if msg.BlockHeight > 0 {
				m.blockHeight = msg.BlockHeight
			}
			m.updateTableData()
			m.fadeIn = 0.0
			cmds = append(cmds, m.animateTick())
		}

	case SpeedUpFeeEstimatesMsg:
		if m.speedUp != nil && m.speedUp.loading {
			m.speedUp.loading = false
			m.rebuildSpeedUpTray(msg.Estimates)
		}

	case StateChangedMsg:
		blockHeightChanged := msg.State.BlockHeight > 0 && msg.State.BlockHeight != m.blockHeight
		if msg.State.BlockHeight > 0 {
			m.blockHeight = msg.State.BlockHeight
		}

		if len(msg.State.Transactions) > 0 {
			txsChanged := len(m.transactions) != len(msg.State.Transactions)
			if !txsChanged && len(m.transactions) > 0 && len(msg.State.Transactions) > 0 {
				txsChanged = m.transactions[0].TxID != msg.State.Transactions[0].TxID
			}

			if txsChanged {
				m.transactions = msg.State.Transactions
				m.updateTableData()
				m.loading = false
				m.fadeIn = 0.0
				cmds = append(cmds, m.animateTick())
			} else if blockHeightChanged {
				m.updateTableData()
			}
		} else if blockHeightChanged && len(m.transactions) > 0 {
			m.updateTableData()
		}

	case TickMsg:
		if !m.isActive {
			return m, nil
		}

		needsAnimation := false

		if m.fadeIn < 0.99 {
			m.fadeIn, m.fadeVelocity = m.fadeSpring.Update(
				m.fadeIn, m.fadeVelocity, 1.0,
			)
			needsAnimation = true
		} else {
			m.fadeIn = 1.0
			m.fadeVelocity = 0
		}

		m.pulseFrame++
		m.highlightIntensity = 0.5 + 0.5*float64(m.pulseFrame%120)/120.0

		if m.showCopied {
			if time.Since(m.copiedTime) > 2*time.Second {
				m.showCopied = false
				m.exportedFile = ""
			} else {
				needsAnimation = true
			}
		}

		if m.err != nil {
			if time.Since(m.errorTime) > 3*time.Second {
				m.err = nil
			} else {
				needsAnimation = true
			}
		}

		if needsAnimation {
			cmds = append(cmds, m.animateTick())
		}

	case tea.PasteMsg:
		if m.editMode {
			m.labelInput, cmd = m.labelInput.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.searchMode {
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.searchQuery = m.searchInput.Value()
			m.currentPage = 0
			m.updateTableData()
			cmds = append(cmds, cmd)
		}

	case tea.KeyPressMsg:
		// The speed-up tray owns every key while it is open.
		if m.speedUp != nil {
			if handled, cmd := m.handleSpeedUpKey(msg); handled {
				return m, cmd
			}
		}

		// The visualizer owns every key while it is open.
		if m.txFlow != nil {
			if handleTxFlowKey(m.txFlow, msg.String()) {
				m.txFlow = nil
			}
			return m, nil
		}

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
					m.errorTime = time.Now()
					return m, m.animateTick()
				}
				m.editMode = false
				m.editTxID = ""
				m.labelInput.SetValue("")
				m.updateTableData()
				return m, nil
			default:
				var cmd tea.Cmd
				m.labelInput, cmd = m.labelInput.Update(msg)
				return m, cmd
			}
		}

		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.currentPage = 0
				m.updateTableData()
				return m, nil
			case "enter":
				m.searchMode = false
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.searchQuery = m.searchInput.Value()
				m.currentPage = 0
				m.updateTableData()
				return m, cmd
			}
		}

		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, Navigate(StateMainDashboard)
		case "r", "f5":
			m.loading = true
			m.fadeIn = 0.0
			cmds = append(cmds, m.fetchTransactions())

		case "c":
			if len(m.filteredTransactions) > 0 {
				absoluteIndex := m.currentPage*m.itemsPerPage + m.selectedRow
				if absoluteIndex >= 0 && absoluteIndex < len(m.filteredTransactions) {
					tx := m.filteredTransactions[absoluteIndex]
					cmd, err := copyToClipboard(tx.TxID)
					if err == nil || cmd != nil {
						m.showCopied = true
						m.copiedTime = time.Now()
						cmds = append(cmds, m.animateTick(), cmd)
					} else {
						m.err = fmt.Errorf("failed to copy to clipboard: %v", err)
						m.errorTime = time.Now()
						cmds = append(cmds, m.animateTick())
					}
				}
			}

		case "v":
			if tx, ok := m.selectedTransaction(); ok {
				m.openTransactionFlow(tx)
				return m, nil
			}

		case "enter":
			if len(m.filteredTransactions) > 0 {
				absoluteIndex := m.currentPage*m.itemsPerPage + m.selectedRow
				if absoluteIndex >= 0 && absoluteIndex < len(m.filteredTransactions) {
					tx := m.filteredTransactions[absoluteIndex]
					_ = openTransactionInBrowser(walletNetwork(m.walletManager), tx.TxID)
				}
			}

		case "b":
			if len(m.filteredTransactions) > 0 {
				absoluteIndex := m.currentPage*m.itemsPerPage + m.selectedRow
				if absoluteIndex >= 0 && absoluteIndex < len(m.filteredTransactions) {
					tx := m.filteredTransactions[absoluteIndex]
					if tx.Status.Confirmed {
						m.err = fmt.Errorf("already confirmed, nothing to speed up")
						m.errorTime = time.Now()
						cmds = append(cmds, m.animateTick())
					} else if m.canSpeedUpTransaction(tx) {
						cmd = m.openSpeedUpTray(tx)
						return m, cmd
					} else {
						m.isActive = false
						return m, NavigateToFeeBump(tx.TxID)
					}
				}
			}

		case "1":
			m.toggleSort(0)
			m.updateTableData()
		case "2":
			m.toggleSort(1)
			m.updateTableData()
		case "3":
			m.toggleSort(2)
			m.updateTableData()
		case "4":
			m.toggleSort(3)
			m.updateTableData()
		case "5":
			m.toggleSort(4)
			m.updateTableData()
		case "6":
			m.toggleSort(5)
			m.updateTableData()

		case "e":
			if len(m.filteredTransactions) > 0 {
				absoluteIndex := m.currentPage*m.itemsPerPage + m.selectedRow
				if absoluteIndex >= 0 && absoluteIndex < len(m.filteredTransactions) {
					tx := m.filteredTransactions[absoluteIndex]
					m.editMode = true
					m.editTxID = tx.TxID
					existingLabel := m.walletManager.GetTransactionLabel(tx.TxID)
					m.labelInput.SetValue(existingLabel)
					m.labelInput.Focus()
					return m, textinput.Blink
				}
			}

		case "E":
			output, err := m.walletManager.ExportLabelsBIP329()
			if err != nil {
				if errors.Is(err, wallet.ErrNoLabels) {
					m.err = fmt.Errorf("no labels to export - press 'e' to label a transaction, or label addresses on the Addresses screen")
				} else {
					m.err = fmt.Errorf("export failed: %v", err)
				}
				m.errorTime = time.Now()
				cmds = append(cmds, m.animateTick())
			} else {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					m.err = fmt.Errorf("failed to get home directory: %v", err)
					m.errorTime = time.Now()
					cmds = append(cmds, m.animateTick())
				} else {
					walletDir := homeDir + "/.hax"
					filename := fmt.Sprintf("%s/%s_labels_bip329.jsonl", walletDir, m.walletManager.GetWalletName())
					if err := writePrivateFile(filename, []byte(output)); err != nil {
						m.err = fmt.Errorf("failed to write export file: %v", err)
						m.errorTime = time.Now()
						cmds = append(cmds, m.animateTick())
					} else {
						m.err = nil
						m.showCopied = true
						m.exportedFile = filename
						m.copiedTime = time.Now()
						cmds = append(cmds, m.animateTick())
					}
				}
			}

		case "K":
			output, err := m.walletManager.ExportKoinly(m.transactions)
			if err != nil {
				m.err = fmt.Errorf("koinly export failed: %w", err)
				m.errorTime = time.Now()
				cmds = append(cmds, m.animateTick())
			} else {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					m.err = fmt.Errorf("failed to get home directory: %v", err)
					m.errorTime = time.Now()
					cmds = append(cmds, m.animateTick())
				} else {
					walletDir := homeDir + "/.hax"
					filename := fmt.Sprintf("%s/%s_koinly.csv", walletDir, m.walletManager.GetWalletName())
					if err := writePrivateFile(filename, []byte(output)); err != nil {
						m.err = fmt.Errorf("failed to write Koinly export: %v", err)
						m.errorTime = time.Now()
						cmds = append(cmds, m.animateTick())
					} else {
						m.err = nil
						m.showCopied = true
						m.exportedFile = filename
						m.copiedTime = time.Now()
						cmds = append(cmds, m.animateTick())
					}
				}
			}

		case "f":
			switch m.filterType {
			case "all":
				m.filterType = "in"
			case "in":
				m.filterType = "out"
			case "out":
				m.filterType = "all"
			}
			m.currentPage = 0
			m.updateTableData()

		case "x":
			switch m.filterStatus {
			case "all":
				m.filterStatus = "confirmed"
			case "confirmed":
				m.filterStatus = "pending"
			case "pending":
				m.filterStatus = "all"
			}
			m.currentPage = 0
			m.updateTableData()

		case "]", "right", "l":
			totalPages := m.getTotalPages()
			if m.currentPage < totalPages-1 {
				m.currentPage++
				m.updateTableData()
				m.table.GotoTop()
				m.selectedRow = m.table.Cursor()
			}
			cmds = append(cmds, SuppressWheelFor(300*time.Millisecond))
			return m, tea.Batch(cmds...)

		case "[", "left", "h":
			if m.currentPage > 0 {
				m.currentPage--
				m.updateTableData()
				m.table.GotoTop()
				m.selectedRow = m.table.Cursor()
			}
			cmds = append(cmds, SuppressWheelFor(300*time.Millisecond))
			return m, tea.Batch(cmds...)

		case "+", "=":
			if m.columnWidths[m.sortColumn] < 40 {
				m.columnWidths[m.sortColumn] += 2
				m.updateTableData()
			}

		case "-", "_":
			if m.columnWidths[m.sortColumn] > 6 {
				m.columnWidths[m.sortColumn] -= 2
				m.updateTableData()
			}

		case "/":
			m.searchMode = true
			m.searchInput.Focus()
			return m, textinput.Blink
		}

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.table.MoveUp(1)
			m.selectedRow = m.table.Cursor()
			return m, nil
		case tea.MouseWheelDown:
			m.table.MoveDown(1)
			m.selectedRow = m.table.Cursor()
			return m, nil
		}
	}

	m.table, cmd = m.table.Update(msg)
	cmds = append(cmds, cmd)
	m.selectedRow = m.table.Cursor()

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// toggleSort toggles sorting for the given column
func (m *TransactionHistoryModel) toggleSort(column int) {
	if m.sortColumn == column {
		m.sortAscending = !m.sortAscending
	} else {
		m.sortColumn = column
		if column == 0 || column == 2 {
			m.sortAscending = false
		} else {
			m.sortAscending = true
		}
	}
}

func (m *TransactionHistoryModel) updateTableData() {
	if len(m.transactions) == 0 {
		m.table.SetRows([]table.Row{})
		return
	}

	// Update column headers with sort indicator
	sortIndicator := ""
	if m.sortAscending {
		sortIndicator = " ↑"
	} else {
		sortIndicator = " ↓"
	}

	widths := m.effectiveColumnWidths()
	txIDHidden := widths[4] == 0
	actionWidth := m.actionColumnWidth()
	columns := []table.Column{
		{Title: "Time" + (map[bool]string{true: sortIndicator, false: ""})[m.sortColumn == 0], Width: widths[0]},
		{Title: "Type" + (map[bool]string{true: sortIndicator, false: ""})[m.sortColumn == 1], Width: widths[1]},
		{Title: "Amount" + (map[bool]string{true: sortIndicator, false: ""})[m.sortColumn == 2], Width: widths[2]},
		{Title: "Status" + (map[bool]string{true: sortIndicator, false: ""})[m.sortColumn == 3], Width: widths[3]},
		{Title: "TxID" + (map[bool]string{true: sortIndicator, false: ""})[m.sortColumn == 4], Width: widths[4]},
		{Title: "Label" + (map[bool]string{true: sortIndicator, false: ""})[m.sortColumn == 5], Width: widths[5]},
		{Title: "", Width: actionWidth},
	}
	m.table.SetColumns(columns)
	m.table.SetWidth(naturalTableWidth(columns))

	// Get accounts to determine our addresses
	accounts := m.walletManager.GetAccounts()
	ourAddresses, _ := historyAddressSets(accounts)
	addrLabels := make(map[string]string)
	if len(accounts) > 0 {
		for a, l := range accounts[0].Labels {
			addrLabels[a] = strings.ToLower(l)
		}
	}

	// Sort transactions before converting to rows
	m.sortTransactions(ourAddresses)

	// Apply filters (including search) to create filtered transaction list
	m.filteredTransactions = make([]api.Transaction, 0, len(m.transactions))
	for _, tx := range m.transactions {
		// Apply search filter first
		if m.searchQuery != "" {
			searchLower := strings.ToLower(m.searchQuery)
			txID := strings.ToLower(tx.TxID)

			// Calculate net amount for this transaction
			analysis := analyzeTransaction(tx, ourAddresses)
			absAmount := analysis.NetAmount
			if absAmount < 0 {
				absAmount = -absAmount
			}
			amountStr := fmt.Sprintf("%.8f", absAmount.ToBTC())

			// Get status string
			statusStr := "pending"
			if tx.Status.Confirmed {
				statusStr = "confirmed"
			}

			// Get timestamp string
			timestampStr := ""
			if tx.Status.Confirmed && tx.Status.BlockTime > 0 {
				t := time.Unix(tx.Status.BlockTime, 0)
				timestampStr = strings.ToLower(t.Format("2006-01-02 15:04"))
			} else {
				timestampStr = "pending"
			}

			// Get type/direction string
			typeStr := strings.ToLower(analysis.Type) // "↓ in" or "↑ out"

			// Get label string
			labelStr := strings.ToLower(m.walletManager.GetTransactionLabel(tx.TxID))

			// Check if search matches any field
			if !strings.Contains(txID, searchLower) &&
				!strings.Contains(amountStr, searchLower) &&
				!strings.Contains(statusStr, searchLower) &&
				!strings.Contains(timestampStr, searchLower) &&
				!strings.Contains(typeStr, searchLower) &&
				!strings.Contains(labelStr, searchLower) &&
				!txTouchesAddressMatch(tx, searchLower, addrLabels) {
				continue // Skip transactions that don't match search
			}
		}

		// Apply type filter
		if m.filterType != "all" {
			analysis := analyzeTransaction(tx, ourAddresses)

			// Skip if doesn't match type filter
			if m.filterType == "in" && analysis.Type != "↓ IN" {
				continue
			}
			if m.filterType == "out" && analysis.Type != "↑ OUT" {
				continue
			}
		}

		// Apply status filter
		if m.filterStatus != "all" {
			if m.filterStatus == "confirmed" && !tx.Status.Confirmed {
				continue
			}
			if m.filterStatus == "pending" && tx.Status.Confirmed {
				continue
			}
		}

		// Transaction matches filters, add to filtered list
		m.filteredTransactions = append(m.filteredTransactions, tx)
	}

	// Calculate pagination on filtered results
	startIdx := m.currentPage * m.itemsPerPage
	endIdx := startIdx + m.itemsPerPage
	if startIdx >= len(m.filteredTransactions) {
		startIdx = 0
		m.currentPage = 0
	}
	if endIdx > len(m.filteredTransactions) {
		endIdx = len(m.filteredTransactions)
	}

	// Get transactions for current page
	var pageTransactions []api.Transaction
	if len(m.filteredTransactions) > 0 {
		pageTransactions = m.filteredTransactions[startIdx:endIdx]
	}

	// Convert page transactions to table rows
	rows := make([]table.Row, 0, len(pageTransactions))

	for _, tx := range pageTransactions {
		// Analyze transaction to get type and net amount
		analysis := analyzeTransaction(tx, ourAddresses)

		// Format timestamp
		timestamp := ""
		if tx.Status.Confirmed && tx.Status.BlockTime > 0 {
			t := time.Unix(tx.Status.BlockTime, 0)
			timestamp = t.Format("2006-01-02 15:04")
		} else {
			timestamp = "Pending..."
		}

		// Format amount with proper sign
		absAmount := analysis.NetAmount
		if absAmount < 0 {
			absAmount = -absAmount
		}
		amountStr := fmt.Sprintf("%s%.8f BTC",
			getAmountPrefix(analysis.NetAmount),
			absAmount.ToBTC())
		amountStr = maskAmountRight(amountStr, m.stealth)

		// Format status with confirmation progress
		status := formatConfirmations(tx.Status.BlockHeight, m.blockHeight, tx.Status.Confirmed)

		// Format TxID (shortened)
		txID := tx.TxID
		if len(txID) > 20 {
			txID = txID[:10] + "..." + txID[len(txID)-7:]
		}

		// Get label for this transaction
		label := sanitizeLine(m.walletManager.GetTransactionLabel(tx.TxID))
		if label == "" {
			label = "-" // Show dash if no label
		} else {
			// Truncate label to fit column width without splitting runes
			label = truncateRunes(label, widths[5], "...")
		}

		if txIDHidden {
			txID = "" // full TxID is in the details pane
		}

		action := ""
		if actionWidth > 0 && historyCanSpeedUp(tx, analysis) {
			action = m.speedUpMarker()
		}

		rows = append(rows, table.Row{
			timestamp,
			analysis.Type,
			amountStr,
			status,
			txID,
			label,
			action,
		})
	}

	m.table.SetRows(rows)
}

// sortTransactions sorts the transactions based on current sort column and direction
func (m *TransactionHistoryModel) sortTransactions(ourAddresses map[string]bool) {
	if len(m.transactions) == 0 {
		return
	}

	// Helper to get transaction net amount
	getAmount := func(tx api.Transaction) api.Satoshi {
		analysis := analyzeTransaction(tx, ourAddresses)
		return analysis.NetAmount
	}

	// Helper to get transaction type
	getTxType := func(tx api.Transaction) string {
		analysis := analyzeTransaction(tx, ourAddresses)
		return analysis.Type
	}

	// Sort based on column
	switch m.sortColumn {
	case 0: // Time
		// Pending transactions (BlockTime=0) should be treated as most recent
		// Use TxID as secondary sort for deterministic ordering when times are equal
		if m.sortAscending {
			// Oldest first - pending transactions go LAST (most recent)
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					ti := m.transactions[i].Status.BlockTime
					tj := m.transactions[j].Status.BlockTime
					iIsPending := ti == 0
					jIsPending := tj == 0
					shouldSwap := false
					if iIsPending && !jIsPending {
						shouldSwap = true // i is pending (newest), j is confirmed, swap
					} else if !iIsPending && !jIsPending {
						if ti != tj {
							shouldSwap = ti > tj // Both confirmed, normal comparison
						} else {
							// Same time - use TxID for deterministic order
							shouldSwap = m.transactions[i].TxID > m.transactions[j].TxID
						}
					} else if iIsPending && jIsPending {
						// Both pending - use TxID for deterministic order
						shouldSwap = m.transactions[i].TxID > m.transactions[j].TxID
					}
					if shouldSwap {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		} else {
			// Newest first - pending transactions go FIRST (most recent)
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					ti := m.transactions[i].Status.BlockTime
					tj := m.transactions[j].Status.BlockTime
					iIsPending := ti == 0
					jIsPending := tj == 0
					shouldSwap := false
					if !iIsPending && jIsPending {
						shouldSwap = true // j is pending (newest), i is confirmed, swap
					} else if !iIsPending && !jIsPending {
						if ti != tj {
							shouldSwap = ti < tj // Both confirmed, normal comparison
						} else {
							// Same time - use TxID for deterministic order (descending)
							shouldSwap = m.transactions[i].TxID < m.transactions[j].TxID
						}
					} else if iIsPending && jIsPending {
						// Both pending - use TxID for deterministic order (descending)
						shouldSwap = m.transactions[i].TxID < m.transactions[j].TxID
					}
					if shouldSwap {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		}
	case 1: // Type
		if m.sortAscending {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if getTxType(m.transactions[i]) > getTxType(m.transactions[j]) {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		} else {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if getTxType(m.transactions[i]) < getTxType(m.transactions[j]) {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		}
	case 2: // Amount
		if m.sortAscending {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if getAmount(m.transactions[i]) > getAmount(m.transactions[j]) {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		} else {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if getAmount(m.transactions[i]) < getAmount(m.transactions[j]) {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		}
	case 3: // Status
		if m.sortAscending {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if m.transactions[i].Status.Confirmed && !m.transactions[j].Status.Confirmed {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		} else {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if !m.transactions[i].Status.Confirmed && m.transactions[j].Status.Confirmed {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		}
	case 4: // TxID
		if m.sortAscending {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if m.transactions[i].TxID > m.transactions[j].TxID {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		} else {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					if m.transactions[i].TxID < m.transactions[j].TxID {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		}
	case 5: // Label
		if m.sortAscending {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					labelI := m.walletManager.GetTransactionLabel(m.transactions[i].TxID)
					labelJ := m.walletManager.GetTransactionLabel(m.transactions[j].TxID)
					if labelI > labelJ {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		} else {
			for i := 0; i < len(m.transactions)-1; i++ {
				for j := i + 1; j < len(m.transactions); j++ {
					labelI := m.walletManager.GetTransactionLabel(m.transactions[i].TxID)
					labelJ := m.walletManager.GetTransactionLabel(m.transactions[j].TxID)
					if labelI < labelJ {
						m.transactions[i], m.transactions[j] = m.transactions[j], m.transactions[i]
					}
				}
			}
		}
	}
}

// historyAddressSets splits account 0's addresses into the wallet's own
// addresses and the subset used for change. History already uses this
// ownership view for IN/OUT, so the visualizer reports the same thing.
func historyAddressSets(accounts []wallet.AccountData) (mine, change map[string]bool) {
	mine = make(map[string]bool)
	change = make(map[string]bool)
	if len(accounts) == 0 {
		return mine, change
	}
	for _, addr := range accounts[0].Addresses {
		if addr.Address == "" {
			continue
		}
		mine[addr.Address] = true
		if addr.Change {
			change[addr.Address] = true
		}
	}
	return mine, change
}

// selectedTransaction returns the transaction under the table cursor. The
// cursor starts unset (-1), so the first row counts as selected until the
// user moves it.
func (m *TransactionHistoryModel) selectedTransaction() (api.Transaction, bool) {
	if len(m.filteredTransactions) == 0 {
		return api.Transaction{}, false
	}
	row := m.selectedRow
	if row < 0 {
		row = 0
	}
	absoluteIndex := m.currentPage*m.itemsPerPage + row
	if absoluteIndex < 0 || absoluteIndex >= len(m.filteredTransactions) {
		return api.Transaction{}, false
	}
	return m.filteredTransactions[absoluteIndex], true
}

// actionColumnWidth reserves the right-hand speed-up column. Below 100
// columns only the arrow fits; on wider terminals the label does.
func (m TransactionHistoryModel) actionColumnWidth() int {
	if m.width >= 100 {
		return lipgloss.Width("⇡ speed up")
	}
	if m.width >= 60 {
		return lipgloss.Width("⇡")
	}
	return 0
}

func (m TransactionHistoryModel) speedUpMarker() string {
	if m.width >= 100 {
		return "⇡ speed up"
	}
	return "⇡"
}

// historyCanSpeedUp reports whether a pending outgoing transaction signals
// opt-in RBF, the same cheap check the bump flow uses before it fetches the
// transaction.
func historyCanSpeedUp(tx api.Transaction, analysis TransactionAnalysis) bool {
	if tx.Status.Confirmed || analysis.Type != "↑ OUT" {
		return false
	}
	for _, vin := range tx.Vin {
		if vin.Sequence <= speedUpRBFSequence {
			return true
		}
	}
	return false
}

// canSpeedUpTransaction applies historyCanSpeedUp using the wallet's
// address ownership view.
func (m TransactionHistoryModel) canSpeedUpTransaction(tx api.Transaction) bool {
	var accounts []wallet.AccountData
	if m.walletManager != nil {
		accounts = m.walletManager.GetAccounts()
	}
	mine, _ := historyAddressSets(accounts)
	return historyCanSpeedUp(tx, analyzeTransaction(tx, mine))
}

// historyTxVSize returns the virtual size used to turn a feerate into sats.
func historyTxVSize(tx api.Transaction) int {
	if tx.Weight > 0 {
		return (tx.Weight + 3) / 4
	}
	if tx.Size > 0 {
		return tx.Size
	}
	return 1
}

func speedUpSummary(tx api.Transaction, rate float64) []TraySummaryRow {
	vsize := historyTxVSize(tx)
	current := float64(tx.Fee) / float64(vsize)
	rows := []TraySummaryRow{{
		Label: "Current rate",
		Value: fmt.Sprintf("%s sat/vB", formatFeeRate(roundRate(current))),
	}}
	if rate <= 0 {
		return append(rows,
			TraySummaryRow{Label: "New rate", Value: "—"},
			TraySummaryRow{Label: "Extra fee", Value: "—"},
		)
	}
	extra := int64(rate*float64(vsize)+0.999) - int64(tx.Fee)
	if extra < 0 {
		extra = 0
	}
	return append(rows,
		TraySummaryRow{
			Label: "New rate",
			Value: fmt.Sprintf("%s sat/vB", formatFeeRate(roundRate(rate))),
		},
		TraySummaryRow{Label: "Extra fee", Value: fmt.Sprintf("+%s sats", wallet.FormatSats(api.Satoshi(extra)))},
	)
}

// trayWidth matches the history table's outer width, including its border
// and column padding, so the tray sits flush with the frame above it.
func (m TransactionHistoryModel) trayWidth() int {
	if m.width <= 0 {
		return 80
	}
	widths := m.effectiveColumnWidths()
	total, columns := 4, 0 // border plus one cell of padding each side
	for _, w := range widths {
		if w <= 0 {
			continue
		}
		total += w
		columns++
	}
	if action := m.actionColumnWidth(); action > 0 {
		total += action
		columns++
	}
	total += 2 * columns // one cell of padding each side of every column
	if total < 12 {
		total = 12
	}
	return total
}

// openSpeedUpTray opens the inline panel and loads fee estimates when the
// wallet has no cached ones.
func (m *TransactionHistoryModel) openSpeedUpTray(tx api.Transaction) tea.Cmd {
	m.speedUp = &speedUpTrayState{tx: tx, baseLines: m.linesWithoutTray()}

	var estimates *api.FeeEstimates
	if m.walletManager != nil {
		if cached, ok := m.walletManager.GetCachedFeeEstimates(); ok && cached != nil {
			estimates = cached
		} else {
			m.speedUp.loading = true
		}
	} else {
		m.speedUp.loading = true
	}

	m.buildSpeedUpTray(estimates)
	if m.speedUp.loading {
		return m.fetchSpeedUpFeeEstimates()
	}
	return nil
}

func (m *TransactionHistoryModel) rebuildSpeedUpTray(estimates *api.FeeEstimates) {
	m.buildSpeedUpTray(estimates)
}

func (m *TransactionHistoryModel) buildSpeedUpTray(estimates *api.FeeEstimates) {
	st := m.speedUp
	if st == nil {
		return
	}

	rates := [3]float64{}
	if estimates != nil {
		rates = [3]float64{estimates.FastestFee, estimates.HalfHourFee, estimates.HourFee}
	}
	st.rates = rates

	cursor, stage := 0, TrayStageChoose
	if st.tray.title != "" {
		cursor, stage = st.tray.cursor, st.tray.stage
	}

	labels := []string{"Fastest", "30 minutes", "1 hour"}
	times := []string{"~10 min", "~30 min", "~1 hour"}
	options := make([]TrayOption, 0, 4)
	for i := range labels {
		help := "…"
		if !st.loading {
			help = times[i] + " · unavailable"
			if rates[i] > 0 {
				help = fmt.Sprintf("%s · %s sat/vB", times[i], formatFeeRate(roundRate(rates[i])))
			}
		}
		options = append(options, TrayOption{Label: labels[i], Help: help})
	}
	options = append(options, TrayOption{Label: "Custom…", Help: "enter a rate"})

	summaryRate := 0.0
	if cursor >= 0 && cursor < len(rates) {
		summaryRate = rates[cursor]
	}
	tray := NewTray("⚡ Speed Up Transaction", options, speedUpSummary(st.tx, summaryRate), m.trayWidth())
	tray.cursor = cursor
	tray.stage = stage
	tray.SummaryFor = func(selected int) []TraySummaryRow {
		if selected >= 0 && selected < len(rates) {
			return speedUpSummary(st.tx, rates[selected])
		}
		return []TraySummaryRow{{Label: "Fee rate", Value: "set on the next screen"}}
	}
	st.tray = tray
}

func (m TransactionHistoryModel) fetchSpeedUpFeeEstimates() tea.Cmd {
	walletManager := m.walletManager
	return func() tea.Msg {
		if walletManager == nil {
			return SpeedUpFeeEstimatesMsg{}
		}
		estimates, err := walletManager.GetFeeEstimates()
		return SpeedUpFeeEstimatesMsg{Estimates: estimates, Err: err}
	}
}

func (m *TransactionHistoryModel) handleSpeedUpKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	st := m.speedUp
	if st == nil {
		return false, nil
	}
	key := msg.String()

	// Do not confirm a rate the wallet has not supplied yet.
	if st.loading && key == "enter" {
		return true, nil
	}

	// Custom falls through to the existing fee bump flow.
	if st.tray.stage == TrayStageChoose && key == "enter" &&
		st.tray.cursor == speedUpCustomIndex {
		txid := st.tx.TxID
		m.speedUp = nil
		m.isActive = false
		return true, NavigateToFeeBump(txid)
	}

	tray, result := st.tray.Update(msg)
	st.tray = tray

	switch result.Kind {
	case TrayCancelled:
		m.speedUp = nil
	case TrayConfirmed:
		txid := st.tx.TxID
		rate := 0.0
		if result.Selected >= 0 && result.Selected < len(st.rates) {
			rate = st.rates[result.Selected]
		}
		m.speedUp = nil
		m.isActive = false
		if rate <= 0 {
			return true, NavigateToFeeBump(txid)
		}
		return true, NavigateToFeeBumpWithRate(txid, rate)
	}
	return true, nil
}

// linesWithoutTray is the normal history height, used to keep the total
// line count fixed while the tray is open.
func (m TransactionHistoryModel) linesWithoutTray() int {
	clone := m
	clone.speedUp = nil
	return lipgloss.Height(clone.View())
}

// openTransactionFlow shows the selected transaction as a flow diagram.
// Ownership comes from the wallet's address list; no network calls.
func (m *TransactionHistoryModel) openTransactionFlow(tx api.Transaction) {
	var accounts []wallet.AccountData
	var labels func(string) string
	if m.walletManager != nil {
		accounts = m.walletManager.GetAccounts()
		labels = m.walletManager.GetAddressLabel
	}
	mine, change := historyAddressSets(accounts)
	m.txFlow = &TxFlowView{
		Flow: TxFlowFromTransaction(tx,
			func(addr string) bool { return mine[addr] },
			func(addr string) bool { return change[addr] },
			labels),
		Width:  m.width,
		Height: m.height,
	}
}

// analyzeTransaction analyzes a transaction to determine its type and net amount impact on the wallet
func analyzeTransaction(tx api.Transaction, ourAddresses map[string]bool) TransactionAnalysis {
	var inputsFromUs api.Satoshi = 0
	var outputsToUs api.Satoshi = 0

	// Calculate total value of inputs from our addresses
	for _, vin := range tx.Vin {
		// Safety check: ensure prevout exists
		if vin.Prevout != nil && ourAddresses[vin.Prevout.ScriptpubkeyAddress] {
			inputsFromUs += vin.Prevout.Value
		}
	}

	// Calculate total value of outputs to our addresses
	for _, vout := range tx.Vout {
		if ourAddresses[vout.ScriptpubkeyAddress] {
			outputsToUs += vout.Value
		}
	}

	txType := "↓ IN"
	if inputsFromUs > 0 {
		txType = "↑ OUT"
	}

	netAmount := outputsToUs - inputsFromUs

	return TransactionAnalysis{
		Type:      txType,
		NetAmount: netAmount,
	}
}

// CapturesTextInput reports whether search or label editing owns the keyboard.
func (m TransactionHistoryModel) CapturesTextInput() bool {
	return m.txFlow != nil || m.searchMode || m.editMode
}

// historyHeaderBlock is the title, page, filter and copy-status frame.
func (m TransactionHistoryModel) historyHeaderBlock() string {
	header := styles.ScreenHeaderStyle.Render("📜 Transaction History") + stealthBadge(m.stealth)

	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		message := "✓ COPIED!"
		if m.exportedFile != "" {
			message = fmt.Sprintf("✓ EXPORTED: %s", m.exportedFile)
		}
		copyStatus = styles.CopyStatusBadgeStyle.Render(message)
	}

	filterBadges := m.renderFilterBadges()

	totalPages := m.totalPagesForView()
	pageInfo := ""
	if totalPages > 1 {
		displayPage := m.currentPage + 1
		if displayPage < 1 {
			displayPage = 1
		}
		if displayPage > totalPages {
			displayPage = totalPages
		}
		pageInfo = lipgloss.NewStyle().
			Foreground(styles.PastelPurple).
			Bold(true).
			Render(fmt.Sprintf("Page %d/%d", displayPage, totalPages))
	}

	searchBadge := ""
	if m.searchQuery != "" && !m.searchMode {
		searchBadge = styles.Badge(styles.PastelPink).
			Render(fmt.Sprintf("🔍 \"%s\"", m.searchQuery))
	}

	metaParts := make([]string, 0, 4)
	if filterBadges != "" {
		metaParts = append(metaParts, strings.TrimSpace(filterBadges))
	}
	if searchBadge != "" {
		metaParts = append(metaParts, searchBadge)
	}
	if notice := m.historyNotice(); notice != "" {
		metaParts = append(metaParts, notice)
	}
	metaLine := ""
	if len(metaParts) > 0 {
		metaLine = "\n" + strings.Join(metaParts, " ")
	}
	leftHeader := header
	if pageInfo != "" {
		leftHeader += " " + pageInfo
	}

	headerLine := leftHeader
	if copyStatus != "" {
		// Keep copy status directly after title/page indicator.
		headerLine = leftHeader + " " + copyStatus
	}

	return headerLine + metaLine
}

func (m TransactionHistoryModel) View() string {
	headerBlock := m.historyHeaderBlock()

	if m.txFlow != nil {
		flow := *m.txFlow
		flow.Width = m.width
		flow.Height = m.height - lipgloss.Height(headerBlock) - 1
		if flow.Height < 1 {
			flow.Height = 1
		}
		return headerBlock + "\n" + flow.View()
	}

	if m.speedUp != nil {
		return m.viewWithSpeedUp(headerBlock)
	}

	var content string

	searchBox := ""
	if m.searchMode {
		searchBox = "\n" + lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.PastelPink).
			Padding(0, 1).
			Render("🔍 Search: "+m.searchInput.View()) + "\n"
	}

	labelBox := ""
	if m.editMode {
		labelBox = "\n" + styles.EditBoxStyle.Render("✏️  Edit Label: "+m.labelInput.View()) + "\n"
	}

	if m.loading {
		loadingBox := styles.InfoBoxStyle.Render(
			m.spinner.View() + " " + styles.NormalText.Render("Loading transactions from blockchain..."),
		)
		content = headerBlock + searchBox + labelBox + "\n" + loadingBox
	} else if m.err != nil {
		errorBox := styles.RenderErrorBox(
			fmt.Sprintf("Failed to load transactions:\n\n%v", m.err), m.width,
		)
		content = headerBlock + searchBox + labelBox + "\n" + errorBox
	} else if len(m.transactions) == 0 {
		emptyBox := m.renderEmptyState()
		content = headerBlock + searchBox + labelBox + "\n" + emptyBox
	} else {
		tableView := m.renderTransactionTable()
		details := m.renderTransactionDetails()

		content = headerBlock + searchBox + labelBox + "\n" +
			tableView + "\n" +
			details
	}

	help := m.renderAnimatedHelp()
	return content + "\n" + help
}

// historyNotice warns when the backend cannot see spent coins, so the
// list is missing transactions whose outputs are all spent.
func (m TransactionHistoryModel) historyNotice() string {
	if m.walletManager == nil {
		return ""
	}
	hl, ok := m.walletManager.GetBackend().(backend.HistoryLimiter)
	if !ok || !hl.HistoryLimited() {
		return ""
	}
	return styles.WarningText.Render(
		"⚠ Partial history: run Core with -blockfilterindex=1")
}

// totalPagesForView computes total pages for header display.
// It falls back to total transactions when filtered data hasn't been populated yet.
func (m TransactionHistoryModel) totalPagesForView() int {
	if m.itemsPerPage <= 0 {
		return 1
	}

	count := len(m.filteredTransactions)
	if count == 0 && len(m.transactions) > 0 &&
		m.searchQuery == "" && m.filterType == "all" && m.filterStatus == "all" {
		count = len(m.transactions)
	}
	if count <= 0 {
		return 1
	}
	return (count + m.itemsPerPage - 1) / m.itemsPerPage
}

func (m TransactionHistoryModel) renderEmptyState() string {
	phase := float64(m.pulseFrame%180) / 180.0
	var icon string
	if phase < 0.33 {
		icon = "📭"
	} else if phase < 0.66 {
		icon = "📬"
	} else {
		icon = "📪"
	}

	emptyText := fmt.Sprintf("%s No transactions yet\n\n", icon) +
		"Once you receive bitcoin, your\n" +
		"transactions will appear here!\n\n" +
		styles.DimText.Render("Press 'r' to receive coins first")

	return styles.InfoBoxStyle.Render(emptyText)
}

func (m TransactionHistoryModel) renderTransactionTable() string {
	return m.renderTransactionTableAt(0)
}

func (m TransactionHistoryModel) renderTransactionTableAt(tableHeight int) string {
	tbl := m.table
	if tableHeight > 0 {
		tbl.SetHeight(tableHeight)
	}
	view := tbl.View()
	if m.actionColumnWidth() > 0 {
		view = strings.ReplaceAll(view, m.speedUpMarker(),
			styles.DimText.Render(m.speedUpMarker()))
	}
	return styles.TableBorderStyle.Render(view)
}

// viewWithSpeedUp keeps the header and table top in place and gives the
// tray the bottom of the screen. The table loses rows to make room, so the
// total height stays the same as the normal history view.
func (m TransactionHistoryModel) viewWithSpeedUp(headerBlock string) string {
	trayView := m.speedUp.tray.View()
	trayLines := lipgloss.Height(trayView)
	target := m.speedUp.baseLines
	if target <= 0 {
		target = m.height
	}
	if target <= 0 {
		target = lipgloss.Height(headerBlock) + trayLines + 6
	}

	tableBlock := target - lipgloss.Height(headerBlock) - trayLines
	if tableBlock < 5 {
		tableBlock = 5
	}

	return headerBlock + "\n" +
		m.renderTransactionTableAt(tableBlock-2) + "\n" +
		trayView
}

func (m TransactionHistoryModel) renderTransactionDetails() string {
	if len(m.filteredTransactions) == 0 {
		return ""
	}

	absoluteIndex := m.currentPage*m.itemsPerPage + m.selectedRow
	if absoluteIndex < 0 || absoluteIndex >= len(m.filteredTransactions) {
		return ""
	}

	tx := m.filteredTransactions[absoluteIndex]

	details := []string{
		styles.BoldText.Render("Transaction Details:"),
		renderDetailRow("TxID:", sanitizeLine(tx.TxID)),
		renderDetailRow("Size:", fmt.Sprintf("%d bytes", tx.Size)),
		renderDetailRow("Weight:", fmt.Sprintf("%d WU", tx.Weight)),
		renderDetailRow("Fee:", fmt.Sprintf("%d sats", tx.Fee)),
	}

	if tx.Status.Confirmed {
		confStr := formatConfirmations(tx.Status.BlockHeight, m.blockHeight, true)
		details = append(details,
			renderDetailRow("Block:", fmt.Sprintf("#%d", tx.Status.BlockHeight)),
			renderDetailRow("Status:", confStr),
		)
	} else {
		details = append(details,
			styles.WarningText.Render("○ Unconfirmed - waiting for block")+
				styles.DimText.Render(" · b to speed up"),
		)
	}

	detailsWidth := styles.ClampWidth(84, m.width)
	detailsBox := styles.BoxStyle.
		Width(detailsWidth + 2).
		BorderForeground(styles.ElectricBlue).
		Render(strings.Join(details, "\n"))

	return detailsBox
}

func (m TransactionHistoryModel) renderAnimatedHelp() string {
	if m.editMode {
		return styles.RenderHelp(
			styles.HK("type", "to edit label"),
			styles.HK("enter", "save"),
			styles.HK("esc", "cancel"),
		)
	}

	if m.searchMode {
		return styles.RenderHelp(
			styles.HK("type", "to search"),
			styles.HK("enter", "keep filter"),
			styles.HK("esc", "cancel search"),
		)
	}

	return styles.RenderHelpLines(
		[]styles.HelpItem{
			styles.HK("↑/↓", "navigate"),
			styles.HK("enter", "open"),
			styles.HK("c", "copy"),
			styles.HK("e", "label"),
			styles.HK("E", "export"),
			styles.HK("v", "flow"),
			styles.HK("/", "search"),
		},
		[]styles.HelpItem{
			styles.HK("1-6", "sort"),
			styles.HK("f/x", "filter"),
			styles.HK("K", "koinly"),
			styles.HK("b", "bump fee"),
			styles.HK("←/→", "page"),
			styles.HK("esc", "back"),
		},
	)
}

func (m TransactionHistoryModel) renderFilterBadges() string {
	badges := ""

	if m.filterType != "all" {
		typeLabel := ""
		switch m.filterType {
		case "in":
			typeLabel = "↓ IN ONLY"
		case "out":
			typeLabel = "↑ OUT ONLY"
		}
		badges += " " + styles.Badge(styles.PastelBlue).Render(typeLabel)
	}

	if m.filterStatus != "all" {
		statusLabel := ""
		switch m.filterStatus {
		case "confirmed":
			statusLabel = "✓ CONFIRMED"
		case "pending":
			statusLabel = "○ PENDING"
		}
		badges += " " + styles.Badge(styles.PastelYellow).Render(statusLabel)
	}

	return badges
}

// getTotalPages calculates the total number of pages based on filtered results
func (m TransactionHistoryModel) getTotalPages() int {
	if len(m.filteredTransactions) == 0 {
		return 1
	}
	totalPages := (len(m.filteredTransactions) + m.itemsPerPage - 1) / m.itemsPerPage
	if totalPages == 0 {
		return 1
	}
	return totalPages
}

func getAmountPrefix(netAmount api.Satoshi) string {
	if netAmount >= 0 {
		return "+"
	}
	return "-"
}

func renderDetailRow(label, value string) string {
	return styles.DimText.Render(fmt.Sprintf("%-14s", label)) + " " +
		styles.HighlightText.Render(value)
}

// formatConfirmations returns a confirmation label with progress toward 6 confirmations.
func formatConfirmations(txBlockHeight, chainHeight int64, confirmed bool) string {
	if !confirmed {
		return "○ Unconfirmed"
	}

	if chainHeight == 0 || txBlockHeight == 0 {
		return "✓ Confirmed"
	}

	confirmations := chainHeight - txBlockHeight + 1

	if confirmations < 1 {
		return "✓ Confirmed"
	}

	switch {
	case confirmations == 1:
		return "○ 1/6 conf"
	case confirmations == 2:
		return "○ 2/6 conf"
	case confirmations == 3:
		return "○ 3/6 conf"
	case confirmations == 4:
		return "○ 4/6 conf"
	case confirmations == 5:
		return "○ 5/6 conf"
	case confirmations >= 6:
		return fmt.Sprintf("✓ %d conf", confirmations)
	default:
		return fmt.Sprintf("✓ %d conf", confirmations)
	}
}

func openTransactionInBrowser(network, txID string) error {
	url := wallet.ExplorerTxURL(network, txID)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = exec.Command("open", url) // #nosec G204 -- fixed opener; url is a block explorer URL built from a txid
	case "linux":
		cmd = exec.Command("xdg-open", url) // #nosec G204 -- fixed opener; url is a block explorer URL built from a txid
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url) // #nosec G204 -- fixed opener; url is a block explorer URL built from a txid
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}

// writePrivateFile writes data readable only by the current user (0600).
// Exports contain transaction history and labels, which are privacy-sensitive.
func writePrivateFile(path string, data []byte) error {
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

// effectiveColumnWidths fits the user's column widths into the terminal.
// When the table would overflow, the TxID column is hidden first (the full
// TxID is shown in the details pane), then Label shrinks to a minimum of 6.
func (m TransactionHistoryModel) effectiveColumnWidths() [6]int {
	w := m.columnWidths
	if m.width <= 0 {
		return w
	}
	// Border and padding of the box (4) plus one cell of padding each side
	// of every column (2 per column).
	budget := m.width - 4 - 2*len(w) - m.actionColumnWidth()
	sum := func() int {
		t := 0
		for _, c := range w {
			t += c
		}
		return t
	}
	if sum() > budget {
		w[4] = 0
	}
	if over := sum() - budget; over > 0 {
		w[5] -= over
		if w[5] < 6 {
			w[5] = 6
		}
	}
	return w
}

// txTouchesAddressMatch reports whether any input or output address of tx,
// or the label of one of our addresses it touches, contains query
// (already lower-cased). Lets the history search find payments by address
// or by address label.
func txTouchesAddressMatch(tx api.Transaction, query string, addrLabels map[string]string) bool {
	check := func(addr string) bool {
		if addr == "" {
			return false
		}
		if strings.Contains(strings.ToLower(addr), query) {
			return true
		}
		l, ok := addrLabels[addr]
		return ok && strings.Contains(l, query)
	}
	for _, out := range tx.Vout {
		if check(out.ScriptpubkeyAddress) {
			return true
		}
	}
	for _, in := range tx.Vin {
		if in.Prevout != nil && check(in.Prevout.ScriptpubkeyAddress) {
			return true
		}
	}
	return false
}
