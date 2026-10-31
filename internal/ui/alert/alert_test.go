package alert

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

func TestRenderSplicesWideCharactersByDisplayWidth(t *testing.T) {
	m := NewAlertModel(80, 3)
	foreColor, err := colorful.Hex("#00FF00")
	if err != nil {
		t.Fatal(err)
	}
	m.activeAlert = &alert{
		message:     "OK",
		prefix:      "(i)",
		foreColor:   foreColor,
		curLerpStep: 1.0,
	}

	content := "😀😀😀😀😀TAIL"
	got := m.Render(content)

	notif := m.activeAlert.render()
	notifWidth := ansi.StringWidth(notif)
	wantSuffix := ansi.TruncateLeft(content, notifWidth, "")
	firstLine := strings.Split(got, "\n")[0]

	if !strings.HasSuffix(firstLine, wantSuffix) {
		t.Fatalf("wide-character content was spliced at the wrong cell:\ngot  %q\nwant suffix %q",
			ansi.Strip(firstLine), wantSuffix)
	}
	if width := ansi.StringWidth(firstLine); width != ansi.StringWidth(content) {
		t.Errorf("spliced line is %d cells wide, want content width %d", width, ansi.StringWidth(content))
	}
}
