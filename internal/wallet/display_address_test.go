package wallet

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeHWI writes a stand-in for the hwi binary that records its
// arguments and reports address as the one the device showed.
func fakeHWI(t *testing.T, address string) (*HWIClient, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake hwi is a shell script")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" +
		"echo '{\"address\": \"" + address + "\"}'\n"
	path := filepath.Join(dir, "hwi")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { // #nosec G306 -- test helper must be executable
		t.Fatal(err)
	}
	return &HWIClient{hwiPath: path, testnet: true, available: true}, argsFile
}

func TestVerifyAddressOnDevice(t *testing.T) {
	m, _ := newSignerTestManager(t, NetworkTestnet, AddressTypeSegWitNative, 84, true)
	addr, path, err := m.GetAddressByIndex(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if path != "m/84'/1'/0'/0/2" {
		t.Fatalf("path = %s", path)
	}
	dev := HardwareDevice{Type: "trezor", Path: "udp:127.0.0.1:21324"}

	h, argsFile := fakeHWI(t, addr)
	if err := m.verifyAddressOnDevice(h, dev, addr, path); err != nil {
		t.Fatalf("matching address: %v", err)
	}
	args, _ := os.ReadFile(argsFile) // #nosec G304 -- file in the test temp dir
	want := "--chain test -d udp:127.0.0.1:21324 -t trezor displayaddress --path m/84'/1'/0'/0/2 --addr-type wit"
	if strings.TrimSpace(string(args)) != want {
		t.Fatalf("hwi args:\n got %s\nwant %s", args, want)
	}

	other, _, _ := m.GetAddressByIndex(0, 3)
	h, _ = fakeHWI(t, other)
	if err := m.verifyAddressOnDevice(h, dev, addr, path); err == nil ||
		!strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("different address: %v, want mismatch", err)
	}

	h, _ = fakeHWI(t, "")
	if err := m.verifyAddressOnDevice(h, dev, addr, path); err == nil {
		t.Fatal("no address reported, want an error")
	}
}

func TestHWIAddrType(t *testing.T) {
	for in, want := range map[string]string{
		"legacy": "legacy", "segwit_nested": "sh_wit",
		"segwit_native": "wit", "taproot": "tap", "": "",
	} {
		if got := hwiAddrType(in); got != want {
			t.Errorf("hwiAddrType(%q) = %q, want %q", in, got, want)
		}
	}
}
