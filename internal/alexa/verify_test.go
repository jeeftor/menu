package alexa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestValidateCertURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid", "https://s3.amazonaws.com/echo.api/echo-api-cert.pem", false},
		{"wrong scheme", "http://s3.amazonaws.com/echo.api/echo-api-cert.pem", true},
		{"wrong host", "https://example.com/echo.api/echo-api-cert.pem", true},
		{"wrong path", "https://s3.amazonaws.com/other/echo-api-cert.pem", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCertURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVerifyTimestamp(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := VerifyTimestamp(now, defaultMaxTimestampAge); err != nil {
		t.Fatalf("recent timestamp should pass: %v", err)
	}

	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	if err := VerifyTimestamp(old, defaultMaxTimestampAge); err == nil {
		t.Fatal("stale timestamp should fail")
	}

	if err := VerifyTimestamp("not-a-timestamp", defaultMaxTimestampAge); err == nil {
		t.Fatal("invalid timestamp should fail")
	}
}

func TestVerifySignature(t *testing.T) {
	certPEM, key, cleanup := makeTestCert(t)
	defer cleanup()

	body := []byte(`{"request":{"type":"LaunchRequest"}}`)
	hash := sha1.Sum(body)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, hash[:])
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	// Override cache fetcher to return our test cert and root pool to trust it.
	oldFetcher := sharedCertCache.fetcher
	sharedCertCache.fetcher = func(string) ([]byte, error) { return certPEM, nil }
	defer func() { sharedCertCache.fetcher = oldFetcher }()

	pool := x509.NewCertPool()
	block, _ := pem.Decode(certPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)
	pool.AddCert(cert)
	oldRoots := rootsProvider
	rootsProvider = func() (*x509.CertPool, error) { return pool, nil }
	defer func() { rootsProvider = oldRoots }()

	// Clear cache so our fetcher is used.
	sharedCertCache.mu.Lock()
	sharedCertCache.entries = make(map[string]*cachedCert)
	sharedCertCache.mu.Unlock()

	url := "https://s3.amazonaws.com/echo.api/echo-api-cert.pem"
	if err := VerifySignature(body, base64.StdEncoding.EncodeToString(sig), url); err != nil {
		t.Fatalf("valid signature should verify: %v", err)
	}

	badSig := append([]byte{0}, sig...)
	if err := VerifySignature(body, base64.StdEncoding.EncodeToString(badSig), url); err == nil {
		t.Fatal("invalid signature should fail")
	}
}

// makeTestCert returns a self-signed cert valid for echo-api.amazon.com, its
// private key, and a cleanup function.
func makeTestCert(t *testing.T) ([]byte, *rsa.PrivateKey, func()) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "echo-api.amazon.com"},
		DNSNames:     []string{"echo-api.amazon.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return certPEM, key, func() {}
}
