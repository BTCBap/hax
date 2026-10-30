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
								if ansi.StringWidth(item.Address) <= available {
									if !strings.Contains(ansi.Strip(view), item.Address) {
										t.Errorf("%s cursor %d: detail does not show %q", sideName(side), i, item.Address)
									}
								} else if !strings.Contains(ansi.Strip(view), "…") {
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
	if !strings.Contains(plain, txFlowAddr(24)) {
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
		if strings.ContainsRune(lines[i], '┃') {
			t.Fatalf("dangling bus on padding line %d: %q", i, lines[i])
		}
	}
	if !strings.ContainsRune(lines[diagramStart+usedRows-1], '┛') {
		t.Errorf("left bus does not close on its last row: %q", lines[diagramStart+usedRows-1])
	}
	if !strings.ContainsRune(lines[diagramStart+usedRows-1], '┗') {
		t.Errorf("right bus does not close on its last row: %q", lines[diagramStart+usedRows-1])
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
	if string(want) == got {
		return
	}
	wantLines := strings.Split(string(want), "\n")
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
