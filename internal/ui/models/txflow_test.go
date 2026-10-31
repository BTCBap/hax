package models

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/wallet"
)

func txFlowAddr(i int) string {
	return fmt.Sprintf("bc1q%056d", i)
}

func txFlowFixture(inputs, outputs int) TxFlow {
	flow := TxFlow{
		TxID:     strings.Repeat("ab", 32),
		Fee:      1_500,
		FeeKnown: true,
		FeeRate:  10,
		VSize:    150,
	}
	for i := 0; i < inputs; i++ {
		kind := KindForeignInput
		mine := false
		if i == 0 {
			kind = KindOwnInput
			mine = true
		}
		flow.Inputs = append(flow.Inputs, TxFlowItem{
			Address:    txFlowAddr(i),
			Label:      fmt.Sprintf("input %d", i),
			OutPoint:   fmt.Sprintf("%s:%d", strings.Repeat("cd", 32), i),
			Value:      api.Satoshi(40_000 + i*1_000),
			ValueKnown: i != 1,
			Mine:       mine,
			Kind:       kind,
		})
	}
	for i := 0; i < outputs; i++ {
		kind := KindExternal
		mine := false
		switch i {
		case 0:
			kind, mine = KindReceive, true
		case 1:
			kind, mine = KindChange, true
		}
		flow.Outputs = append(flow.Outputs, TxFlowItem{
			Address:    txFlowAddr(100 + i),
			Label:      fmt.Sprintf("output %d", i),
			Value:      api.Satoshi(20_000 + i*500),
			ValueKnown: true,
			Mine:       mine,
			Kind:       kind,
		})
	}
	return flow
}

// txFlowExample mirrors the 2-input, 2-output-plus-fee diagram used by the
// design mockup.
func txFlowExample() TxFlow {
	addr := func(prefix, suffix string) string {
		return prefix + strings.Repeat("0", 40) + suffix
	}
	return TxFlow{
		TxID:     "example",
		Fee:      141,
		FeeKnown: true,
		FeeRate:  1,
		VSize:    141,
		Inputs: []TxFlowItem{
			{
				Address:    addr("tb1qrp", "0sl5k7"),
				OutPoint:   strings.Repeat("aa", 32) + ":0",
				Value:      700_000,
				ValueKnown: true,
				Mine:       true,
				Kind:       KindOwnInput,
			},
			{
				Address:    addr("tb1qw5", "xpjzsx"),
				OutPoint:   strings.Repeat("bb", 32) + ":1",
				Value:      50_000,
				ValueKnown: true,
				Mine:       true,
				Kind:       KindOwnInput,
			},
		},
		Outputs: []TxFlowItem{
			{
				Address:    addr("tb1q9v", "vgkx0l"),
				Value:      100_000,
				ValueKnown: true,
				Kind:       KindExternal,
			},
			{
				Address:    addr("tb1qqq", "rxh6hy"),
				Value:      649_859,
				ValueKnown: true,
				Mine:       true,
				Kind:       KindChange,
			},
		},
	}
}

func TestTxFlowFromInspection(t *testing.T) {
	insp := &wallet.PSBTInspection{
		Inputs: []wallet.InspectedInput{
			{OutPoint: "aa:0", Value: 100_000, ValueKnown: true, Address: "bc1qown", Mine: true},
			{OutPoint: "bb:1", Address: "bc1qforeign"},
		},
		Outputs: []wallet.InspectedOutput{
			{Address: "bc1qrecv", Value: 40_000, Owner: wallet.OutputReceive},
			{Address: "bc1qchange", Value: 50_000, Owner: wallet.OutputChange},
			{Address: "bc1qexternal", Value: 9_000, Owner: wallet.OutputExternal},
			{Address: "", Value: 0, Owner: wallet.OutputExternal},
		},
		Fee: 1_000, FeeKnown: true, FeeRate: 5, VSize: 200,
	}
	labels := func(addr string) string {
		return "label:" + addr
	}
	flow := TxFlowFromInspection(insp, labels)

	if len(flow.Inputs) != 2 || len(flow.Outputs) != 4 {
		t.Fatalf("flow has %d inputs and %d outputs", len(flow.Inputs), len(flow.Outputs))
	}
	if flow.Inputs[0].Kind != KindOwnInput || !flow.Inputs[0].Mine {
		t.Errorf("own input = %+v", flow.Inputs[0])
	}
	if flow.Inputs[1].Kind != KindForeignInput || flow.Inputs[1].Mine {
		t.Errorf("foreign input = %+v", flow.Inputs[1])
	}
	if flow.Inputs[1].ValueKnown {
		t.Error("unknown input value was reported as known")
	}
	if flow.Inputs[0].OutPoint != "aa:0" {
		t.Errorf("outpoint = %q", flow.Inputs[0].OutPoint)
	}
	if flow.Outputs[0].Kind != KindReceive || !flow.Outputs[0].Mine {
		t.Errorf("receive output = %+v", flow.Outputs[0])
	}
	if flow.Outputs[1].Kind != KindChange || !flow.Outputs[1].Mine {
		t.Errorf("change output = %+v", flow.Outputs[1])
	}
	if flow.Outputs[2].Kind != KindExternal || flow.Outputs[2].Mine {
		t.Errorf("external output = %+v", flow.Outputs[2])
	}
	if flow.Outputs[3].Kind != KindOpReturn {
		t.Errorf("OP_RETURN output = %+v", flow.Outputs[3])
	}
	if flow.Outputs[0].Label != "label:bc1qrecv" {
		t.Errorf("label = %q", flow.Outputs[0].Label)
	}
	if flow.Fee != 1_000 || !flow.FeeKnown || flow.VSize != 200 || flow.FeeRate != 5 {
		t.Errorf("fee data = %+v", flow)
	}
}

func TestTxFlowFromTransaction(t *testing.T) {
	tx := api.Transaction{
		TxID:   "txid",
		Fee:    500,
		Size:   250,
		Weight: 800,
		Vin: []api.Vin{
			{TxID: "prev1", Vout: 0, Prevout: &api.Vout{
				ScriptpubkeyAddress: "bc1qown", Value: 100_000}},
			{TxID: "prev2", Vout: 3},
		},
		Vout: []api.Vout{
			{ScriptpubkeyAddress: "bc1qrecv", Value: 60_000},
			{ScriptpubkeyAddress: "bc1qchange", Value: 39_000},
			{ScriptpubkeyAddress: "", Value: 0},
			{ScriptpubkeyAddress: "bc1qexternal", Value: 500},
		},
	}
	mine := map[string]bool{"bc1qown": true, "bc1qrecv": true, "bc1qchange": true}
	change := map[string]bool{"bc1qchange": true}
	labels := map[string]string{"bc1qown": "cold coin", "bc1qrecv": "salary"}
	flow := TxFlowFromTransaction(tx,
		func(addr string) bool { return mine[addr] },
		func(addr string) bool { return change[addr] },
		func(addr string) string { return labels[addr] })

	if flow.TxID != "txid" || flow.Fee != 500 || !flow.FeeKnown {
		t.Fatalf("flow header = %+v", flow)
	}
	if flow.VSize != 200 || flow.FeeRate != 2.5 {
		t.Errorf("vsize/feerate = %d/%v, want 200/2.5", flow.VSize, flow.FeeRate)
	}
	if flow.Inputs[0].Kind != KindOwnInput || flow.Inputs[0].OutPoint != "prev1:0" ||
		flow.Inputs[0].Label != "cold coin" {
		t.Errorf("own input = %+v", flow.Inputs[0])
	}
	if flow.Inputs[1].Kind != KindForeignInput || flow.Inputs[1].ValueKnown {
		t.Errorf("foreign input = %+v", flow.Inputs[1])
	}
	wantKinds := []TxFlowKind{KindReceive, KindChange, KindOpReturn, KindExternal}
	for i, want := range wantKinds {
		if flow.Outputs[i].Kind != want {
			t.Errorf("output %d kind = %v, want %v", i, flow.Outputs[i].Kind, want)
		}
	}
	if flow.Outputs[0].Label != "salary" {
		t.Errorf("receive label = %q", flow.Outputs[0].Label)
	}
}

func TestTxFlow_RenderingFitsAndStaysStable(t *testing.T) {
	cases := []struct {
		name    string
		inputs  int
		outputs int
	}{
		{name: "1-in-2-out", inputs: 1, outputs: 2},
		{name: "3-in-2-out", inputs: 3, outputs: 2},
		{name: "25-in-12-out", inputs: 25, outputs: 12},
	}
	for _, tc := range cases {
		for _, width := range []int{60, 80, 120, 200} {
			for _, height := range []int{12, 24, 40} {
				t.Run(fmt.Sprintf("%s/%dx%d", tc.name, width, height), func(t *testing.T) {
					v := TxFlowView{Flow: txFlowFixture(tc.inputs, tc.outputs), Width: width, Height: height}
					for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
						v.SwitchSide(side)
						n := len(v.itemsFor(side))
						for i := 0; i < n; i++ {
							view := v.View()
							lines := strings.Split(ansi.Strip(view), "\n")
							if len(lines) != height {
								t.Fatalf("%s cursor %d: %d lines, want %d", sideName(side), i, len(lines), height)
							}
							for lineNo, line := range lines {
								if w := ansi.StringWidth(line); w > width {
									t.Errorf("%s cursor %d line %d: %d cells, max %d: %q",
										sideName(side), i, lineNo, w, width, line)
								}
							}
							item := v.itemsFor(side)[i]
							if item.Address != "" {
								available := width - ansi.StringWidth("Address  ")
								formatWidth := min(ansi.StringWidth(item.Address), available)
								wantText := ansi.Strip(ui.FormatAddress(item.Address, formatWidth))
								if !strings.Contains(ansi.Strip(view), wantText) {
									t.Errorf("%s cursor %d: detail does not show %q", sideName(side), i, wantText)
								}
								if ansi.StringWidth(item.Address) > available &&
									!strings.Contains(ansi.Strip(view), "…") {
									t.Errorf("%s cursor %d: long address is not truncated", sideName(side), i)
								}
							}
							v.MoveDown()
						}
					}
				})
			}
		}
	}
}

func sideName(side TxFlowSide) string {
	if side == FlowInput {
		return "inputs"
	}
	return "outputs"
}

func TestTxFlow_ManyItemsScrollWithMoreRow(t *testing.T) {
	v := TxFlowView{Flow: txFlowFixture(25, 12), Width: 80, Height: 24}
	plain := ansi.Strip(v.View())
	if !strings.Contains(plain, "+9 more") {
		t.Fatalf("first view does not show the hidden input count:\n%s", plain)
	}
	wantLines := len(strings.Split(plain, "\n"))
	for i := 0; i < 24; i++ {
		v.MoveDown()
	}
	if v.Offset[FlowInput] == 0 {
		t.Fatal("cursor stayed at the top after moving through 25 inputs")
	}
	plain = ansi.Strip(v.View())
	wantLast := ansi.Strip(ui.FormatAddress(txFlowAddr(24), ansi.StringWidth(txFlowAddr(24))))
	if !strings.Contains(plain, wantLast) {
		t.Fatalf("last input is not visible after scrolling:\n%s", plain)
	}
	if strings.Contains(plain, "+9 more") {
		t.Fatalf("bottom more row still shown after scrolling to the end:\n%s", plain)
	}
	if !strings.Contains(plain, "9 more ↑") {
		t.Fatalf("top more indicator is missing after scrolling:\n%s", plain)
	}
	if got := len(strings.Split(plain, "\n")); got != wantLines {
		t.Fatalf("line count changed from %d to %d while scrolling", wantLines, got)
	}

	v.MoveUp()
	if got := len(strings.Split(ansi.Strip(v.View()), "\n")); got != wantLines {
		t.Fatalf("line count changed after moving up: %d, want %d", got, wantLines)
	}
}

func TestTxFlow_NoDanglingBusInPadding(t *testing.T) {
	v := TxFlowView{Flow: txFlowExample(), Width: 80, Height: 24}
	lines := strings.Split(ansi.Strip(v.View()), "\n")
	const diagramStart = 2 // header plus blank row
	const usedRows = 5     // three items with spacer rows
	for i := diagramStart + usedRows; i < len(lines); i++ {
		for _, glyph := range flowBusGlyphs {
			if strings.ContainsRune(lines[i], glyph) {
				t.Fatalf("dangling bus glyph %q on line %d: %q", glyph, i, lines[i])
			}
		}
	}
}

var flowBusGlyphs = []rune{'━', '┃', '┏', '┓', '┗', '┛', '┣', '┫', '┳', '┻', '╋'}

func TestTxFlow_CentersShorterSideAndBusConnections(t *testing.T) {
	const width, height = 80, 24
	leftW, bandW, _ := flowColumnWidths(width)
	busL, busR := flowBusColumns(leftW, bandW)

	// 2 inputs and 3 outputs (including fee): the left bus connects only on
	// diagram rows 1..3, centered around the core row 2.
	v := TxFlowView{Flow: txFlowFixture(2, 2), Width: width, Height: height}
	lines := strings.Split(ansi.Strip(v.View()), "\n")
	diagramStart, diagramEnd := flowTestDiagram(lines)
	if diagramEnd-diagramStart != 5 {
		t.Fatalf("2-in/3-out diagram has %d rows, want 5", diagramEnd-diagramStart)
	}
	wantLeft := []rune{' ', '┓', '┣', '┛', ' '}
	wantRight := []rune{'┏', '┃', '╋', '┃', '┗'}
	for row, want := range wantLeft {
		if got := flowTestRuneAt(lines[diagramStart+row], busL); got != want {
			t.Errorf("left bus row %d = %q, want %q", row, got, want)
		}
	}
	for row, want := range wantRight {
		if got := flowTestRuneAt(lines[diagramStart+row], busR); got != want {
			t.Errorf("right bus row %d = %q, want %q", row, got, want)
		}
	}

	shapes := []struct {
		name    string
		inputs  int
		outputs int
	}{
		{name: "1-in-1-out", inputs: 1, outputs: 0},
		{name: "1-in-3-out", inputs: 1, outputs: 2},
		{name: "3-in-1-out", inputs: 3, outputs: 0},
		{name: "2-in-3-out", inputs: 2, outputs: 2},
		{name: "5-in-2-out", inputs: 5, outputs: 1},
		{name: "overflow-25-in-12-out", inputs: 25, outputs: 12},
	}
	for _, shape := range shapes {
		for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
			t.Run(fmt.Sprintf("%s/%s", shape.name, sideName(side)), func(t *testing.T) {
				v := TxFlowView{Flow: txFlowFixture(shape.inputs, shape.outputs), Width: width, Height: height}
				v.SwitchSide(side)
				lines := strings.Split(ansi.Strip(v.View()), "\n")
				if len(lines) != height {
					t.Fatalf("%d lines, want %d", len(lines), height)
				}
				start, end := flowTestDiagram(lines)
				flowTestBusArms(t, lines, start, end, busL, true)
				flowTestBusArms(t, lines, start, end, busR, false)
			})
		}
	}
}

// flowTestDiagram locates the diagram block: it starts after the header and
// blank row, and ends at the blank row before the detail panel.
func flowTestDiagram(lines []string) (start, end int) {
	for i, line := range lines {
		if strings.HasPrefix(line, "Address") {
			return 2, i - 1
		}
	}
	return 2, 2
}

func flowTestRuneAt(line string, col int) rune {
	runes := []rune(line)
	if col < 0 || col >= len(runes) {
		return ' '
	}
	return runes[col]
}

type flowTestArms struct {
	left, right, up, down bool
}

var flowTestBusArmsByRune = map[rune]flowTestArms{
	'━': {left: true, right: true},
	'┃': {up: true, down: true},
	'┏': {right: true, down: true},
	'┓': {left: true, down: true},
	'┗': {right: true, up: true},
	'┛': {left: true, up: true},
	'┣': {right: true, up: true, down: true},
	'┫': {left: true, up: true, down: true},
	'┳': {left: true, right: true, down: true},
	'┻': {left: true, right: true, up: true},
	'╋': {left: true, right: true, up: true, down: true},
}

// flowTestBusArms verifies that a bus has no gaps, stops at its first and
// last connecting rows, and that every arm meets a matching arm in the
// neighbouring cell. outwardLeft selects the side whose connector leaves
// toward the left column.
func flowTestBusArms(t *testing.T, lines []string, start, end, col int, outwardLeft bool) {
	t.Helper()
	rows := make([]int, 0, end-start)
	for row := start; row < end; row++ {
		if _, ok := flowTestBusArmsByRune[flowTestRuneAt(lines[row], col)]; ok {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		t.Fatalf("no bus glyphs in column %d", col)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i] != rows[i-1]+1 {
			t.Fatalf("bus column %d has a gap between rows %d and %d", col, rows[i-1], rows[i])
		}
	}

	for _, row := range rows {
		arms := flowTestBusArmsByRune[flowTestRuneAt(lines[row], col)]
		if arms.up {
			if row == start {
				t.Fatalf("bus column %d row %d has an up arm above the diagram", col, row)
			}
			above, ok := flowTestBusArmsByRune[flowTestRuneAt(lines[row-1], col)]
			if !ok || !above.down {
				t.Fatalf("bus column %d row %d has an up arm but the cell above (row %d) has no down arm", col, row, row-1)
			}
		}
		if arms.down {
			if row == end-1 {
				t.Fatalf("bus column %d row %d has a down arm below the diagram", col, row)
			}
			below, ok := flowTestBusArmsByRune[flowTestRuneAt(lines[row+1], col)]
			if !ok || !below.up {
				t.Fatalf("bus column %d row %d has a down arm but the cell below (row %d) has no up arm", col, row, row+1)
			}
		}
		if outwardLeft && arms.left && flowTestRuneAt(lines[row], col-1) == ' ' {
			t.Fatalf("bus column %d row %d has a left arm with no connector", col, row)
		}
		if !outwardLeft && arms.right && flowTestRuneAt(lines[row], col+1) == ' ' {
			t.Fatalf("bus column %d row %d has a right arm with no connector", col, row)
		}
	}

	// No bus glyph may appear above or below the contiguous span.
	for row := start; row < rows[0]; row++ {
		if _, ok := flowTestBusArmsByRune[flowTestRuneAt(lines[row], col)]; ok {
			t.Fatalf("bus column %d has a dangling glyph on row %d", col, row)
		}
	}
	for row := rows[len(rows)-1] + 1; row < end; row++ {
		if _, ok := flowTestBusArmsByRune[flowTestRuneAt(lines[row], col)]; ok {
			t.Fatalf("bus column %d has a dangling glyph on row %d", col, row)
		}
	}
}

func TestTxFlow_CenterLabelAndJoints(t *testing.T) {
	v := TxFlowView{Flow: txFlowExample(), Width: 80, Height: 24}
	plain := ansi.Strip(v.View())
	lines := strings.Split(plain, "\n")
	centerLine := 2 + (2*3-1)/2
	if !strings.Contains(lines[centerLine], "1 s/vB") {
		t.Fatalf("center row does not contain the fee rate: %q", lines[centerLine])
	}
	for _, glyph := range []string{"┓", "┛", "┏", "┗", "╋", "█"} {
		if !strings.Contains(plain, glyph) {
			t.Errorf("view is missing %q:\n%s", glyph, plain)
		}
	}
	for _, marker := range []string{"●", "◆", "○", "⛁"} {
		if !strings.Contains(plain, marker) {
			t.Errorf("view is missing marker %q:\n%s", marker, plain)
		}
	}
	if !strings.Contains(lines[2], "…") {
		t.Errorf("addresses are not truncated in the middle: %q", lines[2])
	}
}

func TestTxFlow_ConnectorWeightTiers(t *testing.T) {
	flow := TxFlow{
		Fee: 100_000, FeeKnown: true, FeeRate: 2, VSize: 50_000,
		Inputs: []TxFlowItem{
			{Address: "bc1qunknown", Kind: KindForeignInput},
			{Address: "bc1qlarge", Value: 600_000, ValueKnown: true, Kind: KindOwnInput},
			{Address: "bc1qmedium", Value: 300_000, ValueKnown: true, Kind: KindOwnInput},
			{Address: "bc1qsmall", Value: 100_000, ValueKnown: true, Kind: KindOwnInput},
		},
		Outputs: []TxFlowItem{
			{Address: "bc1qdest", Value: 900_000, ValueKnown: true, Kind: KindExternal},
		},
	}
	v := TxFlowView{Flow: flow, Width: 80, Height: 24}
	lines := strings.Split(ansi.Strip(v.View()), "\n")
	// Items sit on diagram rows 0, 2, 4, and 6.
	if !strings.ContainsRune(lines[2], '┄') {
		t.Errorf("unknown value does not use a dotted connector: %q", lines[2])
	}
	if !strings.ContainsRune(lines[4], '█') {
		t.Errorf(">50%% item does not use a thick connector: %q", lines[4])
	}
	if !strings.ContainsRune(lines[6], '━') {
		t.Errorf("15-50%% item does not use a heavy connector: %q", lines[6])
	}
	if !strings.ContainsRune(lines[8], '─') {
		t.Errorf("<15%% item does not use a light connector: %q", lines[8])
	}
	if !strings.Contains(lines[5], "2 s/vB") {
		t.Errorf("center row does not contain the fee rate: %q", lines[5])
	}
}

func TestTxFlow_80(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.TrueColor) })

	v := TxFlowView{Flow: txFlowExample(), Width: 80, Height: 24}
	got := v.View()

	const path = "testdata/TestTxFlow_80.golden"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run UPDATE_GOLDEN=1 go test -run TestTxFlow_80): %v", err)
	}
	// Git may check the golden out with CRLF on Windows; compare against LF.
	wantText := strings.ReplaceAll(string(want), "\r\n", "\n")
	if wantText == got {
		return
	}
	wantLines := strings.Split(wantText, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var wantLine, gotLine string
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine != gotLine {
			t.Fatalf("golden mismatch at line %d:\nwant %q\ngot  %q", i, wantLine, gotLine)
		}
	}
}

func TestTxFlow_UnknownValueAndOpReturn(t *testing.T) {
	flow := TxFlow{
		Inputs: []TxFlowItem{{
			Address: "bc1qunknown", Kind: KindForeignInput, OutPoint: "aa:1",
		}},
		Outputs: []TxFlowItem{
			{Address: "bc1qdata", Kind: KindReceive, Value: 1_000, ValueKnown: true},
			{Address: "", Kind: KindOpReturn, Value: 0, ValueKnown: true},
		},
		Fee: 500, FeeKnown: true,
	}
	v := TxFlowView{Flow: flow, Width: 80, Height: 24, Side: FlowInput}
	plain := ansi.Strip(v.View())
	if !strings.Contains(plain, "unknown") {
		t.Fatalf("unknown input value is not marked:\n%s", plain)
	}
	if !strings.Contains(plain, "∅") {
		t.Fatalf("OP_RETURN marker is missing:\n%s", plain)
	}
	v.SwitchSide(FlowOutput)
	v.MoveDown()
	plain = ansi.Strip(v.View())
	if !strings.Contains(plain, "OP_RETURN") || !strings.Contains(plain, "external") {
		t.Fatalf("OP_RETURN detail is missing:\n%s", plain)
	}
}

func TestTxFlow_UnknownValueKeepsAmountColumnAligned(t *testing.T) {
	known := api.Satoshi(42_000)
	flow := TxFlow{
		Fee: 500, FeeKnown: true, FeeRate: 1, VSize: 500,
		Inputs: []TxFlowItem{
			{
				Address:    "bc1qknown0000000000000000000000000000000000",
				Value:      known,
				ValueKnown: true,
				Kind:       KindOwnInput,
			},
			{
				Address:  "bc1qunknown00000000000000000000000000000000",
				OutPoint: "aa:1",
				Kind:     KindForeignInput,
			},
		},
		Outputs: []TxFlowItem{
			{Address: "bc1qdest00000000000000000000000000000000000", Value: 41_500, ValueKnown: true, Kind: KindExternal},
		},
	}
	v := TxFlowView{Flow: flow, Width: 80, Height: 24, Side: FlowInput}
	lines := strings.Split(ansi.Strip(v.View()), "\n")
	start, end := flowTestDiagram(lines)

	entries := []struct {
		prefix string
		amount string
	}{
		{prefix: " ● ", amount: formatFlowCoin(known)},
		{prefix: " ○ ", amount: "unknown"},
	}
	var ends []int
	for _, entry := range entries {
		found := false
		for _, line := range lines[start:end] {
			if !strings.HasPrefix(line, entry.prefix) {
				continue
			}
			idx := strings.Index(line, entry.amount)
			if idx < 0 {
				continue
			}
			// The amount column is right-aligned, so its right edge is
			// fixed even when "unknown" is shorter.
			ends = append(ends, ansi.StringWidth(line[:idx])+ansi.StringWidth(entry.amount))
			found = true
			break
		}
		if !found {
			t.Fatalf("input row with %q amount is missing:\n%s", entry.amount, strings.Join(lines, "\n"))
		}
	}
	if ends[0] != ends[1] {
		t.Fatalf("amount column ends at offsets %v, want one fixed offset:\n%s",
			ends, strings.Join(lines, "\n"))
	}
}

func TestTxFlow_EmptyOutPointTxIDShowsInputIndex(t *testing.T) {
	tx := api.Transaction{
		TxID: "history",
		Fee:  500,
		Size: 200,
		Vin: []api.Vin{
			{Vout: 0, Prevout: &api.Vout{ScriptpubkeyAddress: "bc1qspent", Value: 10_000}},
			{TxID: "abcd", Vout: 1, Prevout: &api.Vout{ScriptpubkeyAddress: "bc1qother", Value: 20_000}},
		},
		Vout: []api.Vout{{ScriptpubkeyAddress: "bc1qdest", Value: 29_500}},
	}
	flow := TxFlowFromTransaction(tx, nil, nil, nil)
	if flow.Inputs[0].OutPoint != ":0" {
		t.Fatalf("empty-txid outpoint = %q, want %q", flow.Inputs[0].OutPoint, ":0")
	}

	v := TxFlowView{Flow: flow, Width: 80, Height: 24, Side: FlowInput}
	plain := ansi.Strip(v.View())
	if !strings.Contains(plain, "input 0") {
		t.Fatalf("detail does not name the empty-txid input:\n%s", plain)
	}
	if strings.Contains(plain, ":0") {
		t.Fatalf("detail still shows the bare vout:\n%s", plain)
	}

	v.MoveDown()
	plain = ansi.Strip(v.View())
	if !strings.Contains(plain, "abcd:1") {
		t.Fatalf("detail does not show the known outpoint:\n%s", plain)
	}
	if strings.Contains(plain, "input 1") {
		t.Fatalf("detail replaced a known outpoint with the input index:\n%s", plain)
	}
}

func TestTxFlow_MarkersAndWordsWithoutColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.TrueColor) })

	flow := txFlowFixture(3, 3)
	v := TxFlowView{Flow: flow, Width: 120, Height: 40, Side: FlowInput}

	var seen strings.Builder
	for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
		v.SwitchSide(side)
		for i := 0; i < len(v.itemsFor(side)); i++ {
			seen.WriteString(ansi.Strip(v.View()))
			seen.WriteByte('\n')
			v.MoveDown()
		}
	}
	plain := seen.String()
	for _, want := range []string{"●", "◆", "○", "⛁", "yours", "change", "external", "fee"} {
		if !strings.Contains(plain, want) {
			t.Errorf("no-color view is missing %q:\n%s", want, plain)
		}
	}
}
