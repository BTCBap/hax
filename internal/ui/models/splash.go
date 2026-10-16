package models

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/version"
)

// SplashModel represents the splash screen
type SplashModel struct {
	width  int
	height int

	// Animation state
	progress float64
	velocity float64
	spring   harmonica.Spring
	target   float64
	frame    int

	ready bool
}

// TickMsg is sent on each animation frame
type TickMsg time.Time

// NewSplashModel creates a new splash screen model
func NewSplashModel() SplashModel {
	return SplashModel{
		progress: 0.0,
		velocity: 0.0,
		target:   1.0,
		spring:   harmonica.NewSpring(harmonica.FPS(60), 6.0, 0.8), // Quick, slightly bouncy
		ready:    false,
	}
}

// CheckWalletsMsg is sent when wallet check completes
type CheckWalletsMsg struct {
	HasWallets bool
}

// Init initializes the splash screen
func (m SplashModel) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		// Navigate immediately after a short delay
		tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
			return NavigateMsg{Target: StateWalletSetup}
		}),
	)
}

// tick creates a command that sends a TickMsg every frame
func tick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Update handles messages
func (m SplashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TickMsg:
		// Update spring animation
		m.progress, m.velocity = m.spring.Update(m.progress, m.velocity, m.target)
		m.frame++

		// Continue animation
		if m.progress < 0.99 {
			return m, tick()
		}
		return m, nil

	case tea.KeyMsg:
		// Allow skipping splash with any key
		if msg.String() != "ctrl+c" {
			return m, func() tea.Msg {
				return NavigateMsg{Target: StateWalletSetup}
			}
		}
	}

	return m, nil
}

// View renders the splash screen
func (m SplashModel) View() string {
	width := m.width
	if width == 0 {
		width = 80
	}

	height := m.height
	if height == 0 {
		height = 24
	}

	// HAX logo (ASCII art)
	logo := []string{
		"██╗  ██╗ █████╗ ██╗  ██╗",
		"██║  ██║██╔══██╗╚██╗██╔╝",
		"███████║███████║ ╚███╔╝ ",
		"██╔══██║██╔══██║ ██╔██╗ ",
		"██║  ██║██║  ██║██╔╝ ██╗",
		"╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝",
	}

	// Title with animation
	opacity := int(m.progress * 100)
	titleColor := styles.BitcoinOrange
	if opacity < 50 {
		titleColor = styles.MediumGray
	}

	title := styles.TitleStyle.
		Foreground(titleColor).
		Render("BITCOIN WALLET")

	subtitle := styles.SubtitleStyle.
		Render("Testnet Wallet")

	versionStr := styles.DimText.Render(version.String())

	// Loading indicator
	loadingDots := m.getLoadingDots()
	loading := styles.NormalText.Render(fmt.Sprintf("Initializing%s", loadingDots))

	// Combine elements
	var content strings.Builder

	// Add some top padding
	topPadding := (height / 2) - 10
	if topPadding < 0 {
		topPadding = 0
	}
	for i := 0; i < topPadding; i++ {
		content.WriteString("\n")
	}

	// Logo
	for _, line := range logo {
		styledLine := styles.HighlightText.Render(line)
		content.WriteString(styles.CenterHorizontal(styledLine, width))
		content.WriteString("\n")
	}
	content.WriteString("\n")

	// Title
	content.WriteString(styles.CenterHorizontal(title, width))
	content.WriteString("\n")

	// Subtitle
	content.WriteString(styles.CenterHorizontal(subtitle, width))
	content.WriteString("\n\n")

	// Version
	content.WriteString(styles.CenterHorizontal(versionStr, width))
	content.WriteString("\n\n")

	// Loading
	content.WriteString(styles.CenterHorizontal(loading, width))
	content.WriteString("\n\n")

	// Help text
	help := styles.HelpDescStyle.Render("Press any key to continue")
	content.WriteString(styles.CenterHorizontal(help, width))

	return content.String()
}

// getLoadingDots returns animated loading dots
func (m SplashModel) getLoadingDots() string {
	dots := (m.frame / 20) % 4
	return strings.Repeat(".", dots)
}
