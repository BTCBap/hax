package models

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// TxFlowSide selects which column of a transaction flow is in focus.
type TxFlowSide int

const (
	FlowInput TxFlowSide = iota
	FlowOutput
)

// TxFlowKind describes who owns an input or output.
type TxFlowKind int

const (
	KindExternal     TxFlowKind     = iota // someone else's address
	KindReceive                            // one of our receive addresses
	KindChange                             // one of our change addresses
	KindOwnInput                           // an input this wallet can sign for
	KindForeignInput                       // an input from someone else
	KindFee                                // the network fee pseudo-output
	KindOpReturn                           // a data output without an address
	KindUnknown      = KindOpReturn        // scripts with no recognizable owner
)

// TxFlowItem is one input, output, or the fee row. OutPoint is set for
// inputs; ValueKnown is false when the amount could not be verified.
type TxFlowItem struct {
	Address    string
	Label      string
	OutPoint   string
	Value      api.Satoshi
	ValueKnown bool
	Mine       bool
	Kind       TxFlowKind
}

// TxFlow is the data behind a transaction flow diagram.
type TxFlow struct {
	TxID     string
	Inputs   []TxFlowItem
	Outputs  []TxFlowItem
	Fee      api.Satoshi
	FeeKnown bool
	FeeRate  float64
	VSize    int
}

// TxFlowFromInspection builds a flow from the wallet's PSBT inspection.
// Nothing is taken on trust: the inspection has already checked ownership.
func TxFlowFromInspection(insp *wallet.PSBTInspection, labels func(addr string) string) TxFlow {
	flow := TxFlow{}
	if insp == nil {
		return flow
	}
	flow.Fee = insp.Fee
	flow.FeeKnown = insp.FeeKnown
	flow.FeeRate = insp.FeeRate
	flow.VSize = insp.VSize

	for _, in := range insp.Inputs {
		kind := KindForeignInput
		if in.Mine {
			kind = KindOwnInput
		}
		flow.Inputs = append(flow.Inputs, TxFlowItem{
			Address:    in.Address,
			Label:      flowLabel(labels, in.Address),
			OutPoint:   in.OutPoint,
			Value:      in.Value,
			ValueKnown: in.ValueKnown,
			Mine:       in.Mine,
			Kind:       kind,
		})
	}
	for _, out := range insp.Outputs {
		kind := KindExternal
		switch {
		case out.Address == "":
			kind = KindOpReturn
		case out.Owner == wallet.OutputReceive:
			kind = KindReceive
		case out.Owner == wallet.OutputChange:
			kind = KindChange
		}
		flow.Outputs = append(flow.Outputs, TxFlowItem{
			Address:    out.Address,
			Label:      flowLabel(labels, out.Address),
			Value:      out.Value,
			ValueKnown: true,
			Mine:       kind == KindReceive || kind == KindChange,
			Kind:       kind,
		})
	}
	return flow
}

// TxFlowFromTransaction builds a flow from a confirmed or mempool
// transaction. Ownership is supplied by the caller because a backend
// transaction has no derivation paths.
func TxFlowFromTransaction(tx api.Transaction, isMine func(addr string) bool, isChange func(addr string) bool, labels func(string) string) TxFlow {
	flow := TxFlow{TxID: tx.TxID}
	for _, vin := range tx.Vin {
		item := TxFlowItem{
			OutPoint: fmt.Sprintf("%s:%d", vin.TxID, vin.Vout),
			Kind:     KindForeignInput,
		}
		if vin.Prevout != nil {
			item.Address = vin.Prevout.ScriptpubkeyAddress
			item.Value = vin.Prevout.Value
			item.ValueKnown = true
		}
		item.Mine = item.Address != "" && flowBool(isMine, item.Address)
		if item.Mine {
			item.Kind = KindOwnInput
		}
		item.Label = flowLabel(labels, item.Address)
		flow.Inputs = append(flow.Inputs, item)
	}
	for _, vout := range tx.Vout {
		addr := vout.ScriptpubkeyAddress
		mine := addr != "" && flowBool(isMine, addr)
		kind := KindExternal
		switch {
		case addr == "":
			kind = KindOpReturn
		case mine && flowBool(isChange, addr):
			kind = KindChange
		case mine:
			kind = KindReceive
		}
		flow.Outputs = append(flow.Outputs, TxFlowItem{
			Address:    addr,
			Label:      flowLabel(labels, addr),
			Value:      vout.Value,
			ValueKnown: true,
			Mine:       mine,
			Kind:       kind,
		})
	}
	flow.Fee = tx.Fee
	flow.FeeKnown = true
	flow.VSize = tx.Size
	if tx.Weight > 0 {
		flow.VSize = (tx.Weight + 3) / 4
	}
	if flow.VSize > 0 {
		flow.FeeRate = float64(flow.Fee) / float64(flow.VSize)
	}
	return flow
}

func flowLabel(labels func(string) string, addr string) string {
	if labels == nil || addr == "" {
		return ""
	}
	return sanitizeLine(labels(addr))
}

func flowBool(f func(string) bool, addr string) bool {
	return f != nil && f(addr)
}

// TxFlowView renders a TxFlow. Width and Height describe the whole
// component, including the header, detail reserve, and key hint.
type TxFlowView struct {
	Flow   TxFlow
	Width  int
	Height int
	Side   TxFlowSide
	Cursor int
	// Offset is the first visible item on each side, indexed by TxFlowSide.
	Offset [2]int
}

// handleTxFlowKey applies a visualizer key and reports whether it asks to
// leave the view. Up/down or k/j move the cursor, tab toggles the side,
// left/right select a side directly, and esc/q close.
func handleTxFlowKey(v *TxFlowView, key string) (back bool) {
	switch key {
	case "up", "k":
		v.MoveUp()
	case "down", "j":
		v.MoveDown()
	case "tab":
		v.SwitchSide()
	case "left":
		v.SwitchSide(FlowInput)
	case "right":
		v.SwitchSide(FlowOutput)
	case "esc", "q":
		return true
	}
	return false
}

// MoveUp selects the previous item on the current side.
func (v *TxFlowView) MoveUp() {
	n := len(v.itemsFor(v.Side))
	if n == 0 {
		v.Cursor = 0
		return
	}
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor >= n {
		v.Cursor = n - 1
	}
	v.ensureOffset()
}

// MoveDown selects the next item on the current side.
func (v *TxFlowView) MoveDown() {
	n := len(v.itemsFor(v.Side))
	if n == 0 {
		v.Cursor = 0
		return
	}
	if v.Cursor < n-1 {
		v.Cursor++
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	v.ensureOffset()
}

// SwitchSide moves focus to the other column (tab or left/right). An
// optional side argument moves directly to that column.
func (v *TxFlowView) SwitchSide(side ...TxFlowSide) {
	if len(side) > 0 {
		v.Side = side[0]
	} else if v.Side == FlowInput {
		v.Side = FlowOutput
	} else {
		v.Side = FlowInput
	}
	n := len(v.itemsFor(v.Side))
	if n == 0 {
		v.Cursor = 0
		v.Offset[v.Side.index()] = 0
		return
	}
	off := v.Offset[v.Side.index()]
	if off < 0 {
		off = 0
	}
	if off > n-1 {
		off = n - 1
	}
	v.Cursor = off
	v.ensureOffset()
}

// View renders the flow at a stable line count for every cursor position,
// side, and scroll offset.
func (v TxFlowView) View() string {
	width := v.effectiveWidth()
	height := v.effectiveHeight()

	lines := make([]string, 0, height)
	lines = append(lines, v.headerLine(width))
	lines = append(lines, "")
	lines = append(lines, v.diagramLines(width, v.diagramRows())...)
	lines = append(lines, "")
	lines = append(lines, v.detailLines(width)...)
	if len(lines) > height-1 {
		lines = lines[:max(0, height-1)]
	}
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, v.hintLine(width))

	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (v TxFlowView) effectiveWidth() int {
	if v.Width > 0 {
		return v.Width
	}
	return 80
}

func (v TxFlowView) effectiveHeight() int {
	if v.Height > 0 {
		return v.Height
	}
	return 24
}

func (v TxFlowView) diagramRows() int {
	rows := v.effectiveHeight() - 7
	if rows < 0 {
		rows = 0
	}
	return rows
}

func (v TxFlowView) itemsFor(side TxFlowSide) []TxFlowItem {
	if side == FlowInput {
		return v.Flow.Inputs
	}
	out := make([]TxFlowItem, 0, len(v.Flow.Outputs)+1)
	out = append(out, v.Flow.Outputs...)
	for _, item := range out {
		if item.Kind == KindFee {
			return out
		}
	}
	return append(out, TxFlowItem{
		Value:      v.Flow.Fee,
		ValueKnown: v.Flow.FeeKnown,
		Kind:       KindFee,
	})
}

func (s TxFlowSide) index() int {
	if s == FlowOutput {
		return 1
	}
	return 0
}

func (v TxFlowView) effectiveOffset(side TxFlowSide) int {
	n := len(v.itemsFor(side))
	if n == 0 {
		return 0
	}
	off := clampFlowIndex(v.Offset[side.index()], n)
	if v.Side != side {
		return off
	}
	rows := v.diagramRows()
	if rows < 1 {
		rows = 1
	}
	return flowFitOffset(n, off, v.Cursor, rows)
}

func (v *TxFlowView) ensureOffset() {
	n := len(v.itemsFor(v.Side))
	side := v.Side.index()
	if n == 0 {
		v.Cursor = 0
		v.Offset[side] = 0
		return
	}
	v.Cursor = clampFlowIndex(v.Cursor, n)
	rows := v.diagramRows()
	if rows < 1 {
		rows = 1
	}
	v.Offset[side] = flowFitOffset(n, v.Offset[side], v.Cursor, rows)
}

func clampFlowIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// flowFitOffset moves offset the smallest amount that keeps cursor visible.
func flowFitOffset(n, off, cursor, rows int) int {
	off = clampFlowIndex(off, n)
	cursor = clampFlowIndex(cursor, n)
	for i := 0; i < 4; i++ {
		_, count, _, _ := flowWindow(n, off, rows)
		if count < 1 {
			count = 1
		}
		switch {
		case cursor < off:
			off = cursor
		case cursor >= off+count:
			off = cursor - count + 1
		default:
			return off
		}
		off = clampFlowIndex(off, n)
	}
	return off
}

// flowWindow returns the visible item range and the counts of items hidden
// above and below it. A scrolled window reserves rows for the indicators.
func flowWindow(n, offset, rows int) (start, count, above, below int) {
	if n <= 0 || rows <= 0 {
		return 0, 0, 0, 0
	}
	offset = clampFlowIndex(offset, n)
	if n <= rows {
		return 0, n, 0, 0
	}
	if rows == 1 {
		return offset, 1, 0, 0
	}
	if offset == 0 {
		count = rows - 1
		return 0, count, 0, n - count
	}
	remaining := n - offset
	if remaining <= rows-1 {
		return offset, remaining, offset, 0
	}
	count = rows - 2
	if count < 1 {
		count = 1
	}
	return offset, count, offset, n - offset - count
}

const flowAmountWidth = len("0.00000000")

func flowAmountText(item TxFlowItem) string {
	if !item.ValueKnown {
		return "unknown"
	}
	return formatFlowCoin(item.Value)
}

// flowColumnWidths splits the terminal into two label columns and the
// central canvas. The canvas takes about a third of the width so the
// ribbons have room to curve; the labels keep the rest and elide.
func flowColumnWidths(width int) (left, canvas, right int) {
	if width <= 0 {
		return 0, 0, 0
	}
	canvas = int(math.Round(float64(width) * 0.33))
	if canvas < 20 {
		canvas = 20
	}
	if canvas > 52 {
		canvas = 52
	}
	if canvas > width {
		canvas = width
	}
	left = (width - canvas) / 2
	right = width - canvas - left
	return left, canvas, right
}

func (v TxFlowView) headerLine(width int) string {
	leftW, canvasW, rightW := flowColumnWidths(width)
	inTotal, _ := flowTotal(v.Flow.Inputs)
	outTotal, _ := flowTotal(v.Flow.Outputs)
	left := fitFlowText(flowHeaderText(" Inputs", v.Flow.Inputs, inTotal, leftW), leftW)
	left = padFlowRight(left, leftW)
	right := fitFlowText(flowHeaderText("Outputs", v.Flow.Outputs, outTotal, rightW), rightW)
	right = padFlowLeft(right, rightW)
	return styles.HighlightText.Render(left) +
		strings.Repeat(" ", canvasW) +
		styles.HighlightText.Render(right)
}

func flowHeaderText(prefix string, items []TxFlowItem, total api.Satoshi, width int) string {
	totalText := flowHeaderTotal(items, total)
	for _, gap := range []string{"   ", " ", ""} {
		text := prefix + gap + totalText
		if ansi.StringWidth(text) <= width {
			return text
		}
	}
	return prefix + " " + totalText
}

func flowHeaderTotal(items []TxFlowItem, total api.Satoshi) string {
	text := formatFlowBTC(total)
	if _, unknown := flowTotal(items); unknown > 0 {
		text += " + ?"
	}
	return text
}

func flowTotal(items []TxFlowItem) (api.Satoshi, int) {
	var total api.Satoshi
	unknown := 0
	for _, item := range items {
		if item.ValueKnown {
			total += item.Value
		} else {
			unknown++
		}
	}
	return total, unknown
}

// flowEntry is one label row in the diagram: an item, or an aggregate row
// standing in for the items hidden above or below the window.
type flowEntry struct {
	item    int // index into the side's items, -1 for an aggregate
	more    int
	up      bool
	row     int
	kind    TxFlowKind
	value   api.Satoshi
	known   bool
	unknown int
}

func (e flowEntry) aggregate() bool { return e.item < 0 }

func (e flowEntry) dashed() bool { return !e.known || e.aggregate() }

// flowEntries collects the visible entries for one side. Rows and band
// heights come from flowDiagramLayouts, which lays out both sides against
// one shared band budget. The windowing rules are the same as the old bus
// diagram.
func (v TxFlowView) flowEntries(side TxFlowSide, rows int) []flowEntry {
	items := v.itemsFor(side)
	n := len(items)
	if n == 0 || rows <= 0 {
		return nil
	}
	start, count, above, below := flowWindow(n, v.effectiveOffset(side), rows)
	entries := make([]flowEntry, 0, count+2)
	if above > 0 {
		entry := flowEntry{item: -1, more: above, up: true}
		entry.value, entry.known, entry.unknown = flowAggregateValues(items[:start])
		entries = append(entries, entry)
	}
	for i := start; i < start+count; i++ {
		item := items[i]
		entries = append(entries, flowEntry{
			item:  i,
			kind:  item.Kind,
			value: item.Value,
			known: item.ValueKnown,
		})
	}
	if below > 0 {
		entry := flowEntry{item: -1, more: below}
		entry.value, entry.known, entry.unknown = flowAggregateValues(items[start+count:])
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

func flowAggregateValues(items []TxFlowItem) (api.Satoshi, bool, int) {
	var value api.Satoshi
	unknown := 0
	for _, item := range items {
		if item.ValueKnown {
			value += item.Value
		} else {
			unknown++
		}
	}
	return value, unknown == 0, unknown
}

// flowSub is the number of sub-pixel rows in one text row. The band canvas
// resolves ribbons at this vertical resolution and cells() packs a text row
// back into one of the eighth-block glyphs.
const flowSub = 8

// flowMinRibbon is the thinnest ribbon, in sub-pixels.
const flowMinRibbon = flowSub / 4

// flowMaxRibbon is the thickest ribbon, in sub-pixels. It keeps a single
// dominant input or output from turning into a heavy slab.
const flowMaxRibbon = flowSub * 3 / 2

// flowAllocate divides a sub-pixel budget between entries. Every entry keeps
// the minimum; the rest follows the square root of the known values, which
// keeps small flows visible. Unknown and aggregate entries stay at the
// minimum so they read as a hairline rather than a false proportion. No
// ribbon may take more than 45% of the budget when there is another weighted
// entry to share with, and none may exceed flowMaxRibbon. Budget left over
// after a ribbon hits one of those caps is dropped: the remaining ribbons
// keep their proportional thickness instead of growing to fill the trunk.
func flowAllocate(entries []flowEntry, budget int) []int {
	n := len(entries)
	if n == 0 {
		return nil
	}
	if budget < n*flowMinRibbon {
		budget = n * flowMinRibbon
	}
	heights := make([]int, n)
	for i := range heights {
		heights[i] = flowMinRibbon
	}

	weighted := make([]int, 0, n)
	weightSum := 0.0
	weights := make([]float64, n)
	for i, entry := range entries {
		if entry.aggregate() || !entry.known || entry.value <= 0 {
			continue
		}
		weights[i] = math.Sqrt(float64(entry.value))
		weightSum += weights[i]
		weighted = append(weighted, i)
	}
	if len(weighted) == 0 || weightSum <= 0 {
		return heights
	}
	extra := budget - n*flowMinRibbon
	if extra <= 0 {
		return heights
	}

	shares := make([]float64, n)
	for _, i := range weighted {
		shares[i] = float64(flowMinRibbon) + float64(extra)*weights[i]/weightSum
	}

	capShare := 0.0
	if len(weighted) >= 2 {
		capShare = 0.45 * float64(budget)
		if capShare < float64(flowMinRibbon) {
			capShare = float64(flowMinRibbon)
		}
		capped := make([]bool, n)
		for {
			excess := 0.0
			changed := false
			for _, i := range weighted {
				if capped[i] || shares[i] <= capShare {
					continue
				}
				excess += shares[i] - capShare
				shares[i] = capShare
				capped[i] = true
				changed = true
			}
			if !changed {
				break
			}
			uncappedWeight := 0.0
			for _, i := range weighted {
				if !capped[i] {
					uncappedWeight += weights[i]
				}
			}
			if uncappedWeight <= 0 {
				break
			}
			for _, i := range weighted {
				if !capped[i] {
					shares[i] += excess * weights[i] / uncappedWeight
				}
			}
		}
	}

	total := 0
	fracs := make([]float64, n)
	for i := range heights {
		if shares[i] > float64(flowMaxRibbon) {
			shares[i] = float64(flowMaxRibbon)
		}
		whole := int(math.Floor(shares[i]))
		if whole < flowMinRibbon {
			whole = flowMinRibbon
		}
		heights[i] = whole
		fracs[i] = shares[i] - float64(whole)
		total += whole
	}
	for leftover := budget - total; leftover > 0; leftover-- {
		best := -1
		for _, i := range weighted {
			if fracs[i] <= 0 || heights[i]+1 > flowMaxRibbon {
				continue
			}
			if len(weighted) >= 2 && float64(heights[i]+1) > capShare {
				continue
			}
			if best < 0 || fracs[i] > fracs[best] ||
				(fracs[i] == fracs[best] && heights[i] > heights[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		heights[best]++
		fracs[best]--
	}
	// A proportional pass can only overshoot after clamping a minimum, but
	// guard the invariant the renderer relies on.
	for total := flowHeightsTotal(heights); total > budget; total-- {
		best := -1
		for i := range heights {
			if heights[i] <= flowMinRibbon {
				continue
			}
			if best < 0 || heights[i] > heights[best] {
				best = i
			}
		}
		if best < 0 {
			break
		}
		heights[best]--
	}
	return heights
}

func flowHeightsTotal(heights []int) int {
	total := 0
	for _, h := range heights {
		total += h
	}
	return total
}

// flowSideLayout turns a side's entries into rows and ribbon thicknesses.
// Slices are the pixel share each ribbon gets inside the trunk; edges are
// the same thickness capped to the entry's label slot and kept constant
// from label to trunk. Heights and tops are text rows.
type flowSideLayout struct {
	entries   []flowEntry
	slices    []int
	heights   []int
	edges     []int
	tops      []int
	gap       int
	stackTop  int
	stackRows int
	trunkPx   int
}

func (l flowSideLayout) edgeThickness(i int) int {
	return l.edges[i]
}

// edgeCenter places the edge band on its label row. The band's vertical
// center is the pixel center of the label row, shifted only as far as
// needed to stay inside the entry's slot.
func (l flowSideLayout) edgeCenter(i int) float64 {
	thickness := float64(l.edges[i])
	slotTop := float64(flowSub * l.tops[i])
	slotBottom := float64(flowSub * (l.tops[i] + l.heights[i]))
	center := float64(flowSub*l.entries[i].row + flowSub/2)
	if center-thickness/2 < slotTop {
		center = slotTop + thickness/2
	}
	if center+thickness/2 > slotBottom {
		center = slotBottom - thickness/2
	}
	return center
}

// flowTrunkPixels is the shared sub-pixel budget for the trunk body:
// about a fifth of the diagram height, at least flowSub/4 per entry and
// capped so a margin always remains above and below.
func flowTrunkPixels(inCount, outCount, rows int) int {
	if rows <= 0 {
		return 0
	}
	minimum := max(inCount, outCount) * flowMinRibbon
	maximum := flowSub*rows - flowSub
	if maximum < minimum {
		maximum = minimum
	}
	budget := int(math.Round(0.20 * float64(flowSub*rows)))
	if budget < minimum {
		budget = minimum
	}
	if budget > maximum {
		budget = maximum
	}
	return budget
}

// flowLayoutSide sizes the labels for one side and spreads them over the
// diagram. Each entry claims max(1, ceil(slice/flowSub)) rows; leftover rows
// become equal gaps (at most four) with any remainder split as top and
// bottom margin. That fans the ribbons over most of the height.
func flowLayoutSide(entries []flowEntry, rows, trunkPx int) flowSideLayout {
	return flowLayoutSideWithBudget(entries, rows, trunkPx, trunkPx)
}

// flowLayoutSideWithBudget lays out one side with an explicit allocation
// budget while still reporting trunkPx as the shared trunk value. A side
// whose ribbons hit their per-ribbon caps can fall short of the budget, so
// flowDiagramLayouts uses this to re-allocate the taller side down to the
// shorter one's total.
func flowLayoutSideWithBudget(entries []flowEntry, rows, trunkPx, allocBudget int) flowSideLayout {
	layout := flowSideLayout{entries: entries, trunkPx: trunkPx}
	n := len(entries)
	if n == 0 || rows <= 0 {
		return layout
	}
	layout.slices = flowAllocate(entries, allocBudget)
	layout.heights = make([]int, n)
	total := 0
	for i, px := range layout.slices {
		height := (px + flowSub - 1) / flowSub
		if height < 1 {
			height = 1
		}
		layout.heights[i] = height
		total += height
	}
	// A dense side may need more rows than the diagram has. Drop rows from
	// the tallest slots until every label still gets its own row.
	for total > rows {
		best := -1
		for i := range layout.heights {
			if layout.heights[i] <= 1 {
				continue
			}
			if best < 0 || layout.heights[i] > layout.heights[best] {
				best = i
			}
		}
		if best < 0 {
			break
		}
		layout.heights[best]--
		total--
	}
	leftover := rows - total
	if leftover < 0 {
		leftover = 0
	}
	layout.gap = 0
	if n > 1 {
		layout.gap = leftover / (n - 1)
		if layout.gap > 4 {
			layout.gap = 4
		}
	}
	slack := leftover - layout.gap*(n-1)
	layout.stackTop = slack / 2
	layout.stackRows = total + layout.gap*(n-1)
	layout.tops = make([]int, n)
	layout.edges = make([]int, n)
	top := layout.stackTop
	for i := range entries {
		layout.tops[i] = top
		entries[i].row = top + (layout.heights[i]-1)/2
		edge := layout.slices[i]
		if limit := flowSub * layout.heights[i]; edge > limit {
			edge = limit
		}
		// Touching bands lose one pixel so a hairline of background keeps
		// them apart.
		if n > 1 && layout.gap == 0 && edge > 1 {
			edge--
		}
		if edge < 1 {
			edge = 1
		}
		layout.edges[i] = edge
		top += layout.heights[i] + layout.gap
	}
	return layout
}

// flowDiagramLayouts lays out both sides against one shared trunk pixel
// budget and the same diagram rows. The per-ribbon caps can leave one side
// shorter than the other even with a shared budget, so the taller side is
// re-allocated down to the shorter side's total. That way the two stacks
// meet at the trunk with equal heights, as a Sankey should.
func flowDiagramLayouts(in, out []flowEntry, rows int) (flowSideLayout, flowSideLayout) {
	trunkPx := flowTrunkPixels(len(in), len(out), rows)
	inLayout := flowLayoutSide(in, rows, trunkPx)
	outLayout := flowLayoutSide(out, rows, trunkPx)
	inSum := flowHeightsTotal(inLayout.slices)
	outSum := flowHeightsTotal(outLayout.slices)
	if inSum == outSum {
		return inLayout, outLayout
	}
	// The reduced budget never drops below the minimum every ribbon needs,
	// and it never exceeds the reported trunk budget.
	if inSum > outSum {
		budget := max(outSum, len(in)*flowMinRibbon)
		inLayout = flowLayoutSideWithBudget(in, rows, trunkPx, budget)
	} else {
		budget := max(inSum, len(out)*flowMinRibbon)
		outLayout = flowLayoutSideWithBudget(out, rows, trunkPx, budget)
	}
	return inLayout, outLayout
}

// Labels -----------------------------------------------------------------

type flowLabelLayout struct {
	markerCol int
	addrStart int
	addrWidth int
	amountCol int
}

func flowLabelLayoutFor(colW int, right bool) flowLabelLayout {
	layout := flowLabelLayout{}
	if right {
		// Mirrored: amount, marker, address. The leading blank cell keeps
		// a gutter between the canvas and the amount column.
		layout.amountCol = 1
		layout.markerCol = layout.amountCol + flowAmountWidth + 1
		layout.addrStart = layout.markerCol + 2
		layout.addrWidth = colW - layout.addrStart - 1
	} else {
		// Marker, address, then the right-aligned amount with one blank
		// cell before the canvas.
		layout.markerCol = 1
		layout.addrStart = 3
		layout.amountCol = colW - flowAmountWidth - 1
		layout.addrWidth = layout.amountCol - 1 - layout.addrStart
	}
	if layout.addrWidth < 0 {
		layout.addrWidth = 0
	}
	if layout.markerCol >= colW {
		layout.markerCol = colW - 1
	}
	if layout.markerCol < 0 {
		layout.markerCol = 0
	}
	return layout
}

func flowLabelAddressWidth(colW int, right bool) int {
	return flowLabelLayoutFor(colW, right).addrWidth
}

type flowTextCell struct {
	ch  string
	sgr string
}

type flowTextCells []flowTextCell

func newFlowTextCells(width int) flowTextCells {
	cells := make([]flowTextCell, width)
	for i := range cells {
		cells[i].ch = " "
	}
	return cells
}

func (c flowTextCells) put(col int, text, sgr string) {
	if col < 0 {
		return
	}
	pos := col
	for _, r := range text {
		if pos >= len(c) {
			return
		}
		c[pos] = flowTextCell{ch: string(r), sgr: sgr}
		pos++
	}
}

// putStyled writes an address rendered by ui.FormatAddress and keeps each
// group's color. Selected rows add bold without dropping the address hues.
func (c flowTextCells) putStyled(col int, formatted string, bold bool) {
	if col < 0 {
		return
	}
	pos := col
	for _, cell := range styledAddressCells(formatted) {
		style := cell.style
		if bold {
			style = "\x1b[1m" + style
		}
		for _, r := range cell.text {
			if pos >= len(c) {
				return
			}
			c[pos] = flowTextCell{ch: string(r), sgr: style}
			pos++
		}
	}
}

func flowRenderCells(cells flowTextCells) string {
	var b strings.Builder
	for i := 0; i < len(cells); {
		sgr := cells[i].sgr
		j := i + 1
		for j < len(cells) && cells[j].sgr == sgr {
			j++
		}
		if sgr != "" {
			b.WriteString(sgr)
		}
		for k := i; k < j; k++ {
			b.WriteString(cells[k].ch)
		}
		if sgr != "" {
			b.WriteString("\x1b[0m")
		}
		i = j
	}
	return b.String()
}

func flowLabelLine(v TxFlowView, side TxFlowSide, entries []flowEntry, row, colW int) string {
	cells := newFlowTextCells(colW)
	if colW <= 0 || len(entries) == 0 {
		return flowRenderCells(cells)
	}
	var entry *flowEntry
	for i := range entries {
		if entries[i].row == row {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return flowRenderCells(cells)
	}

	layout := flowLabelLayoutFor(colW, side == FlowOutput)
	selected := v.Side == side && entry.item >= 0 && entry.item == v.Cursor

	marker := "·"
	markerSGR := sgrPrefix(styles.DimText)
	var item TxFlowItem
	if entry.item >= 0 {
		items := v.itemsFor(side)
		if entry.item < len(items) {
			item = items[entry.item]
			marker = flowMarker(item.Kind)
			markerSGR = flowMarkerSGR(item.Kind, selected)
		}
	}
	cells.put(layout.markerCol, marker, markerSGR)

	if layout.addrWidth > 0 {
		switch {
		case entry.item >= 0 && item.Address != "":
			cells.putStyled(layout.addrStart, ui.FormatAddress(item.Address, layout.addrWidth), selected)
		case entry.item >= 0:
			name := flowDiagramName(item)
			sgr := flowNameSGR(item.Kind, selected)
			cells.put(layout.addrStart, fitFlowText(name, layout.addrWidth), sgr)
		default:
			text := fmt.Sprintf("+%d more", entry.more)
			if entry.up {
				text = fmt.Sprintf("%d more ↑", entry.more)
			}
			cells.put(layout.addrStart, fitFlowText(text, layout.addrWidth), sgrPrefix(styles.DimText))
		}
	}

	if entry.item >= 0 && layout.amountCol >= 0 && layout.amountCol+flowAmountWidth <= colW {
		amount := padFlowLeft(flowAmountText(item), flowAmountWidth)
		cells.put(layout.amountCol, amount, flowAmountSGR(item, selected))
	}
	return flowRenderCells(cells)
}

func flowDiagramName(item TxFlowItem) string {
	switch item.Kind {
	case KindFee:
		return "fee"
	case KindOpReturn:
		return "data"
	default:
		return "unknown"
	}
}

func flowMarker(kind TxFlowKind) string {
	switch kind {
	case KindOwnInput, KindReceive:
		return "●"
	case KindChange:
		return "◆"
	case KindFee:
		return "▪"
	case KindOpReturn:
		return "◇"
	default:
		return "○"
	}
}

func flowMarkerSGR(kind TxFlowKind, selected bool) string {
	return sgrPrefix(flowLabelStyle(kind, selected))
}

func flowNameSGR(kind TxFlowKind, selected bool) string {
	return sgrPrefix(flowLabelStyle(kind, selected))
}

func flowAmountSGR(item TxFlowItem, selected bool) string {
	style := lipgloss.NewStyle().Foreground(styles.TextSecondary)
	if selected {
		style = lipgloss.NewStyle().Foreground(styles.TxFlowAccent).Bold(true)
	}
	if !item.ValueKnown {
		style = lipgloss.NewStyle().Foreground(styles.TextMuted)
	}
	return sgrPrefix(style)
}

func flowLabelStyle(kind TxFlowKind, selected bool) lipgloss.Style {
	if selected {
		return lipgloss.NewStyle().Foreground(styles.TxFlowAccent).Bold(true)
	}
	switch kind {
	case KindChange:
		return lipgloss.NewStyle().Foreground(styles.TxFlowChange)
	case KindFee:
		return lipgloss.NewStyle().Foreground(styles.TxFlowFee)
	case KindOpReturn:
		return lipgloss.NewStyle().Foreground(styles.TextMuted)
	default:
		return lipgloss.NewStyle().Foreground(styles.TextSecondary)
	}
}

func flowOwnershipStyle(kind TxFlowKind) lipgloss.Style {
	switch kind {
	case KindOwnInput, KindReceive:
		return styles.SuccessText
	case KindChange:
		return lipgloss.NewStyle().Foreground(styles.TxFlowChange)
	case KindFee:
		return lipgloss.NewStyle().Foreground(styles.TxFlowFee).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(styles.TextMuted)
	}
}

// Canvas -----------------------------------------------------------------

type flowCanvasCell struct {
	ch     string
	sgr    string
	filled bool
}

type flowCanvas struct {
	w     int
	h     int
	cells []flowCanvasCell
}

func newFlowCanvas(w, h int) *flowCanvas {
	cells := make([]flowCanvasCell, w*h)
	for i := range cells {
		cells[i].ch = " "
	}
	return &flowCanvas{w: w, h: h, cells: cells}
}

func (c *flowCanvas) put(x, y int, ch, sgr string) {
	if x < 0 || x >= c.w || y < 0 || y >= c.h {
		return
	}
	c.cells[y*c.w+x] = flowCanvasCell{ch: ch, sgr: sgr, filled: true}
}

func (c *flowCanvas) free(x0, x1, y int) bool {
	if y < 0 || y >= c.h || x0 < 0 {
		return false
	}
	if x1 > c.w {
		x1 = c.w
	}
	for x := x0; x < x1; x++ {
		if c.cells[y*c.w+x].filled {
			return false
		}
	}
	return true
}

func (c *flowCanvas) placeText(x, y int, text, sgr string) {
	pos := x
	for _, r := range text {
		c.put(pos, y, string(r), sgr)
		pos++
	}
}

// flowCaptionCandidates lists the caption from fullest to shortest. A busy
// diagram may only have a narrow free run under the trunk.
func flowCaptionCandidates(v TxFlowView) []string {
	full := v.coreLabel()
	out := []string{full}
	if rate, _, ok := strings.Cut(full, " · "); ok {
		out = append(out, rate)
	}
	return out
}

// placeCaption puts the fee-rate caption on the first free row just below
// or above the trunk. A busy diagram with no free row omits the longer
// forms and finally the caption itself.
func (c *flowCanvas) placeCaption(candidates []string, trunkTop, trunkBottom int) {
	sgr := sgrPrefix(styles.DimText)
	for _, text := range candidates {
		text = fitFlowText(text, c.w)
		width := ansi.StringWidth(text)
		if width <= 0 {
			continue
		}
		x0 := (c.w - width) / 2
		for row := trunkBottom + 1; row < c.h; row++ {
			if c.free(x0, x0+width, row) {
				c.placeText(x0, row, text, sgr)
				return
			}
		}
		for row := trunkTop - 1; row >= 0; row-- {
			if c.free(x0, x0+width, row) {
				c.placeText(x0, row, text, sgr)
				return
			}
		}
	}
}

func (c *flowCanvas) lines() []string {
	out := make([]string, c.h)
	for y := 0; y < c.h; y++ {
		var b strings.Builder
		for x := 0; x < c.w; {
			sgr := c.cells[y*c.w+x].sgr
			j := x + 1
			for j < c.w && c.cells[y*c.w+j].sgr == sgr {
				j++
			}
			if sgr != "" {
				b.WriteString(sgr)
			}
			for k := x; k < j; k++ {
				b.WriteString(c.cells[y*c.w+k].ch)
			}
			if sgr != "" {
				b.WriteString("\x1b[0m")
			}
			x = j
		}
		out[y] = b.String()
	}
	return out
}

// flowBandPixel is one optional sub-pixel colour.
type flowBandPixel struct {
	col color.Color
	set bool
}

type flowPixelCanvas struct {
	w      int
	h      int
	pixels []flowBandPixel
}

func newFlowPixelCanvas(w, h int) *flowPixelCanvas {
	return &flowPixelCanvas{w: w, h: h, pixels: make([]flowBandPixel, w*h)}
}

func (c *flowPixelCanvas) set(x, y int, col color.Color) {
	if x < 0 || x >= c.w || y < 0 || y >= c.h {
		return
	}
	c.pixels[y*c.w+x] = flowBandPixel{
		col: col,
		set: true,
	}
}

func flowGlyphSGR(fg color.Color, hasFG bool, bg color.Color, hasBG bool) string {
	style := lipgloss.NewStyle()
	if hasFG {
		style = style.Foreground(fg)
	}
	if hasBG {
		style = style.Background(bg)
	}
	return sgrPrefix(style)
}

// flowEighthGlyph is the lower-eighth block for k filled eighths.
func flowEighthGlyph(k int) string {
	glyphs := []rune("▁▂▃▄▅▆▇")
	if k < 1 {
		k = 1
	}
	if k > len(glyphs) {
		k = len(glyphs)
	}
	return string(glyphs[k-1])
}

// flowHalfChoice is one candidate colour for half a text row; set false
// means the terminal background shows through.
type flowHalfChoice struct {
	col color.Color
	set bool
}

func flowChoiceDistinct(upper, lower flowHalfChoice) int {
	switch {
	case upper.set && lower.set && upper.col != lower.col:
		return 2
	case upper.set || lower.set:
		return 1
	default:
		return 0
	}
}

// flowSubCellStyle renders one text row's flowSub sub-pixels as the closest
// eighth-block glyph. Empty parts never get an explicit background: a
// coloured upper part with an empty lower part uses reverse video instead so
// the terminal background shows in the gap.
func flowSubCellStyle(upper color.Color, hasUpper bool, lower color.Color, hasLower bool, k int) (string, string) {
	switch {
	case !hasUpper && !hasLower:
		return " ", ""
	case !hasUpper:
		if k >= flowSub {
			return "█", flowGlyphSGR(lower, true, nil, false)
		}
		return flowEighthGlyph(k), flowGlyphSGR(lower, true, nil, false)
	case !hasLower:
		switch k {
		case 0:
			return "█", flowGlyphSGR(upper, true, nil, false)
		case flowSub / 2:
			return "▀", flowGlyphSGR(upper, true, nil, false)
		case flowSub - 1:
			return "▔", flowGlyphSGR(upper, true, nil, false)
		}
		style := lipgloss.NewStyle().Foreground(upper).Reverse(true)
		return flowEighthGlyph(k), sgrPrefix(style)
	case upper == lower:
		return "█", flowGlyphSGR(upper, true, nil, false)
	default:
		return flowEighthGlyph(k), flowGlyphSGR(lower, true, upper, true)
	}
}

// cells converts sub-pixels to text rows. Each text row takes its flowSub
// sub-pixels and picks the upper/lower colours and split that mismatch the
// fewest of them. A cell with any set sub-pixel always paints a glyph. Empty
// cells stay plain spaces so the terminal background shows through and no
// background SGR is emitted.
func (c *flowPixelCanvas) cells() *flowCanvas {
	rows := (c.h + flowSub - 1) / flowSub
	cells := newFlowCanvas(c.w, rows)
	subs := make([]flowBandPixel, flowSub)
	// flowHalfOption is one candidate cover for half a text row, with the
	// number of sub-pixels it would mismatch.
	type flowHalfOption struct {
		choice flowHalfChoice
		miss   int
	}
	// halfOptions lists the empty cover and every colour in subs[from:to).
	halfOptions := func(from, to int) []flowHalfOption {
		type count struct {
			col color.Color
			n   int
		}
		empty := 0
		counts := make([]count, 0, to-from)
		for i := from; i < to; i++ {
			if !subs[i].set {
				empty++
				continue
			}
			found := false
			for j := range counts {
				if counts[j].col == subs[i].col {
					counts[j].n++
					found = true
					break
				}
			}
			if !found {
				counts = append(counts, count{col: subs[i].col, n: 1})
			}
		}
		options := make([]flowHalfOption, 0, len(counts)+1)
		options = append(options, flowHalfOption{miss: (to - from) - empty})
		for _, c := range counts {
			options = append(options, flowHalfOption{
				choice: flowHalfChoice{col: c.col, set: true},
				miss:   (to - from) - c.n,
			})
		}
		return options
	}
	for row := 0; row < rows; row++ {
		for x := 0; x < c.w; x++ {
			filled := 0
			for k := 0; k < flowSub; k++ {
				y := row*flowSub + k
				if y < c.h {
					subs[k] = c.pixels[y*c.w+x]
				} else {
					subs[k] = flowBandPixel{}
				}
				if subs[k].set {
					filled++
				}
			}
			if filled == 0 {
				continue
			}
			same := true
			for k := 1; k < flowSub; k++ {
				if !subs[k].set || subs[k].col != subs[0].col {
					same = false
					break
				}
			}
			if same {
				cells.put(x, row, "█", flowGlyphSGR(subs[0].col, true, nil, false))
				continue
			}

			bestMismatch := flowSub + 1
			bestDistinct := 3
			bestK := 0
			var bestUpper, bestLower flowHalfChoice
			for k := 0; k <= flowSub; k++ {
				for _, upper := range halfOptions(0, flowSub-k) {
					for _, lower := range halfOptions(flowSub-k, flowSub) {
						// A cell that holds sub-pixels must never come
						// out blank: a thin ribbon crossing the middle of
						// the row has no half majority, so the empty
						// cover would win on mismatch. Require a painted
						// half before comparing mismatch and distinct.
						if !upper.choice.set && !lower.choice.set {
							continue
						}
						mismatch := upper.miss + lower.miss
						distinct := flowChoiceDistinct(upper.choice, lower.choice)
						if mismatch < bestMismatch ||
							(mismatch == bestMismatch && distinct < bestDistinct) {
							bestMismatch = mismatch
							bestDistinct = distinct
							bestK = k
							bestUpper, bestLower = upper.choice, lower.choice
						}
					}
				}
			}
			glyph, sgr := flowSubCellStyle(bestUpper.col, bestUpper.set, bestLower.col, bestLower.set, bestK)
			cells.put(x, row, glyph, sgr)
		}
	}
	return cells
}

// flowDrawBand fills one band between two x columns. Top and bottom edges
// are interpolated separately with the same easing, so neighbouring bands
// keep their order and never overlap on their way into the trunk.
func flowDrawBand(c *flowPixelCanvas, x0, x1 int, startCenter, startThick, endCenter, endThick float64, dashed bool, col color.Color) {
	if x1 < x0 {
		x0, x1 = x1, x0
		startCenter, endCenter = endCenter, startCenter
		startThick, endThick = endThick, startThick
	}
	startTop := startCenter - startThick/2
	startBottom := startCenter + startThick/2
	endTop := endCenter - endThick/2
	endBottom := endCenter + endThick/2

	span := x1 - x0
	edgeAt := func(pos float64) (top, bottom float64) {
		t := 0.0
		if span > 0 {
			t = (pos - float64(x0)) / float64(span)
		}
		t = math.Max(0, math.Min(1, t))
		eased := t * t * (3 - 2*t)
		top = startTop + (endTop-startTop)*eased
		bottom = startBottom + (endBottom-startBottom)*eased
		return top, bottom
	}
	// The union span keeps neighbouring columns touching on steep sections,
	// where a one-pixel band would otherwise break into separated dots.
	minTop := math.Min(startTop, endTop)
	maxBottom := math.Max(startBottom, endBottom)
	for x := x0; x <= x1; x++ {
		// Dashes need enough run to survive steep sections, where an
		// isolated column would read as a dot: draw three, skip one.
		if dashed && (x-x0)%4 == 3 {
			continue
		}
		leftTop, leftBottom := edgeAt(float64(x) - 0.5)
		rightTop, rightBottom := edgeAt(float64(x) + 0.5)
		top := math.Max(minTop, math.Min(leftTop, rightTop))
		bottom := math.Min(maxBottom, math.Max(leftBottom, rightBottom))
		first := int(math.Ceil(top - 0.5))
		last := int(math.Floor(bottom - 0.5))
		if last < first {
			last = first
		}
		for y := first; y <= last; y++ {
			c.set(x, y, col)
		}
	}
}

func flowBandColor(entry flowEntry, selected bool) color.Color {
	if selected {
		return styles.TxFlowAccent
	}
	if entry.aggregate() {
		return styles.TxFlowBandDim
	}
	switch entry.kind {
	case KindChange:
		return styles.TxFlowChange
	case KindFee:
		return styles.TxFlowFee
	case KindOwnInput, KindReceive:
		return styles.TxFlowBandOwn
	default:
		return styles.TxFlowBand
	}
}

// flowTrunkCols keeps the body compact so the ribbons get long, gentle
// curves. A narrow canvas still needs a visible two-column body.
func flowTrunkCols(canvasW int) int {
	switch {
	case canvasW < 28:
		return 2
	case canvasW < 40:
		return 3
	default:
		return 4
	}
}

// flowTrunkSlice is one band's contiguous slot inside the trunk body.
type flowTrunkSlice struct {
	top       int
	thickness int
}

// flowTrunkSlices stacks a side's ribbons inside the trunk, centered on
// the canvas mid-line. Slice thickness equals the ribbon's edge thickness
// so the ribbon keeps the same height from label to body.
func flowTrunkSlices(layout flowSideLayout, rows int) []flowTrunkSlice {
	slices := make([]flowTrunkSlice, len(layout.entries))
	total := 0
	for i := range layout.entries {
		slices[i].thickness = layout.edgeThickness(i)
		total += slices[i].thickness
	}
	top := rows*flowSub/2 - total/2
	if top < 0 {
		top = 0
	}
	for i := range slices {
		slices[i].top = top
		top += slices[i].thickness
	}
	return slices
}

func flowBandCanvasLines(v TxFlowView, in, out flowSideLayout, canvasW, rows int) []string {
	if canvasW <= 0 || rows <= 0 {
		return make([]string, max(0, rows))
	}
	trunkCols := flowTrunkCols(canvasW)
	trunkLeft := (canvasW - trunkCols) / 2
	if trunkLeft < 1 {
		trunkLeft = 1
	}
	trunkRight := trunkLeft + trunkCols
	if trunkRight > canvasW-1 {
		trunkRight = canvasW - 1
	}
	trunkMid := trunkLeft + (trunkRight-trunkLeft)/2
	if trunkMid <= trunkLeft {
		trunkMid = trunkLeft + 1
	}

	pixels := newFlowPixelCanvas(canvasW, rows*flowSub)
	inSlices := flowTrunkSlices(in, rows)
	outSlices := flowTrunkSlices(out, rows)

	drawSide := func(layout flowSideLayout, slices []flowTrunkSlice, side TxFlowSide, selectedPass bool) {
		for i, entry := range layout.entries {
			selected := v.Side == side && entry.item >= 0 && entry.item == v.Cursor
			if selected != selectedPass {
				continue
			}
			thickness := float64(layout.edgeThickness(i))
			trunkCenter := float64(slices[i].top) + thickness/2
			edgeCenter := layout.edgeCenter(i)
			col := flowBandColor(entry, selected)
			if side == FlowInput {
				flowDrawBand(pixels, 0, trunkLeft-1, edgeCenter, thickness, trunkCenter, thickness, entry.dashed(), col)
			} else {
				flowDrawBand(pixels, trunkRight, canvasW-1, trunkCenter, thickness, edgeCenter, thickness, entry.dashed(), col)
			}
		}
	}
	// Draw the selected path last so it stays crisp where bands meet.
	for pass := 0; pass < 2; pass++ {
		drawSide(in, inSlices, FlowInput, pass == 1)
		drawSide(out, outSlices, FlowOutput, pass == 1)
	}

	// The body is the two slice stacks meeting: inputs colour the left
	// half, outputs the right half. There is no separate trunk block.
	drawTrunkHalf := func(layout flowSideLayout, slices []flowTrunkSlice, x0, x1 int, side TxFlowSide) {
		for i, entry := range layout.entries {
			selected := v.Side == side && entry.item >= 0 && entry.item == v.Cursor
			col := flowBandColor(entry, selected)
			for x := x0; x < x1; x++ {
				for y := slices[i].top; y < slices[i].top+slices[i].thickness; y++ {
					pixels.set(x, y, col)
				}
			}
		}
	}
	drawTrunkHalf(in, inSlices, trunkLeft, trunkMid, FlowInput)
	drawTrunkHalf(out, outSlices, trunkMid, trunkRight, FlowOutput)

	cells := pixels.cells()
	topPx, bottomPx := rows*flowSub, -1
	for _, stack := range [][]flowTrunkSlice{inSlices, outSlices} {
		for _, slice := range stack {
			if slice.thickness <= 0 {
				continue
			}
			if slice.top < topPx {
				topPx = slice.top
			}
			if bottom := slice.top + slice.thickness - 1; bottom > bottomPx {
				bottomPx = bottom
			}
		}
	}
	if bottomPx < topPx {
		// No ribbons: keep the caption on the middle rows.
		topPx, bottomPx = rows*flowSub/2, rows*flowSub/2-1
	}
	cells.placeCaption(flowCaptionCandidates(v), topPx/flowSub, bottomPx/flowSub)
	return cells.lines()
}

// Diagram ----------------------------------------------------------------

func (v TxFlowView) diagramLines(width, height int) []string {
	if height <= 0 {
		return nil
	}
	in, out := v.flowEntries(FlowInput, height), v.flowEntries(FlowOutput, height)
	if len(in) == 0 && len(out) == 0 {
		return nil
	}
	inLayout, outLayout := flowDiagramLayouts(in, out, height)
	leftW, canvasW, rightW := flowColumnWidths(width)
	canvas := flowBandCanvasLines(v, inLayout, outLayout, canvasW, height)
	lines := make([]string, height)
	for row := 0; row < height; row++ {
		canvasRow := ""
		if row < len(canvas) {
			canvasRow = canvas[row]
		}
		line := flowLabelLine(v, FlowInput, in, row, leftW) +
			padFlowRight(canvasRow, canvasW) +
			flowLabelLine(v, FlowOutput, out, row, rightW)
		lines[row] = strings.TrimRight(line, " ")
	}
	return lines
}

func (v TxFlowView) coreLabel() string {
	rate := v.Flow.FeeRate
	if rate <= 0 && v.Flow.FeeKnown && v.Flow.VSize > 0 && v.Flow.Fee > 0 {
		rate = float64(v.Flow.Fee) / float64(v.Flow.VSize)
	}
	text := "tx"
	if rate > 0 {
		if math.Abs(rate-math.Round(rate)) < 0.05 {
			text = fmt.Sprintf("%.0f s/vB", rate)
		} else {
			text = fmt.Sprintf("%.1f s/vB", rate)
		}
	}
	if v.Flow.VSize > 0 {
		if rate > 0 {
			text += fmt.Sprintf(" · %d vB", v.Flow.VSize)
		} else {
			text = fmt.Sprintf("%d vB", v.Flow.VSize)
		}
	}
	return text
}

func (v TxFlowView) detailLines(width int) []string {
	item, ok := v.selectedItem()
	if !ok {
		side := "inputs"
		if v.Side == FlowOutput {
			side = "outputs"
		}
		return []string{
			flowDetailRow("Address  ", "no "+side+" selected", width),
			flowDetailRow("Amount   ", "unknown", width),
			styles.DimText.Render(fitFlowText("—", width)),
		}
	}

	address := item.Address
	hasAddress := address != ""
	switch {
	case hasAddress:
	case item.Kind == KindOpReturn:
		address = "OP_RETURN"
	case item.Kind == KindFee:
		address = "fee"
	default:
		address = "(no address)"
	}

	amount := "unknown"
	if item.ValueKnown {
		amount = formatFlowBTC(item.Value) + " · " + formatFlowSats(item.Value) + " sats"
	}

	label := item.Label
	if label == "" {
		label = "no label"
	}
	third := flowOwnership(item.Kind) + " · " + label
	if v.Side == FlowInput {
		if outPoint := flowInputOutPoint(item, v.Cursor); outPoint != "" {
			third += " · " + outPoint
		}
	}

	addressRow := flowDetailRow("Address  ", address, width)
	if hasAddress {
		addressRow = flowDetailAddressRow("Address  ", address, width)
	}
	return []string{
		addressRow,
		flowDetailRow("Amount   ", amount, width),
		v.ownershipLine(item.Kind, third, width),
	}
}

// flowInputOutPoint renders the spent outpoint. History transactions can
// omit the previous txid; then the input position is the best identifier.
func flowInputOutPoint(item TxFlowItem, index int) string {
	if item.OutPoint == "" {
		return ""
	}
	if txid, _, _ := strings.Cut(item.OutPoint, ":"); txid == "" {
		return fmt.Sprintf("input %d", index)
	}
	return item.OutPoint
}

func flowDetailRow(label, value string, width int) string {
	labelW := ansi.StringWidth(label)
	if labelW >= width {
		return styles.DimText.Render(fitFlowText(label, width))
	}
	value = fitFlowText(value, width-labelW)
	return styles.DimText.Render(label) + styles.NormalText.Render(value)
}

// flowDetailAddressRow keeps the detail line exactly as wide as the plain
// address made it, so opening the visualizer cannot shift the panel.
func flowDetailAddressRow(label, address string, width int) string {
	labelW := ansi.StringWidth(label)
	if labelW >= width {
		return styles.DimText.Render(fitFlowText(label, width))
	}
	valueWidth := width - labelW
	formatWidth := ansi.StringWidth(address)
	if formatWidth > valueWidth {
		formatWidth = valueWidth
	}
	value := ui.FormatAddress(address, formatWidth)
	return styles.DimText.Render(label) + value
}

func (v TxFlowView) ownershipLine(kind TxFlowKind, text string, width int) string {
	ownership := flowOwnership(kind)
	ownW := ansi.StringWidth(ownership)
	if ownW >= width {
		return styles.DimText.Render(fitFlowText(text, width))
	}
	suffix := strings.TrimPrefix(text, ownership)
	suffix = fitFlowText(suffix, width-ownW)
	return flowOwnershipStyle(kind).Render(ownership) +
		styles.DimText.Render(suffix)
}

func (v TxFlowView) hintLine(width int) string {
	return styles.DimText.Render(fitFlowText("↑/↓ select • tab side • esc back", width))
}

func (v TxFlowView) selectedItem() (TxFlowItem, bool) {
	items := v.itemsFor(v.Side)
	if len(items) == 0 || v.Cursor < 0 || v.Cursor >= len(items) {
		return TxFlowItem{}, false
	}
	return items[v.Cursor], true
}

func flowOwnership(kind TxFlowKind) string {
	switch kind {
	case KindOwnInput, KindReceive:
		return "yours"
	case KindChange:
		return "change"
	case KindFee:
		return "fee"
	default:
		return "external"
	}
}

func formatFlowCoin(v api.Satoshi) string {
	neg := v < 0
	if neg {
		v = -v
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%08d", sign, int64(v)/100_000_000, int64(v)%100_000_000)
}

func formatFlowBTC(v api.Satoshi) string {
	return formatFlowCoin(v) + " BTC"
}

func formatFlowSats(v api.Satoshi) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := strconv.FormatInt(int64(v), 10)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func fitFlowText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func padFlowRight(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func padFlowLeft(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}
