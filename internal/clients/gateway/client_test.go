// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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
	"testing"
	"time"
)

func TestProxyK8sRequest(t *testing.T) {
	tests := []struct {
		name          string
		planeType     string
		planeID       string
		crNamespace   string
		crName        string
		k8sPath       string
		rawQuery      string
		serverStatus  int
		serverBody    string
		serverHeaders map[string]string
		wantURLPath   string
		wantErr       bool
	}{
		{
			name:         "namespace-scoped pod list",
			planeType:    "dataplane",
			planeID:      "prod-cluster",
			crNamespace:  "acme",
			crName:       "prod-dp",
			k8sPath:      "api/v1/namespaces/default/pods",
			rawQuery:     "",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods",
		},
		{
			name:         "namespace-scoped with query params",
			planeType:    "dataplane",
			planeID:      "prod-cluster",
			crNamespace:  "acme",
			crName:       "prod-dp",
			k8sPath:      "api/v1/namespaces/default/events",
			rawQuery:     "fieldSelector=involvedObject.name%3Dmy-pod",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"EventList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/events",
		},
		{
			name:         "cluster-scoped with _cluster namespace",
			planeType:    "dataplane",
			planeID:      "shared-cluster",
			crNamespace:  "_cluster",
			crName:       "shared-dp",
			k8sPath:      "api/v1/namespaces/default/pods",
			rawQuery:     "",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/shared-cluster/_cluster/shared-dp/k8s/api/v1/namespaces/default/pods",
		},
		{
			name:         "workflowplane proxy",
			planeType:    "workflowplane",
			planeID:      "ci-cluster",
			crNamespace:  "acme",
			crName:       "ci-wp",
			k8sPath:      "api/v1/namespaces/workflow-ns/pods",
			rawQuery:     "",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/workflowplane/ci-cluster/acme/ci-wp/k8s/api/v1/namespaces/workflow-ns/pods",
		},
		{
			name:         "observabilityplane proxy",
			planeType:    "observabilityplane",
			planeID:      "obs-cluster",
			crNamespace:  "acme",
			crName:       "obs-op",
			k8sPath:      "api/v1/namespaces/monitoring/pods",
			rawQuery:     "",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/observabilityplane/obs-cluster/acme/obs-op/k8s/api/v1/namespaces/monitoring/pods",
		},
		{
			name:         "server returns 404",
			planeType:    "dataplane",
			planeID:      "prod-cluster",
			crNamespace:  "acme",
			crName:       "prod-dp",
			k8sPath:      "api/v1/namespaces/default/pods/nonexistent",
			rawQuery:     "",
			serverStatus: http.StatusNotFound,
			serverBody:   `{"kind":"Status","status":"Failure","message":"pods \"nonexistent\" not found"}`,
			wantURLPath:  "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods/nonexistent",
		},
		{
			name:          "response headers are preserved",
			planeType:     "dataplane",
			planeID:       "prod-cluster",
			crNamespace:   "acme",
			crName:        "prod-dp",
			k8sPath:       "api/v1/namespaces/default/pods",
			rawQuery:      "",
			serverStatus:  http.StatusOK,
			serverBody:    `{"kind":"PodList","items":[]}`,
			serverHeaders: map[string]string{"X-Custom-Header": "test-value"},
			wantURLPath:   "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedPath string
			var receivedQuery string

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedQuery = r.URL.RawQuery

				// Verify it's a GET request
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				// Verify Accept header
				if r.Header.Get("Accept") != "application/json" {
					t.Errorf("Expected Accept: application/json, got %s", r.Header.Get("Accept"))
				}

				// Set custom headers if specified
				for k, v := range tt.serverHeaders {
					w.Header().Set(k, v)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			client := &Client{
				baseURL:    server.URL,
				httpClient: server.Client(),
			}

			resp, err := client.ProxyK8sRequest(context.Background(), tt.planeType, tt.planeID, tt.crNamespace, tt.crName, tt.k8sPath, tt.rawQuery)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			defer resp.Body.Close()

			// Verify the URL path
			if receivedPath != tt.wantURLPath {
				t.Errorf("Request URL path = %q, want %q", receivedPath, tt.wantURLPath)
			}

			// Verify query string
			if tt.rawQuery != "" && receivedQuery != tt.rawQuery {
				t.Errorf("Request query = %q, want %q", receivedQuery, tt.rawQuery)
			}

			// Verify response status code
			if resp.StatusCode != tt.serverStatus {
				t.Errorf("Response status = %d, want %d", resp.StatusCode, tt.serverStatus)
			}

			// Verify response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
			if string(body) != tt.serverBody {
				t.Errorf("Response body = %q, want %q", string(body), tt.serverBody)
			}

			// Verify custom response headers
			for k, v := range tt.serverHeaders {
				if resp.Header.Get(k) != v {
					t.Errorf("Response header %s = %q, want %q", k, resp.Header.Get(k), v)
				}
			}
		})
	}
}

func TestProxyK8sRequest_NetworkError(t *testing.T) {
	client := &Client{
		baseURL:    "https://localhost:1", // unreachable port
		httpClient: http.DefaultClient,
	}

	_, err := client.ProxyK8sRequest(context.Background(), "dataplane", "test", "ns", "name", "api/v1/pods", "")
	if err == nil {
		t.Error("Expected error for unreachable server, got nil")
	}

	if !IsTransientError(err) {
		t.Errorf("Expected TransientError, got %T: %v", err, err)
	}
}

func TestProxyK8sRequest_URLConstruction(t *testing.T) {
	tests := []struct {
		name         string
		planeType    string
		planeID      string
		crNamespace  string
		crName       string
		k8sPath      string
		rawQuery     string
		wantContains string
	}{
		{
			name:         "URL without query",
			planeType:    "dataplane",
			planeID:      "prod",
			crNamespace:  "ns",
			crName:       "dp",
			k8sPath:      "api/v1/pods",
			rawQuery:     "",
			wantContains: "/api/proxy/dataplane/prod/ns/dp/k8s/api/v1/pods",
		},
		{
			name:         "URL with query",
			planeType:    "dataplane",
			planeID:      "prod",
			crNamespace:  "ns",
			crName:       "dp",
			k8sPath:      "api/v1/events",
			rawQuery:     "fieldSelector=key%3Dvalue",
			wantContains: "?fieldSelector=key%3Dvalue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedURL string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedURL = r.URL.String()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := &Client{
				baseURL:    server.URL,
				httpClient: server.Client(),
			}

			resp, err := client.ProxyK8sRequest(context.Background(), tt.planeType, tt.planeID, tt.crNamespace, tt.crName, tt.k8sPath, tt.rawQuery)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			resp.Body.Close()

			if !strings.Contains(receivedURL, tt.wantContains) {
				t.Errorf("Received URL %q does not contain %q", receivedURL, tt.wantContains)
			}
		})
	}
}

func TestProxyK8sRequest_CallerMustCloseBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test body"))
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	resp, err := client.ProxyK8sRequest(context.Background(), "dataplane", "test", "ns", "name", "api/v1/pods", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify body is readable (not already closed)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if string(body) != "test body" {
		t.Errorf("Body = %q, want %q", string(body), "test body")
	}

	// Now close it (caller's responsibility)
	resp.Body.Close()
}

// writeTestKeyPairFiles generates a self-signed certificate and its EC private
// key, writes both to PEM files in a temp dir, and returns their paths. The pair
// is valid for tls.LoadX509KeyPair.
func writeTestKeyPairFiles(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "tls.crt")
	keyFile = filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

// caPEM returns a self-signed CA certificate encoded as PEM.
func caPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestBuildTLSConfig(t *testing.T) {
	certFile, keyFile := writeTestKeyPairFiles(t)

	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, caPEM(t), 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	badPairDir := t.TempDir()
	badCert := filepath.Join(badPairDir, "bad.crt")
	badKey := filepath.Join(badPairDir, "bad.key")
	if err := os.WriteFile(badCert, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write bad cert: %v", err)
	}
	if err := os.WriteFile(badKey, []byte("not a key"), 0o600); err != nil {
		t.Fatalf("write bad key: %v", err)
	}

	tests := []struct {
		name       string
		config     *TLSConfig
		wantErr    bool
		wantErrMsg string
		verify     func(t *testing.T, c *tlsConfigResult)
	}{
		{
			name:   "mTLS with both cert and key loads client certificate",
			config: &TLSConfig{ClientCertFile: certFile, ClientKeyFile: keyFile},
			verify: func(t *testing.T, c *tlsConfigResult) {
				if c.numCertificates != 1 {
					t.Errorf("expected 1 client certificate, got %d", c.numCertificates)
				}
			},
		},
		{
			name:       "mTLS with only cert file returns error",
			config:     &TLSConfig{ClientCertFile: certFile},
			wantErr:    true,
			wantErrMsg: "both ClientCertFile and ClientKeyFile must be set",
		},
		{
			name:       "mTLS with only key file returns error",
			config:     &TLSConfig{ClientKeyFile: keyFile},
			wantErr:    true,
			wantErrMsg: "both ClientCertFile and ClientKeyFile must be set",
		},
		{
			name:       "mTLS with invalid cert/key pair returns error",
			config:     &TLSConfig{ClientCertFile: badCert, ClientKeyFile: badKey},
			wantErr:    true,
			wantErrMsg: "failed to load client key pair",
		},
		{
			name:   "insecure skip verify",
			config: &TLSConfig{InsecureSkipVerify: true},
			verify: func(t *testing.T, c *tlsConfigResult) {
				if !c.insecureSkipVerify {
					t.Error("expected InsecureSkipVerify to be true")
				}
			},
		},
		{
			name:   "CA data populates root pool",
			config: &TLSConfig{CAData: caPEM(t)},
			verify: func(t *testing.T, c *tlsConfigResult) {
				if !c.hasRootCAs {
					t.Error("expected RootCAs to be set from CAData")
				}
			},
		},
		{
			name:   "CA file populates root pool",
			config: &TLSConfig{CAFile: caFile},
			verify: func(t *testing.T, c *tlsConfigResult) {
				if !c.hasRootCAs {
					t.Error("expected RootCAs to be set from CAFile")
				}
			},
		},
		{
			name:       "missing CA file returns error",
			config:     &TLSConfig{CAFile: filepath.Join(t.TempDir(), "missing.crt")},
			wantErr:    true,
			wantErrMsg: "failed to read CA file",
		},
		{
			name:       "invalid CA data returns error",
			config:     &TLSConfig{CAData: []byte("not a certificate")},
			wantErr:    true,
			wantErrMsg: "failed to parse CA certificate",
		},
		{
			name:   "server name is set and TLS 1.2 enforced",
			config: &TLSConfig{ServerName: "gateway.example.com"},
			verify: func(t *testing.T, c *tlsConfigResult) {
				if c.serverName != "gateway.example.com" {
					t.Errorf("expected serverName gateway.example.com, got %q", c.serverName)
				}
			},
		},
		{
			name:   "mTLS combined with CA verification",
			config: &TLSConfig{CAData: caPEM(t), ClientCertFile: certFile, ClientKeyFile: keyFile},
			verify: func(t *testing.T, c *tlsConfigResult) {
				if !c.hasRootCAs {
					t.Error("expected RootCAs to be set")
				}
				if c.numCertificates != 1 {
					t.Errorf("expected 1 client certificate, got %d", c.numCertificates)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := buildTLSConfig(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// MinVersion must always be enforced.
			if cfg.MinVersion == 0 {
				t.Error("expected MinVersion to be set (TLS 1.2)")
			}
			if tt.verify != nil {
				tt.verify(t, &tlsConfigResult{
					numCertificates:    len(cfg.Certificates),
					hasRootCAs:         cfg.RootCAs != nil,
					insecureSkipVerify: cfg.InsecureSkipVerify,
					serverName:         cfg.ServerName,
				})
			}
		})
	}
}

// tlsConfigResult flattens the fields of a *tls.Config asserted by the tests so
// the table stays readable.
type tlsConfigResult struct {
	numCertificates    int
	hasRootCAs         bool
	insecureSkipVerify bool
	serverName         string
}

func TestNewClientWithConfig(t *testing.T) {
	certFile, keyFile := writeTestKeyPairFiles(t)

	t.Run("empty baseURL returns error", func(t *testing.T) {
		_, err := NewClientWithConfig(&Config{})
		if err == nil {
			t.Fatal("expected error for empty baseURL, got nil")
		}
		if !strings.Contains(err.Error(), "baseURL is required") {
			t.Errorf("error = %q, want it to mention baseURL", err.Error())
		}
	})

	t.Run("valid config with mTLS returns client", func(t *testing.T) {
		client, err := NewClientWithConfig(&Config{
			BaseURL: "https://gateway.example.com",
			TLS:     TLSConfig{ClientCertFile: certFile, ClientKeyFile: keyFile},
			Timeout: 5 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("expected non-nil client")
		}
	})

	t.Run("invalid TLS config surfaces build error", func(t *testing.T) {
		_, err := NewClientWithConfig(&Config{
			BaseURL: "https://gateway.example.com",
			TLS:     TLSConfig{ClientCertFile: certFile}, // key missing
		})
		if err == nil {
			t.Fatal("expected error for incomplete mTLS config, got nil")
		}
		if !strings.Contains(err.Error(), "failed to build TLS config") {
			t.Errorf("error = %q, want it to wrap the TLS build failure", err.Error())
		}
	})
}
