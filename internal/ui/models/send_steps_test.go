package models

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderStepBar(t *testing.T) {
	bar := ansi.Strip(renderStepBar(3, 100))
	want := "Address › Amount › Recipients › Coins › Fee › Review › Sign"
	if strings.TrimSpace(bar) != want {
		t.Fatalf("bar = %q, want %q", bar, want)
	}
	if w := ansi.StringWidth(renderStepBar(7, 80)); w > 80 {
		t.Fatalf("bar is %d cells wide at 80 columns", w)
	}
	if got := strings.TrimSpace(ansi.Strip(renderStepBar(5, 40))); got != "Step 5/7 · Fee" {
		t.Fatalf("narrow bar = %q", got)
	}
	if renderStepBar(0, 80) != "" || renderStepBar(8, 80) != "" {
		t.Fatal("out-of-range step should render nothing")
	}
}
