package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Anchor places an overlay inside the composited frame.
type Anchor int

const (
	// AnchorTopLeft keeps the overlay at its explicit X and Y.
	AnchorTopLeft Anchor = iota
	AnchorTopCenter
	AnchorTopRight
	AnchorCenter
	AnchorBottomLeft
	AnchorBottomCenter
	AnchorBottomRight
)

// Overlay is one layer drawn over a base frame. Content is measured in
// terminal cells with lipgloss.Width and lipgloss.Height, so wide runes and
// styled text land on the same cells the terminal uses.
type Overlay struct {
	Content string
	X, Y    int

	anchor Anchor
}

// TopLeft places the overlay at Content's explicit X and Y.
func (o Overlay) TopLeft() Overlay {
	o.anchor = AnchorTopLeft
	return o
}

// TopCenter centers the overlay horizontally against the frame's top edge.
func (o Overlay) TopCenter() Overlay {
	o.anchor = AnchorTopCenter
	return o
}

// TopRight aligns the overlay to the frame's top-right corner.
func (o Overlay) TopRight() Overlay {
	o.anchor = AnchorTopRight
	return o
}

// Center centers the overlay in the frame.
func (o Overlay) Center() Overlay {
	o.anchor = AnchorCenter
	return o
}

// BottomLeft aligns the overlay to the frame's bottom-left corner.
func (o Overlay) BottomLeft() Overlay {
	o.anchor = AnchorBottomLeft
	return o
}

// BottomCenter centers the overlay against the frame's bottom edge.
func (o Overlay) BottomCenter() Overlay {
	o.anchor = AnchorBottomCenter
	return o
}

// BottomRight aligns the overlay to the frame's bottom-right corner.
func (o Overlay) BottomRight() Overlay {
	o.anchor = AnchorBottomRight
	return o
}

// position resolves the overlay's top-left cell in a frame of the given size.
func (o Overlay) position(frameWidth, frameHeight int) (int, int) {
	width, height := lipgloss.Width(o.Content), lipgloss.Height(o.Content)
	x, y := o.X, o.Y

	switch o.anchor {
	case AnchorTopCenter:
		x, y = (frameWidth-width)/2, 0
	case AnchorTopRight:
		x, y = frameWidth-width, 0
	case AnchorCenter:
		x, y = (frameWidth-width)/2, (frameHeight-height)/2
	case AnchorBottomLeft:
		x, y = 0, frameHeight-height
	case AnchorBottomCenter:
		x, y = (frameWidth-width)/2, frameHeight-height
	case AnchorBottomRight:
		x, y = frameWidth-width, frameHeight-height
	}

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// Composite draws overlays over base and returns exactly width by height
// cells. The base is padded (or clipped) to the frame first, so adding or
// removing an overlay never changes the frame size. Overlays are clipped to
// the frame instead of wrapping.
func Composite(base string, width, height int, overlays ...Overlay) string {
	if width <= 0 || height <= 0 {
		return base
	}

	layers := make([]*lipgloss.Layer, 0, len(overlays)+1)
	layers = append(layers, lipgloss.NewLayer(fitBlock(base, width, height)).Z(0))

	for i, overlay := range overlays {
		content := overlay.Content
		if content == "" {
			continue
		}
		x, y := overlay.position(width, height)
		content = clipBlock(content, width, height, x, y)
		if content == "" {
			continue
		}
		layers = append(layers, lipgloss.NewLayer(content).X(x).Y(y).Z(i+1))
	}

	rendered := lipgloss.NewCompositor(layers...).Render()
	return fitBlock(rendered, width, height)
}

// fitBlock clips content to width by height and pads every short line with
// spaces so the block always occupies the whole frame.
func fitBlock(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}

	blank := strings.Repeat(" ", width)
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			lines[i] = ansi.Truncate(line, width, "")
		} else if pad := width - w; pad > 0 {
			lines[i] = line + strings.Repeat(" ", pad)
		}
	}
	for len(lines) < height {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

// clipBlock trims overlay content so it stays inside the frame at x,y. Rows
// or columns that fall outside are dropped; a long line is truncated at the
// frame edge instead of being wrapped onto a new row.
func clipBlock(content string, width, height, x, y int) string {
	lines := strings.Split(content, "\n")

	if y < 0 {
		drop := -y
		if drop >= len(lines) {
			return ""
		}
		lines = lines[drop:]
		y = 0
	}
	if maxRows := height - y; maxRows < len(lines) {
		if maxRows <= 0 {
			return ""
		}
		lines = lines[:maxRows]
	}

	available := width - x
	if available <= 0 {
		return ""
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > available {
			line = ansi.Truncate(line, available, "")
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
