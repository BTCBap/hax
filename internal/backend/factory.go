package backend

import "fmt"

// NewBackend creates a backend instance based on configuration
func NewBackend(config *BackendConfig) (BitcoinBackend, error) {
	if config == nil {
		return nil, fmt.Errorf("backend configuration is required")
	}

	switch config.Type {
	case BackendTypeElectrum:
		return NewElectrumBackend(config)
	case BackendTypeRPC:
		return NewRPCBackend(config)
	default:
		// Default to Electrum for any unknown or legacy type (including API)
		electrumConfig := DefaultElectrumConfig("testnet4")
		return NewElectrumBackend(electrumConfig)
	}
}

// NewDefaultBackend creates the default backend for a network (Electrum)
func NewDefaultBackend(network string) (BitcoinBackend, error) {
	config := DefaultElectrumConfig(network)
	return NewBackend(config)
}
