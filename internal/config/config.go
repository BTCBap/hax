package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/fsutil"
)

// maxConfigSize bounds config.json so a corrupt or hostile file cannot
// exhaust memory. Configs are a few KiB at most.
const maxConfigSize = 1 << 20

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
	// AutoLockMinutes locks an unlocked wallet after this many idle minutes.
	// 0 uses DefaultAutoLockMinutes; a negative value disables auto-lock.
	AutoLockMinutes int `json:"auto_lock_minutes,omitempty"`
	// StealthMode hides balances and amounts on balance screens.
	StealthMode bool `json:"stealth_mode,omitempty"`
}

// DefaultAutoLockMinutes is the idle timeout used when none is configured.
const DefaultAutoLockMinutes = 10

// AutoLockAfter returns the idle timeout, or 0 when auto-lock is disabled.
func (c *AppConfig) AutoLockAfter() time.Duration {
	if c == nil || c.AutoLockMinutes == 0 {
		return DefaultAutoLockMinutes * time.Minute
	}
	if c.AutoLockMinutes < 0 {
		return 0
	}
	return time.Duration(c.AutoLockMinutes) * time.Minute
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

// Load reads the configuration from disk.
func (cm *ConfigManager) Load() (*AppConfig, error) {
	f, err := os.Open(cm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config exists - return nil to indicate first run
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	defer func() { _ = f.Close() }()

	if info, err := f.Stat(); err == nil {
		_ = fsutil.TightenFile(cm.configPath, 0o600)
		if info.Size() > maxConfigSize {
			return nil, fmt.Errorf("config file is too large (%d bytes)", info.Size())
		}
	}

	data, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	if len(data) > maxConfigSize {
		return nil, fmt.Errorf("config file is too large")
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Normalize network aliases and files written before the version field
	// existed (H3-32/H3-33). Old files must keep loading.
	if config.Version == 0 {
		config.Version = 1
	}
	if config.Backend != nil {
		config.Backend.Network = backend.NormalizeNetwork(config.Backend.Network)
	}

	if result := cm.Validate(&config); !result.Valid {
		return nil, fmt.Errorf("invalid config: %s", result.Errors[0].Message)
	}

	cm.config = &config
	return &config, nil
}

// Save writes the configuration to disk.
func (cm *ConfigManager) Save(config *AppConfig) error {
	if result := cm.Validate(config); !result.Valid {
		return fmt.Errorf("invalid config: %s", result.Errors[0].Message)
	}

	dir := filepath.Dir(cm.configPath)
	if err := fsutil.EnsurePrivateDir(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write atomically with owner-only permissions (H3-16/H3-17).
	if err := fsutil.WriteFileAtomic(cm.configPath, data, 0o600); err != nil {
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
		// A first-run config is written before the user picks a server.
		// It is valid, just incomplete; the backend is validated once set.
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

	if net := backend.NormalizeNetwork(bc.Network); net != "mainnet" && net != "testnet" {
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
	} else if err := backend.ValidateHost(ec.Host); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.electrum.host",
			Message: fmt.Sprintf("Invalid Electrum server host: %v", err),
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
	} else if err := backend.ValidateHost(rc.Host); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backend.rpc.host",
			Message: fmt.Sprintf("Invalid RPC host: %v", err),
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
