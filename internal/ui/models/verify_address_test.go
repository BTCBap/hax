package models

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/ui/styles"
)

const (
	verifyTestAddress = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7"
	verifyTestDevice  = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k8"
	verifyTestPath    = "m/84'/1'/0'/0/0"
)

func newVerifyView(state AddressVerifyState, width int) AddressVerifyView {
	v := AddressVerifyView{
		Address: verifyTestAddress,
		Path:    verifyTestPath,
		State:   state,
		Width:   width,
		Frame:   3,
	}
	if state == AddressVerifyWaiting {
		v.StartedAt = time.Now().Add(-3 * time.Second)
	}
	if state == AddressVerifyMismatch {
		v.DeviceAddress = verifyTestDevice
	}
	if state == AddressVerifyError {
		v.ErrMessage = "device is in mainnet mode but wallet uses testnet; verify the derivation path matches"
	}
	return v
}

func TestAddressVerifyViewIdleRendersNothing(t *testing.T) {
	v := newVerifyView(AddressVerifyIdle, 80)
	if got := v.View(); got != "" {
		t.Fatalf("idle panel rendered %q", got)
	}
	if got := v.Height(); got != 0 {
		t.Fatalf("idle panel height = %d, want 0", got)
	}
}

func TestAddressVerifyViewStates(t *testing.T) {
	tests := []struct {
		state AddressVerifyState
		want  []string
		avoid []string
	}{
		{
			state: AddressVerifyWaiting,
			want:  []string{"Verify on device", verifyTestPath, "Wallet", "Device", "Confirm on your device…"},
			avoid: []string{"Matches device", "different address", "v retry"},
		},
		{
			state: AddressVerifyMatch,
			want:  []string{"Verify on device", "Wallet", "Device", "✓ Matches device"},
			avoid: []string{"Confirm on your device", "different address", "v retry"},
		},
		{
			state: AddressVerifyMismatch,
			want:  []string{"Verify on device", "Wallet", "Device", "✗ Device shows a different address", "0sl5k8"},
			avoid: []string{"Matches device", "v retry"},
		},
		{
			state: AddressVerifyError,
			want:  []string{"Verify on device", "Wallet", "Device", "device is in mainnet mode"},
			avoid: []string{"Matches device", "different address", "v retry"},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("state_%d", tc.state), func(t *testing.T) {
			view := newVerifyView(tc.state, 80).View()
			plain := stripANSI(view)
			for _, want := range tc.want {
				if !strings.Contains(plain, want) {
					t.Fatalf("view does not contain %q:\n%s", want, plain)
				}
			}
			for _, avoid := range tc.avoid {
				if strings.Contains(plain, avoid) {
					t.Fatalf("view should not contain %q:\n%s", avoid, plain)
				}
			}
			if strings.Contains(plain, "\n\n\n") {
				t.Fatalf("panel should not have a run of blank lines:\n%s", plain)
			}
		})
	}
}

func TestAddressVerifyViewAddressLayout(t *testing.T) {
	plain := stripANSI(newVerifyView(AddressVerifyMismatch, 80).View())
	for _, want := range []string{
		"Wallet  tb1qrp33 g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf 3q0sl5k7",
		"Device  tb1qrp33 g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf 3q0sl5k8",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view does not contain %q:\n%s", want, plain)
		}
	}

	match := stripANSI(newVerifyView(AddressVerifyMatch, 80).View())
	if got := strings.Count(match, "g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf"); got != 1 {
		t.Fatalf("match should show the wallet address once, got %d copies:\n%s", got, match)
	}
	if !strings.Contains(match, "Device") {
		t.Fatalf("match should still label the empty device rows:\n%s", match)
	}
}

func TestAddressVerifyViewWrapsAddress(t *testing.T) {
	v := newVerifyView(AddressVerifyWaiting, 40)
	if got, want := v.reservedRows(), 3; got != want {
		t.Fatalf("reservedRows() = %d, want %d", got, want)
	}
	if got, want := v.Height(), 2*3+8; got != want {
		t.Fatalf("Height() = %d, want %d", got, want)
	}

	formatted := ui.FormatAddress(verifyTestAddress, 0)
	lines := wrapAddressCells(styledAddressCells(formatted), v.addressRowWidth())
	if len(lines) != 3 {
		t.Fatalf("address wrapped into %d lines, want 3", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "\x1b[") {
			t.Fatalf("wrapped line %d lost its style: %q", i, line)
		}
		if got, want := lipgloss.Width(line), lipgloss.Width(stripANSI(line)); got != want {
			t.Fatalf("wrapped line %d width = %d, want %d", i, got, want)
		}
	}

	plain := stripANSI(v.View())
	visible := stripANSI(formatted)
	wantRows := []string{
		"Wallet  " + visible[:28],
		"        " + visible[28:56],
		"        " + visible[56:],
	}
	for _, want := range wantRows {
		if !strings.Contains(plain, want) {
			t.Fatalf("view does not contain %q:\n%s", want, plain)
		}
	}
}

func TestAddressVerifyViewMismatchHighlightsDifferentCharacters(t *testing.T) {
	rendered := newVerifyView(AddressVerifyMismatch, 80).View()
	plain := stripANSI(rendered)
	if strings.Count(plain, "Device  tb1qrp33") != 1 {
		t.Fatalf("device address was not rendered:\n%s", plain)
	}
	for _, same := range []string{"t", "b", "1", "q", "r", "p", "3", "g", "0", "5", "c", "s", "9", "a", "y", "x", "4", "k", "6", "z", "d", "f", "n", "e", "j", "7"} {
		if strings.Contains(rendered, styles.ErrorText.Render(same)) {
			t.Fatalf("matching character %q was highlighted", same)
		}
	}
	if !strings.Contains(rendered, styles.ErrorText.Render("8")) {
		t.Fatalf("different character was not highlighted")
	}
}

func TestAddressVerifyViewReservedRows(t *testing.T) {
	for _, width := range []int{80, 40} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			short := AddressVerifyView{
				Address: "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
				Path:    verifyTestPath,
				State:   AddressVerifyWaiting,
				Width:   width,
			}
			long := newVerifyView(AddressVerifyWaiting, width)

			if short.Height() != long.Height() {
				t.Fatalf("short address height = %d, long address height = %d", short.Height(), long.Height())
			}
			if got, want := requireLineCount(t, short.View()), requireLineCount(t, long.View()); got != want {
				t.Fatalf("short address rendered %d lines, long address %d", got, want)
			}
		})
	}
}

func TestAddressVerifyViewLayoutStable(t *testing.T) {
	states := []AddressVerifyState{
		AddressVerifyWaiting,
		AddressVerifyMatch,
		AddressVerifyMismatch,
		AddressVerifyError,
	}

	for _, width := range []int{80, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			var wantLines int
			wantTitle := viewPosition{line: -1}
			wantStatusLine := -1

			for i, state := range states {
				v := newVerifyView(state, width)
				view := v.View()
				title := requireTextPosition(t, view, "Verify on device")
				var status viewPosition
				switch state {
				case AddressVerifyWaiting:
					status = requireTextPosition(t, view, "Confirm on your device")
				case AddressVerifyMatch:
					status = requireTextPosition(t, view, "✓ Matches device")
				case AddressVerifyMismatch:
					status = requireTextPosition(t, view, "Device shows a different address")
				case AddressVerifyError:
					status = requireTextPosition(t, view, "device is in mainnet")
				}
				lines := requireLineCount(t, view)

				if got := v.Height(); got != lines {
					t.Fatalf("Height() = %d, rendered %d lines", got, lines)
				}
				if i == 0 {
					wantLines, wantTitle, wantStatusLine = lines, title, status.line
					continue
				}
				if lines != wantLines {
					t.Fatalf("state %d has %d lines, want %d", state, lines, wantLines)
				}
				if title != wantTitle {
					t.Fatalf("state %d title moved from %+v to %+v", state, wantTitle, title)
				}
				if status.line != wantStatusLine {
					t.Fatalf("state %d status moved from line %d to %d", state, status.line, wantStatusLine)
				}
			}
		})
	}
}

func TestAddressVerifyViewErrorTruncated(t *testing.T) {
	v := newVerifyView(AddressVerifyError, 80)
	v.ErrMessage = strings.Repeat("x", 200)

	status := stripANSI(v.statusLine(v.innerWidth()))
	if !strings.HasSuffix(status, "…") {
		t.Fatalf("error status was not truncated: %q", status)
	}
	if got, max := len([]rune(status)), v.innerWidth(); got > max {
		t.Fatalf("error status is %d runes, want at most %d", got, max)
	}
}

func TestAddressVerifyViewFitsPanelWidth(t *testing.T) {
	for _, width := range []int{80, 120, 40, 24} {
		for _, state := range []AddressVerifyState{
			AddressVerifyIdle, AddressVerifyWaiting, AddressVerifyMatch,
			AddressVerifyMismatch, AddressVerifyError,
		} {
			v := newVerifyView(state, width)
			for i, line := range strings.Split(v.View(), "\n") {
				if got := len([]rune(stripANSI(line))); got > width {
					t.Fatalf("state %d line %d is %d cells wide, max %d", state, i, got, width)
				}
			}
		}
	}
}
