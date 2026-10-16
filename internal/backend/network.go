package backend

// NormalizeNetwork maps network aliases to the names stored in wallet files
// and configs: "mainnet" or "testnet" (which means testnet4).
func NormalizeNetwork(network string) string {
	switch network {
	case "testnet4", "testnet":
		return "testnet"
	default:
		return network
	}
}

// NetworkReporter is implemented by backends that know which network they
// were configured for.
type NetworkReporter interface {
	ConfiguredNetwork() string
}

// ConfiguredNetwork reports the configured network of b, or "" if unknown.
func ConfiguredNetwork(b BitcoinBackend) string {
	if nr, ok := b.(NetworkReporter); ok {
		return NormalizeNetwork(nr.ConfiguredNetwork())
	}
	return ""
}

// ConfiguredNetwork implements NetworkReporter.
func (b *ElectrumBackend) ConfiguredNetwork() string {
	if b.config == nil {
		return ""
	}
	return b.config.Network
}

// ConfiguredNetwork implements NetworkReporter.
func (b *RPCBackend) ConfiguredNetwork() string {
	if b.config == nil {
		return ""
	}
	return b.config.Network
}
