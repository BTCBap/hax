package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"hax/internal/backend"
)

// DisplayUnit represents the unit for displaying Bitcoin amounts (BIP-177)
type DisplayUnit string

const (
	DisplayUnitBTC     DisplayUnit = "btc"     // Legacy: 1.00000000 BTC (100M base units)
	DisplayUnitBitcoin DisplayUnit = "bitcoin" // BIP-177: ₿112,987 (base unit with symbol)
)

// AppConfig represents the application configuration
type AppConfig struct {
	Version          int                    `json:"version"`
	FirstRunComplete bool                   `json:"first_run_complete"`
	LastWalletName   string                 `json:"last_wallet_name,omitempty"`
	Backend          *backend.BackendConfig `json:"backend"`
	DisplayUnit      DisplayUnit            `json:"display_unit,omitempty"`   // BIP-177 unit preference
	GapLimit         int                    `json:"gap_limit,omitempty"`      // Address gap limit (default 20)
	RecentServers    []backend.SavedServer  `json:"recent_servers,omitempty"` // Recently used Electrum servers
}

// ValidationError represents a field-specific validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResult contains all validation errors for display
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors"`
}

// ConfigManager handles loading, saving, and validating app configuration
type ConfigManager struct {
	configPath string
	config     *AppConfig
}

// NewConfigManager creates a new config manager
func NewConfigManager(walletDir string) (*ConfigManager, error) {
	configPath := filepath.Join(walletDir, "config.json")
	return &ConfigManager{
		configPath: configPath,
	}, nil
}

// Load reads the configuration from disk
func (cm *ConfigManager) Load() (*AppConfig, error) {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config exists - return nil to indicate first run
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cm.config = &config
	return &config, nil
}

// Save writes the configuration to disk
func (cm *ConfigManager) Save(config *AppConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write with secure permissions (owner read/write only)
	if err := os.WriteFile(cm.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	cm.config = config
	return nil
}

// IsFirstRun returns true if no valid config exists
func (cm *ConfigManager) IsFirstRun() bool {
	config, err := cm.Load()
	if err != nil || config == nil {
		return true
	}
	return !config.FirstRunComplete
}

// Validate checks the configuration for errors
func (cm *ConfigManager) Validate(config *AppConfig) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if config == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "config",
			Message: "Configuration is missing",
		})
		return result
	}

	if config.Version < 1 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "version",
			Message: "Invalid version number",
		})
	}

	if config.Backend == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend",
			Message: "Backend configuration is required",
		})
		return result
	}

	// Validate backend config
	cm.validateBackend(config.Backend, result)

	return result
}

// validateBackend validates the backend-specific configuration
func (cm *ConfigManager) validateBackend(bc *backend.BackendConfig, result *ValidationResult) {
	validTypes := map[backend.BackendType]bool{
		backend.BackendTypeElectrum: true,
		backend.BackendTypeRPC:      true,
	}

	if !validTypes[bc.Type] {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.type",
			Message: fmt.Sprintf("Invalid backend type: %s", bc.Type),
		})
	}

	if bc.Network != "mainnet" && bc.Network != "testnet" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.network",
			Message: fmt.Sprintf("Invalid network: %s (must be mainnet or testnet)", bc.Network),
		})
	}

	switch bc.Type {
	case backend.BackendTypeElectrum:
		cm.validateElectrumConfig(bc.Electrum, result)
	case backend.BackendTypeRPC:
		cm.validateRPCConfig(bc.RPC, result)
	}
}

func (cm *ConfigManager) validateElectrumConfig(ec *backend.ElectrumConfig, result *ValidationResult) {
	if ec == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.electrum",
			Message: "Electrum configuration is required for Electrum backend type",
		})
		return
	}

	if ec.Host == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.electrum.host",
			Message: "Electrum server host is required",
		})
	}

	if ec.Port <= 0 || ec.Port > 65535 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.electrum.port",
			Message: "Electrum server port must be between 1 and 65535",
		})
	}

	if ec.UseProxy {
		if ec.ProxyHost == "" {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   "backend.electrum.proxy_host",
				Message: "Proxy host is required when proxy is enabled",
			})
		}
		if ec.ProxyPort <= 0 || ec.ProxyPort > 65535 {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   "backend.electrum.proxy_port",
				Message: "Proxy port must be between 1 and 65535",
			})
		}
	}
}

func (cm *ConfigManager) validateRPCConfig(rc *backend.RPCConfig, result *ValidationResult) {
	if rc == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.rpc",
			Message: "RPC configuration is required for RPC backend type",
		})
		return
	}

	if rc.Host == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.rpc.host",
			Message: "RPC host is required",
		})
	}

	if rc.Port <= 0 || rc.Port > 65535 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.rpc.port",
			Message: "RPC port must be between 1 and 65535",
		})
	}

	if rc.Username == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.rpc.username",
			Message: "RPC username is required",
		})
	}

	if rc.Password == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.rpc.password",
			Message: "RPC password is required",
		})
	}
}

// GetConfig returns the currently loaded config
func (cm *ConfigManager) GetConfig() *AppConfig {
	return cm.config
}

// ConfigPath returns the path to the config file
func (cm *ConfigManager) ConfigPath() string {
	return cm.configPath
}

// DefaultConfig returns a default configuration with Electrum backend
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Version:          1,
		FirstRunComplete: false,
		Backend:          backend.DefaultElectrumConfig("testnet"),
	}
}

// RawConfigJSON returns the raw JSON content of the config file for display
func (cm *ConfigManager) RawConfigJSON() (string, error) {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
