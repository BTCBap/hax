package models

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
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
	sizes := []struct{ w, h int }{
		{80, 24}, {110, 34}, {120, 40}, {160, 50},
	}
	for _, tc := range cases {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size.w, size.h), func(t *testing.T) {
				v := TxFlowView{Flow: txFlowFixture(tc.inputs, tc.outputs), Width: size.w, Height: size.h}
				for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
					v.SwitchSide(side)
					n := len(v.itemsFor(side))
					for i := 0; i < n; i++ {
						view := v.View()
						lines := strings.Split(ansi.Strip(view), "\n")
						if len(lines) != size.h {
							t.Fatalf("%s cursor %d: %d lines, want %d", sideName(side), i, len(lines), size.h)
						}
						for lineNo, line := range lines {
							if w := ansi.StringWidth(line); w > size.w {
								t.Errorf("%s cursor %d line %d: %d cells, max %d: %q",
									sideName(side), i, lineNo, w, size.w, line)
							}
						}
						item := v.itemsFor(side)[i]
						if item.Address != "" {
							available := size.w - ansi.StringWidth("Address  ")
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
					for _, off := range []int{0, 1, n / 2, n - 1} {
						v.Offset[side.index()] = off
						lines := strings.Split(ansi.Strip(v.View()), "\n")
						if len(lines) != size.h {
							t.Fatalf("%s offset %d: %d lines, want %d", sideName(side), off, len(lines), size.h)
						}
					}
					v.Offset[side.index()] = 0
				}
			})
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
	leftW, _, _ := flowColumnWidths(80)
	wantLast := ansi.Strip(ui.FormatAddress(txFlowAddr(24), flowLabelAddressWidth(leftW, false)))
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

func flowTestDiagram(lines []string) (start, end int) {
	for i, line := range lines {
		if strings.HasPrefix(line, "Address") {
			return 2, i - 1
		}
	}
	return 2, 2
}

func TestTxFlow_CenterCaptionAndMarkers(t *testing.T) {
	v := TxFlowView{Flow: txFlowExample(), Width: 80, Height: 24}
	plain := ansi.Strip(v.View())
	if !strings.Contains(plain, "1 s/vB · 141 vB") {
		t.Fatalf("diagram does not contain the centered fee caption:\n%s", plain)
	}
	for _, marker := range []string{"●", "◆", "○", "▪"} {
		if !strings.Contains(plain, marker) {
			t.Errorf("view is missing marker %q:\n%s", marker, plain)
		}
	}
	if strings.ContainsAny(plain, "┏┓┗┛┣┫┳┻╋") {
		t.Errorf("bands renderer still draws bus glyphs:\n%s", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("addresses are not truncated in the middle:\n%s", plain)
	}
}

func TestTxFlow_TrunkThicknessFollowsValue(t *testing.T) {
	entries := []flowEntry{
		{item: 0, value: 600_000, known: true},
		{item: 1, value: 300_000, known: true},
		{item: 2, value: 100_000, known: true},
		{item: 3, known: false},
	}
	thickness := flowAllocate(entries, 40)
	if thickness[0] < thickness[1] || thickness[1] < thickness[2] {
		t.Fatalf("thickness is not monotonic in value: %v", thickness)
	}
	if thickness[3] != 1 {
		t.Fatalf("unknown value got %d pixels, want 1", thickness[3])
	}
	sum := 0
	for _, n := range thickness {
		sum += n
	}
	if sum != 40 {
		t.Fatalf("thickness sum = %d, want the full trunk span 40", sum)
	}

	// Rendered trunk slices keep the same ordering.
	flow := TxFlow{
		Fee: 100_000, FeeKnown: true, FeeRate: 2, VSize: 50_000,
		Inputs: []TxFlowItem{
			{Address: "bc1qlarge", Value: 600_000, ValueKnown: true, Kind: KindOwnInput},
			{Address: "bc1qmedium", Value: 300_000, ValueKnown: true, Kind: KindOwnInput},
			{Address: "bc1qsmall", Value: 100_000, ValueKnown: true, Kind: KindOwnInput},
			{Address: "bc1qunknown", Kind: KindForeignInput},
		},
		Outputs: []TxFlowItem{
			{Address: "bc1qdest", Value: 900_000, ValueKnown: true, Kind: KindExternal},
		},
	}
	view := TxFlowView{Flow: flow, Width: 80, Height: 24}
	lines := strings.Split(ansi.Strip(view.View()), "\n")
	start, end := flowTestDiagram(lines)
	found := false
	for _, line := range lines[start:end] {
		if strings.Contains(line, "2 s/vB") {
			found = true
		}
	}
	if !found {
		t.Fatalf("caption is missing from the diagram:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTxFlow_BandLayoutIsProportionalAndShared(t *testing.T) {
	assertLayout := func(t *testing.T, v TxFlowView, rows int) {
		t.Helper()
		in, out := v.flowEntries(FlowInput, rows), v.flowEntries(FlowOutput, rows)
		inLayout, outLayout := flowDiagramLayouts(in, out, rows)
		if inLayout.trunkPx != outLayout.trunkPx {
			t.Fatalf("side trunk budgets differ: %d and %d", inLayout.trunkPx, outLayout.trunkPx)
		}
		if want := flowTrunkPixels(len(in), len(out), rows); inLayout.trunkPx != want {
			t.Fatalf("trunk budget = %d, want %d", inLayout.trunkPx, want)
		}
		minimum, maximum := max(len(in), len(out)), 2*rows-2
		if maximum < minimum {
			maximum = minimum
		}
		if inLayout.trunkPx < minimum || inLayout.trunkPx > maximum {
			t.Fatalf("trunk budget %d outside [%d, %d]", inLayout.trunkPx, minimum, maximum)
		}
		for name, layout := range map[string]flowSideLayout{"inputs": inLayout, "outputs": outLayout} {
			n := len(layout.entries)
			if layout.stackRows > rows {
				t.Errorf("%s stack uses %d of %d rows", name, layout.stackRows, rows)
			}
			if want := (rows - layout.stackRows) / 2; layout.stackTop != want {
				t.Errorf("%s stack top = %d, want %d", name, layout.stackTop, want)
			}
			sum := 0
			for _, px := range layout.slices {
				sum += px
			}
			if sum != layout.trunkPx {
				t.Errorf("%s slices sum to %d px, want the trunk budget %d", name, sum, layout.trunkPx)
			}
			seenRows := make(map[int]bool, n)
			for i, entry := range layout.entries {
				if layout.heights[i] < 1 {
					t.Errorf("%s entry %d has %d rows", name, i, layout.heights[i])
				}
				wantRow := layout.tops[i] + (layout.heights[i]-1)/2
				if entry.row != wantRow {
					t.Errorf("%s entry %d row = %d, want %d", name, i, entry.row, wantRow)
				}
				if entry.row < 0 || entry.row >= rows {
					t.Errorf("%s entry %d row %d is outside the diagram", name, i, entry.row)
				}
				if seenRows[entry.row] {
					t.Errorf("%s label row %d is used twice", name, entry.row)
				}
				seenRows[entry.row] = true
				if entry.aggregate() && layout.heights[i] != 1 {
					t.Errorf("%s aggregate entry has %d rows, want 1", name, layout.heights[i])
				}
				edge := layout.edgeThickness(i)
				if edge < 1 || edge > layout.slices[i] || edge > 2*layout.heights[i] {
					t.Errorf("%s entry %d edge = %d, want 1..min(%d, %d)",
						name, i, edge, layout.slices[i], 2*layout.heights[i])
				}
				if layout.gap > 0 && edge != layout.slices[i] {
					t.Errorf("%s entry %d edge %d != slice %d without a cap",
						name, i, edge, layout.slices[i])
				}
				if layout.gap == 0 && n > 1 && layout.slices[i] > 1 && edge != layout.slices[i]-1 {
					t.Errorf("%s entry %d edge = %d, want %d for touching ribbons",
						name, i, edge, layout.slices[i]-1)
				}
				center := layout.edgeCenter(i)
				slotTop := float64(2 * layout.tops[i])
				slotBottom := float64(2 * (layout.tops[i] + layout.heights[i]))
				half := float64(edge) / 2
				if center-half < slotTop-1e-9 || center+half > slotBottom+1e-9 {
					t.Errorf("%s entry %d edge span %.1f..%.1f escapes slot %.1f..%.1f",
						name, i, center-half, center+half, slotTop, slotBottom)
				}
			}
			slices := flowTrunkSlices(layout, rows)
			total := 0
			for i := range slices {
				if slices[i].thickness != layout.edgeThickness(i) {
					t.Errorf("%s entry %d trunk slice = %d px, edge = %d px",
						name, i, slices[i].thickness, layout.edgeThickness(i))
				}
				total += slices[i].thickness
			}
			if len(slices) > 0 && slices[0].top != rows-total/2 {
				t.Errorf("%s trunk stack top = %d, want %d", name, slices[0].top, rows-total/2)
			}
			for i := range layout.entries {
				for j := range layout.entries {
					if layout.entries[i].aggregate() || layout.entries[j].aggregate() ||
						!layout.entries[i].known || !layout.entries[j].known {
						continue
					}
					if layout.entries[i].value < layout.entries[j].value &&
						layout.slices[i] > layout.slices[j] {
						t.Errorf("%s thickness is not monotonic in value: %v vs %v px %v",
							name, layout.entries[i].value, layout.entries[j].value, layout.slices)
					}
				}
			}
		}
	}

	for _, size := range []struct{ w, h int }{{80, 24}, {110, 34}, {120, 40}, {160, 50}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			for _, fixture := range [][2]int{{1, 2}, {3, 3}, {9, 4}, {25, 12}} {
				v := TxFlowView{
					Flow:   txFlowFixture(fixture[0], fixture[1]),
					Width:  size.w,
					Height: size.h,
				}
				assertLayout(t, v, v.diagramRows())
			}
		})
	}
}

func TestTxFlow_CanvasWidthRange(t *testing.T) {
	cases := []struct {
		width  int
		minCol int
		maxCol int
	}{
		{width: 80, minCol: 20, maxCol: 27},
		{width: 110, minCol: 34, maxCol: 38},
		{width: 120, minCol: 38, maxCol: 42},
		{width: 160, minCol: 44, maxCol: 52},
	}
	for _, tc := range cases {
		left, canvas, right := flowColumnWidths(tc.width)
		if canvas < tc.minCol || canvas > tc.maxCol {
			t.Errorf("width %d: canvas = %d, want %d..%d", tc.width, canvas, tc.minCol, tc.maxCol)
		}
		if left+canvas+right != tc.width {
			t.Errorf("width %d: columns %d+%d+%d do not fill the view", tc.width, left, canvas, right)
		}
		if left > tc.width || right > tc.width {
			t.Errorf("width %d: label columns %d/%d exceed the view", tc.width, left, right)
		}
		v := TxFlowView{Flow: txFlowFixture(3, 3), Width: tc.width, Height: 24}
		for i, line := range strings.Split(ansi.Strip(v.View()), "\n") {
			if w := ansi.StringWidth(line); w > tc.width {
				t.Errorf("width %d: line %d is %d cells", tc.width, i, w)
			}
		}
	}
}

func TestTxFlow_80(t *testing.T) {
	v := TxFlowView{Flow: txFlowExample(), Width: 80, Height: 24}
	got := ansi.Strip(v.View())

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
	if !strings.Contains(plain, "◇") || !strings.Contains(plain, "data") {
		t.Fatalf("OP_RETURN marker or data label is missing:\n%s", plain)
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
	for _, want := range []string{"●", "◆", "○", "▪", "yours", "change", "external", "fee"} {
		if !strings.Contains(plain, want) {
			t.Errorf("no-color view is missing %q:\n%s", want, plain)
		}
	}
}

func TestTxFlow_SelectedBandUsesAccent(t *testing.T) {
	v := TxFlowView{Flow: txFlowExample(), Width: 110, Height: 34, Side: FlowInput, Cursor: 0}
	accent := sgrPrefix(lipgloss.NewStyle().Foreground(styles.TxFlowAccent))
	for _, line := range strings.Split(v.View(), "\n") {
		if !strings.Contains(line, accent) {
			continue
		}
		if strings.ContainsAny(ansi.Strip(line), "█▀▄") {
			return
		}
	}
	t.Fatalf("selected band does not use the accent color:\n%s", ansi.Strip(v.View()))
}

func TestTxFlow_EmptyCanvasCellsHaveNoBackground(t *testing.T) {
	canvas := newFlowPixelCanvas(6, 4)
	lines := canvas.cells().lines()
	for i, line := range lines {
		if line != strings.Repeat(" ", 6) {
			t.Fatalf("empty canvas row %d = %q, want six spaces", i, line)
		}
		if strings.Contains(line, "\x1b") {
			t.Fatalf("empty canvas row %d carries SGR: %q", i, line)
		}
	}

	// A single half pixel uses a foreground only; two different colors put
	// the bottom tone in the background.
	canvas.set(0, 0, styles.TxFlowBand)
	line := canvas.cells().lines()[0]
	if !strings.Contains(line, "▀") {
		t.Fatalf("canvas row does not use a half block: %q", line)
	}
	if strings.Contains(line, "48;2;") {
		t.Fatalf("single-color half block emitted a background: %q", line)
	}

	canvas.set(0, 1, styles.TxFlowChange)
	line = canvas.cells().lines()[0]
	if !strings.Contains(line, "48;2;") {
		t.Fatalf("two-color half block did not use a background: %q", line)
	}
}

// TestFlowCanvasLinesCoalesceSGR keeps the canvas-to-text conversion from
// re-emitting a style for every half-block cell of one coloured ribbon.
func TestFlowCanvasLinesCoalesceSGR(t *testing.T) {
	const sgr = "\x1b[38;2;1;2;3m"
	canvas := newFlowCanvas(4, 1)
	for x := 0; x < 4; x++ {
		canvas.put(x, 0, "▀", sgr)
	}
	line := canvas.lines()[0]
	if got := strings.Count(line, sgr); got != 1 {
		t.Fatalf("line emits the SGR %d times, want one run: %q", got, line)
	}
	if got := strings.Count(line, "\x1b[0m"); got != 1 {
		t.Fatalf("line emits %d resets, want one per run: %q", got, line)
	}
	if plain := ansi.Strip(line); plain != "▀▀▀▀" {
		t.Fatalf("stripped line = %q, want the same glyphs", plain)
	}
}

// TestTxFlow_FeeRibbonColumnsTouch guards the half-block band against steep
// one-pixel ribbons: every column must share a filled pixel row with the
// column before it, so the ribbon stays connected instead of dotting.
func TestTxFlow_FeeRibbonColumnsTouch(t *testing.T) {
	const rows = 40
	flow := txFlowFixture(1, 6)
	v := TxFlowView{Flow: flow, Width: 40, Height: rows}
	in, out := v.flowEntries(FlowInput, rows), v.flowEntries(FlowOutput, rows)
	_, outLayout := flowDiagramLayouts(in, out, rows)
	_, canvasW, _ := flowColumnWidths(40)
	trunkRight := (canvasW-flowTrunkCols(canvasW))/2 + flowTrunkCols(canvasW)
	slices := flowTrunkSlices(outLayout, rows)

	fee := -1
	for i, entry := range outLayout.entries {
		if entry.kind == KindFee {
			fee = i
			break
		}
	}
	if fee < 0 {
		t.Fatal("fee entry missing from the output layout")
	}
	thickness := outLayout.edgeThickness(fee)
	if thickness != 1 {
		t.Fatalf("fee edge thickness = %d, want a thin one-pixel band", thickness)
	}

	canvas := newFlowPixelCanvas(canvasW, rows*2)
	trunkCenter := float64(slices[fee].top) + float64(thickness)/2
	flowDrawBand(canvas, trunkRight, canvasW-1, trunkCenter, float64(thickness), outLayout.edgeCenter(fee), float64(thickness), false, styles.TxFlowFee)

	columnPixels := func(x int) []int {
		var ys []int
		for y := 0; y < canvas.h; y++ {
			if pixel := canvas.pixels[y*canvasW+x]; pixel.hasTop || pixel.hasBottom {
				ys = append(ys, y)
			}
		}
		return ys
	}
	previous := columnPixels(trunkRight)
	if len(previous) == 0 {
		t.Fatalf("column %d of the ribbon is empty", trunkRight)
	}
	for x := trunkRight + 1; x < canvasW; x++ {
		current := columnPixels(x)
		if len(current) == 0 {
			t.Fatalf("column %d of the ribbon is empty", x)
		}
		touches := false
		for _, y := range current {
			for _, prev := range previous {
				if y >= prev-1 && y <= prev+1 {
					touches = true
				}
			}
		}
		if !touches {
			t.Fatalf("column %d does not touch column %d: rows %v vs %v", x, x-1, current, previous)
		}
		previous = current
	}
}
