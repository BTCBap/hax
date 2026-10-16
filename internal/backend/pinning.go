package backend

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrCertPinMismatch is returned when an Electrum server presents a public
// key different from the one pinned on first use.
var ErrCertPinMismatch = errors.New("electrum server certificate changed")

// PinStore persists trust-on-first-use pins for Electrum servers, keyed by
// host:port. A pin is the hex SHA-256 of the leaf certificate's
// SubjectPublicKeyInfo, so routine renewals that keep the key still match.
type PinStore struct {
	mu   sync.Mutex
	path string
}

var (
	defaultPinsMu sync.RWMutex
	defaultPins   *PinStore
)

// SetDefaultPinStore configures the pin file used by Electrum backends.
// An empty path disables pinning (CA verification still applies).
func SetDefaultPinStore(path string) {
	defaultPinsMu.Lock()
	defer defaultPinsMu.Unlock()
	if path == "" {
		defaultPins = nil
		return
	}
	defaultPins = &PinStore{path: path}
}

func getDefaultPinStore() *PinStore {
	defaultPinsMu.RLock()
	defer defaultPinsMu.RUnlock()
	return defaultPins
}

// NewPinStore returns a store backed by path.
func NewPinStore(path string) *PinStore { return &PinStore{path: path} }

// Path returns the backing file path.
func (s *PinStore) Path() string { return s.path }

func (s *PinStore) load() (map[string]string, error) {
	pins := map[string]string{}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return pins, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &pins); err != nil {
		return nil, fmt.Errorf("corrupt pin file %s: %w", s.path, err)
	}
	return pins, nil
}

func (s *PinStore) save(pins map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Forget removes the pin for server so the next connection re-pins.
func (s *PinStore) Forget(server string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pins, err := s.load()
	if err != nil {
		return err
	}
	delete(pins, server)
	return s.save(pins)
}

// Check verifies (or, on first use, records) the pin for server.
func (s *PinStore) Check(server string, state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("%w: %s presented no certificate", ErrCertPinMismatch, server)
	}
	sum := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
	got := hex.EncodeToString(sum[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	pins, err := s.load()
	if err != nil {
		return err
	}
	want, ok := pins[server]
	if !ok {
		pins[server] = got
		return s.save(pins)
	}
	if want != got {
		return fmt.Errorf("%w for %s\n  pinned:    %s\n  presented: %s\n"+
			"This can mean the server rotated its key, or someone is intercepting the connection.\n"+
			"Refusing to connect. If you have verified the change out of band, remove the %q entry from %s.",
			ErrCertPinMismatch, server, want, got, server, s.path)
	}
	return nil
}

// verifyConnection returns a tls.Config.VerifyConnection hook for server, or
// nil when pinning is disabled. It runs after normal chain verification.
func verifyConnection(server string) func(tls.ConnectionState) error {
	store := getDefaultPinStore()
	if store == nil {
		return nil
	}
	return func(cs tls.ConnectionState) error { return store.Check(server, cs) }
}
