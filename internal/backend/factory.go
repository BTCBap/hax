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
		return nil, fmt.Errorf("unsupported backend type %q", config.Type)
	}
}

// NewDefaultBackend creates the default backend for a network (Electrum)
func NewDefaultBackend(network string) (BitcoinBackend, error) {
	config := DefaultElectrumConfig(network)
	return NewBackend(config)
}
