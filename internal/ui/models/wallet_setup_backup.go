package models

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// backupCheckWords is how many seed words the user re-enters after creating
// a wallet.
const backupCheckWords = 3

// startBackupCheck picks distinct random word positions and resets the input.
func (m *WalletSetupModel) startBackupCheck() {
	words := strings.Fields(m.generatedMnemonic)
	m.backupWords = pickWordPositions(len(words), backupCheckWords)
	m.backupPos = 0
	m.backupSkipConfirm = false
	m.err = nil
	in := textinput.New()
	in.Placeholder = "word"
	in.CharLimit = 16
	in.Width = 20
	in.Focus()
	m.backupInput = in
}

// pickWordPositions returns k distinct positions in [0, n), ascending.
func pickWordPositions(n, k int) []int {
	if k > n {
		k = n
	}
	seen := make(map[int]bool, k)
	out := make([]int, 0, k)
	for len(out) < k {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			// crypto/rand does not fail on supported platforms; fall back to
			// the first unused position rather than skipping the check.
			for i := 0; i < n; i++ {
				if !seen[i] {
					r = big.NewInt(int64(i))
					break
				}
			}
		}
		i := int(r.Int64())
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	sort.Ints(out)
	return out
}

func (m WalletSetupModel) handleConfirmMnemonic(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.backupSkipConfirm {
		switch msg.String() {
		case "y", "Y":
			m.backupSkipConfirm = false
			return m.finishCreate()
		case "n", "N":
			m.backupSkipConfirm = false
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+s":
		m.backupSkipConfirm = true
		return m, nil
	case "enter":
		words := strings.Fields(m.generatedMnemonic)
		if m.backupPos >= len(m.backupWords) {
			return m.finishCreate()
		}
		want := words[m.backupWords[m.backupPos]]
		got := strings.ToLower(strings.TrimSpace(m.backupInput.Value()))
		if got != want {
			m.err = fmt.Errorf("that is not word #%d; check your backup or press esc to see the seed again", m.backupWords[m.backupPos]+1)
			m.backupInput.SetValue("")
			return m, nil
		}
		m.err = nil
		m.backupInput.SetValue("")
		m.backupPos++
		if m.backupPos >= len(m.backupWords) {
			return m.finishCreate()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.backupInput, cmd = m.backupInput.Update(msg)
	return m, cmd
}

// finishCreate unlocks the newly created wallet, adds its first account, and
// opens the dashboard.
func (m WalletSetupModel) finishCreate() (tea.Model, tea.Cmd) {
	m.backupWords = nil
	m.backupInput.SetValue("")
	if err := m.walletManager.UnlockWallet(m.walletName, m.password); err != nil {
		m.err = err
		return m, nil
	}
	if _, err := m.walletManager.CreateAccount("Main Account", m.selectedScriptType, m.customDerivationPath); err != nil {
		m.err = err
		return m, nil
	}
	m.saveLastWalletName(m.walletName)
	m.flowComplete = true
	m.wipeSecrets()
	return m, func() tea.Msg { return NavigateMsg{Target: StateMainDashboard} }
}

func (m WalletSetupModel) viewConfirmMnemonic() string {
	header := styles.TitleStyle.Render("Check your backup")
	if m.backupSkipConfirm {
		body := styles.WarningText.Render("Skip the backup check?") + "\n\n" +
			styles.NormalText.Render("Only skip if your seed phrase is written down.\nWithout it, funds cannot be recovered if this device is lost.")
		help := styles.RenderHelp(styles.HK("y", "skip"), styles.HK("n", "check words"), styles.HK("esc", "cancel"))
		return styles.CenterHorizontal(header+"\n\n"+body+"\n\n"+help, m.width)
	}

	words := strings.Fields(m.generatedMnemonic)
	errorSample := fmt.Sprintf("Error: that is not word #%d; check your backup or press esc to see the seed again", len(words))
	errorHeight := lipgloss.Height(styles.RenderErrorBox(errorSample, m.width))
	errorSlot := lipgloss.NewStyle().Height(errorHeight).Render("")
	if m.err != nil {
		errorSlot = lipgloss.NewStyle().
			Height(errorHeight).
			MaxHeight(errorHeight).
			Render(styles.RenderErrorBox("Error: "+m.err.Error(), m.width))
	}

	if m.backupPos >= len(m.backupWords) {
		return styles.CenterHorizontal(header+"\n\n"+errorSlot, m.width)
	}
	pos := m.backupWords[m.backupPos]
	progressWidth := len(fmt.Sprintf("%d of %d", len(m.backupWords), len(m.backupWords)))
	progress := styles.DimText.Width(progressWidth).Render(fmt.Sprintf("%d of %d", m.backupPos+1, len(m.backupWords)))
	promptWidth := lipgloss.Width(styles.NormalText.Render(fmt.Sprintf("Enter word #%d:", len(words))))
	prompt := styles.NormalText.Width(promptWidth).Render(fmt.Sprintf("Enter word #%d:", pos+1))
	titleLine := lipgloss.JoinHorizontal(lipgloss.Center, header, "  ", progress)
	inputBlock := lipgloss.NewStyle().Width(23).Render(prompt + "\n" + m.backupInput.View())
	content := titleLine + "\n\n" + inputBlock
	help := styles.RenderHelp(styles.HK("enter", "check"), styles.HK("esc", "show seed"), styles.HK("ctrl+s", "skip"))
	return styles.CenterHorizontal(content+"\n\n"+help+"\n\n"+errorSlot, m.width)
}
