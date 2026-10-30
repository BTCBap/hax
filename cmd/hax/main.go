package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/config"
	"github.com/BTCBap/hax/internal/fsutil"
	"github.com/BTCBap/hax/internal/logsafe"
	"github.com/BTCBap/hax/internal/ui/models"
	"github.com/BTCBap/hax/internal/version"
	"github.com/BTCBap/hax/internal/wallet"
)

func main() {
	var walletDir string
	var walletManager *wallet.WalletManager
	// A panic must restore the terminal and must not leave a stack trace
	// with wallet state on screen. Diagnostic detail goes to a 0600 file.
	defer func() {
		if r := recover(); r != nil {
			if walletManager != nil {
				walletManager.LockWallet()
			}
			if walletDir != "" {
				_ = writeCrashLog(walletDir, r)
			}
			// Leave the alternate screen and show the cursor again.
			_, _ = fmt.Fprint(os.Stderr, "\x1b[?1049l\x1b[?25h")
			_, _ = fmt.Fprintln(os.Stderr, "hax: unexpected error; the wallet was locked and closed.")
			os.Exit(70)
		}
	}()

	if done, code := handleArgs(os.Args[1:], os.Stdout, os.Stderr); done {
		os.Exit(code)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	walletDir, err = resolveWalletDir(homeDir)
	if err != nil {
		fmt.Printf("Error preparing wallet directory: %v\n", err)
		os.Exit(1)
	}

	if len(os.Getenv("DEBUG")) > 0 {
		f, err := setupDebugLog(walletDir)
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer func() { _ = f.Close() }()
	} else {
		log.SetOutput(io.Discard)
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

	walletManager, err = wallet.NewWalletManager(walletDir, b)
	if err != nil {
		fmt.Printf("Error creating wallet manager: %v\n", err)
		os.Exit(1)
	}
	defer walletManager.LockWallet()

	// The wallet's network decides which server is used. Honor a saved
	// server only when it matches that network.
	walletManager.SetBackendResolver(func(network string) (backend.BitcoinBackend, error) {
		cfg, err := configManager.Load()
		if err != nil {
			cfg = nil
		}
		var saved *backend.BackendConfig
		if cfg != nil {
			saved = cfg.Backend
		}
		return backend.NewBackend(backend.ResolveBackendConfig(saved, network))
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

// maxDebugLogSize caps debug.log; older content is rotated to debug.log.1.
const maxDebugLogSize = 5 << 20

// writeCrashLog records a panic diagnostic under the private wallet
// directory without secrets or an on-screen stack trace.
func writeCrashLog(walletDir string, recovered any) error {
	if err := fsutil.EnsurePrivateDir(walletDir, 0o700); err != nil {
		return err
	}
	body := fmt.Sprintf("hax crash at %s\npanic: %v\n\n%s\n",
		time.Now().Format(time.RFC3339), recovered, debug.Stack())
	return fsutil.WriteFileAtomic(filepath.Join(walletDir, "crash.log"),
		[]byte(logsafe.Redact(body)), 0o600)
}

// setupDebugLog opens a 0600 debug log under the private wallet directory.
func setupDebugLog(walletDir string) (*os.File, error) {
	if err := fsutil.EnsurePrivateDir(walletDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(walletDir, "debug.log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxDebugLogSize {
		_ = os.Remove(path + ".1")
		if err := os.Rename(path, path+".1"); err != nil {
			_ = os.Remove(path)
		}
	}
	f, err := tea.LogToFile(path, "debug")
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return f, nil
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

const usage = `usage: hax [--version | --help]

hax is a terminal Bitcoin wallet. Run it without arguments to start.
Wallets and settings live in ~/.hax.
`

// handleArgs deals with command-line flags. It reports whether the
// program should exit instead of starting the UI, and with what code.
func handleArgs(args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "--version", "-version", "-v":
		_, _ = fmt.Fprintf(stdout, "hax %s (commit %s, built %s)\n",
			version.String(), version.GitCommit, version.BuildDate)
		return true, 0
	case "--help", "-help", "-h":
		_, _ = fmt.Fprint(stdout, usage)
		return true, 0
	}
	_, _ = fmt.Fprintf(stderr, "hax: unknown argument %q\n\n%s", args[0], usage)
	return true, 2
}
