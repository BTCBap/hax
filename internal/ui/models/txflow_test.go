package models

import (
	"fmt"
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
			for _, height := range []int{24, 40} {
				t.Run(fmt.Sprintf("%s/%dx%d", tc.name, width, height), func(t *testing.T) {
					v := TxFlowView{Flow: txFlowFixture(tc.inputs, tc.outputs), Width: width, Height: height}
					wantLines := height
					for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
						v.SwitchSide(side)
						n := len(v.itemsFor(side))
						for i := 0; i < n; i++ {
							view := v.View()
							lines := strings.Split(ansi.Strip(view), "\n")
							if len(lines) != wantLines {
								t.Fatalf("%s cursor %d: %d lines, want %d", sideName(side), i, len(lines), wantLines)
							}
							for lineNo, line := range lines {
								if w := ansi.StringWidth(line); w > width {
									t.Errorf("%s cursor %d line %d: %d cells, max %d: %q",
										sideName(side), i, lineNo, w, width, line)
								}
							}
							item := v.itemsFor(side)[i]
							if item.Address != "" && !strings.Contains(ansi.Strip(view), item.Address) {
								t.Errorf("%s cursor %d: detail does not show %q", sideName(side), i, item.Address)
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
	if !strings.Contains(plain, "+7 more") {
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
	if strings.Contains(plain, "more") {
		t.Fatalf("more row still shown at the bottom:\n%s", plain)
	}
	if got := len(strings.Split(plain, "\n")); got != wantLines {
		t.Fatalf("line count changed from %d to %d while scrolling", wantLines, got)
	}

	v.MoveUp()
	if got := len(strings.Split(ansi.Strip(v.View()), "\n")); got != wantLines {
		t.Fatalf("line count changed after moving up: %d, want %d", got, wantLines)
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
