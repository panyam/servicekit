// Package testcert makes a throwaway self-signed certificate for TLS tests.
// The certificate is valid for 127.0.0.1, ::1 and localhost. Tests in http/
// and grpcws/ share it, so it imports nothing from servicekit.
package testcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Cert is a self-signed certificate and the ways a test needs to use it.
type Cert struct {
	// CertFile and KeyFile are PEM files in the test's temp dir.
	CertFile, KeyFile string
	// Pair is the loaded certificate, for a server tls.Config.
	Pair tls.Certificate
	// Pool trusts only this certificate, for a client tls.Config.
	Pool *x509.CertPool
}

// New makes a certificate and writes it under t.TempDir(). It fails the test
// on any error.
func New(t testing.TB) Cert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("testcert: generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "servicekit test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("testcert: create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("testcert: marshal key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	c := Cert{CertFile: filepath.Join(dir, "cert.pem"), KeyFile: filepath.Join(dir, "key.pem")}
	if err := os.WriteFile(c.CertFile, certPEM, 0o600); err != nil {
		t.Fatalf("testcert: %v", err)
	}
	if err := os.WriteFile(c.KeyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("testcert: %v", err)
	}
	if c.Pair, err = tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("testcert: load pair: %v", err)
	}
	c.Pool = x509.NewCertPool()
	c.Pool.AppendCertsFromPEM(certPEM)
	return c
}
