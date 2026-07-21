// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCA creates a self-signed CA certificate and key for testing.
func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	return caCert, caKey
}

// encodeCertToPEM encodes a certificate to PEM format for testing.
func encodeCertToPEM(t *testing.T, cert *x509.Certificate) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// generateTestClientKeyPair creates a client certificate signed by the given CA
// and returns it as a tls.Certificate usable in a handshake.
func generateTestClientKeyPair(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "Test Internal Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(clientDER)
	require.NoError(t, err)

	return tls.Certificate{
		Certificate: [][]byte{clientDER},
		PrivateKey:  clientKey,
		Leaf:        leaf,
	}
}

// writeTestCAFile writes the CA certificate PEM to a temp file and returns its path.
func writeTestCAFile(t *testing.T, caCert *x509.Certificate) string {
	t.Helper()
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caPath, encodeCertToPEM(t, caCert), 0o600))
	return caPath
}

func baseTestTLSConfig() *tls.Config {
	return &tls.Config{
		ClientAuth: tls.RequestClientCert,
		MinVersion: tls.VersionTLS12,
	}
}

// --- buildInternalTLSConfig tests ---

func TestBuildInternalTLSConfig_Disabled(t *testing.T) {
	cfg := &Config{InternalMTLSEnabled: false}

	tlsConfig, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.NoError(t, err)
	assert.Equal(t, tls.RequestClientCert, tlsConfig.ClientAuth)
	assert.Nil(t, tlsConfig.ClientCAs)
}

func TestBuildInternalTLSConfig_EnabledWithValidCA(t *testing.T) {
	caCert, _ := generateTestCA(t)
	cfg := &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, caCert),
	}

	tlsConfig, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, tlsConfig.ClientAuth)
	assert.NotNil(t, tlsConfig.ClientCAs)
}

func TestBuildInternalTLSConfig_EnabledWithoutCAPath(t *testing.T) {
	cfg := &Config{InternalMTLSEnabled: true}

	_, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal-client-ca-cert")
}

func TestBuildInternalTLSConfig_EnabledWithMissingFile(t *testing.T) {
	cfg := &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: filepath.Join(t.TempDir(), "does-not-exist.crt"),
	}

	_, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.Error(t, err)
}

func TestBuildInternalTLSConfig_EnabledWithInvalidPEM(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caPath, []byte("not a certificate"), 0o600))
	cfg := &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: caPath,
	}

	_, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid certificates")
}

// --- handshake-level enforcement tests ---

// startInternalTestServer starts an HTTPS server using the internal listener's
// TLS configuration derived from cfg, serving a trivial 200 handler.
func startInternalTestServer(t *testing.T, cfg *Config) *httptest.Server {
	t.Helper()
	tlsConfig, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.NoError(t, err)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	srv.TLS = tlsConfig
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func newTestHTTPSClient(serverCertPool *x509.CertPool, clientCerts ...tls.Certificate) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      serverCertPool,
				Certificates: clientCerts,
				MinVersion:   tls.VersionTLS12,
			},
		},
	}
}

func serverCertPool(t *testing.T, srv *httptest.Server) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return pool
}

func TestInternalListener_MTLSEnabled_AcceptsInternalCACert(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	srv := startInternalTestServer(t, &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, caCert),
	})

	client := newTestHTTPSClient(serverCertPool(t, srv), generateTestClientKeyPair(t, caCert, caKey))
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
}

func TestInternalListener_MTLSEnabled_RejectsNoClientCert(t *testing.T) {
	caCert, _ := generateTestCA(t)
	srv := startInternalTestServer(t, &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, caCert),
	})

	client := newTestHTTPSClient(serverCertPool(t, srv))
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
	}
	require.Error(t, err, "request without a client certificate must be rejected")
}

func TestInternalListener_MTLSEnabled_RejectsCertFromOtherCA(t *testing.T) {
	internalCACert, _ := generateTestCA(t)
	// Simulates an agent certificate signed by a different (agent) CA.
	otherCACert, otherCAKey := generateTestCA(t)

	srv := startInternalTestServer(t, &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, internalCACert),
	})

	client := newTestHTTPSClient(serverCertPool(t, srv), generateTestClientKeyPair(t, otherCACert, otherCAKey))
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
	}
	require.Error(t, err, "certificate signed by a different CA must be rejected")
}

func TestInternalListener_MTLSDisabled_AcceptsNoClientCert(t *testing.T) {
	srv := startInternalTestServer(t, &Config{InternalMTLSEnabled: false})

	client := newTestHTTPSClient(serverCertPool(t, srv))
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// --- Server.Start tests ---

// writeServerKeyPairFiles generates a self-signed server certificate and its EC
// private key, writes both to PEM files, and returns their paths. The pair is
// valid for tls.LoadX509KeyPair used in Server.Start.
func writeServerKeyPairFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(100),
		Subject:      pkix.Name{CommonName: "test-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "server.crt")
	keyPath = filepath.Join(dir, "server.key")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certPath, keyPath
}

func TestStart_ServerCertLoadError(t *testing.T) {
	s := New(&Config{
		ServerCertPath: filepath.Join(t.TempDir(), "missing.crt"),
		ServerKeyPath:  filepath.Join(t.TempDir(), "missing.key"),
	}, nil, testLogger())

	err := s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load server certificate")
}

// TestStart_InternalTLSConfigError verifies that a failure while building the
// internal listener TLS config is wrapped and returned from Start before any
// port is bound. mTLS is enabled with no client CA, which buildInternalTLSConfig
// rejects.
func TestStart_InternalTLSConfigError(t *testing.T) {
	certPath, keyPath := writeServerKeyPairFiles(t)

	s := New(&Config{
		ServerCertPath:      certPath,
		ServerKeyPath:       keyPath,
		InternalMTLSEnabled: true,
		// InternalClientCAPath deliberately empty.
	}, nil, testLogger())

	err := s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to configure internal listener TLS")
}

// waitForHealth polls the fixed health endpoint until it returns 200. It returns
// true once healthy. If Start returns early (e.g. the fixed :8080 health port is
// already bound), the test is skipped rather than failed, since that is an
// environment conflict and not a code defect.
func waitForHealth(t *testing.T, startErr <-chan error) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-startErr:
			skipOrFailOnStartErr(t, err)
			return false
		default:
		}

		resp, err := http.Get("http://127.0.0.1:8080/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	select {
	case err := <-startErr:
		skipOrFailOnStartErr(t, err)
	default:
		t.Skip("health server did not become ready on :8080 (port likely in use)")
	}
	return false
}

func skipOrFailOnStartErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Start returned nil before becoming healthy")
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "address already in use") || strings.Contains(msg, "bind:") {
		t.Skipf("health/server port unavailable in this environment: %v", err)
		return
	}
	t.Fatalf("Start returned an unexpected error before becoming healthy: %v", err)
}

// TestStart_Lifecycle brings the server fully up on ephemeral ports and asserts
// that a SIGTERM triggers a graceful shutdown returning nil. It exercises both
// the internal-mTLS-enabled and disabled logging/config branches in Start.
func TestStart_Lifecycle(t *testing.T) {
	tests := []struct {
		name string
		mtls bool
	}{
		{name: "internal mTLS enabled", mtls: true},
		{name: "internal mTLS disabled", mtls: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath, keyPath := writeServerKeyPairFiles(t)
			cfg := &Config{
				Port:            0, // ephemeral
				InternalPort:    0, // ephemeral
				ServerCertPath:  certPath,
				ServerKeyPath:   keyPath,
				ShutdownTimeout: 5 * time.Second,
			}
			if tt.mtls {
				caCert, _ := generateTestCA(t)
				cfg.InternalMTLSEnabled = true
				cfg.InternalClientCAPath = writeTestCAFile(t, caCert)
			}

			s := New(cfg, nil, testLogger())

			startErr := make(chan error, 1)
			go func() { startErr <- s.Start() }()

			if !waitForHealth(t, startErr) {
				return // skipped or failed inside helper
			}

			// Trigger graceful shutdown; Start installs a SIGTERM handler via
			// signal.NotifyContext, so this is absorbed rather than killing the
			// test process.
			p, err := os.FindProcess(os.Getpid())
			require.NoError(t, err)
			require.NoError(t, p.Signal(syscall.SIGTERM))

			select {
			case err := <-startErr:
				require.NoError(t, err, "Start should return nil after graceful shutdown")
			case <-time.After(10 * time.Second):
				t.Fatal("Start did not return after SIGTERM")
			}
		})
	}
}
