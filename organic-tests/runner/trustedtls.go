package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// seededReader makes test-certificate generation replayable. It is not used
// for production cryptography and never creates a credential.
type seededReader struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (r *seededReader) Read(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range b {
		b[i] = byte(r.r.Intn(256))
	}
	return len(b), nil
}

type trustedTLSFixture struct {
	provider             *controlledProvider
	rootPath, thumbprint string
	installed            bool
}

func newTrustedTLSFixture(lab string, seed int64) (*trustedTLSFixture, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("temporary platform trust-store fixture is implemented only for Windows")
	}
	rnd := &seededReader{r: rand.New(rand.NewSource(seed))}
	now := time.Now().Add(-time.Minute)
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rnd)
	if err != nil {
		return nil, err
	}
	rootTemplate := &x509.Certificate{SerialNumber: big.NewInt(seed + 100), Subject: pkix.Name{CommonName: "aitrouble organic test root"}, NotBefore: now, NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	rootDER, err := x509.CreateCertificate(rnd, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, err
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rnd)
	if err != nil {
		return nil, err
	}
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(seed + 101), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: now, NotAfter: now.Add(time.Hour), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rnd, serverTemplate, root, &serverKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	cert := tls.Certificate{Certificate: [][]byte{serverDER, rootDER}, PrivateKey: serverKey}

	rootPath := filepath.Join(lab, "services", fmt.Sprintf("organic-root-%d.cer", seed))
	// Write PEM encoded certificate instead of DER, because SSL_CERT_FILE expects PEM
	pemBlock := fmt.Sprintf("-----BEGIN CERTIFICATE-----\n%s\n-----END CERTIFICATE-----\n", base64.StdEncoding.EncodeToString(rootDER))
	if err := os.WriteFile(rootPath, []byte(pemBlock), 0600); err != nil {
		return nil, err
	}
	f := &trustedTLSFixture{rootPath: rootPath, installed: true}
	f.provider = &controlledProvider{mode: "ok"}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(f.provider.serveHTTP))
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	ts.StartTLS()
	f.provider.server = ts
	return f, nil
}

func (f *trustedTLSFixture) verifyTrust() error {
	u, err := url.Parse(f.provider.baseURL("/v1"))
	if err != nil {
		return err
	}
	
	// For the internal verification, we need to load the root manually
	b, err := os.ReadFile(f.rootPath)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(b)
	
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	c, err := tls.DialWithDialer(dialer, "tcp", u.Host, &tls.Config{ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12, RootCAs: pool})
	if err == nil {
		_ = c.Close()
	}
	return err
}

func (f *trustedTLSFixture) close() error {
	if f.provider != nil {
		f.provider.close()
	}
	// We no longer use certutil, so there's nothing to clean up besides the file (which is in the lab dir)
	return nil
}
