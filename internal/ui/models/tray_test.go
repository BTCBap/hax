package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func testTray() Tray {
	options := []TrayOption{
		{Label: "Fastest", Help: "~10 min · 12 sat/vB"},
		{Label: "30 minutes", Help: "~30 min · 8 sat/vB"},
		{Label: "1 hour", Help: "~1 hour · 3 sat/vB"},
		{Label: "Custom…", Help: "enter a rate"},
	}
	summary := []TraySummaryRow{
		{Label: "Current rate", Value: "2 sat/vB"},
		{Label: "New rate", Value: "12 sat/vB"},
		{Label: "Extra fee", Value: "+1,400 sats"},
	}
	return NewTray("⚡ Speed Up Transaction", options, summary, 80)
}

func trayKey(kind rune) tea.KeyPressMsg {
	return keyCode(kind)
}

func TestTrayLineCountStableAcrossStagesAndSelections(t *testing.T) {
	tray := testTray()
	wantLines := requireLineCount(t, tray.View())
	wantHelper := requireTextPosition(t, tray.View(), "~10 min · 12 sat/vB")

	for i := 0; i < len(tray.options); i++ {
		view := tray.View()
		if got := requireLineCount(t, view); got != wantLines {
			t.Fatalf("option %d has %d lines, want %d", i, got, wantLines)
		}
		if got := requireTextPosition(t, view, "~10 min · 12 sat/vB"); got != wantHelper {
			t.Fatalf("option %d moved the helper column from %+v to %+v", i, wantHelper, got)
		}
		if i < len(tray.options)-1 {
			tray, _ = tray.Update(trayKey(tea.KeyDown))
		}
	}

	tray, result := tray.Update(trayKey(tea.KeyEnter))
	if result.Kind != TrayNone {
		t.Fatalf("enter should move to confirm without a result, got %v", result.Kind)
	}
	if tray.stage != TrayStageConfirm {
		t.Fatal("enter did not open the confirm stage")
	}
	confirm := tray.View()
	if got := requireLineCount(t, confirm); got != wantLines {
		t.Fatalf("confirm stage has %d lines, want %d", got, wantLines)
	}
	if !strings.Contains(stripANSI(confirm), "New rate") || !strings.Contains(stripANSI(confirm), "Extra fee") {
		t.Fatalf("confirm stage is missing summary rows:\n%s", stripANSI(confirm))
	}
}

func TestTrayHelperColumnSameXForEveryOption(t *testing.T) {
	tray := testTray()
	view := tray.View()

	first := requireTextPosition(t, view, "~10 min · 12 sat/vB")
	second := requireTextPosition(t, view, "~30 min · 8 sat/vB")
	third := requireTextPosition(t, view, "~1 hour · 3 sat/vB")
	custom := requireTextPosition(t, view, "enter a rate")

	if second.col != first.col || third.col != first.col || custom.col != first.col {
		t.Fatalf("helper column x differs: %d, %d, %d, %d",
			first.col, second.col, third.col, custom.col)
	}
	if !strings.Contains(stripANSI(view), "›") {
		t.Fatalf("selected marker missing:\n%s", stripANSI(view))
	}
	assertFits(t, view, 80)
}

func TestTrayEscFromConfirmGoesBackToChoose(t *testing.T) {
	tray := testTray()
	tray, _ = tray.Update(trayKey(tea.KeyEnter))
	if tray.stage != TrayStageConfirm {
		t.Fatal("expected confirm stage")
	}

	tray, result := tray.Update(trayKey(tea.KeyEsc))
	if tray.stage != TrayStageChoose {
		t.Fatal("esc on confirm should return to choose")
	}
	if result.Kind != TrayNone {
		t.Fatalf("esc on confirm should not finish the tray, got %v", result.Kind)
	}
}

func TestTrayEscFromChooseCancels(t *testing.T) {
	tray := testTray()
	_, result := tray.Update(trayKey(tea.KeyEsc))
	if result.Kind != TrayCancelled {
		t.Fatalf("esc on choose should cancel, got %v", result.Kind)
	}

	tray = testTray()
	tray, _ = tray.Update(trayKey(tea.KeyDown))
	tray, _ = tray.Update(trayKey(tea.KeyEnter))
	tray, _ = tray.Update(trayKey(tea.KeyEsc))
	_, result = tray.Update(trayKey(tea.KeyEsc))
	if result.Kind != TrayCancelled {
		t.Fatalf("esc from choose after returning from confirm should cancel, got %v", result.Kind)
	}
}

func TestTrayConfirmReturnsSelectedIndex(t *testing.T) {
	tray := testTray()
	tray, _ = tray.Update(trayKey(tea.KeyDown))
	tray, _ = tray.Update(trayKey(tea.KeyEnter))
	_, result := tray.Update(trayKey(tea.KeyEnter))
	if result.Kind != TrayConfirmed {
		t.Fatalf("expected confirm result, got %v", result.Kind)
	}
	if result.Selected != 1 {
		t.Fatalf("selected index = %d, want 1", result.Selected)
	}
}

func TestTrayFitsNarrowWidth(t *testing.T) {
	tray := NewTray("⚡ Speed Up Transaction", testTray().options, testTray().summary, 40)
	assertFits(t, tray.View(), 40)
}

func TestTrayOptionMarkersKeepColumnsAligned(t *testing.T) {
	tray := testTray()
	selected := tray.renderOptionRow(0, tray.options[0])
	unselected := tray.renderOptionRow(1, tray.options[1])

	if !strings.HasPrefix(stripANSI(selected), "› ") {
		t.Fatalf("selected row lacks the accent marker: %q", stripANSI(selected))
	}
	if !strings.HasPrefix(stripANSI(unselected), "  ") {
		t.Fatalf("unselected row should start with two spaces: %q", stripANSI(unselected))
	}
	if got, want := lipgloss.Width(selected), lipgloss.Width(unselected); got != want {
		t.Fatalf("selected row is %d cells, unselected is %d", got, want)
	}
}
