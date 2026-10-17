package models

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/alert"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var debugEnabled = len(os.Getenv("DEBUG")) > 0

func debugLog(format string, args ...interface{}) {
	if debugEnabled {
		log.Printf(format, args...)
	}
}

// AppState represents the current screen/state of the application
type AppState int

const (
	StateSplash AppState = iota
	StateConnectionSetup
	StateWalletSetup
	StateUnlock
	StateMainDashboard
	StateReceive
	StateSend
	StateTransactionHistory
	StateAddresses
	StateUTXOs
	StateSettings
	StateHelp
)

const (
	AlertTypeNewTx    = "new_tx"
	AlertTypeNewBlock = "new_block"
	AlertTypeError    = "error"
)

type NewTransactionNotification struct {
	Address string
	Gen     int // listener generation that produced it
}

type NewBlockNotification struct {
	Height int64
	Gen    int // listener generation that produced it
}

// StateChangedMsg broadcasts wallet state updates to screen models.
type StateChangedMsg struct {
	State wallet.WalletStateSnapshot
}

// RootModel is the top-level model that manages all screens
type RootModel struct {
	state  AppState
	width  int
	height int

	splashModel             tea.Model
	connectionSetupModel    tea.Model
	walletSetupModel        tea.Model
	unlockModel             tea.Model
	mainDashboardModel      tea.Model
	receiveModel            tea.Model
	sendModel               tea.Model
	transactionHistoryModel tea.Model
	addressesModel          tea.Model
	utxosModel              tea.Model
	settingsModel           tea.Model
	helpModel               tea.Model

	alertModel alert.AlertModel

	walletManager WalletService
	configManager *config.ConfigManager
	walletLoaded  bool
	testnet       bool
	err           error

	contentViewport viewport.Model

	// Prevent inertial wheel carryover across screen transitions.
	suppressWheelActive   bool
	suppressWheelMaxUntil time.Time
	suppressWheelToken    int

	// Auto-lock: time of the last key or mouse input while unlocked.
	lastActivity     time.Time
	autoLockOverride time.Duration // tests only; <0 disables
	listenerStarted  bool
	// listenGen and listenStop tie the notification listener to one
	// backend. A backend change bumps the generation and closes the stop
	// channel so the old listener exits and a new one subscribes.
	listenGen  int
	listenStop chan struct{}
	// setupNetwork, when set, preselects the network offered by the next
	// connection setup screen.
	setupNetwork string
}

const (
	wheelSuppressAfterKey    = 220 * time.Millisecond
	wheelSuppressAfterNav    = 260 * time.Millisecond
	wheelSuppressQuietWindow = 180 * time.Millisecond
	wheelSuppressMaxWindow   = 3 * time.Second
)

// usesRootViewport reports whether the active screen should use root-level overflow scrolling.
func (m RootModel) usesRootViewport() bool {
	switch m.state {
	case StateAddresses, StateSend, StateSettings, StateWalletSetup, StateHelp, StateUTXOs, StateTransactionHistory:
		return false
	default:
		return true
	}
}

// NavigateMsg is used to switch between screens
type NavigateMsg struct {
	Target  AppState
	Refresh bool // If true, trigger a balance refresh on dashboard
}

// Navigate returns a Cmd that navigates to a new screen
func Navigate(target AppState) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Target: target, Refresh: false}
	}
}

// NavigateWithRefresh returns a Cmd that navigates to a screen and triggers a refresh
func NavigateWithRefresh(target AppState) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Target: target, Refresh: true}
	}
}

// ErrorMsg represents an error that occurred
type ErrorMsg struct {
	Err error
}

// ScreenActiveMsg is sent when a screen becomes visible again.
type ScreenActiveMsg struct{}

type SuppressWheelMsg struct {
	Duration time.Duration
}

type wheelSuppressReleaseMsg struct {
	Token int
}

// SuppressWheelFor returns a command that asks root to suppress wheel events globally.
func SuppressWheelFor(d time.Duration) tea.Cmd {
	return func() tea.Msg {
		return SuppressWheelMsg{Duration: d}
	}
}

func scheduleWheelSuppressRelease(token int, after time.Duration) tea.Cmd {
	return tea.Tick(after, func(time.Time) tea.Msg {
		return wheelSuppressReleaseMsg{Token: token}
	})
}

// NewRootModel creates a new root model
func NewRootModel(walletManager WalletService, configManager *config.ConfigManager) RootModel {
	alertModel := alert.NewAlertModel(80, 3)

	alertModel.RegisterNewAlertType(alert.AlertDefinition{
		Key:       AlertTypeNewTx,
		ForeColor: "#B5EAD7",
		Prefix:    "⚡",
	})
	alertModel.RegisterNewAlertType(alert.AlertDefinition{
		Key:       AlertTypeNewBlock,
		ForeColor: "#87CEEB",
		Prefix:    "🧱",
	})
	alertModel.RegisterNewAlertType(alert.AlertDefinition{
		Key:       AlertTypeError,
		ForeColor: "#FFB3BA",
		Prefix:    "⚠",
	})

	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 1
	vp.KeyMap.PageDown = key.NewBinding(key.WithKeys("pgdown"))
	vp.KeyMap.PageUp = key.NewBinding(key.WithKeys("pgup"))
	vp.KeyMap.HalfPageUp.SetEnabled(false)
	vp.KeyMap.HalfPageDown.SetEnabled(false)
	vp.KeyMap.Up.SetEnabled(false)
	vp.KeyMap.Down.SetEnabled(false)

	return RootModel{
		state:           StateSplash,
		testnet:         true, // Always testnet for safety
		walletLoaded:    false,
		walletManager:   walletManager,
		configManager:   configManager,
		splashModel:     NewSplashModel(),
		alertModel:      *alertModel,
		contentViewport: vp,
		listenStop:      make(chan struct{}),
	}
}

// Init initializes the root model
func (m RootModel) Init() tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, m.alertModel.Init())

	if m.splashModel != nil {
		cmds = append(cmds, m.splashModel.Init())
	}
	cmds = append(cmds, scheduleIdleCheck())

	return tea.Batch(cmds...)
}

// Update handles messages and updates the model
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		m.lastActivity = time.Now()
	}

	switch msg := msg.(type) {
	case idleCheckMsg:
		if m.shouldAutoLock(time.Time(msg)) {
			locked, cmd := m.autoLock()
			return locked, tea.Batch(cmd, scheduleIdleCheck())
		}
		return m, scheduleIdleCheck()

	case tea.KeyMsg:
		m.suppressWheelActive = true
		m.suppressWheelMaxUntil = time.Now().Add(wheelSuppressMaxWindow)
		m.suppressWheelToken++
		cmds = append(cmds, scheduleWheelSuppressRelease(m.suppressWheelToken, wheelSuppressAfterKey))

		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+t" {
			cmds = append(cmds, m.alertModel.NewAlertCmd(AlertTypeNewTx, "Test alert - notifications working!"))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.contentViewport.Width = msg.Width
		m.contentViewport.Height = msg.Height
	case BackendChosenMsg:
		return m, m.applySavedBackend()
	case ChooseServerMsg:
		m.setupNetwork = msg.Network
		return m.Update(NavigateMsg{Target: StateConnectionSetup})
	case BackendChangedMsg:
		var restart tea.Cmd
		m, restart = m.restartListener()
		return m, tea.Batch(restart, Navigate(msg.Then))
	case NavigateMsg:
		if msg.Target == StateWalletSetup && m.state == StateSplash {
			if m.configManager != nil && m.configManager.IsFirstRun() {
				msg.Target = StateConnectionSetup
			}
		}

		prevState := m.state

		m.state = msg.Target
		m.contentViewport.GotoTop()
		m.suppressWheelActive = true
		m.suppressWheelMaxUntil = time.Now().Add(wheelSuppressMaxWindow)
		m.suppressWheelToken++
		cmds = append(cmds, scheduleWheelSuppressRelease(m.suppressWheelToken, wheelSuppressAfterNav))

		var isNewModel bool
		switch msg.Target {
		case StateConnectionSetup:
			returnToSettings := prevState == StateSettings
			cs := NewConnectionSetupModel(m.configManager, returnToSettings)
			cs.returnToUnlock = prevState == StateUnlock
			if m.setupNetwork != "" {
				cs.setNetwork(m.setupNetwork)
				m.setupNetwork = ""
			}
			m.connectionSetupModel = cs
			isNewModel = true
		case StateWalletSetup:
			if m.walletSetupModel == nil {
				m.walletSetupModel = NewWalletSetupModel(m.walletManager, m.configManager)
				isNewModel = true
			}
		case StateUnlock:
			m.unlockModel = NewUnlockModel(m.walletManager)
			isNewModel = true
		case StateMainDashboard:
			if m.mainDashboardModel == nil {
				m.mainDashboardModel = NewMainDashboardModel(m.walletManager, m.configManager)
				isNewModel = true
				m.lastActivity = time.Now()
				// One listener per process; it keeps re-arming itself, so
				// re-creating the dashboard after an auto-lock must not
				// start a second one.
				if !m.listenerStarted {
					m.listenerStarted = true
					debugLog("[ROOT] Starting notification listener for dashboard")
					cmds = append(cmds, StartListeningWithSubscriptions(m.walletManager, m.listenGen, m.listenStop))
				}
			}
		case StateReceive:
			m.receiveModel = NewReceiveModel(m.walletManager)
			isNewModel = true
		case StateSend:
			state := m.walletManager.GetState()
			m.sendModel = NewSendModel(m.walletManager, state.Balance, state.UTXOs)
			isNewModel = true
		case StateTransactionHistory:
			if m.transactionHistoryModel == nil {
				m.transactionHistoryModel = NewTransactionHistoryModel(m.walletManager)
				isNewModel = true
			}
		case StateAddresses:
			m.addressesModel = NewAddressesModel(m.walletManager, m.configManager)
			isNewModel = true
		case StateUTXOs:
			m.utxosModel = NewUTXOsModel(m.walletManager, m.configManager)
			isNewModel = true
		case StateSettings:
			if m.settingsModel == nil {
				m.settingsModel = NewSettingsModel(m.walletManager, m.configManager)
				isNewModel = true
			}
		case StateHelp:
			if m.helpModel == nil {
				m.helpModel = NewHelpModel(m.walletManager)
				isNewModel = true
			}
		}

		updateAlertAndReturn := func(otherCmd tea.Cmd) (RootModel, tea.Cmd) {
			var alertCmd tea.Cmd
			var outAlert tea.Model
			outAlert, alertCmd = m.alertModel.Update(msg)
			m.alertModel = outAlert.(alert.AlertModel)
			allCmds := append(cmds, alertCmd)
			if otherCmd != nil {
				allCmds = append(allCmds, otherCmd)
			}
			return m, tea.Batch(allCmds...)
		}

		if isNewModel {
			if m.width > 0 {
				sized, _ := m.getActiveModel().Update(tea.WindowSizeMsg{
					Width: m.width, Height: m.height,
				})
				m.setActiveModel(msg.Target, sized)
			}
			if initializer, ok := m.getActiveModel().(interface{ Init() tea.Cmd }); ok {
				return updateAlertAndReturn(initializer.Init())
			}
		} else {
			activeModel := m.getActiveModel()
			if activeModel != nil {
				var activeCmd tea.Cmd
				var updated tea.Model
				updated, activeCmd = activeModel.Update(ScreenActiveMsg{})
				m.setActiveModel(msg.Target, updated)
				if activeCmd != nil {
					return updateAlertAndReturn(activeCmd)
				}
			}
		}

		if msg.Refresh && msg.Target == StateMainDashboard {
			if m.mainDashboardModel != nil {
				var refreshCmd tea.Cmd
				m.mainDashboardModel, refreshCmd = m.mainDashboardModel.Update(RefreshBalanceMsg{})
				return updateAlertAndReturn(refreshCmd)
			}
		}

		return updateAlertAndReturn(nil)

	case SuppressWheelMsg:
		dur := msg.Duration
		if dur <= 0 {
			dur = wheelSuppressAfterNav
		}
		m.suppressWheelActive = true
		m.suppressWheelMaxUntil = time.Now().Add(wheelSuppressMaxWindow)
		m.suppressWheelToken++
		cmds = append(cmds, scheduleWheelSuppressRelease(m.suppressWheelToken, dur))

	case wheelSuppressReleaseMsg:
		if msg.Token != m.suppressWheelToken {
			break
		}
		if !m.suppressWheelMaxUntil.IsZero() && time.Now().After(m.suppressWheelMaxUntil) {
			m.suppressWheelActive = false
			break
		}
		m.suppressWheelActive = false

	case ErrorMsg:
		m.err = msg.Err

	case BalanceMsg:
		if m.mainDashboardModel != nil {
			m.mainDashboardModel, cmd = m.mainDashboardModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		var outAlert tea.Model
		outAlert, cmd = m.alertModel.Update(msg)
		m.alertModel = outAlert.(alert.AlertModel)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case UTXOsMsg:
	case StateChangedMsg:
		if m.mainDashboardModel != nil {
			m.mainDashboardModel, cmd = m.mainDashboardModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.sendModel != nil {
			m.sendModel, cmd = m.sendModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.receiveModel != nil {
			m.receiveModel, cmd = m.receiveModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.transactionHistoryModel != nil {
			m.transactionHistoryModel, cmd = m.transactionHistoryModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		var outAlert tea.Model
		outAlert, cmd = m.alertModel.Update(msg)
		m.alertModel = outAlert.(alert.AlertModel)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case TransactionsMsg:
		if m.transactionHistoryModel != nil {
			m.transactionHistoryModel, cmd = m.transactionHistoryModel.Update(msg)
			cmds = append(cmds, cmd)
		}
		var outAlert tea.Model
		outAlert, cmd = m.alertModel.Update(msg)
		m.alertModel = outAlert.(alert.AlertModel)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case NewTransactionNotification:
		debugLog("[ROOT] NewTransactionNotification received: %s", msg.Address)
		if msg.Gen != m.listenGen {
			return m, nil // from a backend we no longer use
		}

		cmds = append(cmds, calculateAndShowBalanceChange(m.walletManager, &m.alertModel, msg.Gen))

	case PostBroadcastRefreshDoneMsg:
		debugLog("[ROOT] PostBroadcastRefreshDoneMsg received: change=%d", msg.BalanceChange)
		if msg.BalanceChange != 0 {
			var alertMsg string
			if msg.BalanceChange > 0 {
				btc := float64(msg.BalanceChange) / 100000000.0
				alertMsg = fmt.Sprintf("+%.8f BTC", btc)
			} else {
				btc := float64(-msg.BalanceChange) / 100000000.0
				alertMsg = fmt.Sprintf("-%.8f BTC", btc)
			}
			cmds = append(cmds, m.alertModel.NewAlertCmd(AlertTypeNewTx, alertMsg))
		}
		stateMsg := StateChangedMsg{State: msg.State}
		if m.mainDashboardModel != nil {
			m.mainDashboardModel, cmd = m.mainDashboardModel.Update(stateMsg)
			cmds = append(cmds, cmd)
		}
		if m.sendModel != nil {
			m.sendModel, cmd = m.sendModel.Update(stateMsg)
			cmds = append(cmds, cmd)
		}
		if m.transactionHistoryModel != nil {
			m.transactionHistoryModel, cmd = m.transactionHistoryModel.Update(stateMsg)
			cmds = append(cmds, cmd)
		}

	case BalanceChangeMsg:
		debugLog("[ROOT] BalanceChangeMsg received: change=%d", msg.Change)
		if msg.Change != 0 {
			var alertMsg string
			if msg.Change > 0 {
				btc := float64(msg.Change) / 100000000.0
				alertMsg = fmt.Sprintf("+%.8f BTC", btc)
			} else {
				btc := float64(-msg.Change) / 100000000.0
				alertMsg = fmt.Sprintf("-%.8f BTC", btc)
			}
			debugLog("[ROOT] Showing alert: %s", alertMsg)
			cmds = append(cmds, m.alertModel.NewAlertCmd(AlertTypeNewTx, alertMsg))
		} else {
			debugLog("[ROOT] Skipping notification - no balance change")
		}
		if msg.Gen == m.listenGen {
			cmds = append(cmds, ContinueListening(m.walletManager, m.listenGen, m.listenStop))
		}
		if m.mainDashboardModel != nil {
			m.mainDashboardModel, cmd = m.mainDashboardModel.Update(StateChangedMsg{State: msg.State})
			cmds = append(cmds, cmd)
		}
		if m.sendModel != nil {
			m.sendModel, cmd = m.sendModel.Update(StateChangedMsg{State: msg.State})
			cmds = append(cmds, cmd)
		}
		if m.transactionHistoryModel != nil {
			m.transactionHistoryModel, cmd = m.transactionHistoryModel.Update(StateChangedMsg{State: msg.State})
			cmds = append(cmds, cmd)
		}
		if msg.Change != 0 {
			cmds = append(cmds, asyncRefreshAndEmitState(m.walletManager))
		}

	case NewBlockNotification:
		debugLog("[ROOT] NewBlockNotification received: block %d", msg.Height)
		if msg.Gen != m.listenGen {
			return m, nil // from a backend we no longer use
		}
		cmds = append(cmds, m.alertModel.NewAlertCmd(AlertTypeNewBlock, fmt.Sprintf("New Block: #%d", msg.Height)))
		cmds = append(cmds, ContinueListening(m.walletManager, m.listenGen, m.listenStop))

		m.walletManager.GetStatePtr().UpdateBlockHeight(msg.Height)
		stateMsg := StateChangedMsg{State: m.walletManager.GetState()}
		if m.mainDashboardModel != nil {
			m.mainDashboardModel, cmd = m.mainDashboardModel.Update(stateMsg)
			cmds = append(cmds, cmd)
		}
		if m.transactionHistoryModel != nil {
			m.transactionHistoryModel, cmd = m.transactionHistoryModel.Update(stateMsg)
			cmds = append(cmds, cmd)
		}
	}

	var alertCmd tea.Cmd
	var outAlert tea.Model
	outAlert, alertCmd = m.alertModel.Update(msg)
	m.alertModel = outAlert.(alert.AlertModel)
	if alertCmd != nil {
		cmds = append(cmds, alertCmd)
	}

	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		if m.suppressWheelActive {
			if !m.suppressWheelMaxUntil.IsZero() && time.Now().After(m.suppressWheelMaxUntil) {
				m.suppressWheelActive = false
			} else {
				_ = mouseMsg
				m.suppressWheelToken++
				cmds = append(cmds, scheduleWheelSuppressRelease(m.suppressWheelToken, wheelSuppressQuietWindow))
				return m, tea.Batch(cmds...)
			}
		}
		if m.suppressWheelActive {
			return m, tea.Batch(cmds...)
		}
	}

	// Route messages to active sub-model
	if activeModel := m.getActiveModel(); activeModel != nil {
		var updated tea.Model
		updated, cmd = activeModel.Update(msg)
		m.setActiveModel(m.state, updated)
		cmds = append(cmds, cmd)
	}

	// Update viewport with current screen content for scroll support
	if activeModel := m.getActiveModel(); activeModel != nil && m.usesRootViewport() {
		m.contentViewport.SetContent(activeModel.View())
	}

	// Forward mouse events to viewport for scroll wheel
	if _, ok := msg.(tea.MouseMsg); ok && m.usesRootViewport() {
		m.contentViewport, cmd = m.contentViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Forward PgUp/PgDn to viewport for keyboard scrolling
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if (keyMsg.String() == "pgup" || keyMsg.String() == "pgdown") && m.usesRootViewport() {
			if m.contentViewport.TotalLineCount() > m.height {
				m.contentViewport, cmd = m.contentViewport.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// getActiveModel returns the currently active model
func (m RootModel) getActiveModel() tea.Model {
	switch m.state {
	case StateSplash:
		return m.splashModel
	case StateConnectionSetup:
		return m.connectionSetupModel
	case StateWalletSetup:
		return m.walletSetupModel
	case StateUnlock:
		return m.unlockModel
	case StateMainDashboard:
		return m.mainDashboardModel
	case StateReceive:
		return m.receiveModel
	case StateSend:
		return m.sendModel
	case StateTransactionHistory:
		return m.transactionHistoryModel
	case StateAddresses:
		return m.addressesModel
	case StateUTXOs:
		return m.utxosModel
	case StateSettings:
		return m.settingsModel
	case StateHelp:
		return m.helpModel
	default:
		return nil
	}
}

// setActiveModel assigns a model back to the correct field for the given state.
// This is the write counterpart to getActiveModel, needed because BubbleTea
// uses value semantics (Update returns a new model that must be stored).
func (m *RootModel) setActiveModel(state AppState, model tea.Model) {
	switch state {
	case StateSplash:
		m.splashModel = model
	case StateConnectionSetup:
		m.connectionSetupModel = model
	case StateWalletSetup:
		m.walletSetupModel = model
	case StateUnlock:
		m.unlockModel = model
	case StateMainDashboard:
		m.mainDashboardModel = model
	case StateReceive:
		m.receiveModel = model
	case StateSend:
		m.sendModel = model
	case StateTransactionHistory:
		m.transactionHistoryModel = model
	case StateAddresses:
		m.addressesModel = model
	case StateUTXOs:
		m.utxosModel = model
	case StateSettings:
		m.settingsModel = model
	case StateHelp:
		m.helpModel = model
	}
}

// View renders the active screen
func (m RootModel) View() string {
	stateNames := map[AppState]string{
		StateSplash:             "Splash",
		StateConnectionSetup:    "ConnectionSetup",
		StateWalletSetup:        "WalletSetup",
		StateUnlock:             "Unlock",
		StateMainDashboard:      "Dashboard",
		StateReceive:            "Receive",
		StateSend:               "Send",
		StateTransactionHistory: "History",
		StateAddresses:          "Addresses",
		StateUTXOs:              "UTXOs",
		StateSettings:           "Settings",
		StateHelp:               "Help",
	}

	model := m.getActiveModel()
	if model == nil {
		return fmt.Sprintf("State: %s\nError: No active model\nWidth: %d Height: %d",
			stateNames[m.state], m.width, m.height)
	}

	if m.width == 0 {
		view := model.View()
		return fmt.Sprintf("[DEBUG] State: %s | Width: %d | Height: %d\n\n%s",
			stateNames[m.state], m.width, m.height, view)
	}

	// Screens with internal scrolling manage their own layout and clipping.
	if !m.usesRootViewport() {
		return m.alertModel.Render(model.View())
	}

	view := m.contentViewport.View()

	if m.contentViewport.TotalLineCount() > m.height {
		lines := strings.Split(view, "\n")
		if len(lines) > 0 {
			if m.contentViewport.ScrollPercent() < 1.0 {
				scrollHint := styles.DimText.Render("  ↓ PgDn/scroll for more")
				lines[len(lines)-1] = scrollHint
			}
			if m.contentViewport.YOffset > 0 {
				scrollHint := styles.DimText.Render("  ↑ PgUp to scroll up")
				lines[0] = scrollHint
			}
			view = strings.Join(lines, "\n")
		}
	}

	return m.alertModel.Render(view)
}

// renderScrollableView renders content with a fixed header and footer, clipping
// the middle content area to fit the terminal height. When content overflows, it
// uses scrollOffset to determine which slice is visible and shows a scroll indicator.
func renderScrollableView(height, scrollOffset int, header, content, footer string) string {
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	availableHeight := height - headerHeight - footerHeight - 2 // -2 for newline separators

	contentLines := strings.Split(content, "\n")
	totalContent := len(contentLines)

	if availableHeight < 1 || totalContent <= availableHeight {
		return header + "\n" + content + "\n" + footer
	}

	maxOffset := totalContent - availableHeight
	offset := scrollOffset
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}

	end := offset + availableHeight
	if end > totalContent {
		end = totalContent
	}
	visible := strings.Join(contentLines[offset:end], "\n")

	scrollPct := 0
	if maxOffset > 0 {
		scrollPct = (offset * 100) / maxOffset
	}
	var indicator string
	if offset > 0 && end < totalContent {
		indicator = styles.ScrollIndicatorStyle.Render(fmt.Sprintf(" ↑↓ %d%%", scrollPct))
	} else if end < totalContent {
		indicator = styles.ScrollIndicatorStyle.Render(" ↓ scroll")
	} else {
		indicator = styles.ScrollIndicatorStyle.Render(fmt.Sprintf(" ↑ %d%%", scrollPct))
	}

	return header + indicator + "\n" + visible + "\n" + footer
}

// applySavedBackend switches the wallet manager to the backend saved in
// config, then returns to the unlock screen. Errors are left for the next
// unlock attempt to report.
func (m RootModel) applySavedBackend() tea.Cmd {
	wm, cm := m.walletManager, m.configManager
	return func() tea.Msg {
		if wm != nil && cm != nil {
			if cfg, err := cm.Load(); err == nil && cfg != nil && cfg.Backend != nil {
				if b, err := backend.NewBackend(cfg.Backend); err == nil {
					if err := wm.SetBackend(b); err != nil {
						log.Printf("apply backend: %v", err)
					}
				}
			}
		}
		return BackendChangedMsg{Then: StateUnlock}
	}
}

// restartListener stops the notification listener bound to the previous
// backend and, if a wallet is open, starts one on the current backend.
func (m RootModel) restartListener() (RootModel, tea.Cmd) {
	if m.listenStop != nil {
		close(m.listenStop)
	}
	m.listenStop = make(chan struct{})
	m.listenGen++
	m.listenerStarted = false
	if m.walletManager != nil && m.walletManager.IsUnlocked() && m.mainDashboardModel != nil {
		m.listenerStarted = true
		return m, StartListeningWithSubscriptions(m.walletManager, m.listenGen, m.listenStop)
	}
	return m, nil
}
