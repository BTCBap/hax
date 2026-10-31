package models

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/ui/styles"
)

type backupExportStage int

const (
	backupExportStageChoose backupExportStage = iota
	backupExportStageConfirm
	backupExportStageResult
)

const backupExportNote = "Writes public wallet data: descriptors, xpubs, labels. No seed or private keys."

func (m SettingsModel) walletName() string {
	if m.walletManager == nil {
		return "wallet"
	}
	return m.walletManager.GetWalletName()
}

func backupExportOptions(name string) []TrayOption {
	return []TrayOption{
		{Label: "Save to ~/.hax", Help: name + "_backup.json"},
		{Label: "Copy to clipboard", Help: "public data"},
	}
}

func backupExportSummary(selected int, name string) []TraySummaryRow {
	destination := "~/.hax/" + name + "_backup.json"
	if selected == 1 {
		destination = "clipboard"
	}
	// The note below already names the contents. The blank row keeps the
	// tray at its previous height after dropping the redundant row.
	return []TraySummaryRow{
		{Label: "Destination", Value: destination},
		{},
	}
}

func (m *SettingsModel) openBackupExport() {
	name := m.walletName()
	tray := NewTray("Export Backup", backupExportOptions(name), backupExportSummary(0, name), m.width)
	tray.SummaryFor = func(selected int) []TraySummaryRow {
		return backupExportSummary(selected, name)
	}
	m.backupTray = tray.
		WithNote(backupExportNote).
		WithConfirmHint("enter export • esc back")
	m.showBackupExport = true
	m.backupExportStage = backupExportStageChoose
	m.backupExportPath = ""
	m.backupExportResult = ""
	m.backupExportError = ""
}

func (m SettingsModel) updateBackupExport(msg tea.KeyPressMsg) (screenModel, tea.Cmd) {
	switch m.backupExportStage {
	case backupExportStageChoose, backupExportStageConfirm:
		var result TrayResult
		m.backupTray, result = m.backupTray.Update(msg)
		switch result.Kind {
		case TrayCancelled:
			m.closeBackupExport()
			return m, nil
		case TrayConfirmed:
			m.confirmBackupExport(result.Selected)
			return m, nil
		}
		if m.backupTray.stage == TrayStageConfirm {
			m.backupExportStage = backupExportStageConfirm
		} else {
			m.backupExportStage = backupExportStageChoose
		}
		return m, nil
	case backupExportStageResult:
		if msg.String() == "esc" {
			m.closeBackupExport()
		}
		return m, nil
	}
	return m, nil
}

func (m *SettingsModel) confirmBackupExport(selected int) {
	data, err := m.walletManager.ExportUnifiedBackup()
	if err != nil {
		m.backupExportStage = backupExportStageResult
		m.backupExportPath = ""
		m.backupExportResult = ""
		m.backupExportError = "Export failed: " + err.Error()
		return
	}

	if selected == 1 {
		if err := copyToClipboard(string(data)); err != nil {
			m.backupExportStage = backupExportStageResult
			m.backupExportResult = ""
			m.backupExportError = "Copy failed: " + err.Error()
			return
		}
		m.backupExportStage = backupExportStageResult
		m.backupExportPath = ""
		m.backupExportError = ""
		m.backupExportResult = "✓ Backup copied to clipboard"
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		m.backupExportStage = backupExportStageResult
		m.backupExportResult = ""
		m.backupExportError = "Failed to get home directory: " + err.Error()
		return
	}
	path, err := writePrivateExport(filepath.Join(homeDir, ".hax"),
		m.walletName()+"_backup.json", data)
	if err != nil {
		m.backupExportStage = backupExportStageResult
		m.backupExportPath = ""
		m.backupExportResult = ""
		m.backupExportError = "Save failed: " + err.Error()
		return
	}
	m.backupExportStage = backupExportStageResult
	m.backupExportError = ""
	m.backupExportResult = "✓ Backup saved"
	m.backupExportPath = path
}

func (m *SettingsModel) closeBackupExport() {
	m.showBackupExport = false
	m.backupExportStage = backupExportStageChoose
	m.backupExportPath = ""
	m.backupExportResult = ""
	m.backupExportError = ""
}

func (m SettingsModel) viewBackupExport() string {
	header := styles.ScreenHeaderStyle.Render("⚙  Export Backup")
	subtitle := styles.DimText.Render("Descriptors, xpubs and labels · no private keys")

	panel := m.backupTray.View()
	if m.backupExportStage == backupExportStageResult {
		panel = m.viewBackupExportResult()
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", header, subtitle, "", panel)
}

func (m SettingsModel) viewBackupExportResult() string {
	rowsHeight := m.backupTray.rowsHeight
	if rowsHeight < 3 {
		rowsHeight = 3
	}
	inner := make([]string, rowsHeight+4)
	inner[0] = styles.SectionTitleStyle.Render("Export Backup")
	inner[1] = ""

	contentWidth := m.backupTray.contentWidth
	if m.backupExportError != "" {
		lines := wrapWords(sanitizeLine(m.backupExportError), contentWidth)
		for i := 0; i < len(lines) && i < rowsHeight; i++ {
			line := lines[i]
			if i == rowsHeight-1 && len(lines) > rowsHeight {
				line = truncateRunes(line, contentWidth, "…")
			}
			inner[2+i] = styles.ErrorText.Render(truncateRunes(line, contentWidth, "…"))
		}
	} else {
		inner[2] = styles.SuccessText.Render(truncateRunes(m.backupExportResult, contentWidth, "…"))
		if m.backupExportPath != "" {
			inner[4] = styles.DimText.Render("Saved to " + truncateRunes(
				sanitizeLine(displayHomePath(m.backupExportPath)), max(1, contentWidth-9), "…"))
		} else {
			inner[4] = styles.DimText.Render("Clipboard clears in 60 seconds")
		}
	}
	inner[rowsHeight+2] = ""
	inner[rowsHeight+3] = styles.DimText.Render("esc back to settings")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.BitcoinOrange).
		Padding(0, 1).
		Width(m.backupTray.contentWidth + 4).
		Render(strings.Join(inner, "\n"))
}
