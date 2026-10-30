package models

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
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

// View renders the flow at a stable line count for every cursor position.
func (v TxFlowView) View() string {
	width := v.Width
	if width <= 0 {
		width = 80
	}
	rows := v.diagramRows()

	leftW, bandW, rightW := flowColumnWidths(width)
	inputs, outputs := v.Flow.Inputs, v.itemsFor(FlowOutput)

	inStart, inCount, inMore := flowWindow(len(inputs), v.effectiveOffset(FlowInput), rows)
	outStart, outCount, outMore := flowWindow(len(outputs), v.effectiveOffset(FlowOutput), rows)

	var lines []string
	lines = append(lines, flowHeader(v.Flow, leftW, bandW, rightW))

	inTotal, _ := flowTotal(inputs)
	outTotal, _ := flowTotal(outputs)
	for r := 0; r < rows; r++ {
		var inItem, outItem *TxFlowItem
		if r < inCount {
			item := inputs[inStart+r]
			inItem = &item
		}
		if r < outCount {
			item := outputs[outStart+r]
			outItem = &item
		}

		inCell := strings.Repeat(" ", leftW)
		switch {
		case inItem != nil:
			inCell = renderFlowItem(*inItem, leftW, v.Side == FlowInput && v.Cursor == inStart+r)
		case inMore > 0 && r == inCount:
			inCell = renderMoreCell(inMore, leftW)
		}
		outCell := strings.Repeat(" ", rightW)
		switch {
		case outItem != nil:
			outCell = renderFlowItem(*outItem, rightW, v.Side == FlowOutput && v.Cursor == outStart+r)
		case outMore > 0 && r == outCount:
			outCell = renderMoreCell(outMore, rightW)
		}

		band := renderFlowBand(bandW, inItem, outItem,
			r == 0, r == inCount-1, r == 0, r == outCount-1,
			inTotal, outTotal)
		lines = append(lines, inCell+band+outCell)
	}

	lines = append(lines, v.detailLines(width)...)
	lines = append(lines, styles.FooterStyle.Render(
		fitFlowText("↑/↓ select • tab side • esc back", width)))

	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (v TxFlowView) diagramRows() int {
	rows := v.Height
	if rows <= 0 {
		rows = 24
	}
	rows -= 5 // header, three detail rows, key hint
	if rows < 1 {
		rows = 1
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
		Label:      "Network fee",
		Value:      v.Flow.Fee,
		ValueKnown: v.Flow.FeeKnown,
		Kind:       KindFee,
	})
}

func (v TxFlowView) effectiveOffset(side TxFlowSide) int {
	n := len(v.itemsFor(side))
	rows := v.diagramRows()
	if n <= flowRowsFor(rows) {
		return 0
	}
	slots := flowSlots(n, rows)
	off := v.Offset[side.index()]
	if off < 0 {
		off = 0
	}
	maxOff := n - slots
	if off > maxOff {
		off = maxOff
	}
	if v.Side == side {
		c := v.Cursor
		if c < 0 {
			c = 0
		}
		if c >= n {
			c = n - 1
		}
		if c < off {
			off = c
		}
		if c >= off+slots {
			off = c - slots + 1
		}
		if off > maxOff {
			off = maxOff
		}
	}
	if off < 0 {
		off = 0
	}
	return off
}

func (v *TxFlowView) ensureOffset() {
	n := len(v.itemsFor(v.Side))
	rows := v.diagramRows()
	side := v.Side.index()
	if n == 0 {
		v.Cursor = 0
		v.Offset[side] = 0
		return
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.Cursor >= n {
		v.Cursor = n - 1
	}
	if n <= flowRowsFor(rows) {
		v.Offset[side] = 0
		return
	}
	slots := flowSlots(n, rows)
	off := v.Offset[side]
	if off < 0 {
		off = 0
	}
	if off > n-slots {
		off = n - slots
	}
	if v.Cursor < off {
		off = v.Cursor
	}
	if v.Cursor >= off+slots {
		off = v.Cursor - slots + 1
	}
	if off < 0 {
		off = 0
	}
	if off > n-slots {
		off = n - slots
	}
	v.Offset[side] = off
}

func (s TxFlowSide) index() int {
	if s == FlowOutput {
		return 1
	}
	return 0
}

func flowRowsFor(rows int) int {
	if rows < 1 {
		return 1
	}
	return rows
}

// flowSlots is the number of real item rows when a "+N more" row may be
// appended. With a one-row diagram the item takes the row alone.
func flowSlots(n, rows int) int {
	rows = flowRowsFor(rows)
	if n <= rows {
		return n
	}
	if rows == 1 {
		return 1
	}
	return rows - 1
}

func flowWindow(n, offset, rows int) (start, count, more int) {
	rows = flowRowsFor(rows)
	if n == 0 {
		return 0, 0, 0
	}
	if n <= rows {
		return 0, n, 0
	}
	if rows == 1 {
		if offset < 0 {
			offset = 0
		}
		if offset > n-1 {
			offset = n - 1
		}
		return offset, 1, 0
	}
	slots := rows - 1
	if offset < 0 {
		offset = 0
	}
	if offset > n-slots {
		offset = n - slots
	}
	return offset, slots, n - (offset + slots)
}

// flowColumnWidths keeps each address column near 3/8 of the terminal and
// gives the remaining quarter to the flow band. At 80 columns that is
// 30/20/30; wider terminals widen both the columns and the band.
func flowColumnWidths(width int) (left, band, right int) {
	if width <= 0 {
		return 0, 0, 0
	}
	if width < 12 {
		left = width / 2
		right = width - left
		return left, 0, right
	}
	left = width * 3 / 8
	right = width * 3 / 8
	band = width - left - right
	return left, band, right
}

func flowHeader(flow TxFlow, leftW, bandW, rightW int) string {
	in, _ := flowTotal(flow.Inputs)
	out, _ := flowTotal(flow.Outputs)
	left := padFlowRight(fitFlowText("Inputs "+flowHeaderTotal(flow.Inputs, in), leftW), leftW)
	right := padFlowLeft(fitFlowText("Outputs "+flowHeaderTotal(flow.Outputs, out), rightW), rightW)
	return styles.HighlightText.Render(left) +
		strings.Repeat(" ", bandW) +
		styles.HighlightText.Render(right)
}

func flowHeaderTotal(items []TxFlowItem, total api.Satoshi) string {
	if len(items) == 0 {
		return "0.0000 BTC"
	}
	_, unknown := flowTotal(items)
	text := formatFlowTotal(total)
	if unknown > 0 {
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

func renderFlowItem(item TxFlowItem, width int, selected bool) string {
	if width <= 0 {
		return ""
	}
	marker := flowMarker(item.Kind)
	amount := "unknown"
	if item.ValueKnown {
		amount = formatFlowBTC(item.Value)
	}
	markerW := ansi.StringWidth(marker)
	amountW := ansi.StringWidth(amount)
	// Reserve one cell between the shortened address and the amount.
	addrW := width - markerW - 1 - amountW - 1
	if addrW < 1 {
		addrW = 1
	}
	addr := shortFlowAddress(flowItemName(item), addrW)
	text := marker + " " + addr
	gap := width - ansi.StringWidth(text) - amountW
	if gap < 1 {
		gap = 1
	}
	text += strings.Repeat(" ", gap) + amount
	text = fitFlowText(text, width)
	text = padFlowRight(text, width)

	style := flowItemStyle(item.Kind)
	if selected {
		style = style.Reverse(true)
	}
	return style.Render(text)
}

func renderMoreCell(more, width int) string {
	if width <= 0 {
		return ""
	}
	text := fitFlowText(fmt.Sprintf("+%d more", more), width)
	text = padFlowRight(text, width)
	return styles.DimText.Render(text)
}

func flowItemName(item TxFlowItem) string {
	if item.Address != "" {
		return item.Address
	}
	switch item.Kind {
	case KindOpReturn:
		return "OP_RETURN"
	case KindFee:
		return "Network fee"
	default:
		return "(no address)"
	}
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

func flowItemStyle(kind TxFlowKind) lipgloss.Style {
	switch kind {
	case KindOwnInput, KindReceive:
		return lipgloss.NewStyle().Foreground(styles.SuccessGreen).Bold(true)
	case KindChange:
		return lipgloss.NewStyle().Foreground(styles.PastelPurple)
	case KindFee:
		return lipgloss.NewStyle().Foreground(styles.Amber).Bold(true)
	case KindOpReturn:
		return lipgloss.NewStyle().Foreground(styles.TextMuted)
	default:
		return lipgloss.NewStyle().Foreground(styles.TextSecondary)
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
			styles.BoldText.Render(fitFlowText("No "+side+" selected", width)),
			styles.NormalText.Render(fitFlowText("Select an item to see its details", width)),
			styles.DimText.Render(fitFlowText("—", width)),
		}
	}

	address := item.Address
	switch {
	case address != "":
		// Keep the whole address; the detail row truncates only when the
		// terminal itself is narrower than the address.
	case item.Kind == KindOpReturn:
		address = "OP_RETURN  (no address)"
	case item.Kind == KindFee:
		address = "Network fee"
	default:
		address = "(no address)"
	}

	amount := "amount unknown"
	if item.ValueKnown {
		amount = formatFlowBTC(item.Value) + " · " + formatFlowSats(item.Value) + " sats"
	}
	label := item.Label
	if label == "" {
		label = "no label"
	}
	details := amount + " · " + label

	ownership := flowOwnership(item.Kind)
	if v.Side == FlowInput && item.OutPoint != "" {
		details = amount + " · " + label
		ownership += " · " + item.OutPoint
	}

	return []string{
		styles.BoldText.Render(fitFlowText(address, width)),
		styles.NormalText.Render(fitFlowText(details, width)),
		styles.DimText.Render(fitFlowText(ownership, width)),
	}
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

func renderFlowBand(width int, inItem, outItem *TxFlowItem, inTop, inBottom, outTop, outBottom bool, inTotal, outTotal api.Satoshi) string {
	if width <= 0 {
		return ""
	}
	cells := make([]rune, width)
	for i := range cells {
		cells[i] = ' '
	}
	center := width / 2
	cells[center] = '│'

	if inItem != nil {
		weight := flowWeightFor(*inItem, inTotal)
		for i := 0; i < center; i++ {
			cells[i] = flowHorizontal(weight)
		}
		cells[0] = flowLeftEnd(weight, inTop, inBottom)
	}
	if outItem != nil {
		weight := flowWeightFor(*outItem, outTotal)
		for i := center + 1; i < width; i++ {
			cells[i] = flowHorizontal(weight)
		}
		cells[width-1] = flowRightEnd(weight, outTop, outBottom)
	}

	switch {
	case inItem != nil && outItem != nil:
		cells[center] = flowJunction(flowWeightFor(*inItem, inTotal), flowWeightFor(*outItem, outTotal))
	case inItem != nil:
		cells[center] = flowJoinLeft(flowWeightFor(*inItem, inTotal))
	case outItem != nil:
		cells[center] = flowJoinRight(flowWeightFor(*outItem, outTotal))
	}
	return string(cells)
}

type flowWeight int

const (
	flowThin flowWeight = iota
	flowMedium
	flowThick
)

func flowWeightFor(item TxFlowItem, total api.Satoshi) flowWeight {
	if total <= 0 || !item.ValueKnown || item.Value <= 0 {
		return flowThin
	}
	share := float64(item.Value) / float64(total)
	switch {
	case share >= 0.35:
		return flowThick
	case share >= 0.12:
		return flowMedium
	default:
		return flowThin
	}
}

func flowHorizontal(w flowWeight) rune {
	switch w {
	case flowMedium:
		return '━'
	case flowThick:
		return '█'
	default:
		return '─'
	}
}

func flowLeftEnd(w flowWeight, top, bottom bool) rune {
	if w == flowThick {
		switch {
		case top:
			return '█'
		case bottom:
			return '█'
		default:
			return '█'
		}
	}
	if w == flowMedium {
		switch {
		case top:
			return '┏'
		case bottom:
			return '┗'
		default:
			return '┃'
		}
	}
	switch {
	case top:
		return '╭'
	case bottom:
		return '╰'
	default:
		return '│'
	}
}

func flowRightEnd(w flowWeight, top, bottom bool) rune {
	if w == flowThick {
		return '█'
	}
	if w == flowMedium {
		switch {
		case top:
			return '┓'
		case bottom:
			return '┛'
		default:
			return '┃'
		}
	}
	switch {
	case top:
		return '╮'
	case bottom:
		return '╯'
	default:
		return '│'
	}
}

func flowJunction(a, b flowWeight) rune {
	w := a
	if b > w {
		w = b
	}
	switch w {
	case flowThick:
		return '█'
	case flowMedium:
		return '╋'
	default:
		return '┼'
	}
}

func flowJoinLeft(w flowWeight) rune {
	switch w {
	case flowThick:
		return '█'
	case flowMedium:
		return '┫'
	default:
		return '┤'
	}
}

func flowJoinRight(w flowWeight) rune {
	switch w {
	case flowThick:
		return '█'
	case flowMedium:
		return '┣'
	default:
		return '├'
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

func formatFlowBTC(v api.Satoshi) string {
	neg := v < 0
	if neg {
		v = -v
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%08d BTC", sign, int64(v)/100_000_000, int64(v)%100_000_000)
}

func formatFlowTotal(v api.Satoshi) string {
	neg := v < 0
	if neg {
		v = -v
	}
	frac := fmt.Sprintf("%08d", int64(v)%100_000_000)
	frac = strings.TrimRight(frac, "0")
	if len(frac) < 4 {
		frac += strings.Repeat("0", 4-len(frac))
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%s BTC", sign, int64(v)/100_000_000, frac)
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
