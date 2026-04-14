package styles

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	PastelOrange = lipgloss.Color("#FFD4A3")
	PastelPink   = lipgloss.Color("#FFBAC3")
	PastelPurple = lipgloss.Color("#D4BBFF")
	PastelBlue   = lipgloss.Color("#A8D8FF")
	PastelGreen  = lipgloss.Color("#A8E6CF")
	PastelYellow = lipgloss.Color("#FFEAA7")

	BitcoinOrange = lipgloss.Color("#FFB374")
	BitcoinGold   = lipgloss.Color("#F5D890")
	Amber         = lipgloss.Color("#FFCC80")
	Copper        = lipgloss.Color("#E6A57A")

	SuccessGreen  = lipgloss.Color("#A8E6CF")
	ErrorRed      = lipgloss.Color("#FFB3B3")
	WarningYellow = lipgloss.Color("#FFE66D")
	InfoBlue      = lipgloss.Color("#A8D8FF")

	PrimaryBg   = lipgloss.Color("#0F111A")
	SecondaryBg = lipgloss.Color("#1A1D2E")
	AccentBg    = lipgloss.Color("#252A3D")
	ElevatedBg  = lipgloss.Color("#2F3549")

	TextPrimary   = lipgloss.Color("#FAFAFA")
	TextSecondary = lipgloss.Color("#A0AEC0")
	TextMuted     = lipgloss.Color("#718096")
	TextSubtle    = lipgloss.Color("#4A5568")

	DarkGray      = lipgloss.Color("#4A5568")
	MediumGray    = lipgloss.Color("#718096")
	LightGray     = lipgloss.Color("#A0AEC0")
	VeryLightGray = lipgloss.Color("#CBD5E0")
	AlmostWhite   = lipgloss.Color("#FAFAFA")

	NeonOrange   = BitcoinOrange
	NeonPink     = PastelPink
	NeonPurple   = PastelPurple
	NeonBlue     = PastelBlue
	NeonGreen    = PastelGreen
	NeonYellow   = PastelYellow
	ElectricBlue = InfoBlue
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(BitcoinOrange).
			Padding(1, 2)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(BitcoinGold).
			Padding(0, 2)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(AlmostWhite).
			Background(PrimaryBg).
			Padding(0, 1).
			MarginBottom(1)

	FooterStyle = lipgloss.NewStyle().
			Foreground(MediumGray)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BitcoinOrange).
			Padding(1, 2).
			MarginBottom(1)

	InfoBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ElectricBlue).
			Padding(1, 2).
			MarginBottom(1)

	SuccessBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SuccessGreen).
			Padding(1, 2).
			MarginBottom(1)

	ErrorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ErrorRed).
			Padding(1, 2).
			MarginBottom(1)

	WarningBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(WarningYellow).
			Padding(1, 2).
			MarginBottom(1)

	BoldText = lipgloss.NewStyle().
			Bold(true).
			Foreground(AlmostWhite)

	NormalText = lipgloss.NewStyle().
			Foreground(LightGray)

	DimText = lipgloss.NewStyle().
		Foreground(MediumGray)

	HighlightText = lipgloss.NewStyle().
			Foreground(BitcoinOrange).
			Bold(true)

	SuccessText = lipgloss.NewStyle().
			Foreground(SuccessGreen).
			Bold(true)

	ErrorText = lipgloss.NewStyle().
			Foreground(ErrorRed).
			Bold(true)

	WarningText = lipgloss.NewStyle().
			Foreground(WarningYellow).
			Bold(true)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(BitcoinOrange).
				Bold(true).
				PaddingLeft(0)

	UnselectedItemStyle = lipgloss.NewStyle().
				Foreground(LightGray).
				PaddingLeft(0)

	ActiveMenuItemStyle = lipgloss.NewStyle().
				Foreground(AlmostWhite).
				Background(AccentBg).
				Padding(0, 2).
				Bold(true)

	InactiveMenuItemStyle = lipgloss.NewStyle().
				Foreground(MediumGray).
				Padding(0, 2)

	PrimaryButtonStyle = lipgloss.NewStyle().
				Foreground(AlmostWhite).
				Background(BitcoinOrange).
				Padding(0, 3).
				Bold(true)

	SecondaryButtonStyle = lipgloss.NewStyle().
				Foreground(AlmostWhite).
				Background(DarkGray).
				Padding(0, 3)

	DisabledButtonStyle = lipgloss.NewStyle().
				Foreground(MediumGray).
				Background(SecondaryBg).
				Padding(0, 3)

	OnlineIndicator = lipgloss.NewStyle().
			Foreground(SuccessGreen).
			SetString("●")

	OfflineIndicator = lipgloss.NewStyle().
				Foreground(ErrorRed).
				SetString("●")

	PendingIndicator = lipgloss.NewStyle().
				Foreground(WarningYellow).
				SetString("●")

	BalanceLabelStyle = lipgloss.NewStyle().
				Foreground(MediumGray)

	BalanceAmountStyle = lipgloss.NewStyle().
				Foreground(BitcoinOrange).
				Bold(true)

	// Input field styles
	InputStyle = lipgloss.NewStyle().
			Foreground(AlmostWhite).
			Background(SecondaryBg).
			Padding(0, 1)

	InputLabelStyle = lipgloss.NewStyle().
			Foreground(LightGray).
			Bold(true)

	// Help text styles
	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ElectricBlue).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(MediumGray)

	// Table styles
	TableHeaderStyle = lipgloss.NewStyle().
				Foreground(BitcoinOrange).
				Bold(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(DarkGray)

	TableCellStyle = lipgloss.NewStyle().
			Foreground(LightGray).
			Padding(0, 1)

	TableSelectedStyle = lipgloss.NewStyle().
				Foreground(AlmostWhite).
				Background(AccentBg).
				Bold(true)

	// Status indicator styles (extracted from dashboard View)
	StatusConnectingStyle = lipgloss.NewStyle().Foreground(PastelYellow)
	StatusConnectedStyle  = lipgloss.NewStyle().Foreground(PastelBlue)
	StatusSyncingStyle    = lipgloss.NewStyle().Foreground(PastelOrange)
	StatusSyncedStyle     = lipgloss.NewStyle().Foreground(PastelGreen)
	StatusErrorStyle      = lipgloss.NewStyle().Foreground(PastelPink)

	// Balance display style (extracted from dashboard/send View)
	BalanceValueStyle = lipgloss.NewStyle().Foreground(PastelOrange).Bold(true)

	// Screen header style (extracted from history View)
	ScreenHeaderStyle = lipgloss.NewStyle().Foreground(PastelOrange).Bold(true)

	// --- Semantic UI Styles (extracted from inline usage across models) ---

	// Copy/clipboard confirmation text
	CopyStatusStyle = lipgloss.NewStyle().
			Foreground(PastelGreen).
			Bold(true)

	// Copy/clipboard confirmation badge (with background, for history/receive screens)
	CopyStatusBadgeStyle = lipgloss.NewStyle().
				Foreground(PastelGreen).
				Bold(true)

	// Address status badges
	AddressSpentBadge = lipgloss.NewStyle().
				Foreground(PastelPink).
				Bold(true)

	AddressUsedBadge = lipgloss.NewStyle().
				Foreground(PastelYellow)

	// Derivation path display
	DerivationIndexStyle = lipgloss.NewStyle().
				Foreground(PastelYellow).
				Bold(true)

	DerivationPathStyle = lipgloss.NewStyle().
				Foreground(PastelBlue)

	// Transaction direction indicators
	TxDirectionInStyle = lipgloss.NewStyle().
				Foreground(PastelGreen)

	TxDirectionOutStyle = lipgloss.NewStyle().
				Foreground(PastelPink)

	// PSBT signature status
	SignatureCompleteStyle = lipgloss.NewStyle().
				Foreground(PastelGreen).
				Bold(true)

	SignaturePartialStyle = lipgloss.NewStyle().
				Foreground(BitcoinOrange).
				Bold(true)

	// Cosigner status icons (non-bold for visual hierarchy)
	CosignerSignedStyle = lipgloss.NewStyle().
				Foreground(PastelGreen)

	CosignerPendingStyle = lipgloss.NewStyle().
				Foreground(BitcoinOrange)

	// Section subheader (for help screen subsections, etc.)
	SectionSubheaderStyle = lipgloss.NewStyle().
				Foreground(ElectricBlue).
				Bold(true)

	// Shortcut key display in help screen tables (Width(10) for column alignment)
	ShortcutKeyStyle = lipgloss.NewStyle().
				Foreground(PastelOrange).
				Width(10)

	// Section title (gold bold, used in help feature boxes)
	SectionTitleStyle = lipgloss.NewStyle().
				Foreground(BitcoinGold).
				Bold(true)

	// Italic tagline/pitch text
	TaglineStyle = lipgloss.NewStyle().
			Foreground(ElectricBlue).
			Italic(true)

	// Scroll percentage indicator
	ScrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(PastelPurple)

	// Amount display (blue)
	AmountStyle = lipgloss.NewStyle().
			Foreground(PastelBlue)

	// Currency unit label
	CurrencyUnitStyle = lipgloss.NewStyle().
				Foreground(PastelOrange)

	// Dashboard-specific box variants
	FeeBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(NeonPurple).
			Padding(0, 2)

	MenuBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(PastelGreen).
			Padding(0, 2)

	// Progress step indicator
	ProgressStepStyle = lipgloss.NewStyle().
				Foreground(NeonOrange).
				Bold(true)

	// Edit mode box
	EditBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BitcoinOrange).
			Padding(0, 1)

	// Testing/connecting status indicator
	TestingStatusStyle = lipgloss.NewStyle().
				Foreground(PastelOrange)

	// Table wrapper border (for wrapping table.Model in a rounded box)
	TableBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ElectricBlue).
				Padding(0, 1)

	// Address/UTXO status badges (for table cells)
	StatusUnusedStyle  = lipgloss.NewStyle().Foreground(PastelGreen)
	StatusUsedStyle    = lipgloss.NewStyle().Foreground(PastelYellow)
	StatusPendingStyle = lipgloss.NewStyle().Foreground(PastelPink).Italic(true)
)

// HelpItem represents a key-description pair in a help footer.
type HelpItem struct {
	Key  string
	Desc string
}

// HK creates a HelpItem (shorthand constructor for concise footer definitions).
func HK(key, desc string) HelpItem {
	return HelpItem{Key: key, Desc: desc}
}

// RenderHelp builds a styled help footer with bullet separators.
func RenderHelp(items ...HelpItem) string {
	return RenderHelpStyled(HelpKeyStyle, items...)
}

// RenderHelpStyled builds a styled help footer with a custom key style.
func RenderHelpStyled(keyStyle lipgloss.Style, items ...HelpItem) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = keyStyle.Render(item.Key) + " " + HelpDescStyle.Render(item.Desc)
	}
	return FooterStyle.Render(strings.Join(parts, " • "))
}

// RenderHelpLines builds a multi-line styled help footer.
func RenderHelpLines(lines ...[]HelpItem) string {
	rendered := make([]string, len(lines))
	for i, line := range lines {
		parts := make([]string, len(line))
		for j, item := range line {
			parts[j] = HelpKeyStyle.Render(item.Key) + " " + HelpDescStyle.Render(item.Desc)
		}
		rendered[i] = strings.Join(parts, " • ")
	}
	return FooterStyle.Render(strings.Join(rendered, "\n"))
}

// Helper functions
func WithWidth(style lipgloss.Style, width int) lipgloss.Style {
	return style.Width(width)
}

func WithHeight(style lipgloss.Style, height int) lipgloss.Style {
	return style.Height(height)
}

func CenterHorizontal(text string, width int) string {
	return lipgloss.Place(width, 1, lipgloss.Center, lipgloss.Center, text)
}

func CenterVertical(text string, height int) string {
	return lipgloss.Place(1, height, lipgloss.Center, lipgloss.Center, text)
}

func Center(text string, width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text)
}

// ClampWidth returns the smaller of desired and available width, with a minimum of 20.
// Use this for responsive box/table widths instead of hardcoded values.
func ClampWidth(desired, available int) int {
	// Account for border/padding overhead
	maxWidth := available - 4
	if maxWidth < 20 {
		maxWidth = 20
	}
	if desired > maxWidth {
		return maxWidth
	}
	return desired
}

// RenderErrorBox wraps long error text to fit the available terminal width.
// It keeps error boxes responsive and prevents right-edge truncation.
func RenderErrorBox(message string, availableWidth int) string {
	if availableWidth <= 0 {
		return ErrorBoxStyle.Render(message)
	}

	boxWidth := ClampWidth(88, availableWidth)
	wrapWidth := boxWidth - 6 // rounded border + horizontal padding
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	wrapped := lipgloss.NewStyle().Width(wrapWidth).Render(message)
	return ErrorBoxStyle.Width(boxWidth).Render(wrapped)
}

// Badge creates a styled inline badge with the given foreground color on AccentBg.
func Badge(fg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(AccentBg).
		Bold(true).
		Padding(0, 1)
}

// NewTableStyles returns consistent table styles for data screens.
// This centralizes the table appearance that was previously duplicated
// across addresses.go and utxos.go.
func NewTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(BitcoinOrange).
		BorderBottom(true).
		Bold(true).
		Foreground(BitcoinGold)
	s.Selected = s.Selected.
		Foreground(BitcoinOrange).
		Bold(true).
		Background(lipgloss.Color(""))
	return s
}

// Spinner factories - standardized spinner configuration for consistent UI

// NewStandardSpinner creates a consistent spinner for standard operations
// Uses Dot style with consistent tick rate across all screens
func NewStandardSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: spinner.Dot.Frames,
		FPS:    time.Second / 30, // 30 FPS
	}
	s.Style = lipgloss.NewStyle().Foreground(BitcoinOrange)
	return s
}

// NewHeavySpinner creates a more visible spinner for heavy operations (unlock, sync)
// Uses Points style with same timing for consistency
func NewHeavySpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: spinner.Points.Frames,
		FPS:    time.Second / 30, // 30 FPS - matches standard spinner
	}
	s.Style = lipgloss.NewStyle().Foreground(BitcoinOrange)
	return s
}
