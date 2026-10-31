package models

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/styles"
)

// Fee screen column widths. The bar grows on wide terminals; every other
// column is fixed so loading and refresh states never move the values.
const (
	feesLabelWidth = 10
	feesRateWidth  = 12
	feesETAWidth   = 7
	feesRowGap     = 2
	feesLabelGap   = 3
	feesMaxBar     = 24
	feesBarOffset  = 60 // leaves a 20-cell bar at 80 columns
)

// FeesMsg carries a fee estimate refresh result.
type FeesMsg struct {
	Estimates *api.FeeEstimates
	Err       error
}

type feesAgeTickMsg time.Time

// FeesModel shows the fee targets reported by the selected backend. It reads
// cached estimates first and only asks the wallet for new estimates; it never
// talks to a server directly.
type FeesModel struct {
	width  int
	height int

	walletManager WalletService

	estimates  *api.FeeEstimates
	refreshing bool
	err        error
	updatedAt  time.Time
	serverName string

	spinner  spinner.Model
	isActive bool
}

// NewFeesModel creates the fee screen and seeds it with cached estimates so
// the first frame already has values.
func NewFeesModel(walletManager WalletService, configManager *config.ConfigManager) FeesModel {
	m := FeesModel{
		width:         80,
		height:        24,
		walletManager: walletManager,
		spinner:       styles.NewStandardSpinner(),
		serverName:    feesServerName(configManager),
		refreshing:    true,
		isActive:      true,
	}
	if walletManager != nil {
		if cached, ok := walletManager.GetCachedFeeEstimates(); ok && cached != nil {
			m.estimates = cached
			m.updatedAt = time.Now()
		}
	}
	return m
}

func (m FeesModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchFees(m.walletManager))
}

func fetchFees(wm WalletService) tea.Cmd {
	return func() tea.Msg {
		if wm == nil {
			return FeesMsg{Err: fmt.Errorf("no wallet backend")}
		}
		estimates, err := wm.GetFeeEstimates()
		return FeesMsg{Estimates: estimates, Err: err}
	}
}

func (m FeesModel) Update(msg tea.Msg) (screenModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ScreenActiveMsg:
		m.isActive = true
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case FeesMsg:
		m.refreshing = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, m.ageTick()
		}
		m.err = nil
		if msg.Estimates != nil {
			m.estimates = msg.Estimates
			m.updatedAt = time.Now()
		}
		return m, m.ageTick()

	case spinner.TickMsg:
		if !m.refreshing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case feesAgeTickMsg:
		if !m.isActive {
			return m, nil
		}
		return m, m.ageTick()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			m.isActive = false
			return m, Navigate(StateMainDashboard)
		case "r", "R":
			m.refreshing = true
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, fetchFees(m.walletManager))
		}
	}
	return m, nil
}

func (m FeesModel) ageTick() tea.Cmd {
	if !m.isActive {
		return nil
	}
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return feesAgeTickMsg(t)
	})
}

func (m FeesModel) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}

	barWidth := feesBarWidth(width)
	maxRate := feesMaxRate(m.estimates)

	rows := []struct {
		label string
		rate  float64
		eta   string
	}{
		{"Next block", feeRate(m.estimates, func(f *api.FeeEstimates) float64 { return f.FastestFee }), "~10 min"},
		{"30 minutes", feeRate(m.estimates, func(f *api.FeeEstimates) float64 { return f.HalfHourFee }), "~30 min"},
		{"1 hour", feeRate(m.estimates, func(f *api.FeeEstimates) float64 { return f.HourFee }), "~1 hour"},
		{"1 day", feeRate(m.estimates, func(f *api.FeeEstimates) float64 { return f.EconomyFee }), "~1 day"},
		{"Minimum", feeRate(m.estimates, func(f *api.FeeEstimates) float64 { return f.MinimumFee }), "~1 week"},
	}

	lines := []string{
		renderFeesHeader(width, m.serverName),
		"",
	}
	for _, row := range rows {
		lines = append(lines, renderFeeRow(row.label, row.rate, row.eta, maxRate, barWidth))
	}
	lines = append(lines,
		"",
		m.renderStatus(width),
		styles.RenderHelp(
			styles.HK("r", "refresh"),
			styles.HK("esc", "back"),
		),
	)

	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// renderFeesHeader puts the screen title on the left and the selected server
// label, dimmed, against the right edge.
func renderFeesHeader(width int, server string) string {
	left := "Fees"
	right := ""
	server = sanitizeLine(strings.TrimSpace(server))
	if server != "" {
		right = "from " + server
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		maxRight := width - lipgloss.Width(left) - 1
		if maxRight > 0 {
			right = truncateRunes(right, maxRight, "…")
		} else {
			left = truncateRunes(left, width, "…")
			right = ""
		}
		gap = width - lipgloss.Width(left) - lipgloss.Width(right)
	}
	if gap < 1 {
		gap = 1
	}

	return styles.ScreenHeaderStyle.Render(left) +
		strings.Repeat(" ", gap) +
		styles.DimText.Render(right)
}

func renderFeeRow(label string, rate float64, eta string, maxRate float64, barWidth int) string {
	bar := renderFeeBar(rate, maxRate, barWidth)
	rateCell := styles.HighlightText.Render(formatFeeRateCell(rate))
	etaCell := eta
	if rate <= 0 {
		rateCell = styles.DimText.Render(formatFeeRateCell(rate))
		etaCell = "—"
	}

	return styles.NormalText.Render(padCell(label, feesLabelWidth)) +
		strings.Repeat(" ", feesLabelGap) +
		styles.HighlightText.Render(bar) +
		strings.Repeat(" ", feesRowGap) +
		rateCell +
		strings.Repeat(" ", feesRowGap) +
		styles.DimText.Render(padCell(etaCell, feesETAWidth))
}

func renderFeeBar(rate, maxRate float64, width int) string {
	if width < 1 {
		width = 1
	}
	if rate <= 0 || maxRate <= 0 {
		return strings.Repeat(" ", width)
	}
	cells := int(math.Round(rate / maxRate * float64(width)))
	if cells < 1 {
		cells = 1
	}
	if cells > width {
		cells = width
	}
	return strings.Repeat("█", cells) + strings.Repeat(" ", width-cells)
}

func formatFeeRateCell(rate float64) string {
	if rate <= 0 {
		return padLeftCell("—", feesRateWidth)
	}
	return fmt.Sprintf("%*.1f sat/vB", feesRateWidth-len(" sat/vB"), rate)
}

func feesBarWidth(width int) int {
	bar := width - feesBarOffset
	if bar > feesMaxBar {
		bar = feesMaxBar
	}
	if bar < 1 {
		bar = 1
	}
	return bar
}

func feesMaxRate(fees *api.FeeEstimates) float64 {
	if fees == nil {
		return 0
	}
	maxRate := 0.0
	for _, rate := range []float64{
		fees.FastestFee,
		fees.HalfHourFee,
		fees.HourFee,
		fees.EconomyFee,
		fees.MinimumFee,
	} {
		if rate > maxRate {
			maxRate = rate
		}
	}
	return maxRate
}

func feeRate(fees *api.FeeEstimates, pick func(*api.FeeEstimates) float64) float64 {
	if fees == nil {
		return 0
	}
	return pick(fees)
}

func (m FeesModel) renderStatus(width int) string {
	switch {
	case m.refreshing:
		return m.spinner.View() + " " + styles.DimText.Render("refreshing…")
	case m.err != nil:
		return styles.ErrorText.Render(truncateRunes(sanitizeLine(m.err.Error()), width, "…"))
	case m.estimates != nil:
		return styles.DimText.Render(feesUpdatedLabel(time.Now(), m.updatedAt))
	default:
		return styles.DimText.Render("no estimates yet")
	}
}

func feesUpdatedLabel(now, updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return "updated just now"
	}
	age := now.Sub(updatedAt)
	switch {
	case age < time.Second:
		return "updated just now"
	case age < time.Minute:
		return fmt.Sprintf("updated %ds ago", int(age/time.Second))
	case age < time.Hour:
		return fmt.Sprintf("updated %dm ago", int(age/time.Minute))
	default:
		return fmt.Sprintf("updated %dh ago", int(age/time.Hour))
	}
}

// feesServerName returns the selected server host, or the backend name when
// no host is configured. Credentials are stripped before display.
func feesServerName(configManager *config.ConfigManager) string {
	if configManager != nil {
		if cfg, err := configManager.Load(); err == nil && cfg != nil && cfg.Backend != nil {
			switch cfg.Backend.Type {
			case backend.BackendTypeElectrum:
				if cfg.Backend.Electrum != nil {
					if host := sanitizeServerLabel(cfg.Backend.Electrum.Host); host != "" {
						return host
					}
				}
				return "Electrum"
			case backend.BackendTypeRPC:
				if cfg.Backend.RPC != nil {
					if host := sanitizeServerLabel(cfg.Backend.RPC.Host); host != "" {
						return host
					}
				}
				return "Bitcoin Core"
			}
		}
	}

	return "selected backend"
}

// sanitizeServerLabel keeps only the host part of a server value. It removes
// URL userinfo and any path, query, or fragment so credentials never reach
// the screen.
func sanitizeServerLabel(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		if u, err := url.Parse(value); err == nil && u.Host != "" {
			value = u.Host
		}
	}
	if at := strings.LastIndex(value, "@"); at >= 0 {
		value = value[at+1:]
	}
	if cut := strings.IndexAny(value, "/?#"); cut >= 0 {
		value = value[:cut]
	}
	value = strings.TrimPrefix(strings.TrimSpace(value), "//")
	return sanitizeLine(value)
}
