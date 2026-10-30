package models

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

func TestChunkAddress(t *testing.T) {
	tests := []struct {
		address string
		want    []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"abcd", []string{"abcd"}},
		{"abcdefghij", []string{"abcd", "efgh", "ij"}},
		{verifyTestAddress, []string{
			"tb1q", "rp33", "g0q5", "c5tx", "sp9a", "rysr", "x4k6", "zdkf",
			"s4nc", "e4xj", "0gdc", "ccef", "vpys", "xf3q", "0sl5", "k7",
		}},
	}

	for _, tc := range tests {
		got := chunkAddress(tc.address)
		if len(got) != len(tc.want) {
			t.Fatalf("chunkAddress(%q) = %v, want %v", tc.address, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("chunkAddress(%q)[%d] = %q, want %q", tc.address, i, got[i], tc.want[i])
			}
		}
	}
}

func TestChunkAddressRows(t *testing.T) {
	tests := []struct {
		name    string
		address string
		perRow  int
		want    []string
	}{
		{name: "empty", address: "", perRow: 8},
		{name: "single row", address: "abcdefghijkl", perRow: 8, want: []string{"abcd efgh ijkl"}},
		{name: "fixed group count", address: "abcdefghijklmnop", perRow: 4, want: []string{"abcd efgh ijkl mnop"}},
		{name: "wraps whole groups", address: "abcdefghijklmnop", perRow: 3, want: []string{"abcd efgh ijkl", "mnop"}},
		{
			name:    "longest address uses two rows",
			address: verifyTestAddress,
			perRow:  8,
			want: []string{
				"tb1q rp33 g0q5 c5tx sp9a rysr x4k6 zdkf",
				"s4nc e4xj 0gdc ccef vpys xf3q 0sl5 k7",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chunkAddressRows(tc.address, tc.perRow)
			if len(got) != len(tc.want) {
				t.Fatalf("chunkAddressRows(%q, %d) = %v, want %v", tc.address, tc.perRow, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("line %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
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
			want:  []string{"Verify on device", "Wallet", "Device", "✗ Device shows a different address", "0sl5 k8"},
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

func TestAddressVerifyViewGroupLayout(t *testing.T) {
	plain := stripANSI(newVerifyView(AddressVerifyMismatch, 80).View())
	for _, want := range []string{
		"Wallet  tb1q rp33 g0q5 c5tx sp9a rysr x4k6 zdkf",
		"        s4nc e4xj 0gdc ccef vpys xf3q 0sl5 k7",
		"Device  tb1q rp33 g0q5 c5tx sp9a rysr x4k6 zdkf",
		"        s4nc e4xj 0gdc ccef vpys xf3q 0sl5 k8",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view does not contain %q:\n%s", want, plain)
		}
	}

	match := stripANSI(newVerifyView(AddressVerifyMatch, 80).View())
	if got := strings.Count(match, "rp33"); got != 1 {
		t.Fatalf("match should show the wallet address once, got %d copies:\n%s", got, match)
	}
	if !strings.Contains(match, "Device") {
		t.Fatalf("match should still label the empty device rows:\n%s", match)
	}
}

func TestAddressVerifyViewReservedRows(t *testing.T) {
	short := AddressVerifyView{
		Address: "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		Path:    verifyTestPath,
		State:   AddressVerifyWaiting,
		Width:   80,
	}
	long := newVerifyView(AddressVerifyWaiting, 80)

	if short.Height() != long.Height() {
		t.Fatalf("short address height = %d, long address height = %d", short.Height(), long.Height())
	}
	if got, want := requireLineCount(t, short.View()), requireLineCount(t, long.View()); got != want {
		t.Fatalf("short address rendered %d lines, long address %d", got, want)
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
	for _, width := range []int{80, 120} {
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

func TestAddressVerifyViewMismatchHighlightsDifferentGroups(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	rendered := newVerifyView(AddressVerifyMismatch, 80).View()
	plain := stripANSI(rendered)
	if strings.Count(plain, "Device  tb1q") != 1 {
		t.Fatalf("device address was not rendered:\n%s", plain)
	}
	for _, same := range []string{"tb1q", "rp33", "s4nc", "0sl5", "k7"} {
		if strings.Contains(rendered, styles.ErrorText.Render(same)) {
			t.Fatalf("matching group %q was highlighted", same)
		}
	}
	if !strings.Contains(rendered, styles.ErrorText.Render("k8")) {
		t.Fatalf("different group was not highlighted")
	}
}
