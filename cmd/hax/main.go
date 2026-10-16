package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/ui/models"
	"github.com/BTCBap/hax/internal/wallet"
)

func main() {
	if len(os.Getenv("DEBUG")) > 0 {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer f.Close()
	} else {
		log.SetOutput(io.Discard)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	walletDir, err := resolveWalletDir(homeDir)
	if err != nil {
		fmt.Printf("Error preparing wallet directory: %v\n", err)
		os.Exit(1)
	}

	configManager, err := config.NewConfigManager(walletDir)
	if err != nil {
		fmt.Printf("Error creating config manager: %v\n", err)
		os.Exit(1)
	}

	// Trust-on-first-use pinning for Electrum TLS servers.
	backend.SetDefaultPinStore(filepath.Join(walletDir, "known_servers.json"))

	var b backend.BitcoinBackend
	cfg, err := configManager.Load()
	if err == nil && cfg != nil && cfg.Backend != nil {
		b, err = backend.NewBackend(cfg.Backend)
		if err != nil {
			fmt.Printf("Warning: Failed to create configured backend: %v, using default\n", err)
			b = nil
		}
	}

	walletManager, err := wallet.NewWalletManager(walletDir, b)
	if err != nil {
		fmt.Printf("Error creating wallet manager: %v\n", err)
		os.Exit(1)
	}
	defer walletManager.LockWallet()

	// Opening a wallet whose network differs from the saved server's network
	// falls back to that network's default server for this session.
	walletManager.SetServerResolver(func(network string) (backend.BitcoinBackend, error) {
		if cfg != nil && cfg.Backend != nil && backend.NormalizeNetwork(cfg.Backend.Network) == network {
			return backend.NewBackend(cfg.Backend)
		}
		return backend.NewDefaultBackend(network)
	})

	if cfg != nil && cfg.GapLimit > 0 {
		walletManager.SetGapLimit(cfg.GapLimit)
	}

	model := models.NewRootModel(walletManager, configManager)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}

func resolveWalletDir(homeDir string) (string, error) {
	walletDir := filepath.Join(homeDir, ".hax")
	legacyDir := filepath.Join(homeDir, ".haxwallet")

	if _, err := os.Stat(walletDir); err == nil {
		return walletDir, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", walletDir, err)
	}

	if _, err := os.Stat(legacyDir); err == nil {
		if err := os.Rename(legacyDir, walletDir); err == nil {
			return walletDir, nil
		}
		return legacyDir, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", legacyDir, err)
	}

	return walletDir, nil
}
