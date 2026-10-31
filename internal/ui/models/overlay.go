package models

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
)

// flowBlock renders a visualizer as an opaque block. TxFlowView leaves blank
// rows empty; padding them keeps the screen behind an overlay from showing
// through the diagram.
func flowBlock(v TxFlowView) string {
	return lipgloss.NewStyle().
		Width(v.effectiveWidth()).
		Height(v.effectiveHeight()).
		Render(v.View())
}

// panelContentWidth returns the widest line centeredPanel can show inside a
// frame of the given width: the frame keeps a 2-cell margin on each side and
// the panel spends 6 cells on its border and 1x2 padding.
func panelContentWidth(frameWidth int) int {
	if frameWidth <= 0 {
		return 0
	}
	return max(1, frameWidth-10)
}

// centeredPanel draws content inside a rounded bordered panel centered in a
// frame at least width by height. Lip Gloss v2 counts padding and the border
// in Width, so the panel asks for the widest content line plus 6 cells and
// leaves a 2-cell margin on each side of the frame. Content wider than the
// panel is truncated instead of wrapped, keeping one terminal row per line.
func centeredPanel(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	contentWidth := 0
	for _, line := range lines {
		contentWidth = max(contentWidth, lipgloss.Width(line))
	}

	panelWidth := contentWidth + 6 // 1x2 padding and the rounded border
	if width > 0 {
		panelWidth = max(8, min(panelWidth, width-4))
	}
	innerWidth := panelWidth - 6
	for i, line := range lines {
		if lipgloss.Width(line) > innerWidth {
			lines[i] = ansi.Truncate(line, innerWidth, "…")
		}
	}

	panel := styles.BoxStyle.
		MarginBottom(0).
		BorderForeground(styles.ElectricBlue).
		Padding(1, 2).
		Width(panelWidth).
		Render(strings.Join(lines, "\n"))

	frameWidth := max(width, lipgloss.Width(panel))
	frameHeight := max(height, lipgloss.Height(panel))
	return ui.Composite("", frameWidth, frameHeight,
		ui.Overlay{Content: panel}.Center())
}
