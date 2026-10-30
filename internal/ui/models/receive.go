package models

import (
	"fmt"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

type ReceiveModel struct {
	width  int
	height int

	walletManager   WalletService
	selectedAddress string
	derivationPath  string
	currentIndex    uint32      // Current address index being viewed
	addressBalance  api.Satoshi // Balance of current address
	generatingNew   bool
	showCopied      bool
	copiedTime      time.Time
	err             error

	// Hardware wallet address verification
	verify AddressVerifyView

	// Animation state
	viewport       viewport.Model
	revealProgress float64
	revealVelocity float64
	revealSpring   harmonica.Spring
	glowIntensity  float64
	glowVelocity   float64
	glowSpring     harmonica.Spring
	pulseFrame     int

	// Screen visibility - controls whether tickers run
	isActive bool

	// stealth hides the address balance.
	stealth bool
}

// AddressGeneratedMsg is sent when a new address is generated
type AddressGeneratedMsg struct {
	Address string
	Path    string
	Index   uint32
	Balance api.Satoshi
	Err     error
}

// HWIVerifyMsg carries the result of hardware wallet address verification.
type HWIVerifyMsg struct {
	Err     error
	Attempt uint64
}

// CopiedTimerMsg is sent on each timer tick
type CopiedTimerMsg time.Time

func NewReceiveModel(walletManager WalletService) ReceiveModel {
	vp := viewport.New(80, 20)

	return ReceiveModel{
		walletManager:  walletManager,
		viewport:       vp,
		revealSpring:   harmonica.NewSpring(harmonica.FPS(60), 5.0, 0.7),
		glowSpring:     harmonica.NewSpring(harmonica.FPS(60), 3.0, 0.5),
		revealProgress: 0.0,
		glowIntensity:  0.0,
	}
}

func (m ReceiveModel) Init() tea.Cmd {
	m.isActive = true
	return tea.Batch(
		m.loadAddresses(),
		m.animateTick(),
	)
}

func (m ReceiveModel) animateTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m ReceiveModel) loadAddresses() tea.Cmd {
	return func() tea.Msg {
		m.walletManager.VerifyReceiveAddressCache(0)

		addr, path, index, err := m.walletManager.GetCurrentReceiveAddress(0)
		if err != nil {
			return AddressGeneratedMsg{
				Address: "",
				Path:    "",
				Index:   0,
				Balance: 0,
				Err:     fmt.Errorf("failed to get receive address: %w", err),
			}
		}

		return AddressGeneratedMsg{
			Address: addr,
			Path:    path,
			Index:   index,
			Balance: 0, // Unused addresses have 0 balance by definition
			Err:     nil,
		}
	}
}

func (m ReceiveModel) loadAddressByIndex(index uint32) tea.Cmd {
	return func() tea.Msg {
		addr, path, err := m.walletManager.GetAddressByIndex(0, index)
		if err != nil {
			return AddressGeneratedMsg{
				Address: "",
				Path:    "",
				Index:   index,
				Balance: 0,
				Err:     fmt.Errorf("failed to get address: %w", err),
			}
		}

		balance, _ := m.walletManager.GetCachedAddressBalance(addr)

		return AddressGeneratedMsg{
			Address: addr,
			Path:    path,
			Index:   index,
			Balance: balance,
			Err:     nil,
		}
	}
}

func (m ReceiveModel) verifyAddressOnDevice() tea.Cmd {
	walletMgr := m.walletManager
	addr := m.selectedAddress
	path := m.derivationPath
	attempt := m.verify.Attempt()

	return func() tea.Msg {
		err := walletMgr.DisplayAddressOnDevice(addr, path)
		return HWIVerifyMsg{Err: err, Attempt: attempt}
	}
}

// canVerifyOnDevice reports whether the selected address can be shown on a
// hardware wallet.
func (m ReceiveModel) canVerifyOnDevice() bool {
	return m.walletManager.IsHardwareWallet() && !m.walletManager.IsMultisig() &&
		m.selectedAddress != "" && m.derivationPath != ""
}

func (m ReceiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case StealthToggledMsg:
		m.stealth = msg.On
		return m, nil

	case ScreenActiveMsg:
		m.isActive = true
		if m.revealProgress < 0.99 {
			return m, m.animateTick()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 10

	case AddressGeneratedMsg:
		m.generatingNew = false
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			if msg.Address != m.selectedAddress || msg.Path != m.derivationPath {
				m.verify.Reset(msg.Address, msg.Path)
			}
			m.selectedAddress = msg.Address
			m.derivationPath = msg.Path
			m.currentIndex = msg.Index
			m.addressBalance = msg.Balance
			m.revealProgress = 0.0
			m.glowIntensity = 0.0
			cmds = append(cmds, m.animateTick())
		}

	case TickMsg:
		if !m.isActive {
			return m, nil
		}

		needsAnimation := false

		if m.revealProgress < 0.99 {
			m.revealProgress, m.revealVelocity = m.revealSpring.Update(
				m.revealProgress, m.revealVelocity, 1.0,
			)
			needsAnimation = true
		} else {
			m.revealProgress = 1.0
			m.revealVelocity = 0
		}

		// Update glow animation only if reveal is still in progress
		if needsAnimation {
			targetGlow := 0.5 + 0.5*float64(m.pulseFrame%120)/120.0
			m.glowIntensity, m.glowVelocity = m.glowSpring.Update(
				m.glowIntensity, m.glowVelocity, targetGlow,
			)
			m.pulseFrame++
		}

		// Check if copy message should be hidden (after 2 seconds)
		if m.showCopied {
			if time.Since(m.copiedTime) > 2*time.Second {
				m.showCopied = false
			} else {
				needsAnimation = true // Keep animating to hide the message
			}
		}

		// Only continue animation if something is actively animating
		// Don't run animations indefinitely just because an address exists
		if needsAnimation {
			cmds = append(cmds, m.animateTick())
		}

	case AddressVerifyTickMsg:
		if m.verify.IsWaiting() {
			m.verify.Advance()
			cmds = append(cmds, AddressVerifyTickCmd())
		}

	case HWIVerifyMsg:
		if m.verify.IsWaiting() && (msg.Attempt == 0 || msg.Attempt == m.verify.Attempt()) {
			m.verify.Complete(msg.Err)
		}

	case StateChangedMsg:
		// Update from unified state - refresh address balance if needed
		if m.selectedAddress != "" {
			// If the current address is now used/spent, reload to get the next unused one
			status := m.walletManager.GetAddressStatus(m.selectedAddress)
			if status == "SPENT" || status == "USED" {
				return m, m.loadAddresses()
			}
			cachedAddrs := m.walletManager.GetCachedReceiveAddresses()
			for _, cached := range cachedAddrs {
				if cached.Address == m.selectedAddress {
					m.addressBalance = cached.Balance
					break
				}
			}
		}

	case tea.KeyMsg:
		if m.verify.Active() {
			switch msg.String() {
			case "esc":
				m.verify.Cancel()
				return m, nil
			case "v":
				if m.canVerifyOnDevice() {
					m.verify.Begin()
					cmds = append(cmds, AddressVerifyTickCmd(), m.verifyAddressOnDevice())
				}
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, Navigate(StateMainDashboard)

		case "left", "h":
			// Navigate to previous address
			if m.currentIndex > 0 {
				m.generatingNew = true
				m.revealProgress = 0.0
				cmds = append(cmds, m.loadAddressByIndex(m.currentIndex-1))
			}

		case "right", "l":
			// Navigate to next address
			m.generatingNew = true
			m.revealProgress = 0.0
			cmds = append(cmds, m.loadAddressByIndex(m.currentIndex+1))

		case "n":
			// Generate NEW address (increments the index)
			m.generatingNew = true
			m.revealProgress = 0.0
			cmds = append(cmds, func() tea.Msg {
				addr, err := m.walletManager.GetNewAddress(0)
				if err != nil {
					return AddressGeneratedMsg{Address: "", Path: "", Index: 0, Balance: 0, Err: err}
				}
				_, path, index, pathErr := m.walletManager.GetCurrentReceiveAddress(0)
				if pathErr != nil {
					return AddressGeneratedMsg{Address: addr, Path: "", Index: m.currentIndex + 1, Balance: 0, Err: nil}
				}
				return AddressGeneratedMsg{Address: addr, Path: path, Index: index, Balance: 0, Err: nil}
			})
		case "c":
			if m.selectedAddress != "" {
				err := copyToClipboard(m.selectedAddress)
				if err == nil {
					m.showCopied = true
					m.copiedTime = time.Now()
					cmds = append(cmds, m.animateTick())
				} else {
					m.err = fmt.Errorf("failed to copy to clipboard: %v", err)
				}
			}
		case "v":
			if m.canVerifyOnDevice() {
				if m.verify.Address != m.selectedAddress || m.verify.Path != m.derivationPath {
					m.verify.Reset(m.selectedAddress, m.derivationPath)
				}
				m.verify.Begin()
				cmds = append(cmds, AddressVerifyTickCmd(), m.verifyAddressOnDevice())
			}
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m ReceiveModel) View() string {
	width := m.width
	if width == 0 {
		width = 80
	}

	// Compact header
	header := styles.ScreenHeaderStyle.Render("Receive Bitcoin") + stealthBadge(m.stealth)

	// Copy status - inline with header (pastel green, matching theme)
	copyStatus := ""
	if m.showCopied && time.Since(m.copiedTime) <= 2*time.Second {
		copyStatus = " " + styles.CopyStatusBadgeStyle.Render("✓ COPIED!")
	}

	// Skip loading state - happens too fast to see
	// Just show the previous screen content or wait for the address

	errorLine := ""
	if m.err != nil {
		errorLine = styles.ErrorText.Render("Error: " +
			truncateRunes(m.err.Error(), max(10, width-7), "…"))
	}

	// Single box with QR code and address together.
	content := m.renderCombinedQRAndAddress()

	// In verification mode the panel replaces the QR card.
	if m.verify.Active() {
		verify := m.verify
		verify.Width = width
		content = verify.View()
	}

	// Help
	help := m.renderAnimatedHelp()

	// Final layout - ultra compact for small terminals
	return header + copyStatus + "\n" +
		content + "\n" +
		help + "\n" +
		errorLine
}

func (m ReceiveModel) renderCombinedQRAndAddress() string {
	if m.selectedAddress == "" {
		return ""
	}

	// Generate QR code
	qrCode, err := ui.GenerateQRCodeHalfBlocks(m.selectedAddress)
	if err != nil {
		return ""
	}

	// Derivation path with index and status badge
	pathLine := ""
	if m.derivationPath != "" {
		indexLabel := lipgloss.NewStyle().Width(12).
			Render(styles.DerivationIndexStyle.Render(fmt.Sprintf("Address #%d", m.currentIndex)))
		pathLine = indexLabel + " " +
			styles.DerivationPathStyle.Render(m.derivationPath)
	}

	// Check address status and add badge
	// SPENT = address was used as transaction input (authoritative from GetAddressStatus)
	// USED = address received funds but we don't have tx data to confirm if spent from
	addressStatus := m.walletManager.GetAddressStatus(m.selectedAddress)
	statusBadge := ""
	switch addressStatus {
	case "SPENT":
		// GetAddressStatus checked transaction inputs - address was spent from
		statusBadge = styles.AddressSpentBadge.Render(" ⚠ SPENT")
	case "USED":
		// Address has received funds
		statusBadge = styles.AddressUsedBadge.Render(" • USED")
	}
	// Reserve the badge column so it cannot re-center the path line.
	statusBadge = lipgloss.NewStyle().Width(8).Render(statusBadge)

	// Format address with pastel colors
	coloredAddr := ui.FormatBitcoinAddressColored(m.selectedAddress, 8, 8)

	// Balance line
	balanceLine := ""
	if m.addressBalance > 0 {
		btcBalance := m.addressBalance.ToBTC()
		balanceLine = styles.SuccessText.
			Render("Balance: " + maskAmount(fmt.Sprintf("%.8f BTC", btcBalance), m.stealth))
	} else {
		balanceLine = styles.DimText.Render("Balance: " + maskAmount("0 BTC", m.stealth))
	}

	// Combine all content vertically
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		qrCode,
		"",
		pathLine+statusBadge,
		coloredAddr,
		balanceLine,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.PastelOrange).
		Padding(1, 2).
		Render(content)
}

func (m ReceiveModel) renderAnimatedHelp() string {
	items := []styles.HelpItem{
		styles.HK("←/→", "navigate"),
		styles.HK("n", "new"),
		styles.HK("c", "copy"),
	}

	if m.walletManager.IsHardwareWallet() && !m.walletManager.IsMultisig() {
		items = append(items, styles.HK("v", "verify on device"))
	}

	items = append(items, styles.HK("esc", "back"))

	return styles.RenderHelp(items...)
}
