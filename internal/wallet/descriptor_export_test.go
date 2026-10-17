package wallet

import (
	"strings"
	"testing"
)

// TestExportDescriptorHardwareMatchesSoftware: a watch-only wallet from
// the account xpub exports the same descriptor as the seed it came from,
// including the receive and change paths.
func TestExportDescriptorHardwareMatchesSoftware(t *testing.T) {
	for _, network := range []string{NetworkTestnet, NetworkMainnet} {
		for _, tt := range signerTestTypes {
			sw, _ := newSignerTestManager(t, network, tt.addr, tt.purpose, false)
			hw, _ := newSignerTestManager(t, network, tt.addr, tt.purpose, true)
			want, err := sw.ExportDescriptor(0)
			if err != nil {
				t.Fatal(err)
			}
			got, err := hw.ExportDescriptor(0)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("%s %s:\n hw %s\n sw %s", network, tt.name, got, want)
			}
			if !strings.Contains(got, "/<0;1>/*)") {
				t.Errorf("%s %s: %s has no receive/change range", network, tt.name, got)
			}
		}
	}
}
