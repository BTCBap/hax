package models

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ConnectionSetupStep represents the current step in connection setup
type ConnectionSetupStep int

const (
	StepChooseBackend ConnectionSetupStep = iota
	StepConfigureElectrum
	StepConfigureRPC
	StepTestConnection
	StepConnectionComplete
)

// ButtonState represents the state of the connect/test button
type ButtonState int

const (
	ButtonConnect ButtonState = iota
	ButtonTesting
	ButtonSuccess
	ButtonRetry
)

// ConnectionTestMsg is sent when a connection test completes
type ConnectionTestMsg struct {
	Success         bool
	Error           error
	ServerSoftware  string
	ProtocolVersion string
	BlockHeight     int64
	CertFingerprint string
}

// ConnectionSetupModel handles connection configuration
type ConnectionSetupModel struct {
	width  int
	height int

	// Navigation
	step        ConnectionSetupStep
	cursor      int
	buttonState ButtonState

	// Managers
	configManager *config.ConfigManager

	// UI components
	spinner      spinner.Model
	hostInput    textinput.Model
	portInput    textinput.Model
	userInput    textinput.Model
	passInput    textinput.Model
	torPortInput textinput.Model

	// Data
	selectedBackendType backend.BackendType
	selectedServer      int // Index into combined server list (public + recent)
	customServer        bool
	network             string
	err                 error

	// Electrum settings
	useTor bool
	useSSL bool

	// Recent servers loaded from config
	recentServers []backend.SavedServer

	// Server info from last successful test
	serverSoftware  string
	protocolVersion string
	blockHeight     int64
	certFingerprint string

	// Return navigation
	returnToSettings bool
	// returnToUnlock is set when the unlock screen sent the user here to
	// pick a server that matches the wallet's network.
	returnToUnlock bool
}

// ChooseServerMsg opens connection setup from the unlock screen, listing
// servers for Network (the locked wallet's network, if known).
type ChooseServerMsg struct{ Network string }

// BackendChangedMsg reports that the wallet manager switched to a new
// backend; the root model restarts the notification listener on it.
type BackendChangedMsg struct {
	Then AppState
	Err  error // set when the saved backend could not be applied
}

// setNetwork selects which network's servers are offered.
func (m *ConnectionSetupModel) setNetwork(network string) {
	switch backend.NormalizeNetwork(network) {
	case "mainnet":
		m.network = "mainnet"
	case "testnet":
		m.network = "testnet"
	}
}

// BackendChosenMsg asks the root model to apply the saved backend config to
// the wallet manager and return to the unlock screen.
type BackendChosenMsg struct{ Then AppState }

// NewConnectionSetupModel creates a new connection setup model
func NewConnectionSetupModel(configManager *config.ConfigManager, returnToSettings bool) ConnectionSetupModel {
	s := styles.NewStandardSpinner()

	hostInput := textinput.New()
	hostInput.Placeholder = "hostname or IP"
	hostInput.CharLimit = 100

	portInput := textinput.New()
	portInput.Placeholder = "port"
	portInput.CharLimit = 5

	userInput := textinput.New()
	userInput.Placeholder = "username"
	userInput.CharLimit = 50

	passInput := textinput.New()
	passInput.Placeholder = "password"
	passInput.EchoMode = textinput.EchoPassword
	passInput.CharLimit = 100

	torPortInput := textinput.New()
	torPortInput.Placeholder = "9050"
	torPortInput.SetValue("9050")
	torPortInput.CharLimit = 5

	var recentServers []backend.SavedServer
	if configManager != nil {
		if cfg, err := configManager.Load(); err == nil && cfg != nil {
			recentServers = cfg.RecentServers
		}
	}

	return ConnectionSetupModel{
		step:             StepChooseBackend,
		cursor:           0,
		buttonState:      ButtonConnect,
		configManager:    configManager,
		spinner:          s,
		hostInput:        hostInput,
		portInput:        portInput,
		userInput:        userInput,
		passInput:        passInput,
		torPortInput:     torPortInput,
		network:          "testnet",
		useSSL:           true,
		recentServers:    recentServers,
		returnToSettings: returnToSettings,
	}
}

func (m ConnectionSetupModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func (m ConnectionSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		if m.buttonState == ButtonTesting {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
			cmds = append(cmds, m.spinner.Tick)
		}

	case ConnectionTestMsg:
		if msg.Success {
			m.buttonState = ButtonSuccess
			m.err = nil
			m.serverSoftware = msg.ServerSoftware
			m.protocolVersion = msg.ProtocolVersion
			m.blockHeight = msg.BlockHeight
			m.certFingerprint = msg.CertFingerprint
		} else {
			m.buttonState = ButtonRetry
			m.err = msg.Error
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.step == StepChooseBackend {
				if m.returnToSettings {
					return m, Navigate(StateSettings)
				}
				if m.returnToUnlock {
					return m, Navigate(StateUnlock)
				}
				return m, tea.Quit
			}
			m.step = m.getPreviousStep()
			m.buttonState = ButtonConnect
			m.err = nil
			return m, nil
		}

		// Handle input based on current step
		switch m.step {
		case StepChooseBackend:
			return m.handleChooseBackend(msg)

		case StepConfigureElectrum:
			return m.handleConfigureElectrum(msg)

		case StepConfigureRPC:
			return m.handleConfigureRPC(msg)

		case StepTestConnection:
			return m.handleTestConnection(msg)

		case StepConnectionComplete:
			return m.handleComplete(msg)
		}
	}

	// Update text inputs if in custom server mode
	if m.customServer {
		m.hostInput, cmd = m.hostInput.Update(msg)
		cmds = append(cmds, cmd)
		m.portInput, cmd = m.portInput.Update(msg)
		cmds = append(cmds, cmd)

		// Auto-detect .onion addresses
		if strings.HasSuffix(m.hostInput.Value(), ".onion") {
			m.useTor = true
			m.useSSL = false
		}
	}

	// Update Tor port input if Tor is enabled
	if m.useTor && m.step == StepConfigureElectrum {
		m.torPortInput, cmd = m.torPortInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update RPC inputs
	if m.step == StepConfigureRPC {
		m.hostInput, cmd = m.hostInput.Update(msg)
		cmds = append(cmds, cmd)
		m.portInput, cmd = m.portInput.Update(msg)
		cmds = append(cmds, cmd)
		m.userInput, cmd = m.userInput.Update(msg)
		cmds = append(cmds, cmd)
		m.passInput, cmd = m.passInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// customServerIndex returns the cursor index for the Custom Server option
func (m ConnectionSetupModel) customServerIndex() int {
	servers := backend.PublicElectrumServers[m.network]
	return len(servers) + len(m.recentServers)
}

func (m ConnectionSetupModel) handleChooseBackend(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 { // 2 options now (Electrum, RPC)
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case 0:
			m.selectedBackendType = backend.BackendTypeElectrum
			m.step = StepConfigureElectrum
			m.cursor = 0
		case 1:
			m.selectedBackendType = backend.BackendTypeRPC
			m.step = StepConfigureRPC
			return m, m.hostInput.Focus()
		}
		m.cursor = 0
	}
	return m, nil
}

func (m ConnectionSetupModel) handleConfigureElectrum(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxIndex := m.customServerIndex()

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.customServer = (m.cursor == maxIndex)
		}
	case "down", "j":
		if m.cursor < maxIndex {
			m.cursor++
			m.customServer = (m.cursor == maxIndex)
		}
	case "t", "T":
		// Toggle Tor proxy
		m.useTor = !m.useTor
		if m.useTor {
			m.hostInput.Blur()
			m.portInput.Blur()
			return m, m.torPortInput.Focus()
		}
		m.torPortInput.Blur()
		return m, nil
	case "s", "S":
		// Toggle SSL (only for custom servers)
		if m.customServer {
			m.useSSL = !m.useSSL
		}
		return m, nil
	case "tab":
		if m.customServer && m.useTor {
			if m.hostInput.Focused() {
				m.hostInput.Blur()
				return m, m.portInput.Focus()
			} else if m.portInput.Focused() {
				m.portInput.Blur()
				return m, m.torPortInput.Focus()
			} else {
				m.torPortInput.Blur()
				return m, m.hostInput.Focus()
			}
		} else if m.customServer {
			if m.hostInput.Focused() {
				m.hostInput.Blur()
				return m, m.portInput.Focus()
			} else {
				m.portInput.Blur()
				return m, m.hostInput.Focus()
			}
		} else if m.useTor {
			if !m.torPortInput.Focused() {
				return m, m.torPortInput.Focus()
			}
		}
	case "enter":
		if m.customServer {
			if m.hostInput.Value() == "" {
				m.err = fmt.Errorf("server host is required")
				return m, nil
			}
			port, err := strconv.Atoi(m.portInput.Value())
			if err != nil || port <= 0 || port > 65535 {
				m.err = fmt.Errorf("invalid port number")
				return m, nil
			}
		}
		if m.useTor {
			torPort, err := strconv.Atoi(m.torPortInput.Value())
			if err != nil || torPort <= 0 || torPort > 65535 {
				m.err = fmt.Errorf("invalid Tor SOCKS5 port number")
				return m, nil
			}
		}
		m.selectedServer = m.cursor
		m.step = StepTestConnection
		m.buttonState = ButtonConnect
		m.err = nil
		return m, nil
	case "x", "X":
		// Delete recent server under cursor
		publicCount := len(backend.PublicElectrumServers[m.network])
		recentIdx := m.cursor - publicCount
		if recentIdx >= 0 && recentIdx < len(m.recentServers) {
			m.recentServers = append(m.recentServers[:recentIdx], m.recentServers[recentIdx+1:]...)
			// Adjust cursor if needed
			if m.cursor >= m.customServerIndex() {
				m.cursor = m.customServerIndex()
				m.customServer = true
			}
			// Persist the deletion
			if err := m.saveRecentServers(); err != nil {
				m.err = fmt.Errorf("could not save recent servers: %w", err)
			}
		}
		return m, nil
	}

	// Focus host input when entering custom mode
	if m.customServer && !m.hostInput.Focused() && !m.portInput.Focused() && !m.torPortInput.Focused() {
		return m, m.hostInput.Focus()
	}

	return m, nil
}

func (m ConnectionSetupModel) handleConfigureRPC(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if m.hostInput.Focused() {
			m.hostInput.Blur()
			return m, m.portInput.Focus()
		} else if m.portInput.Focused() {
			m.portInput.Blur()
			return m, m.userInput.Focus()
		} else if m.userInput.Focused() {
			m.userInput.Blur()
			return m, m.passInput.Focus()
		} else {
			m.passInput.Blur()
			return m, m.hostInput.Focus()
		}
	case "enter":
		if m.hostInput.Value() == "" {
			m.err = fmt.Errorf("RPC host is required")
			return m, nil
		}
		port, err := strconv.Atoi(m.portInput.Value())
		if err != nil || port <= 0 || port > 65535 {
			m.err = fmt.Errorf("invalid port number")
			return m, nil
		}
		if m.userInput.Value() == "" {
			m.err = fmt.Errorf("RPC username is required")
			return m, nil
		}
		if m.passInput.Value() == "" {
			m.err = fmt.Errorf("RPC password is required")
			return m, nil
		}
		m.step = StepTestConnection
		m.buttonState = ButtonConnect
		m.err = nil
	}
	return m, nil
}

func (m ConnectionSetupModel) handleTestConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		switch m.buttonState {
		case ButtonConnect, ButtonRetry:
			m.buttonState = ButtonTesting
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, m.testConnection())

		case ButtonSuccess:
			if err := m.saveConfig(); err != nil {
				m.err = err
				m.buttonState = ButtonRetry
				return m, nil
			}
			m.step = StepConnectionComplete
		}
	}
	return m, nil
}

func (m ConnectionSetupModel) handleComplete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.returnToSettings {
			return m, func() tea.Msg { return BackendChosenMsg{Then: StateSettings} }
		}
		if m.returnToUnlock {
			return m, func() tea.Msg { return BackendChosenMsg{Then: StateUnlock} }
		}
		return m, Navigate(StateWalletSetup)
	}
	return m, nil
}

func (m ConnectionSetupModel) testConnection() tea.Cmd {
	cfg := m.buildBackendConfig()

	return func() tea.Msg {
		b, err := backend.NewBackend(cfg)
		if err != nil {
			return ConnectionTestMsg{Success: false, Error: err}
		}

		if err := b.Connect(); err != nil {
			return ConnectionTestMsg{Success: false, Error: err}
		}
		defer func() { _ = b.Disconnect() }()

		err = b.TestConnection()
		if err != nil {
			return ConnectionTestMsg{Success: false, Error: err}
		}

		result := ConnectionTestMsg{Success: true}

		result.ServerSoftware, result.ProtocolVersion = b.ServerInfo()
		result.CertFingerprint = b.CertFingerprint()
		if height, err := b.GetBlockHeight(); err == nil {
			result.BlockHeight = height
		}

		return result
	}
}

func (m ConnectionSetupModel) buildBackendConfig() *backend.BackendConfig {
	switch m.selectedBackendType {
	case backend.BackendTypeElectrum:
		publicServers := backend.PublicElectrumServers[m.network]
		var electrumConfig *backend.ElectrumConfig

		if m.customServer {
			port, _ := strconv.Atoi(m.portInput.Value())
			electrumConfig = &backend.ElectrumConfig{
				Host:   m.hostInput.Value(),
				Port:   port,
				UseSSL: m.useSSL,
			}
		} else if m.selectedServer < len(publicServers) {
			server := publicServers[m.selectedServer]
			electrumConfig = &backend.ElectrumConfig{
				Host:   server.Host,
				Port:   server.Port,
				UseSSL: server.UseSSL,
			}
		} else {
			// Recent server
			recentIdx := m.selectedServer - len(publicServers)
			if recentIdx >= 0 && recentIdx < len(m.recentServers) {
				rs := m.recentServers[recentIdx]
				electrumConfig = &backend.ElectrumConfig{
					Host:   rs.Host,
					Port:   rs.Port,
					UseSSL: rs.UseSSL,
				}
				if rs.UseTor {
					m.useTor = true
				}
			} else {
				// Fallback
				return backend.DefaultElectrumConfig(m.network)
			}
		}

		if m.useTor {
			electrumConfig.UseTor = true
			electrumConfig.TorSOCKS5Port = m.torPortInput.Value()
		}

		return &backend.BackendConfig{
			Type:     backend.BackendTypeElectrum,
			Network:  m.network,
			Electrum: electrumConfig,
		}

	case backend.BackendTypeRPC:
		port, _ := strconv.Atoi(m.portInput.Value())
		return &backend.BackendConfig{
			Type:    backend.BackendTypeRPC,
			Network: m.network,
			RPC: &backend.RPCConfig{
				Host:     m.hostInput.Value(),
				Port:     port,
				Username: m.userInput.Value(),
				Password: m.passInput.Value(),
				UseSSL:   false,
			},
		}
	}

	return backend.DefaultElectrumConfig(m.network)
}

// serverName returns a display name for the currently selected server
func (m ConnectionSetupModel) serverName() string {
	publicServers := backend.PublicElectrumServers[m.network]
	if m.customServer {
		host := m.hostInput.Value()
		if host == "" {
			return "Custom"
		}
		return host
	} else if m.selectedServer < len(publicServers) {
		return publicServers[m.selectedServer].Name
	} else {
		recentIdx := m.selectedServer - len(publicServers)
		if recentIdx >= 0 && recentIdx < len(m.recentServers) {
			return m.recentServers[recentIdx].Name
		}
	}
	return "Unknown"
}

func (m ConnectionSetupModel) saveConfig() error {
	// Load existing config to preserve other settings
	existing, _ := m.configManager.Load()
	cfg := &config.AppConfig{
		Version:          1,
		FirstRunComplete: true,
		Backend:          m.buildBackendConfig(),
	}

	// Merge existing settings
	if existing != nil {
		cfg.LastWalletName = existing.LastWalletName
		cfg.DisplayUnit = existing.DisplayUnit
		cfg.GapLimit = existing.GapLimit
		cfg.RecentServers = existing.RecentServers
	}

	// Add current server to recent servers (Electrum only)
	if m.selectedBackendType == backend.BackendTypeElectrum && cfg.Backend.Electrum != nil {
		ec := cfg.Backend.Electrum
		saved := backend.SavedServer{
			Name:   m.serverName(),
			Host:   ec.Host,
			Port:   ec.Port,
			UseSSL: ec.UseSSL,
			UseTor: ec.UseTor,
		}

		// Deduplicate by host:port and prepend
		deduped := []backend.SavedServer{saved}
		for _, rs := range cfg.RecentServers {
			if rs.Host == saved.Host && rs.Port == saved.Port {
				continue
			}
			deduped = append(deduped, rs)
		}
		// Cap at 5
		if len(deduped) > 5 {
			deduped = deduped[:5]
		}
		cfg.RecentServers = deduped
	}

	return m.configManager.Save(cfg)
}

// saveRecentServers persists the current recent servers list (for deletions)
func (m ConnectionSetupModel) saveRecentServers() error {
	if m.configManager == nil {
		return nil
	}
	existing, err := m.configManager.Load()
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	existing.RecentServers = m.recentServers
	if err := m.configManager.Save(existing); err != nil {
		debugLog("[CONNECTION] saving recent servers failed: %v", err)
		return err
	}
	return nil
}

func (m ConnectionSetupModel) getPreviousStep() ConnectionSetupStep {
	switch m.step {
	case StepConfigureElectrum, StepConfigureRPC:
		return StepChooseBackend
	case StepTestConnection:
		switch m.selectedBackendType {
		case backend.BackendTypeElectrum:
			return StepConfigureElectrum
		case backend.BackendTypeRPC:
			return StepConfigureRPC
		}
	case StepConnectionComplete:
		return StepTestConnection
	}
	return StepChooseBackend
}

func (m ConnectionSetupModel) View() string {
	var content strings.Builder

	switch m.step {
	case StepChooseBackend:
		content.WriteString(m.viewChooseBackend())
	case StepConfigureElectrum:
		content.WriteString(m.viewConfigureElectrum())
	case StepConfigureRPC:
		content.WriteString(m.viewConfigureRPC())
	case StepTestConnection:
		content.WriteString(m.viewTestConnection())
	case StepConnectionComplete:
		content.WriteString(m.viewComplete())
	}

	// Show error if present
	if m.err != nil {
		content.WriteString("\n\n")
		content.WriteString(styles.RenderErrorBox(m.err.Error(), m.width))
	}

	return content.String()
}

func (m ConnectionSetupModel) viewChooseBackend() string {
	title := styles.TitleStyle.Render("Connection Setup")
	subtitle := styles.SubtitleStyle.Render("Choose how to connect to Bitcoin network")

	options := []struct {
		name string
		desc string
	}{
		{"Electrum Server", "Direct protocol connection (Recommended)"},
		{"Bitcoin Core RPC", "Connect to your own Bitcoin Core node"},
	}

	menu := "\n"
	for i, opt := range options {
		cursor := "  "
		if m.cursor == i {
			cursor = styles.HighlightText.Render("▶ ")
			name := styles.SelectedItemStyle.Render(opt.name)
			desc := styles.DimText.Render(" - " + opt.desc)
			menu += cursor + name + desc + "\n"
		} else {
			name := styles.UnselectedItemStyle.Render(opt.name)
			desc := styles.DimText.Render(" - " + opt.desc)
			menu += cursor + name + desc + "\n"
		}
	}

	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "back"),
	)

	return styles.CenterHorizontal(title+"\n"+subtitle+menu+"\n"+help, m.width)
}

func (m ConnectionSetupModel) viewConfigureElectrum() string {
	title := styles.TitleStyle.Render("Electrum Server")
	subtitle := styles.SubtitleStyle.Render("Select a server or enter custom")

	publicServers := backend.PublicElectrumServers[m.network]

	menu := "\n"

	// Public servers
	for i, server := range publicServers {
		cursor := "  "
		if m.cursor == i && !m.customServer {
			cursor = styles.HighlightText.Render("▶ ")
			name := styles.SelectedItemStyle.Render(server.Name)
			host := styles.DimText.Render(fmt.Sprintf(" (%s:%d)", server.Host, server.Port))
			menu += cursor + name + host + "\n"
		} else {
			name := styles.UnselectedItemStyle.Render(server.Name)
			host := styles.DimText.Render(fmt.Sprintf(" (%s:%d)", server.Host, server.Port))
			menu += cursor + name + host + "\n"
		}
	}

	// Recent servers
	if len(m.recentServers) > 0 {
		menu += "\n"
		menu += styles.DimText.Render("  Recent Servers") + "\n"
		for i, rs := range m.recentServers {
			idx := len(publicServers) + i
			cursor := "  "
			if m.cursor == idx && !m.customServer {
				cursor = styles.HighlightText.Render("▶ ")
				name := styles.SelectedItemStyle.Render(rs.Name)
				detail := fmt.Sprintf(" (%s:%d", rs.Host, rs.Port)
				if rs.UseTor {
					detail += ", Tor"
				}
				detail += ")"
				host := styles.DimText.Render(detail)
				menu += cursor + name + host + "\n"
			} else {
				name := styles.UnselectedItemStyle.Render(rs.Name)
				detail := fmt.Sprintf(" (%s:%d", rs.Host, rs.Port)
				if rs.UseTor {
					detail += ", Tor"
				}
				detail += ")"
				host := styles.DimText.Render(detail)
				menu += cursor + name + host + "\n"
			}
		}
	}

	// Custom server option
	menu += "\n"
	cursor := "  "
	if m.customServer {
		cursor = styles.HighlightText.Render("▶ ")
		menu += cursor + styles.SelectedItemStyle.Render("Custom Server") + "\n"
		menu += "\n"
		menu += styles.InputLabelStyle.Render("Host: ") + m.hostInput.View() + "\n"
		menu += styles.InputLabelStyle.Render("Port: ") + m.portInput.View() + "\n"
		// SSL toggle
		sslToggle := "[ ] Use SSL/TLS"
		if m.useSSL {
			sslToggle = "[x] Use SSL/TLS"
		}
		menu += styles.InputLabelStyle.Render("  ") + sslToggle + "\n"
	} else {
		menu += cursor + styles.UnselectedItemStyle.Render("Custom Server") + "\n"
	}

	// Tor proxy settings
	menu += "\n"
	menu += styles.SubtitleStyle.Render("Proxy Settings") + "\n"
	torToggle := "[ ] Use Tor (SOCKS5 proxy)"
	if m.useTor {
		torToggle = "[x] Use Tor (SOCKS5 proxy)"
	}
	menu += styles.InputLabelStyle.Render("  ") + torToggle + "\n"
	if m.useTor {
		menu += styles.InputLabelStyle.Render("  SOCKS5 Port: ") + m.torPortInput.View() + "\n"
	}

	items := []styles.HelpItem{
		styles.HK("↑/↓", "navigate"),
		styles.HK("t", "toggle Tor"),
	}
	if m.customServer {
		items = append(items, styles.HK("s", "toggle SSL"))
	}
	publicCount := len(publicServers)
	recentIdx := m.cursor - publicCount
	if recentIdx >= 0 && recentIdx < len(m.recentServers) {
		items = append(items, styles.HK("x", "remove"))
	}
	items = append(items,
		styles.HK("tab", "fields"),
		styles.HK("enter", "test"),
		styles.HK("esc", "back"),
	)
	help := styles.RenderHelp(items...)

	return styles.CenterHorizontal(title+"\n"+subtitle+menu+"\n"+help, m.width)
}

func (m ConnectionSetupModel) viewConfigureRPC() string {
	title := styles.TitleStyle.Render("Bitcoin Core RPC")
	subtitle := styles.SubtitleStyle.Render("Enter your node connection details")

	form := "\n"
	form += styles.InputLabelStyle.Render("Host:     ") + m.hostInput.View() + "\n"
	form += styles.InputLabelStyle.Render("Port:     ") + m.portInput.View() + "\n"
	form += styles.InputLabelStyle.Render("Username: ") + m.userInput.View() + "\n"
	form += styles.InputLabelStyle.Render("Password: ") + m.passInput.View() + "\n"

	help := styles.RenderHelp(
		styles.HK("tab", "next field"),
		styles.HK("enter", "test connection"),
		styles.HK("esc", "back"),
	)

	return styles.CenterHorizontal(title+"\n"+subtitle+form+"\n"+help, m.width)
}

func (m ConnectionSetupModel) viewTestConnection() string {
	title := styles.TitleStyle.Render("Test Connection")

	// Show current config
	cfg := m.buildBackendConfig()
	var configInfo string
	switch cfg.Type {
	case backend.BackendTypeElectrum:
		configInfo = fmt.Sprintf("Electrum: %s:%d (SSL: %v)", cfg.Electrum.Host, cfg.Electrum.Port, cfg.Electrum.UseSSL)
		if cfg.Electrum.UseTor {
			configInfo += fmt.Sprintf("\nTor SOCKS5: 127.0.0.1:%s", cfg.Electrum.TorSOCKS5Port)
		}
	case backend.BackendTypeRPC:
		configInfo = fmt.Sprintf("RPC: %s:%d", cfg.RPC.Host, cfg.RPC.Port)
	}

	info := styles.InfoBoxStyle.Render(configInfo)

	// Button rendering
	var button string
	switch m.buttonState {
	case ButtonConnect:
		button = styles.HighlightText.Render("[ Connect ]")
	case ButtonTesting:
		button = m.spinner.View() + " " + styles.TestingStatusStyle.Render("Testing connection...")
	case ButtonSuccess:
		button = styles.SuccessText.Render("✓ Connected! Press Enter to continue")
	case ButtonRetry:
		button = styles.ErrorText.Render("✗ Connection failed - Press Enter to retry")
	}

	// Server info section (shown after successful connection)
	var serverInfo string
	if m.buttonState == ButtonSuccess && m.serverSoftware != "" {
		parts := []string{}
		parts = append(parts, fmt.Sprintf("Server: %s", m.serverSoftware))
		parts = append(parts, fmt.Sprintf("Protocol: %s", m.protocolVersion))
		if m.blockHeight > 0 {
			parts = append(parts, fmt.Sprintf("Block Height: %d", m.blockHeight))
		}
		if m.certFingerprint != "" {
			// Show truncated fingerprint (first 24 chars = 8 bytes)
			fp := m.certFingerprint
			if len(fp) > 23 {
				fp = fp[:23] + "..."
			}
			parts = append(parts, fmt.Sprintf("Cert: %s", fp))
		}
		serverInfo = "\n\n" + styles.InfoBoxStyle.MarginBottom(0).Render(strings.Join(parts, "\n"))
	}

	help := styles.RenderHelp(
		styles.HK("enter", m.getButtonAction()),
		styles.HK("esc", "back"),
	)

	return styles.CenterHorizontal(title+"\n\n"+info+"\n\n"+button+serverInfo+"\n\n"+help, m.width)
}

func (m ConnectionSetupModel) getButtonAction() string {
	switch m.buttonState {
	case ButtonConnect:
		return "connect"
	case ButtonTesting:
		return "testing..."
	case ButtonSuccess:
		return "continue"
	case ButtonRetry:
		return "retry"
	}
	return "connect"
}

func (m ConnectionSetupModel) viewComplete() string {
	title := styles.TitleStyle.Render("Connection Configured")

	success := styles.SuccessBoxStyle.Render(
		styles.SuccessText.Render("Connection successful!\n\n") +
			styles.NormalText.Render("Your connection settings have been saved."),
	)

	help := styles.RenderHelp(styles.HK("enter", "continue"))

	return styles.CenterHorizontal(title+"\n\n"+success+"\n\n"+help, m.width)
}
