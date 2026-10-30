package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/backend"
)

func newTestConfigManager(t *testing.T) *ConfigManager {
	t.Helper()
	cm, err := NewConfigManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cm
}

func TestConfigWithoutBackendStillLoads(t *testing.T) {
	cm := newTestConfigManager(t)
	// This is the config written during onboarding before a server exists.
	if err := cm.Save(&AppConfig{Version: 1, FirstRunComplete: true, LastWalletName: "w"}); err != nil {
		t.Fatalf("saving a partial config failed: %v", err)
	}
	info, err := os.Stat(cm.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config mode = %o, want 600", perm)
	}
	got, err := cm.Load()
	if err != nil {
		t.Fatalf("loading a partial config failed: %v", err)
	}
	if got == nil || got.LastWalletName != "w" || !got.FirstRunComplete {
		t.Fatalf("config = %+v", got)
	}
}

func TestLoadNormalizesNetworkAlias(t *testing.T) {
	cm := newTestConfigManager(t)
	raw := `{
  "version": 1,
  "backend": {
    "type": "electrum",
    "network": "testnet4",
    "electrum": {"host": "electrum.example.com", "port": 50002, "use_ssl": true}
  }
}`
	if err := os.WriteFile(cm.ConfigPath(), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := cm.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Backend.Network != "testnet" {
		t.Fatalf("network = %q, want testnet", got.Backend.Network)
	}
}

func TestLoadRejectsCredentialRedirectionHost(t *testing.T) {
	cm := newTestConfigManager(t)
	raw := `{
  "version": 1,
  "backend": {
    "type": "rpc",
    "network": "mainnet",
    "rpc": {"host": "127.0.0.1@attacker.example", "port": 8332, "username": "u", "password": "p"}
  }
}`
	if err := os.WriteFile(cm.ConfigPath(), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cm.Load(); err == nil {
		t.Fatal("config with userinfo in the host was accepted")
	}
}

func TestLoadRejectsOversizedConfig(t *testing.T) {
	cm := newTestConfigManager(t)
	data := strings.Repeat(" ", maxConfigSize+1)
	if err := os.WriteFile(cm.ConfigPath(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cm.Load(); err == nil {
		t.Fatal("oversized config was accepted")
	}
}

func TestSaveRejectsInvalidBackend(t *testing.T) {
	cm := newTestConfigManager(t)
	cfg := &AppConfig{
		Version: 1,
		Backend: &backend.BackendConfig{
			Type:    backend.BackendTypeElectrum,
			Network: "mainnet",
			Electrum: &backend.ElectrumConfig{
				Host: "electrum.example.com",
				Port: 70000,
			},
		},
	}
	if err := cm.Save(cfg); err == nil {
		t.Fatal("config with an invalid port was saved")
	}
	if _, err := os.Stat(filepath.Clean(cm.ConfigPath())); !os.IsNotExist(err) {
		t.Fatalf("invalid config was written to disk: %v", err)
	}
}
