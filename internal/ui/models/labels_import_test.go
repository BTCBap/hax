package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func settingsKey(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return keyCode(tea.KeyEnter)
	case "esc":
		return keyCode(tea.KeyEsc)
	default:
		return keyRunes(s)
	}
}

func settingsUpdate(t *testing.T, m SettingsModel, key tea.KeyPressMsg) SettingsModel {
	t.Helper()
	model, _ := m.Update(key)
	updated, ok := model.(SettingsModel)
	if !ok {
		t.Fatalf("update returned %T, want SettingsModel", model)
	}
	return updated
}

const settingsTestLabels = `{"type":"tx","ref":"aa00000000000000000000000000000000000000000000000000000000000001","label":"first"}
{"type":"tx","ref":"bb00000000000000000000000000000000000000000000000000000000000002","label":"second"}
{"type":"addr","ref":"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx","label":"cold"}
{"type":"output","ref":"cc00000000000000000000000000000000000000000000000000000000000003:0","spendable":false}
{"type":"output","ref":"dd00000000000000000000000000000000000000000000000000000000000004:1","spendable":true}
{"type":"xpub","ref":"zpub6rignoredtype","label":"unknown is skipped"}
`

func TestPreviewBIP329CountsKnownTypes(t *testing.T) {
	preview, err := previewBIP329(settingsTestLabels)
	if err != nil {
		t.Fatalf("previewBIP329: %v", err)
	}
	want := bip329Preview{Tx: 2, Addr: 1, Output: 2, Frozen: 1}
	if preview != want {
		t.Fatalf("preview = %+v, want %+v", preview, want)
	}
	if got, wantLine := preview.summary(), "2 tx labels · 1 address label · 2 outputs (1 frozen)"; got != wantLine {
		t.Fatalf("summary = %q, want %q", got, wantLine)
	}
}

func TestPreviewBIP329SummarySingularForms(t *testing.T) {
	preview := bip329Preview{Tx: 1, Addr: 1, Output: 1, Frozen: 1}
	if got, want := preview.summary(), "1 tx label · 1 address label · 1 output (1 frozen)"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}

	preview = bip329Preview{}
	if got, want := preview.summary(), "0 tx labels · 0 address labels · 0 outputs (0 frozen)"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestPreviewBIP329MalformedLineReportsLineNumber(t *testing.T) {
	data := "{\"type\":\"tx\",\"ref\":\"aa\",\"label\":\"ok\"}\n\n{\"type\":\"addr\",\"ref\":"
	_, err := previewBIP329(data)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error = %q, want the malformed line number", err)
	}
}

func TestPreviewBIP329EmptyInputCountsZero(t *testing.T) {
	preview, err := previewBIP329("\n\n")
	if err != nil {
		t.Fatalf("previewBIP329: %v", err)
	}
	if preview != (bip329Preview{}) {
		t.Fatalf("preview = %+v, want zero counts", preview)
	}
}

func TestReadBIP329FileRejectsOversizeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.jsonl")
	if err := os.WriteFile(path, []byte(settingsTestLabels), 0o600); err != nil { // #nosec G306 -- test fixture
		t.Fatal(err)
	}
	_, err := readBIP329File(path, 32)
	if err == nil {
		t.Fatal("expected an oversize error")
	}
	if !strings.Contains(err.Error(), "larger than 32 bytes") {
		t.Fatalf("error = %q, want a clear size limit", err)
	}
	if maxBIP329ImportBytes != 10<<20 {
		t.Fatalf("import limit = %d, want 10 MiB", maxBIP329ImportBytes)
	}
}

func TestExpandTildeUsesHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := expandTilde("~/.hax/labels.jsonl")
	if err != nil {
		t.Fatalf("expandTilde: %v", err)
	}
	want := filepath.Join(home, ".hax", "labels.jsonl")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestSettings_LabelImportPrefillsDefaultExportPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path := filepath.Join(home, ".hax", "test-wallet_labels_bip329.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(settingsTestLabels), 0o600); err != nil { // #nosec G306 -- test fixture
		t.Fatal(err)
	}

	m := NewSettingsModel(NewMockWalletManager(), nil)
	m = settingsUpdate(t, m, settingsKey("l"))
	if !m.showLabelImport {
		t.Fatal("l did not open the label import screen")
	}
	if got := m.labelImportPath.Value(); got != path {
		t.Fatalf("prefilled path = %q, want %q", got, path)
	}
}

func TestSettings_LabelImportMissingDefaultLeavesPathEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	m := NewSettingsModel(NewMockWalletManager(), nil)
	m = settingsUpdate(t, m, settingsKey("l"))
	if got := m.labelImportPath.Value(); got != "" {
		t.Fatalf("prefilled path = %q, want empty when no export exists", got)
	}
}

func TestSettings_LabelImportConfirmCallsWalletOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path := filepath.Join(home, "labels.jsonl")
	if err := os.WriteFile(path, []byte(settingsTestLabels), 0o600); err != nil { // #nosec G306 -- test fixture
		t.Fatal(err)
	}

	calls := 0
	var gotData string
	mock := NewMockWalletManager()
	mock.ImportLabelsBIP329Fn = func(data string) (int, error) {
		calls++
		gotData = data
		return 4, nil
	}

	m := NewSettingsModel(mock, nil)
	m.width, m.height = 80, 24
	m = settingsUpdate(t, m, settingsKey("l"))
	m.labelImportPath.SetValue(path)

	m = settingsUpdate(t, m, settingsKey("enter"))
	if m.labelImportStage != labelImportStagePreview {
		t.Fatalf("stage = %v, want preview", m.labelImportStage)
	}
	if calls != 0 {
		t.Fatalf("import called %d times before confirm", calls)
	}
	previewView := m.View()
	if !strings.Contains(stripANSI(previewView), "2 tx labels · 1 address label · 2 outputs (1 frozen)") {
		t.Fatalf("preview counts missing:\n%s", stripANSI(previewView))
	}

	m = settingsUpdate(t, m, settingsKey("enter"))
	if calls != 1 {
		t.Fatalf("import called %d times, want 1", calls)
	}
	if gotData != settingsTestLabels {
		t.Fatalf("import data differs from the file contents")
	}
	if m.labelImportStage != labelImportStageResult {
		t.Fatalf("stage = %v, want result", m.labelImportStage)
	}
	if !strings.Contains(stripANSI(m.View()), "✓ Imported 4 labels") {
		t.Fatalf("result is missing the import count:\n%s", stripANSI(m.View()))
	}
}

func TestSettings_LabelImportEscFromPreviewDoesNotImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path := filepath.Join(home, "labels.jsonl")
	if err := os.WriteFile(path, []byte(settingsTestLabels), 0o600); err != nil { // #nosec G306 -- test fixture
		t.Fatal(err)
	}

	calls := 0
	mock := NewMockWalletManager()
	mock.ImportLabelsBIP329Fn = func(string) (int, error) {
		calls++
		return 1, nil
	}

	m := NewSettingsModel(mock, nil)
	m = settingsUpdate(t, m, settingsKey("l"))
	m.labelImportPath.SetValue(path)
	m = settingsUpdate(t, m, settingsKey("enter"))
	m = settingsUpdate(t, m, settingsKey("esc"))

	if calls != 0 {
		t.Fatalf("esc from preview imported %d times", calls)
	}
	if m.labelImportStage != labelImportStagePath {
		t.Fatalf("stage = %v, want the path stage after esc", m.labelImportStage)
	}

	m = settingsUpdate(t, m, settingsKey("esc"))
	if m.showLabelImport {
		t.Fatal("esc from the path stage should return to settings")
	}
}

func TestSettings_LabelImportMalformedFileShowsLineError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path := filepath.Join(home, "labels.jsonl")
	data := "{\"type\":\"tx\",\"ref\":\"aa\",\"label\":\"ok\"}\n{\"type\":\"addr\",\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil { // #nosec G306 -- test fixture
		t.Fatal(err)
	}

	mock := NewMockWalletManager()
	calls := 0
	mock.ImportLabelsBIP329Fn = func(string) (int, error) {
		calls++
		return 0, nil
	}
	m := NewSettingsModel(mock, nil)
	m = settingsUpdate(t, m, settingsKey("l"))
	m.labelImportPath.SetValue(path)
	m = settingsUpdate(t, m, settingsKey("enter"))

	if m.labelImportStage != labelImportStageResult {
		t.Fatalf("stage = %v, want result", m.labelImportStage)
	}
	if !strings.Contains(stripANSI(m.View()), "line 2") {
		t.Fatalf("error view is missing the line number:\n%s", stripANSI(m.View()))
	}
	if calls != 0 {
		t.Fatalf("malformed file imported %d times", calls)
	}
}

func TestSettings_LabelImportPathInputKeepsLayout(t *testing.T) {
	m := NewSettingsModel(NewMockWalletManager(), nil)
	m.width, m.height = 80, 24
	m.showLabelImport = true
	m.labelImportStage = labelImportStagePath

	var wantLines int
	var wantPath viewPosition
	for i, value := range []string{"", "labels.jsonl", strings.Repeat("a", 200)} {
		m.labelImportPath.SetValue(value)
		view := m.View()
		lines := requireLineCount(t, view)
		path := requireTextPosition(t, view, "File path")
		if i == 0 {
			wantLines, wantPath = lines, path
			continue
		}
		if lines != wantLines {
			t.Fatalf("input %d has %d lines, want %d", i, lines, wantLines)
		}
		if path != wantPath {
			t.Fatalf("input %d moved the path label from %+v to %+v", i, wantPath, path)
		}
		assertFits(t, view, 80)
	}
}

func TestSettings_LabelImportStagesKeepLineCount(t *testing.T) {
	sizes := []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}}
	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			m := NewSettingsModel(NewMockWalletManager(), nil)
			m.width, m.height = size.w, size.h
			m.showLabelImport = true
			m.labelImportPath.SetValue(filepath.Join("~", ".hax", "test-wallet_labels_bip329.jsonl"))
			m.labelImportStage = labelImportStagePath
			m.labelImportPreview = bip329Preview{Tx: 2, Addr: 1, Output: 2, Frozen: 1}

			var want int
			views := map[string]func() string{
				"path": func() string { return m.View() },
				"preview": func() string {
					m.labelImportStage = labelImportStagePreview
					return m.View()
				},
				"success": func() string {
					m.labelImportStage = labelImportStageResult
					m.labelImportResult = "✓ Imported 42 labels"
					m.labelImportError = ""
					return m.View()
				},
				"error": func() string {
					m.labelImportStage = labelImportStageResult
					m.labelImportResult = ""
					m.labelImportError = "Import failed: wallet is locked"
					return m.View()
				},
			}
			for _, name := range []string{"path", "preview", "success", "error"} {
				view := views[name]()
				lines := requireLineCount(t, view)
				if want == 0 {
					want = lines
				}
				if lines != want {
					t.Fatalf("%s has %d lines, want %d", name, lines, want)
				}
				assertFits(t, view, size.w)
			}
		})
	}
}
