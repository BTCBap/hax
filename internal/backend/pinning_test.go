package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newServer starts a TLS server with a freshly generated key.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.NotFoundHandler())
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func stateFor(t *testing.T) tls.ConnectionState {
	cert, _ := x509.ParseCertificate(newServer(t).TLS.Certificates[0].Certificate[0])
	return tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
}

func TestPinStore_TOFUThenHardFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_servers.json")
	s := NewPinStore(path)
	a, b := stateFor(t), stateFor(t)

	if err := s.Check("h:50002", a); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("pin file perms: %v %v", fi, err)
	}
	if err := s.Check("h:50002", a); err != nil {
		t.Fatalf("same key: %v", err)
	}
	if err := s.Check("h:50002", b); !errors.Is(err, ErrCertPinMismatch) {
		t.Fatalf("changed key not rejected: %v", err)
	}
	if err := s.Check("other:50002", b); err != nil {
		t.Fatalf("pins are per server: %v", err)
	}
	if err := s.Forget("h:50002"); err != nil {
		t.Fatal(err)
	}
	if err := s.Check("h:50002", b); err != nil {
		t.Fatalf("re-pin after forget: %v", err)
	}
}

func TestPinStore_NoCertAndCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	s := NewPinStore(path)
	if err := s.Check("h:1", tls.ConnectionState{}); err == nil {
		t.Fatal("empty cert chain accepted")
	}
	_ = os.WriteFile(path, []byte("{nope"), 0o600)
	if err := s.Check("h:1", stateFor(t)); err == nil {
		t.Fatal("corrupt pin file should fail closed")
	}
}

// End-to-end: a real TLS handshake fails when the server key changes.
func TestVerifyConnection_HandshakeFailsOnChange(t *testing.T) {
	SetDefaultPinStore(filepath.Join(t.TempDir(), "k.json"))
	t.Cleanup(func() { SetDefaultPinStore("") })

	dial := func(srv *httptest.Server) error {
		pool := x509.NewCertPool()
		cert, _ := x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
		pool.AddCert(cert)
		c, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{
			RootCAs: pool, ServerName: "example.com",
			VerifyConnection: verifyConnection("srv:50002"),
		})
		if err == nil {
			c.Close()
		}
		return err
	}
	s1, s2 := newServer(t), newServer(t)
	if err := dial(s1); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := dial(s2); err == nil {
		t.Fatal("handshake succeeded with a different key")
	}
}
