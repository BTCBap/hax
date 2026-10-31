package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
)

func feesTestModel(t *testing.T) FeesModel {
	t.Helper()
	mock := NewMockWalletManager()
	mock.GetCachedFeeEstimatesFn = func() (*api.FeeEstimates, bool) { return nil, false }
	mock.GetFeeEstimatesFn = func() (*api.FeeEstimates, error) {
		return &api.FeeEstimates{
			FastestFee:  12,
			HalfHourFee: 8,
			HourFee:     3,
			EconomyFee:  1,
			MinimumFee:  0.5,
		}, nil
	}
	m := NewFeesModel(mock, nil)
	m.width, m.height = 80, 24
	m.serverName = "electrum.example.com"
	m.estimates = &api.FeeEstimates{
		FastestFee:  12,
		HalfHourFee: 8,
		HourFee:     3,
		EconomyFee:  1,
		MinimumFee:  0.5,
	}
	m.refreshing = false
	m.err = nil
	m.updatedAt = time.Now().Add(-12 * time.Second)
	return m
}

func TestFees_RendersAllTargets(t *testing.T) {
	m := feesTestModel(t)
	view := stripANSI(m.View())

	for _, text := range []string{
		"Fees", "from electrum.example.com",
		"Next block", "30 minutes", "1 hour", "1 day", "Minimum",
		"12.0 sat/vB", "8.0 sat/vB", "3.0 sat/vB", "1.0 sat/vB", "0.5 sat/vB",
		"~10 min", "~30 min", "~1 hour", "~1 day", "~1 week",
		"updated 12s ago", "r refresh • esc back",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("fees view is missing %q:\n%s", text, view)
		}
	}
}

func TestFees_MissingRatesShowDash(t *testing.T) {
	m := feesTestModel(t)
	m.estimates = &api.FeeEstimates{FastestFee: 12}
	view := stripANSI(m.View())
	if strings.Count(view, "—") < 8 {
		t.Fatalf("expected dashes for four missing rate and ETA cells:\n%s", view)
	}
	if !strings.Contains(view, "12.0 sat/vB") {
		t.Fatalf("available rate should still render:\n%s", view)
	}
}

func TestFees_LayoutStableAcrossStates(t *testing.T) {
	base := feesTestModel(t)
	baseView := base.View()
	wantLines := requireLineCount(t, baseView)

	labels := []string{"Next block", "30 minutes", "1 hour", "1 day", "Minimum"}
	wantPos := make([]viewPosition, len(labels))
	for i, label := range labels {
		wantPos[i] = requireTextPosition(t, baseView, label)
	}
	wantRate := requireTextPosition(t, baseView, "12.0 sat/vB")
	wantHint := requireTextPosition(t, baseView, "r refresh")

	states := []struct {
		name string
		view string
	}{
		{"loading", func() string {
			m := base
			m.refreshing = true
			m.estimates = nil
			return m.View()
		}()},
		{"refreshing with cached", func() string {
			m := base
			m.refreshing = true
			return m.View()
		}()},
		{"refreshed", baseView},
		{"error", func() string {
			m := base
			m.refreshing = false
			m.err = fmt.Errorf("backend unavailable")
			return m.View()
		}()},
	}

	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			if got := requireLineCount(t, state.view); got != wantLines {
				t.Fatalf("%d lines, want %d", got, wantLines)
			}
			for i, label := range labels {
				if got := requireTextPosition(t, state.view, label); got != wantPos[i] {
					t.Fatalf("%q moved from %+v to %+v", label, wantPos[i], got)
				}
			}
			if got := requireTextPosition(t, state.view, "r refresh"); got != wantHint {
				t.Fatalf("hint moved from %+v to %+v", wantHint, got)
			}
			if state.name == "refreshed" || state.name == "error" || state.name == "refreshing with cached" {
				if got := requireTextPosition(t, state.view, "12.0 sat/vB"); got != wantRate {
					t.Fatalf("rate column moved from %+v to %+v", wantRate, got)
				}
			}
		})
	}
}

func TestFees_StatusLineSameRow(t *testing.T) {
	m := feesTestModel(t)
	refreshed := m.View()
	m.refreshing = true
	refreshing := m.View()
	m.refreshing = false
	m.err = fmt.Errorf("backend unavailable")
	failed := m.View()

	positions := []viewPosition{
		requireTextPosition(t, refreshed, "updated 12s ago"),
		requireTextPosition(t, refreshing, "refreshing…"),
		requireTextPosition(t, failed, "backend unavailable"),
	}
	for i := 1; i < len(positions); i++ {
		if positions[i].line != positions[0].line {
			t.Fatalf("status moved from line %d to line %d", positions[0].line, positions[i].line)
		}
	}
}

func TestFees_BarsScaleToMax(t *testing.T) {
	m := feesTestModel(t)
	view := stripANSI(m.View())
	if !strings.Contains(view, strings.Repeat("█", 20)+"   12.0 sat/vB") {
		t.Fatalf("fastest fee should fill the 20-cell bar:\n%s", view)
	}
	if !strings.Contains(view, strings.Repeat("█", 1)+strings.Repeat(" ", 19)+"    0.5 sat/vB") {
		t.Fatalf("minimum fee should keep at least one bar cell:\n%s", view)
	}

	if got := strings.Count(renderFeeBar(12, 12, 20), "█"); got != 20 {
		t.Fatalf("max rate bar has %d cells, want 20", got)
	}
	if got := strings.Count(renderFeeBar(0.5, 12, 20), "█"); got != 1 {
		t.Fatalf("small positive rate bar has %d cells, want 1", got)
	}
	if got := strings.Count(renderFeeBar(0, 12, 20), "█"); got != 0 {
		t.Fatalf("zero rate bar has %d cells, want 0", got)
	}
}

func TestFees_FitAtSupportedSizes(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"120x40", 120, 40}} {
		t.Run(size.name, func(t *testing.T) {
			m := feesTestModel(t)
			m.width, m.height = size.w, size.h
			view := m.View()
			assertFits(t, view, size.w)
			if got := requireLineCount(t, view); got > size.h {
				t.Fatalf("fees view has %d lines, terminal is %d high", got, size.h)
			}

			m.refreshing = true
			m.err = nil
			assertFits(t, m.View(), size.w)
			m.refreshing = false
			m.err = fmt.Errorf("a very long backend error that must be shortened instead of wrapping onto another line and moving every fee row")
			assertFits(t, m.View(), size.w)
		})
	}
}

func TestFees_ServerLabelStripsUserinfo(t *testing.T) {
	cases := map[string]string{
		"electrum.example.com":                          "electrum.example.com",
		"user:value@node.example.com:8332":              "node.example.com:8332",
		"https://user:value@node.example.com:8332/path": "node.example.com:8332",
		"rpc://user:value@node.example.com:8332?x=1":    "node.example.com:8332",
	}
	for raw, want := range cases {
		if got := sanitizeServerLabel(raw); got != want {
			t.Errorf("sanitizeServerLabel(%q) = %q, want %q", raw, got, want)
		}
	}

	dir := t.TempDir()
	cm, err := config.NewConfigManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.AppConfig{
		Version: 1,
		Backend: &backend.BackendConfig{
			Type:    backend.BackendTypeRPC,
			Network: "mainnet",
			RPC: &backend.RPCConfig{
				Host:     "node.example.com",
				Port:     8332,
				Username: "user",
				Password: "value",
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	m := NewFeesModel(NewMockWalletManager(), cm)
	view := stripANSI(m.View())
	if !strings.Contains(view, "from node.example.com") {
		t.Fatalf("view should show the server host:\n%s", view)
	}
	for _, secret := range []string{"user", "value", "https://"} {
		if strings.Contains(view, secret) {
			t.Fatalf("view leaked %q:\n%s", secret, view)
		}
	}
}

func TestFees_OnlyFeeMethodsAreCalled(t *testing.T) {
	mock := NewMockWalletManager()
	cachedCalls, estimateCalls := 0, 0
	mock.GetCachedFeeEstimatesFn = func() (*api.FeeEstimates, bool) {
		cachedCalls++
		return nil, false
	}
	mock.GetFeeEstimatesFn = func() (*api.FeeEstimates, error) {
		estimateCalls++
		return &api.FeeEstimates{FastestFee: 4}, nil
	}

	m := NewFeesModel(mock, nil)
	messages := collectMsgs(m.Init())
	msg, ok := findMsgOfType[FeesMsg](messages)
	if !ok {
		t.Fatalf("Init did not request fee estimates: %v", messages)
	}
	if msg.Err != nil {
		t.Fatalf("unexpected fee error: %v", msg.Err)
	}
	if cachedCalls != 1 || estimateCalls != 1 {
		t.Fatalf("cached calls = %d, estimate calls = %d; want 1 each", cachedCalls, estimateCalls)
	}

	updated, refreshCmd := m.Update(keyRunes("r"))
	refreshed := updated.(FeesModel)
	if !refreshed.refreshing {
		t.Fatal("refresh key should show the refreshing state")
	}
	_ = collectMsgs(refreshCmd)
	if estimateCalls != 2 {
		t.Fatalf("refresh should request estimates once, got %d calls", estimateCalls)
	}
}
func TestFees_EscReturnsToDashboard(t *testing.T) {
	m := feesTestModel(t)
	_, cmd := m.Update(keyCode(tea.KeyEsc))
	assertNavigatesTo(t, cmd, StateMainDashboard)
}

func TestDashboard_FeesKeyNavigates(t *testing.T) {
	m, _ := newTestDashboard(t)
	_, cmd := m.Update(keyRune('F'))
	assertNavigatesTo(t, cmd, StateFees)
}

func TestDashboard_MenuListsFees(t *testing.T) {
	m, _ := newTestDashboard(t)
	m.width = 80
	if !strings.Contains(stripANSI(m.View()), "F  Fees") {
		t.Fatal("dashboard should list the F fee shortcut")
	}
}
