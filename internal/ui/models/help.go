package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/ui/styles"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

type HelpModel struct {
	width    int
	height   int
	viewport viewport.Model
	ready    bool
	wallet   WalletService

	// Animation
	fadeIn       float64
	fadeVelocity float64
	fadeSpring   harmonica.Spring

	// Screen visibility - controls whether tickers run
	isActive bool
}

type helpWalletMode int

const (
	helpWalletGeneric helpWalletMode = iota
	helpWalletSingleSig
	helpWalletWatchOnly
	helpWalletHardware
	helpWalletMultisig
)

func NewHelpModel(wallet WalletService) HelpModel {
	vp := viewport.New(80, 20)
	vp.MouseWheelDelta = 1

	model := HelpModel{
		fadeSpring: harmonica.NewSpring(harmonica.FPS(60), 4.0, 0.7),
		fadeIn:     0.0,
		viewport:   vp,
		ready:      true,
		wallet:     wallet,
	}

	model.viewport.SetContent(model.renderHelpContent())

	return model
}

func (m HelpModel) Init() tea.Cmd {
	m.isActive = true
	return m.animateTick()
}

func (m HelpModel) animateTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m HelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ScreenActiveMsg:
		m.isActive = true
		m.viewport.SetContent(m.renderHelpContent())
		if m.fadeIn < 0.99 {
			return m, m.animateTick()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Keep viewport content width inside bordered container frame:
		// border(2) + padding left/right (4) = 6 columns.
		contentWidth := msg.Width - 8
		if contentWidth < 20 {
			contentWidth = 20
		}
		m.viewport.Width = contentWidth
		m.viewport.Height = msg.Height - 8
		m.viewport.SetContent(m.renderHelpContent())

	case TickMsg:
		if !m.isActive {
			return m, nil
		}

		if m.fadeIn < 1.0 {
			m.fadeIn, m.fadeVelocity = m.fadeSpring.Update(
				m.fadeIn, m.fadeVelocity, 1.0,
			)
		}

		if m.fadeIn < 0.99 {
			cmds = append(cmds, m.animateTick())
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "?":
			m.isActive = false
			return m, Navigate(StateMainDashboard)
		}
	}

	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m HelpModel) View() string {
	header := styles.ScreenHeaderStyle.Render("Help")

	scrollInfo := ""
	if m.viewport.TotalLineCount() > m.viewport.Height {
		scrollPct := int(m.viewport.ScrollPercent() * 100)
		scrollInfo = styles.ScrollIndicatorStyle.Render(fmt.Sprintf("  %d%%", scrollPct))
	}

	viewportStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.DarkGray).
		Padding(1, 2)
	boxWidth := m.width - 2
	if boxWidth < 24 {
		boxWidth = 24
	}
	viewportStyle = viewportStyle.Width(boxWidth)

	content := viewportStyle.Render(m.viewport.View())

	footer := styles.RenderHelp(
		styles.HK("\u2191/\u2193", "scroll"),
		styles.HK("esc", "back"),
	)

	return header + scrollInfo + "\n\n" + content + "\n" + footer
}

func (m HelpModel) renderHelpContent() string {
	var sections []string

	sections = append(sections, m.renderGettingStarted())
	sections = append(sections, m.renderKeyboardShortcuts())
	sections = append(sections, m.renderSendFlow())
	sections = append(sections, m.renderSecurityWarnings())
	sections = append(sections, m.renderBackupRecovery())
	sections = append(sections, m.renderTroubleshooting())

	return strings.Join(sections, "\n\n")
}

// ---------------------------------------------------------------------------
// Sections
// ---------------------------------------------------------------------------

func (m HelpModel) renderGettingStarted() string {
	title := styles.SectionTitleStyle.Render("GETTING STARTED")

	var steps []string
	switch m.walletMode() {
	case helpWalletMultisig:
		steps = []string{
			"1. Create or import the multisig wallet and confirm the M-of-N policy",
			"2. Back up the descriptor and enough cosigner backups to satisfy the quorum",
			"3. Review cosigners in Settings (t) then (i); export Coldcard config with (w) when needed",
			"4. Use fresh receive addresses and sign sends through the PSBT workflow",
		}
	case helpWalletHardware:
		steps = []string{
			"1. Connect and unlock the hardware wallet, then open this wallet",
			"2. Back up the hardware wallet recovery seed and any passphrase before funding it",
			"3. Generate a receive address (r); use (v) to verify it on the device",
			"4. Sign sends through Sign & Broadcast using HWI, QR, or file import/export",
		}
	case helpWalletWatchOnly:
		steps = []string{
			"1. Open the watch-only wallet and confirm the descriptor/xpub matches your signer",
			"2. Back up the descriptor/xpub and the external signer that actually controls the funds",
			"3. Generate a receive address (r) to accept bitcoin",
			"4. Use Settings (t) then (i) to import the matching seed for local signing",
		}
	default:
		steps = []string{
			"1. Create or restore a wallet with your password",
			"2. Back up your seed phrase immediately - Settings (t) then (m)",
			"3. Generate a receive address (r) to accept bitcoin",
			"4. Monitor balance, history, addresses, and UTXOs from the dashboard",
		}
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	for _, step := range steps {
		lines = append(lines, "  "+styles.NormalText.Render(step))
	}

	return strings.Join(lines, "\n")
}

func (m HelpModel) renderKeyboardShortcuts() string {
	title := styles.SectionTitleStyle.Render("KEYBOARD SHORTCUTS")

	var lines []string
	lines = append(lines, title)

	// Navigation
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  Navigation"))
	lines = append(lines, m.shortcut("esc", "Go back / Cancel"))
	lines = append(lines, m.shortcut("q", "Back on many screens; some dialogs use esc only; quit from dashboard"))
	lines = append(lines, m.shortcut("?", "Open help from dashboard / close help"))
	lines = append(lines, m.shortcut("ctrl+c", "Quit application"))

	// Dashboard
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  Dashboard"))
	lines = append(lines, m.shortcut("\u2191/\u2193", "Navigate menu"))
	lines = append(lines, m.shortcut("enter", "Open selected screen"))
	lines = append(lines, m.shortcut("r", "Receive bitcoin"))
	lines = append(lines, m.shortcut("s", "Send bitcoin"))
	lines = append(lines, m.shortcut("h", "Transaction history"))
	lines = append(lines, m.shortcut("a", "Addresses"))
	lines = append(lines, m.shortcut("u", "UTXOs"))
	lines = append(lines, m.shortcut("t", "Settings"))
	lines = append(lines, m.shortcut("T", "Re-trust a changed server key (when shown)"))

	// Receive
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  Receive"))
	lines = append(lines, m.shortcut("esc/q", "Back"))
	lines = append(lines, m.shortcut("n", "Generate new address"))
	lines = append(lines, m.shortcut("c", "Copy address to clipboard"))
	lines = append(lines, m.shortcut("\u2190/\u2192", "Navigate addresses"))
	if m.walletMode() == helpWalletHardware {
		lines = append(lines, m.shortcut("v", "Verify address on hardware device"))
	}

	// History
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  History"))
	lines = append(lines, m.shortcut("esc/q", "Back"))
	lines = append(lines, m.shortcut("enter", "Open selected transaction in browser"))
	lines = append(lines, m.shortcut("f/x", "Cycle type/status filters"))
	lines = append(lines, m.shortcut("/", "Search transactions"))
	lines = append(lines, m.shortcut("e", "Edit transaction label"))
	lines = append(lines, m.shortcut("E", "Export labels (BIP-329)"))
	lines = append(lines, m.shortcut("K", "Export Koinly CSV"))
	lines = append(lines, m.shortcut("c", "Copy transaction ID"))
	lines = append(lines, m.shortcut("b", "Speed up a pending transaction (RBF or CPFP)"))
	lines = append(lines, m.shortcut("1-6", "Sort by column"))
	lines = append(lines, m.shortcut("[/] or \u2190/\u2192", "Page navigation"))
	lines = append(lines, m.shortcut("+/-", "Resize current sorted column"))

	// Addresses
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  Addresses"))
	lines = append(lines, m.shortcut("esc/q", "Back"))
	lines = append(lines, m.shortcut("\u2191/\u2193", "Navigate rows"))
	lines = append(lines, m.shortcut("PgUp/PgDn", "Scroll faster"))
	lines = append(lines, m.shortcut("tab", "Toggle receive / change"))
	lines = append(lines, m.shortcut("c", "Copy address"))
	lines = append(lines, m.shortcut("e", "Edit address label"))
	lines = append(lines, m.shortcut("r", "Refresh"))

	// UTXOs
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  UTXOs"))
	lines = append(lines, m.shortcut("esc/q", "Back"))
	lines = append(lines, m.shortcut("\u2191/\u2193", "Navigate rows"))
	lines = append(lines, m.shortcut("PgUp/PgDn", "Scroll faster"))
	lines = append(lines, m.shortcut("\u2190/\u2192", "Page navigation"))
	lines = append(lines, m.shortcut("c", "Copy outpoint (txid:vout)"))
	lines = append(lines, m.shortcut("e", "Edit UTXO label"))
	lines = append(lines, m.shortcut("r", "Refresh"))

	// Settings
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  Settings"))
	lines = append(lines, m.shortcut("esc/q", "Back"))
	lines = append(lines, m.shortcut("b", "Backend / connection settings"))
	if m.walletMode() != helpWalletMultisig {
		lines = append(lines, m.shortcut("c", "Copy xpub"))
	}
	lines = append(lines, m.shortcut("d", "Open descriptor screen"))
	lines = append(lines, m.shortcut("u", "Toggle display unit"))
	lines = append(lines, m.shortcut("g", "Cycle address gap limit"))
	switch m.walletMode() {
	case helpWalletMultisig:
		lines = append(lines, m.shortcut("i", "Manage cosigners / import cosigner seeds"))
		lines = append(lines, m.shortcut("w", "Export Coldcard multisig config"))
		lines = append(lines, m.shortcut("1-N / s", "Cosigner list edit / scan local wallets"))
	case helpWalletHardware, helpWalletWatchOnly:
		lines = append(lines, m.shortcut("i", "Import matching seed for local signing"))
	default:
		lines = append(lines, m.shortcut("m", "Reveal seed phrase after warning"))
	}
	lines = append(lines, m.shortcut("descriptor: c/e/esc", "Copy / export / back"))

	return strings.Join(lines, "\n")
}

func (m HelpModel) renderSendFlow() string {
	title := styles.SectionTitleStyle.Render("SENDING BITCOIN")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	stepStyle := lipgloss.NewStyle().Foreground(styles.BitcoinOrange).Bold(true)

	steps := []struct{ step, desc string }{
		{"Step 1", "Enter recipient address (ctrl+v to paste, i to import PSBT)"},
		{"Step 2", "Enter amount (tab toggles BTC/sats, m sends all funds)"},
		{"Step 3", "Review recipients (\u2191/\u2193 to select, a/+ adds, d/delete/backspace removes)"},
		{"Step 4", "Select UTXOs (space/enter toggles, a selects all, n clears, tab continues)"},
		{"Step 5", "Choose fee rate (\u2191/\u2193 selects a preset, c opens custom sat/vB input)"},
		{"Step 6", "Review the previewed transaction, then enter opens Sign & Broadcast"},
	}

	for _, s := range steps {
		lines = append(lines, fmt.Sprintf("  %s  %s",
			stepStyle.Render(s.step),
			styles.NormalText.Render(s.desc)))
	}

	// Sign & Broadcast
	lines = append(lines, "")
	lines = append(lines, styles.SectionSubheaderStyle.Render("  Sign & Broadcast (PSBT)"))
	lines = append(lines, m.shortcut("s", "Sign with local wallet keys when available"))
	lines = append(lines, m.shortcut("h", "Enumerate HWI devices and sign with hardware wallet"))
	lines = append(lines, m.shortcut("f", "Save PSBT to file"))
	lines = append(lines, m.shortcut("i", "Import signed PSBT from file"))
	lines = append(lines, m.shortcut("u", "Paste signed UR"))
	lines = append(lines, m.shortcut("c", "Copy PSBT to clipboard"))
	lines = append(lines, m.shortcut("q", "Display QR code (BC-UR / BBQr)"))
	lines = append(lines, m.shortcut("b", "Broadcast once the PSBT is fully signed"))
	lines = append(lines, m.shortcut("file browser: \u2191/\u2193 enter p r", "Navigate / select / manual path / refresh"))
	lines = append(lines, m.shortcut("QR view: 1/2 esc", "Switch BC-UR / BBQr / back"))
	lines = append(lines, m.shortcut("path or UR input: enter/esc", "Confirm import/export or cancel"))

	return strings.Join(lines, "\n")
}

func (m HelpModel) renderSecurityWarnings() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.PastelPink).
		Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render("SECURITY"))
	lines = append(lines, "")

	doStyle := lipgloss.NewStyle().Foreground(styles.PastelGreen).Bold(true)
	dontStyle := lipgloss.NewStyle().Foreground(styles.PastelPink).Bold(true)

	dos := []string{
		m.primarySecurityDo(),
		"Use a strong, unique password for wallet encryption",
		"Verify recipient addresses carefully before sending",
		"Generate a new address for each transaction you receive",
	}

	donts := []string{
		"Never share your recovery material with anyone",
		"Never store recovery material digitally or in the cloud",
		"Never enter recovery material on websites or untrusted apps",
		"Avoid reusing addresses - it links your transactions publicly",
	}

	for _, d := range dos {
		lines = append(lines, "  "+doStyle.Render("DO")+"    "+styles.NormalText.Render(d))
	}
	lines = append(lines, "")
	for _, d := range donts {
		lines = append(lines, "  "+dontStyle.Render("DON'T")+" "+styles.NormalText.Render(d))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.PastelPink).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (m HelpModel) renderBackupRecovery() string {
	title := styles.SectionTitleStyle.Render("BACKUP & RECOVERY")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	for _, line := range m.backupIntroLines() {
		lines = append(lines, "  "+styles.NormalText.Render(line))
	}
	lines = append(lines, "")

	keyStyle := lipgloss.NewStyle().Foreground(styles.PastelOrange).Width(24)

	recovery := m.recoveryScenarios()

	for _, r := range recovery {
		lines = append(lines, fmt.Sprintf("  %s %s",
			keyStyle.Render(r.problem),
			styles.DimText.Render(r.solution)))
	}

	return strings.Join(lines, "\n")
}

func (m HelpModel) renderTroubleshooting() string {
	title := styles.SectionTitleStyle.Render("TROUBLESHOOTING")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	keyStyle := lipgloss.NewStyle().Foreground(styles.PastelOrange).Width(24)

	issues := []struct{ problem, solution string }{
		{"Balance/history stale", "Use the screen refresh action where available (for example r in Addresses or UTXOs)"},
		{"Transaction missing", "Clear History search/filters (/ f x) and wait for backend sync to catch up"},
		{"Connection failed", "Open Settings (t) then b to review backend/server settings"},
		{"Cannot sign", m.signingTroubleshooting()},
		{"Debug logging", "Run with DEBUG=1 environment variable"},
	}

	for _, issue := range issues {
		lines = append(lines, fmt.Sprintf("  %s %s",
			keyStyle.Render(issue.problem),
			styles.DimText.Render(issue.solution)))
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m HelpModel) shortcut(key, desc string) string {
	return fmt.Sprintf("    %s  %s",
		styles.ShortcutKeyStyle.Render(key),
		styles.DimText.Render(desc))
}

func (m HelpModel) walletMode() helpWalletMode {
	if m.wallet == nil {
		return helpWalletGeneric
	}
	if m.wallet.IsMultisig() {
		return helpWalletMultisig
	}
	if m.wallet.IsHardwareWallet() {
		return helpWalletHardware
	}
	if m.wallet.IsWatchOnly() {
		return helpWalletWatchOnly
	}
	return helpWalletSingleSig
}

func (m HelpModel) primarySecurityDo() string {
	switch m.walletMode() {
	case helpWalletMultisig:
		return "Back up the wallet descriptor and enough cosigner backups to meet the quorum"
	case helpWalletHardware:
		return "Record the hardware wallet recovery seed and any passphrase separately and securely"
	case helpWalletWatchOnly:
		return "Back up the descriptor/xpub and the signer backup that actually controls the funds"
	default:
		return "Write down your seed phrase on paper and store it securely"
	}
}

func (m HelpModel) backupIntroLines() []string {
	switch m.walletMode() {
	case helpWalletMultisig:
		return []string{
			"No single seed phrase restores this wallet.",
			"Recovery requires the wallet descriptor plus enough cosigner seeds/devices to satisfy the M-of-N policy.",
			"Your password only encrypts the local wallet file and any cosigner seeds imported into Hax.",
		}
	case helpWalletHardware:
		return []string{
			"This is a hardware-backed watch-only wallet.",
			"Hax stores the xpub, descriptor, and hardware metadata; the hardware wallet recovery seed remains the spending backup.",
			"Importing the matching seed in Settings (t) then (i) converts it to local signing.",
		}
	case helpWalletWatchOnly:
		return []string{
			"This is a watch-only wallet.",
			"Spending access comes from the external seed or signer that matches this wallet's xpub/descriptor.",
			"Your password only encrypts the local watch-only file.",
		}
	default:
		return []string{
			"Your 12 or 24-word seed phrase is the master key to this wallet.",
			"It works with BIP39-compatible recovery flows for spending access.",
			"Your password only encrypts the local wallet file.",
		}
	}
}

func (m HelpModel) recoveryScenarios() []struct{ problem, solution string } {
	switch m.walletMode() {
	case helpWalletMultisig:
		return []struct{ problem, solution string }{
			{"Lost password", "Restore from descriptor plus enough cosigner backups to meet the quorum"},
			{"One signer unavailable", "You can still spend if the remaining signers satisfy the M-of-N policy"},
			{"Cosigner maintenance", "Use Settings (t) then (i) to review/import cosigner data"},
			{"Coldcard export", "Use Settings (t) then (w) to write the multisig config file"},
		}
	case helpWalletHardware:
		return []struct{ problem, solution string }{
			{"Lost password", "Recreate the watch-only wallet from descriptor/xpub; password only protects the local file"},
			{"Replace device", "Restore the hardware wallet from its recovery seed and passphrase"},
			{"Need local signing", "Use Settings (t) then (i) to import the matching seed"},
			{"Need descriptor", "Use Settings (t) then (d) to open the descriptor export screen"},
		}
	case helpWalletWatchOnly:
		return []struct{ problem, solution string }{
			{"Need signing", "Use Settings (t) then (i) to import the matching seed"},
			{"Lost password", "Recreate the watch-only wallet from descriptor/xpub; password only protects the local file"},
			{"Need descriptor", "Use Settings (t) then (d) to open the descriptor export screen"},
			{"Lost external seed", "Funds are only recoverable from the original signer backup"},
		}
	default:
		return []struct{ problem, solution string }{
			{"Lost password", "Restore wallet from the seed phrase"},
			{"Compromised wallet", "Create a new wallet and move funds immediately"},
			{"Need descriptor", "Use Settings (t) then (d) to open the descriptor export screen"},
			{"Lost seed phrase", "View it in Settings (t) then (m) while the wallet is unlocked"},
		}
	}
}

func (m HelpModel) signingTroubleshooting() string {
	switch m.walletMode() {
	case helpWalletMultisig:
		return "Collect enough local, HWI, QR, or file-based PSBT signatures before broadcast"
	case helpWalletHardware:
		return "Use Sign & Broadcast with h/q/f/i/u, or import the matching seed in Settings (t) then (i)"
	case helpWalletWatchOnly:
		return "Use PSBT export/import in Sign & Broadcast, or import the matching seed in Settings (t) then (i)"
	default:
		return "Use Sign & Broadcast; s signs locally, and h/q/f/i/u cover external signers"
	}
}
