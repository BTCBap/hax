package backend

import "testing"

func TestDefaultElectrumConfig_PerNetwork(t *testing.T) {
	for _, c := range []struct{ in, net, host string }{
		{"mainnet", "mainnet", "electrum.blockstream.info"},
		{"testnet", "testnet", "mempool.space"},
		{"testnet4", "testnet", "mempool.space"},
	} {
		cfg := DefaultElectrumConfig(c.in)
		if cfg.Network != c.net || cfg.Electrum.Host != c.host {
			t.Errorf("DefaultElectrumConfig(%q) = %s %s, want %s %s", c.in, cfg.Network, cfg.Electrum.Host, c.net, c.host)
		}
		b, err := NewBackend(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got := ConfiguredNetwork(b); got != c.net {
			t.Errorf("ConfiguredNetwork = %q, want %q", got, c.net)
		}
	}
}

func TestResolveBackendConfig(t *testing.T) {
	savedTestnet := DefaultElectrumConfig("testnet")
	savedMainnet := &BackendConfig{
		Type:    BackendTypeElectrum,
		Network: "mainnet",
		Electrum: &ElectrumConfig{
			Host:   "custom.example.com",
			Port:   50002,
			UseSSL: true,
		},
	}

	cases := []struct {
		name       string
		saved      *BackendConfig
		wallet     string
		wantNet    string
		wantHost   string
		wantPort   int
		wantSameAs *BackendConfig
	}{
		{"nil saved, mainnet wallet", nil, "mainnet", "mainnet", "electrum.blockstream.info", 50002, nil},
		{"nil saved, testnet4 wallet", nil, "testnet4", "testnet", "mempool.space", 40002, nil},
		{"nil saved, testnet wallet", nil, "testnet", "testnet", "mempool.space", 40002, nil},
		{"testnet saved, mainnet wallet", savedTestnet, "mainnet", "mainnet", "electrum.blockstream.info", 50002, nil},
		{"mainnet saved, mainnet wallet", savedMainnet, "mainnet", "mainnet", "custom.example.com", 50002, savedMainnet},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveBackendConfig(c.saved, c.wallet)
			if got.Network != c.wantNet {
				t.Errorf("Network = %q, want %q", got.Network, c.wantNet)
			}
			if got.Electrum == nil {
				t.Fatal("Electrum config is nil")
			}
			if got.Electrum.Host != c.wantHost || got.Electrum.Port != c.wantPort {
				t.Errorf("server = %s:%d, want %s:%d", got.Electrum.Host, got.Electrum.Port, c.wantHost, c.wantPort)
			}
			if c.wantSameAs != nil && got != c.wantSameAs {
				t.Error("saved config for the same network was not returned")
			}
		})
	}
}
