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
