package models

import (
	"fmt"
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

// flowColumnWidths splits the terminal into two item columns and a central
// band. At 80 columns that is 32/16/32.
func flowColumnWidths(width int) (left, band, right int) {
	if width <= 0 {
		return 0, 0, 0
	}
	band = width / 5
	if band < 16 {
		band = 16
	}
	if band > width {
		band = width
	}
	left = (width - band) / 2
	right = width - band - left
	return left, band, right
}

// flowBusColumns keeps both buses inside the band with room for the core
// label between them.
func flowBusColumns(leftW, band int) (busL, busR int) {
	if band <= 0 {
		return leftW, leftW
	}
	busL = leftW + 3
	busR = leftW + band - 4
	if busR-busL < 2 {
		busL = leftW + band/2 - 1
		busR = leftW + band/2
		if busL < leftW {
			busL = leftW
		}
		if busR >= leftW+band {
			busR = leftW + band - 1
		}
	}
	return busL, busR
}

func (v TxFlowView) headerLine(width int) string {
	leftW, bandW, rightW := flowColumnWidths(width)
	inTotal, _ := flowTotal(v.Flow.Inputs)
	outTotal, _ := flowTotal(v.Flow.Outputs)
	left := fitFlowText(flowHeaderText(" Inputs", v.Flow.Inputs, inTotal, leftW), leftW)
	left = padFlowRight(left, leftW)
	right := fitFlowText(flowHeaderText("Outputs", v.Flow.Outputs, outTotal, rightW), rightW)
	right = padFlowLeft(right, rightW)
	return styles.HighlightText.Render(left) +
		strings.Repeat(" ", bandW) +
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

// flowSideTotal is the value used for connector weighting. The fee is not
// part of the output total because the header reports it separately.
func flowSideTotal(items []TxFlowItem) api.Satoshi {
	var total, fee api.Satoshi
	for _, item := range items {
		if !item.ValueKnown {
			continue
		}
		if item.Kind == KindFee {
			fee += item.Value
			continue
		}
		total += item.Value
	}
	if total == 0 {
		return fee
	}
	return total
}

func (v TxFlowView) diagramLines(width, height int) []string {
	if height <= 0 {
		return nil
	}
	inputs := v.Flow.Inputs
	outputs := v.itemsFor(FlowOutput)
	n := len(inputs)
	if len(outputs) > n {
		n = len(outputs)
	}
	if n == 0 {
		return nil
	}

	spaced := 2*n-1 <= height
	used := n
	if spaced {
		used = 2*n - 1
	}
	if used > height {
		used = height
	}
	center := used / 2

	leftW, bandW, rightW := flowColumnWidths(width)
	busL, busR := flowBusColumns(leftW, bandW)
	grid := newFlowGrid(width, used)

	inLayout := v.layoutSide(FlowInput, inputs, used, center, spaced)
	outLayout := v.layoutSide(FlowOutput, outputs, used, center, spaced)

	inLayout.draw(grid, center, FlowInput, leftW, bandW, rightW, busL, busR, width, v)
	outLayout.draw(grid, center, FlowOutput, leftW, bandW, rightW, busL, busR, width, v)
	v.drawCore(grid, center, busL, busR)

	return grid.lines()
}

func (v TxFlowView) layoutSide(side TxFlowSide, items []TxFlowItem, used, center int, spaced bool) flowSideLayout {
	layout := flowSideLayout{
		items: items,
		rows:  make([]flowSideRow, used),
		total: flowSideTotal(items),
	}
	for i := range layout.rows {
		layout.rows[i].item = -1
	}
	m := len(items)
	if m == 0 || used <= 0 {
		return layout
	}

	switch {
	case m == 1:
		row := center
		if row < 0 {
			row = 0
		}
		if row >= used {
			row = used - 1
		}
		layout.rows[row].item = 0
	case m <= used && spaced:
		start := center - (m - 1)
		if start < 0 {
			start = 0
		}
		if start+2*(m-1) >= used {
			start = used - 1 - 2*(m-1)
		}
		for i := 0; i < m; i++ {
			row := start + 2*i
			if row >= 0 && row < used {
				layout.rows[row].item = i
			}
		}
	case m <= used:
		start := (used - m) / 2
		for i := 0; i < m; i++ {
			row := start + i
			if row >= 0 && row < used {
				layout.rows[row].item = i
			}
		}
	default:
		start, count, above, below := flowWindow(m, v.effectiveOffset(side), used)
		itemStart := 0
		if above > 0 {
			layout.rows[0] = flowSideRow{item: -1, more: above, up: true}
			itemStart = 1
		}
		for i := 0; i < count; i++ {
			row := itemStart + i
			if row >= 0 && row < used {
				layout.rows[row].item = start + i
			}
		}
		if below > 0 {
			layout.rows[used-1] = flowSideRow{item: -1, more: below}
		}
	}

	first, last := -1, -1
	for r, row := range layout.rows {
		if row.item >= 0 || row.more > 0 {
			if first < 0 {
				first = r
			}
			last = r
		}
	}
	if first < 0 {
		return layout
	}
	if center < first {
		first = center
	}
	if center > last {
		last = center
	}
	layout.hasBus = true
	layout.busTop = first
	layout.busBottom = last
	return layout
}

type flowSideRow struct {
	item int
	more int
	up   bool
}

type flowSideLayout struct {
	items     []TxFlowItem
	rows      []flowSideRow
	total     api.Satoshi
	busTop    int
	busBottom int
	hasBus    bool
}

func (l flowSideLayout) draw(grid *flowGrid, center int, side TxFlowSide, leftW, bandW, rightW, busL, busR, width int, v TxFlowView) {
	if !l.hasBus {
		return
	}
	isLeft := side == FlowInput
	busCol := busR
	rightStart := leftW + bandW
	if isLeft {
		busCol = busL
	}
	connStart := flowLeftConnectorStart(leftW)
	connEnd := flowRightConnectorEnd(rightStart, rightW)

	for row := l.busTop; row <= l.busBottom; row++ {
		var sideRow flowSideRow
		if row >= 0 && row < len(l.rows) {
			sideRow = l.rows[row]
		}
		hasContent := sideRow.item >= 0 || sideRow.more > 0
		selected := sideRow.item >= 0 && v.Side == side && v.Cursor == sideRow.item

		if hasContent {
			if isLeft {
				if sideRow.item >= 0 {
					item := l.items[sideRow.item]
					grid.text(row, 0, flowLeftItemText(item, leftW), flowCellKindFor(item.Kind), selected)
				} else {
					grid.text(row, 0, flowMoreCellText(sideRow.more, sideRow.up, leftW, true), flowCellDim, false)
				}
			} else if sideRow.item >= 0 {
				item := l.items[sideRow.item]
				grid.text(row, rightStart, flowRightItemText(item, rightW), flowCellKindFor(item.Kind), selected)
			} else {
				grid.text(row, rightStart, flowMoreCellText(sideRow.more, sideRow.up, rightW, false), flowCellDim, false)
			}

			weight := flowThin
			connKind := flowCellConnector
			if sideRow.item >= 0 {
				weight = flowWeightFor(l.items[sideRow.item], l.total)
			}
			if sideRow.more > 0 {
				connKind = flowCellDim
			}
			if selected {
				connKind = flowCellAccent
			}
			if isLeft {
				for col := connStart; col < busCol; col++ {
					grid.set(row, col, flowHorizontal(weight), connKind, false)
				}
			} else {
				for col := busCol + 1; col <= connEnd; col++ {
					grid.set(row, col, flowHorizontal(weight), connKind, false)
				}
			}
		}

		glyph, jointKind := l.joint(row, center, side, hasContent, selected)
		grid.set(row, busCol, glyph, jointKind, false)
	}
}

func (l flowSideLayout) joint(row, center int, side TxFlowSide, hasContent, selected bool) (rune, flowCellKind) {
	isLeft := side == FlowInput
	kind := flowCellBus
	if hasContent && selected {
		kind = flowCellAccent
	}
	var left, right bool
	if hasContent {
		if isLeft {
			left = true
		} else {
			right = true
		}
	}
	if row == center {
		// The core sits inward of the bus on the center row.
		if isLeft {
			right = true
		} else {
			left = true
		}
	}
	return flowJointRune(left, right, row > l.busTop, row < l.busBottom), kind
}

// flowJointRune picks the box-drawing glyph whose arms match exactly the
// connections present at a bus cell.
func flowJointRune(left, right, up, down bool) rune {
	switch {
	case left && right && up && down:
		return '╋'
	case left && up && down:
		return '┫'
	case right && up && down:
		return '┣'
	case left && right && down:
		return '┳'
	case left && right && up:
		return '┻'
	case left && down:
		return '┓'
	case left && up:
		return '┛'
	case right && down:
		return '┏'
	case right && up:
		return '┗'
	case left || right:
		return '━'
	case up || down:
		return '┃'
	default:
		return ' '
	}
}

func (v TxFlowView) drawCore(grid *flowGrid, center, busL, busR int) {
	if center < 0 || center >= grid.height {
		return
	}
	gap := busR - busL - 1
	if gap <= 0 {
		return
	}
	label := fitFlowText(v.coreLabel(), gap)
	pad := (gap - ansi.StringWidth(label)) / 2
	grid.text(center, busL+1+pad, label, flowCellDim, false)
}

func (v TxFlowView) coreLabel() string {
	rate := v.Flow.FeeRate
	if rate <= 0 && v.Flow.FeeKnown && v.Flow.VSize > 0 && v.Flow.Fee > 0 {
		rate = float64(v.Flow.Fee) / float64(v.Flow.VSize)
	}
	if rate <= 0 {
		return "tx"
	}
	if math.Abs(rate-math.Round(rate)) < 0.05 {
		return fmt.Sprintf("%.0f s/vB", rate)
	}
	return fmt.Sprintf("%.1f s/vB", rate)
}

func flowAmountReserve(colW int) int {
	if colW >= 28 {
		return 4
	}
	return 2
}

// flowAmountWidth is the cell width of a BTC amount. Shorter text such as
// "unknown" is right-aligned inside it so every row spends the same number
// of cells on the amount and the address column keeps a stable width.
const flowAmountWidth = len("0.00000000")

// flowRightAmountReserve adds the cell that the left column spends on its
// leading blank, keeping both amount columns equally far from the band.
func flowRightAmountReserve(colW int) int {
	return flowAmountReserve(colW) + 1
}

func flowLeftConnectorStart(leftW int) int {
	start := leftW - flowAmountReserve(leftW) + 2
	if start < 1 {
		start = 1
	}
	return start
}

func flowRightConnectorEnd(rightStart, rightW int) int {
	return rightStart + flowRightAmountReserve(rightW) - 3
}

func flowLeftItemText(item TxFlowItem, colW int) string {
	reserve := flowAmountReserve(colW)
	amount := padFlowLeft(flowAmountText(item), flowAmountWidth)
	amountW := ansi.StringWidth(amount)
	amountStart := colW - reserve - amountW
	if amountStart < 1 {
		amountStart = 1
	}
	marker := flowMarker(item.Kind)
	markerW := ansi.StringWidth(marker)
	addrW := amountStart - 2 - (markerW + 2)
	if addrW < 1 {
		addrW = 1
	}
	prefix := " " + marker + " " + flowAddressText(item, addrW)
	prefixW := ansi.StringWidth(prefix)
	if prefixW > amountStart {
		prefix = ansi.Truncate(prefix, amountStart, "")
		prefixW = ansi.StringWidth(prefix)
	}
	text := prefix + strings.Repeat(" ", max(0, amountStart-prefixW)) + amount
	return fitFlowText(text, colW)
}

func flowRightItemText(item TxFlowItem, colW int) string {
	reserve := flowRightAmountReserve(colW)
	amount := padFlowLeft(flowAmountText(item), flowAmountWidth)
	amountW := ansi.StringWidth(amount)
	marker := flowMarker(item.Kind)
	markerW := ansi.StringWidth(marker)
	addrStart := reserve + amountW + 2 + markerW + 1
	addrW := colW - addrStart
	if addrW < 1 {
		addrW = 1
	}
	text := strings.Repeat(" ", reserve) + amount + "  " + marker + " " +
		flowAddressText(item, addrW)
	return fitFlowText(text, colW)
}

func flowMoreCellText(more int, up bool, colW int, left bool) string {
	text := fmt.Sprintf("+%d more", more)
	if up {
		text = fmt.Sprintf("%d more ↑", more)
	}
	reserve := flowAmountReserve(colW)
	if !left {
		reserve = flowRightAmountReserve(colW)
	}
	area := colW - reserve
	if area < 1 {
		area = 1
	}
	text = fitFlowText(text, area)
	if left {
		return padFlowLeft(text, area)
	}
	return strings.Repeat(" ", reserve) + padFlowRight(text, area)
}

func flowAmountText(item TxFlowItem) string {
	if !item.ValueKnown {
		return "unknown"
	}
	return formatFlowCoin(item.Value)
}

func flowItemName(item TxFlowItem) string {
	if item.Address != "" {
		return item.Address
	}
	switch item.Kind {
	case KindOpReturn:
		return "OP_RETURN"
	case KindFee:
		return "fee"
	default:
		return "(no address)"
	}
}

// flowAddressText renders a row's address in the shared format. Labels that
// stand in for an address keep their plain shortened form.
func flowAddressText(item TxFlowItem, width int) string {
	if item.Address == "" {
		return shortFlowAddress(flowItemName(item), width)
	}
	return ui.FormatAddress(item.Address, width)
}

func flowMarker(kind TxFlowKind) string {
	switch kind {
	case KindOwnInput, KindReceive:
		return "●"
	case KindChange:
		return "◆"
	case KindFee:
		return "⛁"
	case KindOpReturn:
		return "∅"
	default:
		return "○"
	}
}

type flowWeight int

const (
	flowThin flowWeight = iota
	flowMedium
	flowThick
	flowDotted
)

func flowWeightFor(item TxFlowItem, total api.Satoshi) flowWeight {
	if !item.ValueKnown {
		return flowDotted
	}
	if total <= 0 || item.Value <= 0 {
		return flowThin
	}
	share := float64(item.Value) / float64(total)
	switch {
	case share > 0.5:
		return flowThick
	case share >= 0.15:
		return flowMedium
	default:
		return flowThin
	}
}

func flowHorizontal(weight flowWeight) rune {
	switch weight {
	case flowMedium:
		return '━'
	case flowThick:
		return '█'
	case flowDotted:
		return '┄'
	default:
		return '─'
	}
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
	return flowCellStyle(flowCellKindFor(kind), false).Render(ownership) +
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

func shortFlowAddress(addr string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(addr) <= width {
		return addr
	}
	if width == 1 {
		return "…"
	}
	keep := width - 1
	left := (keep + 1) / 2
	right := keep - left
	r := []rune(addr)
	if left > len(r) {
		left = len(r)
	}
	if right > len(r)-left {
		right = len(r) - left
	}
	return string(r[:left]) + "…" + string(r[len(r)-right:])
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

type flowCellKind int

const (
	flowCellBlank flowCellKind = iota
	flowCellYours
	flowCellChange
	flowCellExternal
	flowCellFee
	flowCellOpReturn
	flowCellDim
	flowCellConnector
	flowCellBus
	flowCellAccent
)

func flowCellKindFor(kind TxFlowKind) flowCellKind {
	switch kind {
	case KindOwnInput, KindReceive:
		return flowCellYours
	case KindChange:
		return flowCellChange
	case KindFee:
		return flowCellFee
	case KindOpReturn:
		return flowCellOpReturn
	default:
		return flowCellExternal
	}
}

func flowCellStyle(kind flowCellKind, selected bool) lipgloss.Style {
	var style lipgloss.Style
	switch kind {
	case flowCellYours:
		style = lipgloss.NewStyle().Foreground(styles.SuccessGreen).Bold(true)
	case flowCellChange:
		style = lipgloss.NewStyle().Foreground(styles.InfoBlue)
	case flowCellFee:
		style = lipgloss.NewStyle().Foreground(styles.Amber).Bold(true)
	case flowCellOpReturn:
		style = lipgloss.NewStyle().Foreground(styles.TextMuted)
	case flowCellDim:
		style = lipgloss.NewStyle().Foreground(styles.TextMuted)
	case flowCellConnector:
		style = lipgloss.NewStyle().Foreground(styles.TextSecondary)
	case flowCellBus:
		style = lipgloss.NewStyle().Foreground(styles.TextMuted)
	case flowCellAccent:
		style = lipgloss.NewStyle().Foreground(styles.BitcoinOrange).Bold(true)
	default:
		style = lipgloss.NewStyle()
	}
	if selected && kind >= flowCellYours && kind <= flowCellOpReturn {
		style = style.Reverse(true)
	}
	return style
}

type flowCell struct {
	r        rune
	kind     flowCellKind
	selected bool
	// sgr is set for cells that carry their own style, such as a formatted
	// address. It replaces the cell kind's style so the address keeps its
	// pastel groups even on the selected row.
	sgr string
}

type flowGrid struct {
	width  int
	height int
	cells  [][]flowCell
}

func newFlowGrid(width, height int) *flowGrid {
	cells := make([][]flowCell, height)
	for r := range cells {
		cells[r] = make([]flowCell, width)
		for c := range cells[r] {
			cells[r][c] = flowCell{r: ' ', kind: flowCellBlank}
		}
	}
	return &flowGrid{width: width, height: height, cells: cells}
}

func (g *flowGrid) set(row, col int, r rune, kind flowCellKind, selected bool) {
	if row < 0 || row >= g.height || col < 0 || col >= g.width {
		return
	}
	g.cells[row][col] = flowCell{r: r, kind: kind, selected: selected}
}

func (g *flowGrid) text(row, col int, text string, kind flowCellKind, selected bool) {
	if col < 0 {
		return
	}
	if strings.ContainsRune(text, '\x1b') {
		if row < 0 || row >= g.height {
			return
		}
		pos := col
		for _, cell := range styledAddressCells(text) {
			for _, r := range cell.text {
				if pos >= g.width {
					return
				}
				g.cells[row][pos] = flowCell{r: r, kind: kind, selected: selected, sgr: cell.style}
				pos++
			}
		}
		return
	}
	pos := col
	for _, r := range text {
		if pos >= g.width {
			return
		}
		g.set(row, pos, r, kind, selected)
		pos++
	}
}

func (g *flowGrid) lines() []string {
	out := make([]string, g.height)
	for row := 0; row < g.height; row++ {
		last := g.width - 1
		for last >= 0 && g.cells[row][last].r == ' ' {
			last--
		}
		if last < 0 {
			out[row] = ""
			continue
		}
		var b strings.Builder
		for col := 0; col <= last; {
			kind := g.cells[row][col].kind
			selected := g.cells[row][col].selected
			sgr := g.cells[row][col].sgr
			start := col
			for col <= last && g.cells[row][col].kind == kind &&
				g.cells[row][col].selected == selected && g.cells[row][col].sgr == sgr {
				col++
			}
			var seg strings.Builder
			for i := start; i < col; i++ {
				seg.WriteRune(g.cells[row][i].r)
			}
			if sgr != "" {
				b.WriteString(sgr)
				b.WriteString(seg.String())
				b.WriteString("\x1b[0m")
				continue
			}
			b.WriteString(flowCellStyle(kind, selected).Render(seg.String()))
		}
		out[row] = b.String()
	}
	return out
}
