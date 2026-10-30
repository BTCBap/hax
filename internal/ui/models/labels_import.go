package models

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BTCBap/hax/internal/ui/styles"
)

// maxBIP329ImportBytes caps a label import file. Labels are small, and a
// larger file is more likely a mistake than a real export.
const maxBIP329ImportBytes int64 = 10 << 20

// labelImportRows is the inner height of the import panel. Every stage
// renders exactly this many rows so switching stages never moves content.
const labelImportRows = 9

type labelImportStage int

const (
	labelImportStagePath labelImportStage = iota
	labelImportStagePreview
	labelImportStageResult
)

// bip329Preview counts the records a file will import, without touching
// wallet state.
type bip329Preview struct {
	Tx     int
	Addr   int
	Output int
	Frozen int
}

func (p bip329Preview) summary() string {
	return fmt.Sprintf("%d tx labels · %d address labels · %d outputs (%d frozen)",
		p.Tx, p.Addr, p.Output, p.Frozen)
}

type bip329PreviewRecord struct {
	Type      string  `json:"type"`
	Ref       string  `json:"ref"`
	Label     *string `json:"label"`
	Spendable *bool   `json:"spendable"`
}

// previewBIP329 parses BIP-329 JSON Lines locally and counts the records
// that ImportLabelsBIP329 would apply. Unknown types and records without a
// ref are skipped; a malformed line fails with its 1-based line number.
func previewBIP329(data string) (bip329Preview, error) {
	var p bip329Preview
	for i, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var rec bip329PreviewRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return bip329Preview{}, fmt.Errorf("line %d: invalid JSON", i+1)
		}
		if rec.Ref == "" {
			continue
		}
		switch rec.Type {
		case "tx":
			if rec.Label != nil {
				p.Tx++
			}
		case "addr":
			if rec.Label != nil {
				p.Addr++
			}
		case "output":
			if rec.Spendable != nil {
				p.Output++
				if !*rec.Spendable {
					p.Frozen++
				}
			}
		}
	}
	return p, nil
}

// readBIP329File reads a label file and rejects anything over limit bytes.
func readBIP329File(path string, limit int64) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- user chose this import path
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	if int64(len(data)) > limit {
		if limit%(1<<20) == 0 {
			return "", fmt.Errorf("file is larger than %d MiB", limit>>20)
		}
		return "", fmt.Errorf("file is larger than %d bytes", limit)
	}
	return string(data), nil
}

// expandTilde expands a leading "~" or "~/" with the user's home directory.
func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	rest := path[2:]
	return filepath.Join(home, rest), nil
}

// displayHomePath shortens a path under the home directory to "~/...".
func displayHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
}

func (m *SettingsModel) openLabelImport() tea.Cmd {
	name := ""
	if m.walletManager != nil {
		name = m.walletManager.GetWalletName()
	}
	m.showLabelImport = true
	m.labelImportStage = labelImportStagePath
	m.labelImportData = ""
	m.labelImportPreview = bip329Preview{}
	m.labelImportResult = ""
	m.labelImportError = ""
	m.labelImportPath.SetValue("")

	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".hax", name+"_labels_bip329.jsonl")
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			m.labelImportPath.SetValue(path)
		}
	}
	m.labelImportPath.Focus()
	return textinput.Blink
}

func (m SettingsModel) updateLabelImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.labelImportStage {
	case labelImportStagePath:
		switch msg.String() {
		case "esc":
			m.closeLabelImport()
			return m, nil
		case "enter":
			return m.beginLabelImport()
		default:
			var cmd tea.Cmd
			m.labelImportPath, cmd = m.labelImportPath.Update(msg)
			return m, cmd
		}
	case labelImportStagePreview:
		switch msg.String() {
		case "esc":
			m.labelImportStage = labelImportStagePath
			m.labelImportData = ""
			m.labelImportPath.Focus()
			return m, textinput.Blink
		case "enter":
			return m.importLabels()
		}
		return m, nil
	case labelImportStageResult:
		if msg.String() == "esc" {
			m.closeLabelImport()
		}
		return m, nil
	}
	return m, nil
}

func (m SettingsModel) beginLabelImport() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.labelImportPath.Value())
	if path == "" {
		m.failLabelImport("Enter a file path")
		return m, nil
	}
	expanded, err := expandTilde(path)
	if err != nil {
		m.failLabelImport(err.Error())
		return m, nil
	}
	data, err := readBIP329File(expanded, maxBIP329ImportBytes)
	if err != nil {
		m.failLabelImport(err.Error())
		return m, nil
	}
	preview, err := previewBIP329(data)
	if err != nil {
		m.failLabelImport("Invalid BIP329 file: " + err.Error())
		return m, nil
	}
	m.labelImportPath.Blur()
	m.labelImportData = data
	m.labelImportPreview = preview
	m.labelImportStage = labelImportStagePreview
	return m, nil
}

func (m SettingsModel) importLabels() (tea.Model, tea.Cmd) {
	n, err := m.walletManager.ImportLabelsBIP329(m.labelImportData)
	if err != nil {
		m.failLabelImport("Import failed: " + err.Error())
		return m, nil
	}
	m.labelImportData = ""
	m.labelImportError = ""
	m.labelImportResult = fmt.Sprintf("✓ Imported %d labels", n)
	m.labelImportStage = labelImportStageResult
	return m, nil
}

func (m *SettingsModel) failLabelImport(message string) {
	m.labelImportStage = labelImportStageResult
	m.labelImportData = ""
	m.labelImportResult = ""
	m.labelImportError = message
}

func (m *SettingsModel) closeLabelImport() {
	m.showLabelImport = false
	m.labelImportStage = labelImportStagePath
	m.labelImportData = ""
	m.labelImportPreview = bip329Preview{}
	m.labelImportResult = ""
	m.labelImportError = ""
	m.labelImportPath.Blur()
	m.labelImportPath.SetValue("")
}

func (m SettingsModel) viewLabelImport() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	boxWidth := styles.ClampWidth(74, width)
	contentWidth := boxWidth - 2 // one cell of padding each side

	rows := make([]string, labelImportRows)
	title := styles.SectionTitleStyle.Render("Import labels (BIP329)")
	rows[0] = title
	rows[1] = ""
	rows[8] = styles.DimText.Render("esc back to settings")

	switch m.labelImportStage {
	case labelImportStagePath:
		rows[2] = styles.DimText.Render("File path")
		rows[3] = m.labelImportPath.View()
		rows[5] = styles.DimText.Render("JSON Lines file · up to 10 MiB")
		rows[8] = styles.DimText.Render("enter continue • esc cancel")
	case labelImportStagePreview:
		rows[2] = styles.NormalText.Render(m.labelImportPreview.summary())
		rows[4] = styles.DimText.Render("From " + truncateRunes(
			sanitizeLine(displayHomePath(m.labelImportPath.Value())), max(1, contentWidth-7), "…"))
		rows[8] = styles.DimText.Render("enter import • esc back")
	case labelImportStageResult:
		if m.labelImportError != "" {
			lines := wrapWords(sanitizeLine(m.labelImportError), contentWidth)
			errorRows := rows[2:7]
			for i := 0; i < len(errorRows) && i < len(lines); i++ {
				line := lines[i]
				if i == len(errorRows)-1 && len(lines) > len(errorRows) {
					line = truncateRunes(line, contentWidth, "…")
				}
				errorRows[i] = styles.ErrorText.Render(truncateRunes(line, contentWidth, "…"))
			}
		} else {
			rows[2] = styles.SuccessText.Render(truncateRunes(m.labelImportResult, contentWidth, "…"))
			rows[4] = styles.DimText.Render("From " + truncateRunes(
				sanitizeLine(displayHomePath(m.labelImportPath.Value())), max(1, contentWidth-7), "…"))
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.BitcoinOrange).
		Padding(0, 1).
		Width(boxWidth).
		Render(strings.Join(rows, "\n"))

	header := styles.ScreenHeaderStyle.Render("⚙  Import Labels")
	subtitle := styles.DimText.Render("BIP329 JSON Lines")

	return lipgloss.JoinVertical(lipgloss.Left, "", header, subtitle, "", box)
}
