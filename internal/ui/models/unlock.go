package models

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	"github.com/charmbracelet/harmonica"
)

type UnlockModel struct {
	width  int
	height int

	walletManager WalletService

	// UI components
	passwordInput textinput.Model
	spinner       spinner.Model

	// State
	walletName     string
	walletList     []string
	selectedWallet int
	cursor         int
	unlocking      bool
	syncing        bool
	err            error

	// Set by auto-lock: the wallet to preselect and why we are here.
	preferredWallet string
	notice          string

	// In-process throttle after consecutive failed passwords. It delays
	// guesses without locking the user out of their funds.
	failedAttempts int
	throttleUntil  time.Time

	// Animation
	lockSpinAngle    float64
	lockSpinVelocity float64
	lockSpring       harmonica.Spring
	shakeIntensity   float64
	shakeVelocity    float64
	shakeSpring      harmonica.Spring
	pulseFrame       int
	glowIntensity    float64
}

// UnlockResultMsg is sent when unlock completes
type UnlockResultMsg struct {
	Success bool
	Err     error
}

// WalletListMsg carries the list of available wallets
type WalletListMsg struct {
	Wallets []string
	Err     error
}

// SyncResultMsg is sent when address sync completes
type SyncResultMsg struct {
	Success bool
	Err     error
}

func NewUnlockModel(walletManager WalletService) UnlockModel {
	// Create password input
	pwInput := newTextInput()
	pwInput.Placeholder = "Enter your wallet password"
	pwInput.EchoMode = textinput.EchoPassword
	pwInput.EchoCharacter = '•'
	pwInput.SetWidth(40)
	pwInput.CharLimit = 100
	pwInput.Blur()

	// Create heavy spinner for unlock operation (more visible)
	s := styles.NewHeavySpinner()

	return UnlockModel{
		walletManager:  walletManager,
		passwordInput:  pwInput,
		spinner:        s,
		lockSpring:     harmonica.NewSpring(harmonica.FPS(60), 6.0, 0.8),
		shakeSpring:    harmonica.NewSpring(harmonica.FPS(60), 10.0, 0.3),
		lockSpinAngle:  0.0,
		shakeIntensity: 0.0,
	}
}

func (m UnlockModel) Init() tea.Cmd {
	// Don't start spinner tick - spinner only shows during unlock/sync
	// animateTick is for lock icon animation, started separately when needed
	return tea.Batch(
		textinput.Blink,
		m.fetchWalletList(),
	)
}

func (m UnlockModel) fetchWalletList() tea.Cmd {
	return func() tea.Msg {
		wallets, err := m.walletManager.ListWallets()
		return WalletListMsg{
			Wallets: wallets,
			Err:     err,
		}
	}
}

// unlockBackoff returns how long to wait after failures consecutive failed
// unlock attempts. It starts after the second failure and grows to 8s.
func unlockBackoff(failures int) time.Duration {
	if failures < 2 {
		return 0
	}
	shift := failures - 2
	if shift > 3 {
		shift = 3
	}
	return time.Second << uint(shift)
}

func (m UnlockModel) unlockWallet() tea.Cmd {
	return func() tea.Msg {
		err := m.walletManager.UnlockWallet(m.walletName, m.passwordInput.Value())
		return UnlockResultMsg{
			Success: err == nil,
			Err:     err,
		}
	}
}

func (m UnlockModel) syncAddresses() tea.Cmd {
	return func() tea.Msg {
		err := m.walletManager.SyncAddressUsage(0) // Sync account 0
		return SyncResultMsg{
			Success: err == nil,
			Err:     err,
		}
	}
}

func (m UnlockModel) animateTick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// CapturesTextInput reports whether the password field owns the keyboard.
func (m UnlockModel) CapturesTextInput() bool {
	return m.passwordInput.Focused()
}

func (m UnlockModel) Update(msg tea.Msg) (screenModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		if (m.unlocking || m.syncing) && cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case ThemeChangedMsg:
		m.spinner = styles.NewStandardSpinner()
		m.passwordInput.SetStyles(textInputStyles())

	case WalletListMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.walletList = msg.Wallets
			m.passwordInput.SetValue("")
			if len(msg.Wallets) > 0 {
				m.walletName = msg.Wallets[0]
				for i, w := range msg.Wallets {
					if m.preferredWallet != "" && w == m.preferredWallet {
						m.walletName, m.selectedWallet, m.cursor = w, i, i
						m.passwordInput.Focus()
						cmds = append(cmds, textinput.Blink)
					}
				}
				if !m.passwordInput.Focused() {
					if len(msg.Wallets) == 1 {
						m.passwordInput.Focus()
						cmds = append(cmds, textinput.Blink)
					} else {
						m.passwordInput.Blur()
					}
				}
			}
		}

	case UnlockResultMsg:
		m.unlocking = false
		if msg.Success {
			m.failedAttempts = 0
			m.throttleUntil = time.Time{}
			m.passwordInput.SetValue("")
			m.syncing = true
			return m, tea.Batch(m.syncAddresses(), m.spinner.Tick)
		} else {
			m.err = msg.Err
			m.failedAttempts++
			if delay := unlockBackoff(m.failedAttempts); delay > 0 {
				m.throttleUntil = time.Now().Add(delay)
			}
			m.shakeIntensity = 1.0
			m.passwordInput.SetValue("")
			cmds = append(cmds, m.animateTick())
		}

	case SyncResultMsg:
		m.syncing = false
		if msg.Success {
			return m, Navigate(StateMainDashboard)
		} else {
			return m, Navigate(StateMainDashboard)
		}

	case TickMsg:
		// Update lock spin animation
		targetAngle := 0.0
		if m.unlocking {
			targetAngle = 360.0 * float64(m.pulseFrame%60) / 60.0
		}
		m.lockSpinAngle, m.lockSpinVelocity = m.lockSpring.Update(
			m.lockSpinAngle, m.lockSpinVelocity, targetAngle,
		)

		// Update shake animation (on error)
		m.shakeIntensity, m.shakeVelocity = m.shakeSpring.Update(
			m.shakeIntensity, m.shakeVelocity, 0.0,
		)

		// Update glow
		m.glowIntensity = 0.5 + 0.5*float64(m.pulseFrame%120)/120.0

		m.pulseFrame++

		// Continue animation
		if m.unlocking || m.shakeIntensity > 0.01 {
			cmds = append(cmds, m.animateTick())
		}

	case tea.KeyPressMsg:
		// If selecting wallet from list
		if len(m.walletList) > 1 && !m.passwordInput.Focused() {
			return m.handleWalletSelection(msg)
		}

		// Password input handling
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+s":
			if m.networkMismatch() {
				msg := ChooseServerMsg{}
				var nm *wallet.NetworkMismatchError
				if errors.As(m.err, &nm) {
					msg.Network = nm.WalletNetwork
				}
				return m, func() tea.Msg { return msg }
			}
		case "esc":
			if len(m.walletList) > 1 && m.passwordInput.Focused() {
				// Go back to wallet selection
				m.passwordInput.Blur()
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			if !m.passwordInput.Focused() {
				// Enter password for selected wallet
				m.walletName = m.walletList[m.selectedWallet]
				return m, m.passwordInput.Focus()
			}

			// Attempt unlock
			if m.passwordInput.Value() == "" {
				m.err = fmt.Errorf("please enter a password")
				return m, nil
			}
			// A short growing delay after repeated failures slows
			// guessing without ever locking the user out.
			if time.Now().Before(m.throttleUntil) {
				m.err = fmt.Errorf("too many failed attempts, wait a moment")
				return m, nil
			}

			m.unlocking = true
			m.err = nil
			return m, tea.Batch(m.unlockWallet(), m.spinner.Tick)
		}
	}

	// Update password input
	if m.passwordInput.Focused() {
		m.passwordInput, cmd = m.passwordInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *UnlockModel) handleWalletSelection(msg tea.KeyPressMsg) (screenModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.selectedWallet = m.cursor
		}
	case "down", "j":
		if m.cursor < len(m.walletList)-1 {
			m.cursor++
			m.selectedWallet = m.cursor
		}
	case "enter":
		m.walletName = m.walletList[m.selectedWallet]
		return m, m.passwordInput.Focus()
	case "esc", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m UnlockModel) View() string {
	width := m.width
	if width == 0 {
		width = 80
	}

	lockIcon := m.renderLockIcon()

	glowColor := m.getGlowColor()
	titleStyle := lipgloss.NewStyle().
		Foreground(glowColor).
		Bold(true)

	header := lockIcon + "\n\n" +
		titleStyle.Render("🔐 Unlock Wallet") + "\n" +
		styles.SubtitleStyle.Render("Enter your password to access your Bitcoin wallet")
	if m.notice != "" {
		header += "\n\n" + styles.WarningText.Render(m.notice)
	}

	var content string

	if m.unlocking {
		content = m.renderUnlocking()
	} else if m.syncing {
		content = m.renderSyncing()
	} else if len(m.walletList) > 1 && !m.passwordInput.Focused() {
		content = m.renderWalletSelection()
	} else {
		content = m.renderPasswordEntry()
	}

	// Help text
	help := m.renderHelp()

	// The error row stays reserved so the help line does not move, and it is
	// capped to the stable content width so it cannot re-center the block.
	contentWidth := max(lipgloss.Width(header), lipgloss.Width(content), lipgloss.Width(help))
	errorLine := ""
	if m.err != nil {
		shakeOffset := int(m.shakeIntensity * 3.0)
		errorLine = strings.Repeat(" ", shakeOffset) +
			styles.ErrorText.Render("❌ "+truncateRunes(m.err.Error(), max(10, width-8), "…"))
		errorLine = lipgloss.NewStyle().MaxWidth(contentWidth).Render(errorLine)
	}

	return styles.CenterHorizontal(
		header+"\n\n"+content+"\n\n"+errorLine+"\n"+help,
		width,
	)
}

func (m UnlockModel) renderLockIcon() string {
	// Animated lock that spins when unlocking
	angle := m.lockSpinAngle

	var lockArt string
	if angle > 45 && angle < 135 {
		// Tilted right
		lockArt = "  ▄▄▄\n ║   ║\n ╚═══╝"
	} else if angle > 135 && angle < 225 {
		// Upside down
		lockArt = " ╔═══╗\n ║   ║\n  ▀▀▀"
	} else if angle > 225 && angle < 315 {
		// Tilted left
		lockArt = " ▄▄▄  \n║   ║ \n╚═══╝ "
	} else {
		// Normal
		lockArt = " ╔═══╗\n ║ ◉ ║\n ╚═══╝"
	}

	lockColor := styles.BitcoinOrange
	if m.unlocking {
		lockColor = m.getGlowColor()
	}

	lockStyle := lipgloss.NewStyle().
		Foreground(lockColor).
		Bold(true)

	return lockStyle.Render(lockArt)
}

func (m UnlockModel) renderWalletSelection() string {
	title := styles.BoldText.Render("Select a wallet:")

	menu := "\n"
	for i, walletName := range m.walletList {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}

		walletDisplay := fmt.Sprintf("%s%s", cursor, walletName)

		if i == m.cursor {
			menu += styles.SelectedItemStyle.Render(walletDisplay) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render(walletDisplay) + "\n"
		}
	}

	boxWidth := m.width - 4
	if boxWidth > 50 {
		boxWidth = 50
	}
	if boxWidth < 30 {
		boxWidth = 30
	}
	return styles.BoxStyle.Width(boxWidth + 2).Render(title + menu)
}

func (m UnlockModel) renderPasswordEntry() string {
	boxWidth := m.width - 4
	if boxWidth > 50 {
		boxWidth = 50
	}
	if boxWidth < 30 {
		boxWidth = 30
	}
	walletInfo := styles.BoxStyle.Width(boxWidth + 2).Render(
		styles.DimText.Render("Wallet:") + "\n" +
			styles.HighlightText.Render(m.walletName),
	)

	instruction := styles.NormalText.Render("Password:")

	return walletInfo + "\n\n" +
		lipgloss.NewStyle().Width(43).Render(instruction+"\n"+m.passwordInput.View())
}

func (m UnlockModel) renderUnlocking() string {
	return styles.InfoBoxStyle.Render(
		m.spinner.View() + " " +
			styles.NormalText.Render("Unlocking wallet..."),
	)
}

func (m UnlockModel) renderSyncing() string {
	return styles.InfoBoxStyle.Render(
		m.spinner.View() + " " +
			styles.NormalText.Render("Syncing Wallet..."),
	)
}

func (m UnlockModel) renderHelp() string {
	if len(m.walletList) > 1 && !m.passwordInput.Focused() {
		return styles.RenderHelp(
			styles.HK("↑/↓", "select"),
			styles.HK("enter", "choose"),
			styles.HK("esc", "quit"),
		)
	}

	escDesc := "quit"
	if len(m.walletList) > 1 {
		escDesc = "back"
	}
	normalHelp := styles.RenderHelp(
		styles.HK("enter", "unlock"),
		styles.HK("esc", escDesc),
	)
	mismatchHelp := styles.RenderHelp(
		styles.HK("ctrl+s", "choose server"),
		styles.HK("enter", "unlock"),
		styles.HK("esc", escDesc),
	)
	helpWidth := max(lipgloss.Width(normalHelp), lipgloss.Width(mismatchHelp))
	if m.networkMismatch() {
		return lipgloss.NewStyle().Width(helpWidth).Render(mismatchHelp)
	}
	return lipgloss.NewStyle().Width(helpWidth).Render(normalHelp)
}

// networkMismatch reports whether the last unlock failed because the
// selected server serves a different network than the wallet.
func (m UnlockModel) networkMismatch() bool {
	return errors.Is(m.err, wallet.ErrNetworkMismatch)
}

func (m UnlockModel) getGlowColor() color.Color {
	return styles.BitcoinOrange // Static orange, no animation
}
