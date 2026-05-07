// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTempPEM writes DER-encoded certificate bytes as a PEM CERTIFICATE block
// to a temp file and returns the path.
func writeTempPEM(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "ca-*.pem")
	require.NoError(t, err)
	err = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: data})
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// generateSelfSignedCertDER returns the DER-encoded bytes of a fresh self-signed
// ECDSA certificate. Because httptest.NewTLSServer() always reuses the same
// internal test cert, this helper is needed to produce a cert that is
// genuinely different from the httptest one.
func generateSelfSignedCertDER(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return der
}

func TestBuildTLSTransport_EmptyPath(t *testing.T) {
	tr, err := buildTLSTransport("")
	require.NoError(t, err)
	require.NotNil(t, tr)
	// Cloned from DefaultTransport — no custom CA pinned, no InsecureSkipVerify.
	if tr.TLSClientConfig != nil {
		assert.Nil(t, tr.TLSClientConfig.RootCAs)
		assert.False(t, tr.TLSClientConfig.InsecureSkipVerify)
	}
}

func TestBuildTLSTransport_ValidCACert(t *testing.T) {
	// httptest.NewTLSServer generates a self-signed cert; grab its raw DER bytes.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rawCert := srv.TLS.Certificates[0].Certificate[0]
	caPath := writeTempPEM(t, rawCert)

	tr, err := buildTLSTransport(caPath)
	require.NoError(t, err)
	require.NotNil(t, tr)
	require.NotNil(t, tr.TLSClientConfig)
	assert.NotNil(t, tr.TLSClientConfig.RootCAs, "RootCAs should be set from the provided CA cert")
	assert.False(t, tr.TLSClientConfig.InsecureSkipVerify, "InsecureSkipVerify must remain false")
}

func TestBuildTLSTransport_ConnectsWithPinnedCA(t *testing.T) {
	// Full round-trip: build a transport from the server's own cert as CA,
	// then make a real HTTPS request through it.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rawCert := srv.TLS.Certificates[0].Certificate[0]
	caPath := writeTempPEM(t, rawCert)

	tr, err := buildTLSTransport(caPath)
	require.NoError(t, err)

	client := &http.Client{Transport: tr}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err, "should connect successfully with the pinned CA cert")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestBuildTLSTransport_RejectsUnknownCA(t *testing.T) {
	// Transport built with a *different* CA should reject the server's cert.
	// Note: httptest.NewTLSServer always reuses the same internal test cert, so
	// we generate a fresh self-signed cert to guarantee a distinct CA.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	caPath := writeTempPEM(t, generateSelfSignedCertDER(t))

	tr, err := buildTLSTransport(caPath)
	require.NoError(t, err)

	client := &http.Client{Transport: tr}
	_, err = client.Get(srv.URL)
	require.Error(t, err, "should fail TLS verification when CA does not match")
	assert.Contains(t, err.Error(), "certificate")
}

func TestBuildTLSTransport_NonExistentFile(t *testing.T) {
	_, err := buildTLSTransport(filepath.Join(t.TempDir(), "does-not-exist.pem"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read CA cert")
}

func TestBuildTLSTransport_InvalidPEM(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "bad-*.pem")
	require.NoError(t, err)
	_, err = f.WriteString("this is not valid PEM data")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = buildTLSTransport(f.Name())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid PEM certificates found")
}

func TestBuildTLSTransport_DoesNotSetInsecureSkipVerify(t *testing.T) {
	// Paranoia check: ensure InsecureSkipVerify is never set regardless of input.
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()
	rawCert := srv.TLS.Certificates[0].Certificate[0]
	caPath := writeTempPEM(t, rawCert)

	for _, path := range []string{"", caPath} {
		tr, err := buildTLSTransport(path)
		require.NoError(t, err)
		if tr.TLSClientConfig != nil {
			assert.False(t, tr.TLSClientConfig.InsecureSkipVerify,
				"InsecureSkipVerify must never be set (path=%q)", path)
		}
	}
}
