package alexa

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// allowedCertHost is the only host Amazon serves ASK signature certificates from.
const allowedCertHost = "s3.amazonaws.com"

// allowedCertPathPrefix is the required path prefix for ASK signature certs.
const allowedCertPathPrefix = "/echo.api/"

// allowedCertSAN is the required subject alternative name on the leaf cert.
const allowedCertSAN = "echo-api.amazon.com"

// defaultMaxTimestampAge is how stale an Alexa request timestamp may be.
const defaultMaxTimestampAge = 150 * time.Second

// certCacheTTL is how long downloaded signature certs are cached.
const certCacheTTL = 24 * time.Hour

// certFetcher downloads certificates. It is overridable for tests.
type certFetcher func(url string) ([]byte, error)

var defaultFetcher certFetcher = func(url string) ([]byte, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cert fetch returned status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// certCache stores downloaded certs keyed by URL.
type certCache struct {
	mu      sync.RWMutex
	fetcher certFetcher
	entries map[string]*cachedCert
	maxAge  time.Duration
}

type cachedCert struct {
	leaf          *x509.Certificate
	intermediates []*x509.Certificate
	fetchedAt     time.Time
}

var sharedCertCache = &certCache{
	fetcher: defaultFetcher,
	entries: make(map[string]*cachedCert),
	maxAge:  certCacheTTL,
}

// VerifySignature checks an Alexa request signature against Amazon's certificate.
// It fetches and caches the cert chain from certURL.
func VerifySignature(body []byte, signatureB64, certURL string) error {
	if err := validateCertURL(certURL); err != nil {
		return fmt.Errorf("invalid cert URL: %w", err)
	}
	leaf, intermediates, err := sharedCertCache.getCert(certURL)
	if err != nil {
		return fmt.Errorf("fetching cert: %w", err)
	}
	if err := validateCertificate(leaf, intermediates); err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decoding signature: %w", err)
	}
	hash := sha1.Sum(body)
	pub, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate public key is not RSA")
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA1, hash[:], sig); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	return nil
}

// VerifyTimestamp checks that an ISO-8601 request timestamp is recent enough.
func VerifyTimestamp(timestamp string, maxAge time.Duration) error {
	if timestamp == "" {
		return fmt.Errorf("missing request timestamp")
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("parsing timestamp %q: %w", timestamp, err)
	}
	if time.Since(t) > maxAge {
		return fmt.Errorf("request timestamp %q is older than %s", timestamp, maxAge)
	}
	return nil
}

// validateCertURL ensures the cert URL comes from the official Amazon location.
func validateCertURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return fmt.Errorf("scheme must be https, got %q", u.Scheme)
	}
	if !strings.EqualFold(u.Host, allowedCertHost) {
		return fmt.Errorf("host must be %q, got %q", allowedCertHost, u.Host)
	}
	if !strings.HasPrefix(u.Path, allowedCertPathPrefix) {
		return fmt.Errorf("path must start with %q, got %q", allowedCertPathPrefix, u.Path)
	}
	return nil
}

func (cc *certCache) getCert(certURL string) (*x509.Certificate, []*x509.Certificate, error) {
	cc.mu.RLock()
	ent, ok := cc.entries[certURL]
	cc.mu.RUnlock()
	if ok && time.Since(ent.fetchedAt) < cc.maxAge {
		return ent.leaf, ent.intermediates, nil
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()
	ent, ok = cc.entries[certURL]
	if ok && time.Since(ent.fetchedAt) < cc.maxAge {
		return ent.leaf, ent.intermediates, nil
	}

	data, err := cc.fetcher(certURL)
	if err != nil {
		return nil, nil, err
	}
	leaf, intermediates, err := parseCertificateChain(data)
	if err != nil {
		return nil, nil, err
	}
	cc.entries[certURL] = &cachedCert{
		leaf:          leaf,
		intermediates: intermediates,
		fetchedAt:     time.Now(),
	}
	return leaf, intermediates, nil
}

// parseCertificateChain parses a PEM bundle and returns the leaf (first block)
// plus the remaining intermediate certificates.
func parseCertificateChain(data []byte) (*x509.Certificate, []*x509.Certificate, error) {
	var certs []*x509.Certificate
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			data = rest
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing certificate: %w", err)
		}
		certs = append(certs, cert)
		data = rest
	}
	if len(certs) == 0 {
		return nil, nil, fmt.Errorf("no certificates found in PEM data")
	}
	return certs[0], certs[1:], nil
}

// rootsProvider returns the cert pool used to verify the Amazon cert chain.
// It is overridable for tests.
var rootsProvider = x509.SystemCertPool

// validateCertificate checks the cert chain and SAN per Amazon's ASK rules.
func validateCertificate(leaf *x509.Certificate, intermediates []*x509.Certificate) error {
	roots, err := rootsProvider()
	if err != nil {
		return fmt.Errorf("loading cert pool: %w", err)
	}
	pool := x509.NewCertPool()
	for _, c := range intermediates {
		pool.AddCert(c)
	}
	opts := x509.VerifyOptions{
		DNSName:       allowedCertSAN,
		Roots:         roots,
		Intermediates: pool,
	}
	if _, err := leaf.Verify(opts); err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return fmt.Errorf("certificate is not valid at %s", now)
	}
	return nil
}
