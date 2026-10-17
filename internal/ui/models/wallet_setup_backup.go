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
	if m.backupPos >= len(m.backupWords) {
		return styles.CenterHorizontal(header, m.width)
	}
	pos := m.backupWords[m.backupPos]
	progress := styles.DimText.Render(fmt.Sprintf("%d of %d", m.backupPos+1, len(m.backupWords)))
	prompt := styles.NormalText.Render(fmt.Sprintf("Enter word #%d:", pos+1))
	content := header + "  " + progress + "\n\n" + prompt + "\n" + m.backupInput.View()
	help := styles.RenderHelp(styles.HK("enter", "check"), styles.HK("esc", "show seed"), styles.HK("ctrl+s", "skip"))
	return styles.CenterHorizontal(content+"\n\n"+help, m.width)
}
