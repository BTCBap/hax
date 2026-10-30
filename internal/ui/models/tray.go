package models

import (
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TrayStage is the step of a two-stage tray flow.
type TrayStage int

const (
	TrayStageChoose TrayStage = iota
	TrayStageConfirm
)

// TrayResultKind describes what Tray.Update did with a key press.
type TrayResultKind int

const (
	TrayNone TrayResultKind = iota
	TrayCancelled
	TrayConfirmed
)

// TrayResult is returned by Tray.Update. Confirmed carries the chosen
// option index; the other kinds leave it zero.
type TrayResult struct {
	Kind     TrayResultKind
	Selected int
}

// TrayOption is one selectable row: a short label and dim helper text
// shown in a fixed column to its right.
type TrayOption struct {
	Label string
	Help  string
}

// TraySummaryRow is a label/value pair shown on the confirm stage.
type TraySummaryRow struct {
	Label string
	Value string
}

// Tray is a focused bottom panel with a rounded border, a fixed height
// and a choose -> confirm flow.
type Tray struct {
	title   string
	options []TrayOption
	summary []TraySummaryRow

	// SummaryFor, when set, supplies the confirm rows for the selected
	// option so callers can describe the choice that is about to happen.
	SummaryFor func(selected int) []TraySummaryRow

	// note is a dim, wrapped line shown below the confirm rows. Its rows
	// are reserved on the choose stage too, so the tray keeps one height.
	note        string
	noteLines   []string
	confirmHint string

	width  int
	stage  TrayStage
	cursor int

	contentWidth  int
	labelWidth    int
	helperWidth   int
	summaryLabelW int
	summaryValueW int
	rowsHeight    int
}

// NewTray builds a tray that fits in width cells. The height is fixed from
// the larger of the choose and confirm stages, so moving between stages or
// options never changes the number of lines.
func NewTray(title string, options []TrayOption, summary []TraySummaryRow, width int) Tray {
	t := Tray{
		title:   title,
		options: options,
		summary: summary,
		width:   width,
	}
	if t.SummaryFor != nil {
		t.summary = t.SummaryFor(0)
	}
	t.recalculate()
	return t
}

// WithWidth returns the tray resized for a new terminal width while
// keeping the stage and selection.
func (t Tray) WithWidth(width int) Tray {
	t.width = width
	t.recalculate()
	return t
}

// WithNote sets a dim note shown on the confirm stage. The note is wrapped
// to the tray width and its rows are reserved on both stages.
func (t Tray) WithNote(note string) Tray {
	t.note = note
	t.recalculate()
	return t
}

// WithConfirmHint overrides the confirm stage key hint.
func (t Tray) WithConfirmHint(hint string) Tray {
	t.confirmHint = hint
	return t
}

// View renders the tray with rounded borders.
func (t Tray) View() string {
	rows := make([]string, t.rowsHeight)
	switch t.stage {
	case TrayStageConfirm:
		for i := 0; i < t.rowsHeight && i < len(t.summary); i++ {
			rows[i] = t.renderSummaryRow(t.summary[i])
		}
		if len(t.noteLines) > 0 {
			if i := len(t.summary); i < t.rowsHeight {
				rows[i] = ""
			}
			for j, line := range t.noteLines {
				i := len(t.summary) + 1 + j
				if i >= t.rowsHeight {
					break
				}
				rows[i] = styles.DimText.Render(padCell(line, t.contentWidth))
			}
		}
	default:
		for i := 0; i < t.rowsHeight && i < len(t.options); i++ {
			rows[i] = t.renderOptionRow(i, t.options[i])
		}
	}

	hint := "enter continue • esc cancel"
	if t.stage == TrayStageConfirm {
		hint = "enter confirm • esc back"
		if t.confirmHint != "" {
			hint = t.confirmHint
		}
	}

	inner := []string{
		styles.SectionTitleStyle.Render(t.title),
		"",
	}
	inner = append(inner, rows...)
	inner = append(inner, "", styles.DimText.Render(hint))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.BitcoinOrange).
		Padding(0, 1).
		Width(t.contentWidth + 2).
		Render(strings.Join(inner, "\n"))
}

// Update handles a key press. Enter advances from choose to confirm, and
// confirms from the confirm stage. Esc goes back from confirm and cancels
// from choose.
func (t Tray) Update(msg tea.KeyMsg) (Tray, TrayResult) {
	switch msg.String() {
	case "esc":
		if t.stage == TrayStageConfirm {
			t.stage = TrayStageChoose
			return t, TrayResult{Kind: TrayNone}
		}
		return t, TrayResult{Kind: TrayCancelled}
	case "up", "k":
		if t.stage == TrayStageChoose && t.cursor > 0 {
			t.cursor--
		}
	case "down", "j":
		if t.stage == TrayStageChoose && t.cursor < len(t.options)-1 {
			t.cursor++
		}
	case "enter":
		if t.stage == TrayStageChoose {
			if t.SummaryFor != nil {
				t.summary = t.SummaryFor(t.cursor)
				t.recalculate()
			}
			t.stage = TrayStageConfirm
			return t, TrayResult{Kind: TrayNone}
		}
		return t, TrayResult{Kind: TrayConfirmed, Selected: t.cursor}
	}
	return t, TrayResult{Kind: TrayNone}
}

func (t *Tray) recalculate() {
	outer := t.width
	if outer <= 0 {
		outer = 80
	}
	if outer < 12 {
		outer = 12
	}
	t.width = outer
	t.contentWidth = outer - 4 // rounded border and one cell of padding each side

	if t.note != "" {
		t.noteLines = wrapWords(t.note, t.contentWidth)
	} else {
		t.noteLines = nil
	}

	maxLabel, maxHelper := 0, 0
	for _, opt := range t.options {
		if w := lipgloss.Width(opt.Label); w > maxLabel {
			maxLabel = w
		}
		if w := lipgloss.Width(opt.Help); w > maxHelper {
			maxHelper = w
		}
	}

	available := t.contentWidth - 2 // "› " marker
	if available < 1 {
		available = 1
	}
	t.labelWidth = maxLabel
	t.helperWidth = maxHelper
	if t.labelWidth+t.helperWidth+2 > available {
		if room := available - 2 - t.helperWidth; room < t.labelWidth {
			t.labelWidth = room
		}
		if t.labelWidth < 4 {
			t.labelWidth = 4
		}
		if t.labelWidth+t.helperWidth+2 > available {
			t.helperWidth = available - 2 - t.labelWidth
			if t.helperWidth < 0 {
				t.helperWidth = 0
			}
		}
	}

	t.summaryLabelW, t.summaryValueW = 0, 0
	for _, row := range t.summary {
		if w := lipgloss.Width(row.Label); w > t.summaryLabelW {
			t.summaryLabelW = w
		}
		if w := lipgloss.Width(row.Value); w > t.summaryValueW {
			t.summaryValueW = w
		}
	}
	if t.summaryLabelW+t.summaryValueW+2 > t.contentWidth {
		if room := t.contentWidth - 2 - t.summaryValueW; room < t.summaryLabelW {
			t.summaryLabelW = room
		}
		if t.summaryLabelW < 4 {
			t.summaryLabelW = 4
		}
		if t.summaryLabelW+t.summaryValueW+2 > t.contentWidth {
			t.summaryValueW = t.contentWidth - 2 - t.summaryLabelW
			if t.summaryValueW < 0 {
				t.summaryValueW = 0
			}
		}
	}

	// The height is fixed from the larger stage. Later summary changes pad
	// or truncate to these rows instead of resizing the tray.
	confirmRows := len(t.summary)
	if len(t.noteLines) > 0 {
		confirmRows += 1 + len(t.noteLines)
	}
	t.rowsHeight = len(t.options)
	if confirmRows > t.rowsHeight {
		t.rowsHeight = confirmRows
	}
	if t.rowsHeight < 1 {
		t.rowsHeight = 1
	}
}

func (t Tray) renderOptionRow(index int, opt TrayOption) string {
	marker := "  "
	markerStyle := styles.DimText
	labelStyle := styles.NormalText
	if index == t.cursor {
		marker = "› "
		markerStyle = styles.HighlightText
		labelStyle = styles.HighlightText
	}

	label := labelStyle.Render(padCell(opt.Label, t.labelWidth))
	helper := styles.DimText.Render(padCell(opt.Help, t.helperWidth))
	return markerStyle.Render(marker) + label + "  " + helper
}

func (t Tray) renderSummaryRow(row TraySummaryRow) string {
	label := styles.DimText.Render(padCell(row.Label, t.summaryLabelW))
	value := styles.NormalText.Render(padCell(row.Value, t.summaryValueW))
	return label + "  " + value
}

// padCell truncates text to width and pads it with spaces so following
// columns start at the same x on every row.
func padCell(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = truncateRunes(text, width, "…")
	if pad := width - lipgloss.Width(text); pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	return text
}
