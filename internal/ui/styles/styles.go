package styles

import (
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// Palette colors. SetDarkBackground assigns these and rebuilds every derived
// package-level style so screens pick up a light or dark terminal.
var (
	PastelOrange color.Color
	PastelPink   color.Color
	PastelPurple color.Color
	PastelBlue   color.Color
	PastelGreen  color.Color
	PastelYellow color.Color

	BitcoinOrange color.Color
	BitcoinGold   color.Color
	Amber         color.Color
	Copper        color.Color

	SuccessGreen  color.Color
	ErrorRed      color.Color
	WarningYellow color.Color
	InfoBlue      color.Color

	PrimaryBg   color.Color
	SecondaryBg color.Color
	AccentBg    color.Color
	ElevatedBg  color.Color

	TextPrimary   color.Color
	TextSecondary color.Color
	TextMuted     color.Color
	TextSubtle    color.Color

	DarkGray      color.Color
	MediumGray    color.Color
	LightGray     color.Color
	VeryLightGray color.Color
	AlmostWhite   color.Color

	// Colors used outside this package's style set.
	AddressMiddle    color.Color
	AddressSeparator color.Color
	Placeholder      color.Color

	// Alert colors (toast text, its backdrop, and root's notification types).
	AlertBackdrop color.Color
	AlertInfo     color.Color
	AlertWarn     color.Color
	AlertError    color.Color
	AlertDebug    color.Color
	AlertNewTx    color.Color
	AlertNewBlock color.Color
	AlertTxError  color.Color

	NeonOrange   color.Color
	NeonPink     color.Color
	NeonPurple   color.Color
	NeonBlue     color.Color
	NeonGreen    color.Color
	NeonYellow   color.Color
	ElectricBlue color.Color
)

// Package-level styles, rebuilt whenever the palette changes.
var (
	TitleStyle    lipgloss.Style
	SubtitleStyle lipgloss.Style
	HeaderStyle   lipgloss.Style
	FooterStyle   lipgloss.Style

	BoxStyle        lipgloss.Style
	InfoBoxStyle    lipgloss.Style
	SuccessBoxStyle lipgloss.Style
	ErrorBoxStyle   lipgloss.Style
	WarningBoxStyle lipgloss.Style

	BoldText      lipgloss.Style
	NormalText    lipgloss.Style
	DimText       lipgloss.Style
	HighlightText lipgloss.Style
	SuccessText   lipgloss.Style
	ErrorText     lipgloss.Style
	WarningText   lipgloss.Style

	SelectedItemStyle   lipgloss.Style
	UnselectedItemStyle lipgloss.Style

	ActiveMenuItemStyle   lipgloss.Style
	InactiveMenuItemStyle lipgloss.Style

	PrimaryButtonStyle   lipgloss.Style
	SecondaryButtonStyle lipgloss.Style
	DisabledButtonStyle  lipgloss.Style

	OnlineIndicator  lipgloss.Style
	OfflineIndicator lipgloss.Style
	PendingIndicator lipgloss.Style

	BalanceLabelStyle  lipgloss.Style
	BalanceAmountStyle lipgloss.Style

	// Input field styles
	InputStyle      lipgloss.Style
	InputLabelStyle lipgloss.Style

	// Help text styles
	HelpKeyStyle  lipgloss.Style
	HelpDescStyle lipgloss.Style

	// Table styles
	TableHeaderStyle   lipgloss.Style
	TableCellStyle     lipgloss.Style
	TableSelectedStyle lipgloss.Style

	// Status indicator styles (extracted from dashboard View)
	StatusConnectingStyle lipgloss.Style
	StatusConnectedStyle  lipgloss.Style
	StatusSyncingStyle    lipgloss.Style
	StatusSyncedStyle     lipgloss.Style
	StatusErrorStyle      lipgloss.Style

	// Balance display style (extracted from dashboard/send View)
	BalanceValueStyle lipgloss.Style

	// Screen header style (extracted from history View)
	ScreenHeaderStyle lipgloss.Style

	// --- Semantic UI Styles (extracted from inline usage across models) ---

	// Copy/clipboard confirmation text
	CopyStatusStyle lipgloss.Style

	// Copy/clipboard confirmation badge (with background, for history/receive screens)
	CopyStatusBadgeStyle lipgloss.Style

	// Address status badges
	AddressSpentBadge lipgloss.Style
	AddressUsedBadge  lipgloss.Style

	// Derivation path display
	DerivationIndexStyle lipgloss.Style
	DerivationPathStyle  lipgloss.Style

	// Transaction direction indicators
	TxDirectionInStyle  lipgloss.Style
	TxDirectionOutStyle lipgloss.Style

	// PSBT signature status
	SignatureCompleteStyle lipgloss.Style
	SignaturePartialStyle  lipgloss.Style

	// Cosigner status icons (non-bold for visual hierarchy)
	CosignerSignedStyle  lipgloss.Style
	CosignerPendingStyle lipgloss.Style

	// Section subheader (for help screen subsections, etc.)
	SectionSubheaderStyle lipgloss.Style

	// Shortcut key display in help screen tables (Width(10) for column alignment)
	ShortcutKeyStyle lipgloss.Style

	// Section title (gold bold, used in help feature boxes)
	SectionTitleStyle lipgloss.Style

	// Italic tagline/pitch text
	TaglineStyle lipgloss.Style

	// Scroll percentage indicator
	ScrollIndicatorStyle lipgloss.Style

	// Amount display (blue)
	AmountStyle lipgloss.Style

	// Currency unit label
	CurrencyUnitStyle lipgloss.Style

	// Dashboard-specific box variants
	FeeBoxStyle  lipgloss.Style
	MenuBoxStyle lipgloss.Style

	// Progress step indicator
	ProgressStepStyle lipgloss.Style

	// Edit mode box
	EditBoxStyle lipgloss.Style

	// Testing/connecting status indicator
	TestingStatusStyle lipgloss.Style

	// Table wrapper border (for wrapping table.Model in a rounded box)
	TableBorderStyle lipgloss.Style

	// Address/UTXO status badges (for table cells)
	StatusUnusedStyle  lipgloss.Style
	StatusUsedStyle    lipgloss.Style
	StatusPendingStyle lipgloss.Style

	// Address formatting styles (used by internal/ui formatters).
	AddressHeadStyle      lipgloss.Style
	AddressMiddleStyle    lipgloss.Style
	AddressSeparatorStyle lipgloss.Style
)

func init() {
	SetDarkBackground(true)
}

// rebuildHooks lets packages that derive their own package-level styles from
// the palette refresh them when the background changes.
var rebuildHooks []func()

// OnRebuild registers fn to run after every SetDarkBackground call.
func OnRebuild(fn func()) {
	rebuildHooks = append(rebuildHooks, fn)
}

// SetDarkBackground switches the palette and rebuilds every derived style.
// The default is the dark palette, so an unanswered background query keeps the
// UI unchanged.
func SetDarkBackground(dark bool) {
	if dark {
		setDarkPalette()
	} else {
		setLightPalette()
	}
	rebuildStyles()
	for _, fn := range rebuildHooks {
		fn()
	}
}

// Hex returns c as a #RRGGBB string.
func Hex(c color.Color) string {
	converted, _ := colorful.MakeColor(c)
	return converted.Hex()
}

func setDarkPalette() {
	PastelOrange = lipgloss.Color("#FFD4A3")
	PastelPink = lipgloss.Color("#FFBAC3")
	PastelPurple = lipgloss.Color("#D4BBFF")
	PastelBlue = lipgloss.Color("#A8D8FF")
	PastelGreen = lipgloss.Color("#A8E6CF")
	PastelYellow = lipgloss.Color("#FFEAA7")

	BitcoinOrange = lipgloss.Color("#FFB374")
	BitcoinGold = lipgloss.Color("#F5D890")
	Amber = lipgloss.Color("#FFCC80")
	Copper = lipgloss.Color("#E6A57A")

	SuccessGreen = lipgloss.Color("#A8E6CF")
	ErrorRed = lipgloss.Color("#FFB3B3")
	WarningYellow = lipgloss.Color("#FFE66D")
	InfoBlue = lipgloss.Color("#A8D8FF")

	PrimaryBg = lipgloss.Color("#0F111A")
	SecondaryBg = lipgloss.Color("#1A1D2E")
	AccentBg = lipgloss.Color("#252A3D")
	ElevatedBg = lipgloss.Color("#2F3549")

	TextPrimary = lipgloss.Color("#FAFAFA")
	TextSecondary = lipgloss.Color("#A0AEC0")
	TextMuted = lipgloss.Color("#718096")
	TextSubtle = lipgloss.Color("#4A5568")

	DarkGray = lipgloss.Color("#4A5568")
	MediumGray = lipgloss.Color("#718096")
	LightGray = lipgloss.Color("#A0AEC0")
	VeryLightGray = lipgloss.Color("#CBD5E0")
	AlmostWhite = lipgloss.Color("#FAFAFA")

	AddressMiddle = lipgloss.Color("#BAE1FF")
	AddressSeparator = lipgloss.Color("#B0B0B0")
	Placeholder = lipgloss.Color("240")

	AlertBackdrop = lipgloss.Color("#000000")
	AlertInfo = lipgloss.Color("#00FF00")
	AlertWarn = lipgloss.Color("#FFFF00")
	AlertError = lipgloss.Color("#FF0000")
	AlertDebug = lipgloss.Color("#FF00FF")
	AlertNewTx = lipgloss.Color("#B5EAD7")
	AlertNewBlock = lipgloss.Color("#87CEEB")
	AlertTxError = lipgloss.Color("#FFB3BA")

	NeonOrange = BitcoinOrange
	NeonPink = PastelPink
	NeonPurple = PastelPurple
	NeonBlue = PastelBlue
	NeonGreen = PastelGreen
	NeonYellow = PastelYellow
	ElectricBlue = InfoBlue
}

func setLightPalette() {
	PastelOrange = lipgloss.Color("#B85C00")
	PastelPink = lipgloss.Color("#C2185B")
	PastelPurple = lipgloss.Color("#6A3FB5")
	PastelBlue = lipgloss.Color("#1565C0")
	PastelGreen = lipgloss.Color("#2E7D32")
	PastelYellow = lipgloss.Color("#8D6E00")

	BitcoinOrange = lipgloss.Color("#B85C00")
	BitcoinGold = lipgloss.Color("#8D6E00")
	Amber = lipgloss.Color("#8D6E00")
	Copper = lipgloss.Color("#A0522D")

	SuccessGreen = lipgloss.Color("#2E7D32")
	ErrorRed = lipgloss.Color("#C62828")
	WarningYellow = lipgloss.Color("#8D6E00")
	InfoBlue = lipgloss.Color("#1565C0")

	PrimaryBg = lipgloss.Color("#FFFFFF")
	SecondaryBg = lipgloss.Color("#F5F5F5")
	AccentBg = lipgloss.Color("#E8EBF0")
	ElevatedBg = lipgloss.Color("#DDE2EA")

	TextPrimary = lipgloss.Color("#1A1A1A")
	TextSecondary = lipgloss.Color("#4A5568")
	TextMuted = lipgloss.Color("#5F6B7A")
	TextSubtle = lipgloss.Color("#8A94A6")

	DarkGray = lipgloss.Color("#8A94A6")
	MediumGray = lipgloss.Color("#5F6B7A")
	LightGray = lipgloss.Color("#4A5568")
	VeryLightGray = lipgloss.Color("#1A1A1A")
	AlmostWhite = lipgloss.Color("#1A1A1A")

	AddressMiddle = lipgloss.Color("#1565C0")
	AddressSeparator = lipgloss.Color("#7A8698")
	Placeholder = lipgloss.Color("#5F6B7A")

	AlertBackdrop = lipgloss.Color("#FFFFFF")
	AlertInfo = lipgloss.Color("#2E7D32")
	AlertWarn = lipgloss.Color("#8D6E00")
	AlertError = lipgloss.Color("#C62828")
	AlertDebug = lipgloss.Color("#6A3FB5")
	AlertNewTx = lipgloss.Color("#2E7D32")
	AlertNewBlock = lipgloss.Color("#1565C0")
	AlertTxError = lipgloss.Color("#C62828")

	NeonOrange = BitcoinOrange
	NeonPink = PastelPink
	NeonPurple = PastelPurple
	NeonBlue = PastelBlue
	NeonGreen = PastelGreen
	NeonYellow = PastelYellow
	ElectricBlue = InfoBlue
}

func rebuildStyles() {
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
	StatusConnectedStyle = lipgloss.NewStyle().Foreground(PastelBlue)
	StatusSyncingStyle = lipgloss.NewStyle().Foreground(PastelOrange)
	StatusSyncedStyle = lipgloss.NewStyle().Foreground(PastelGreen)
	StatusErrorStyle = lipgloss.NewStyle().Foreground(PastelPink)

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
	StatusUnusedStyle = lipgloss.NewStyle().Foreground(PastelGreen)
	StatusUsedStyle = lipgloss.NewStyle().Foreground(PastelYellow)
	StatusPendingStyle = lipgloss.NewStyle().Foreground(PastelPink).Italic(true)

	// Address formatting styles (used by internal/ui formatters).
	AddressHeadStyle = lipgloss.NewStyle().
		Foreground(PastelOrange).
		Bold(true)
	AddressMiddleStyle = lipgloss.NewStyle().
		Foreground(AddressMiddle)
	AddressSeparatorStyle = lipgloss.NewStyle().
		Foreground(AddressSeparator)
}

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

// RenderHelpWrapped is RenderHelp, but starts a new line whenever the next
// item would push a line past width cells. width <= 0 means no limit.
func RenderHelpWrapped(width int, items ...HelpItem) string {
	if width <= 0 {
		return RenderHelp(items...)
	}
	var lines [][]HelpItem
	var cur []HelpItem
	curW := 0
	for _, item := range items {
		w := lipgloss.Width(item.Key) + 1 + lipgloss.Width(item.Desc)
		if len(cur) > 0 && curW+3+w > width {
			lines = append(lines, cur)
			cur, curW = nil, 0
		}
		if len(cur) > 0 {
			curW += 3 // " • "
		}
		cur = append(cur, item)
		curW += w
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	return RenderHelpLines(lines...)
}

func CenterHorizontal(text string, width int) string {
	return lipgloss.Place(width, 1, lipgloss.Center, lipgloss.Center, text)
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
	return ErrorBoxStyle.Width(boxWidth + 2).Render(wrapped)
}

// Badge creates a styled inline badge with the given foreground color on AccentBg.
func Badge(fg color.Color) lipgloss.Style {
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
