package egress

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------- reporting

// Checker prints observations and derives a verdict. It never reports PASS
// unless every registered check ran and held.
type Checker struct {
	Q     string
	mu    sync.Mutex
	fails []string
	ran   int
}

func NewChecker(q string) *Checker { return &Checker{Q: q} }

// Obs prints a free-form observation line.
func (c *Checker) Obs(f string, a ...any) { fmt.Printf("[%s] OBS  %s\n", c.Q, fmt.Sprintf(f, a...)) }

// Check records an assertion about observed behaviour.
func (c *Checker) Check(name string, ok bool, f string, a ...any) {
	c.mu.Lock()
	c.ran++
	if !ok {
		c.fails = append(c.fails, name)
	}
	c.mu.Unlock()
	tag := "ok  "
	if !ok {
		tag = "FAIL"
	}
	fmt.Printf("[%s] %s %s :: %s\n", c.Q, tag, name, fmt.Sprintf(f, a...))
}

// Verdict prints the verdict line parsed by run-all.sh.
func (c *Checker) Verdict(note string) {
	v := "PASS"
	if c.ran == 0 {
		v = "NOT TESTED"
	} else if len(c.fails) > 0 {
		v = "FAIL"
		note = "failed checks: " + strings.Join(c.fails, ", ")
	}
	fmt.Printf("VERDICT %s %s | %s (%d checks)\n", c.Q, v, note, c.ran)
}

func (c *Checker) Failed() bool { return len(c.fails) > 0 }

// ------------------------------------------------------------------ scratch

// ScratchDir returns the ephemeral scratch dir. Only /tmp/ms-spike-egress-*
// is ever written.
func ScratchDir() (string, error) {
	if d := os.Getenv("MS_SPIKE_TMP"); d != "" {
		if !strings.HasPrefix(filepath.Clean(d), "/tmp/ms-spike-egress-") {
			return "", fmt.Errorf("MS_SPIKE_TMP %q is outside /tmp/ms-spike-egress-*", d)
		}
		return d, nil
	}
	return os.MkdirTemp("/tmp", "ms-spike-egress-")
}

// WriteScratch writes a file below the scratch dir (0600).
func WriteScratch(dir, name string, data []byte) (string, error) {
	if !strings.HasPrefix(filepath.Clean(dir), "/tmp/ms-spike-egress-") {
		return "", fmt.Errorf("refusing to write outside /tmp/ms-spike-egress-*: %s", dir)
	}
	p := filepath.Join(dir, filepath.Base(name))
	return p, os.WriteFile(p, data, 0o600)
}

// ---------------------------------------------------------------------- PKI

// PKI is an in-process, ephemeral CA plus one leaf certificate.
type PKI struct {
	CAPEM []byte
	Pool  *x509.CertPool
	Leaf  tls.Certificate
}

// NewPKI generates an ECDSA P-256 CA and a leaf valid for names and IPs.
// Keys are never written to disk; only the public CA certificate may be
// exported by the caller for curl.
func NewPKI(names []string, ips []net.IP) (*PKI, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ms-spike ephemeral CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "ms-spike leaf"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     names,
		IPAddresses:  ips,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return &PKI{
		CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		Pool:  pool,
		Leaf:  tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey},
	}, nil
}
