// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- PrepareWebhookNotificationConfig ---

func TestPrepareWebhookNotificationConfig(t *testing.T) {
	logger := discardLogger()

	tests := []struct {
		name          string
		configMapData map[string]string
		secretData    map[string][]byte
		wantErr       bool
		wantErrMsg    string
		check         func(t *testing.T, cfg WebhookConfig)
	}{
		{
			name:          "missing webhook URL returns error",
			configMapData: map[string]string{},
			wantErr:       true,
			wantErrMsg:    "webhook URL not found in ConfigMap or Secret",
		},
		{
			name:          "URL from Secret when the ConfigMap has none",
			configMapData: map[string]string{},
			secretData: map[string][]byte{
				"webhook.url": []byte("https://hooks.example.com/services/T000/B000/token\n"),
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "https://hooks.example.com/services/T000/B000/token", cfg.URL)
				assert.True(t, cfg.URLIsSecret)
			},
		},
		{
			name:          "inline URL in ConfigMap wins over a URL left in the Secret",
			configMapData: map[string]string{"webhook.url": "https://example.com/hook"},
			secretData: map[string][]byte{
				"webhook.url": []byte("https://hooks.example.com/services/stale"),
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "https://example.com/hook", cfg.URL)
				assert.False(t, cfg.URLIsSecret)
			},
		},
		{
			name:          "basic config with URL and no headers",
			configMapData: map[string]string{"webhook.url": "https://example.com/hook"},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "https://example.com/hook", cfg.URL)
				assert.Empty(t, cfg.Headers)
				assert.Empty(t, cfg.PayloadTemplate)
			},
		},
		{
			name: "headers from ConfigMap inline values",
			configMapData: map[string]string{
				"webhook.url":            "https://example.com/hook",
				"webhook.headers":        "X-Token,X-Env",
				"webhook.header.X-Token": "tok123",
				"webhook.header.X-Env":   "prod",
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "tok123", cfg.Headers["X-Token"])
				assert.Equal(t, "prod", cfg.Headers["X-Env"])
			},
		},
		{
			name: "headers resolved from Secret",
			configMapData: map[string]string{
				"webhook.url":     "https://example.com/hook",
				"webhook.headers": "Authorization",
			},
			secretData: map[string][]byte{
				"webhook.header.Authorization": []byte("Bearer secrettoken"),
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "Bearer secrettoken", cfg.Headers["Authorization"])
			},
		},
		{
			name: "header key missing from both ConfigMap and Secret is omitted",
			configMapData: map[string]string{
				"webhook.url":     "https://example.com/hook",
				"webhook.headers": "X-Missing",
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				_, exists := cfg.Headers["X-Missing"]
				assert.False(t, exists)
			},
		},
		{
			name: "empty key in comma-separated header list is skipped",
			configMapData: map[string]string{
				"webhook.url":            "https://example.com/hook",
				"webhook.headers":        "X-Token,,X-Env",
				"webhook.header.X-Token": "t",
				"webhook.header.X-Env":   "e",
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Len(t, cfg.Headers, 2)
			},
		},
		{
			name: "payload template populated from ConfigMap",
			configMapData: map[string]string{
				"webhook.url":             "https://example.com/hook",
				"webhook.payloadTemplate": `{"alert": "alertName"}`,
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, `{"alert": "alertName"}`, cfg.PayloadTemplate)
			},
		},
		{
			name: "nil secret does not panic when headers reference secret",
			configMapData: map[string]string{
				"webhook.url":     "https://example.com/hook",
				"webhook.headers": "X-Token",
			},
			secretData: nil,
			check: func(t *testing.T, cfg WebhookConfig) {
				// Should complete without panic; header omitted since not in ConfigMap or Secret
				_, exists := cfg.Headers["X-Token"]
				assert.False(t, exists)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{Data: tc.configMapData}
			var secret *corev1.Secret
			if tc.secretData != nil {
				secret = &corev1.Secret{Data: tc.secretData}
			}

			cfg, err := PrepareWebhookNotificationConfig(cm, secret, logger)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

// --- SendWebhookWithConfig ---

func TestSendWebhookWithConfig_EmptyURL(t *testing.T) {
	err := SendWebhookWithConfig(context.Background(), &WebhookConfig{}, map[string]interface{}{"key": "val"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook URL is required")
}

func TestSendWebhookWithConfig_Success(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		headers      map[string]string
		checkRequest func(t *testing.T, r *http.Request)
	}{
		{
			name:       "200 response returns nil",
			statusCode: http.StatusOK,
		},
		{
			name:       "201 response returns nil",
			statusCode: http.StatusCreated,
		},
		{
			name:       "custom headers forwarded",
			statusCode: http.StatusOK,
			headers:    map[string]string{"X-Custom": "value123"},
			checkRequest: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "value123", r.Header.Get("X-Custom"))
			},
		},
		{
			name:       "Content-Type is always application/json",
			statusCode: http.StatusOK,
			checkRequest: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.checkRequest != nil {
					tc.checkRequest(t, r)
				}
				w.WriteHeader(tc.statusCode)
			}))
			defer ts.Close()

			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = ts.Client()

			cfg := &WebhookConfig{URL: ts.URL, Headers: tc.headers}
			err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{"alert": "test"})
			assert.NoError(t, err)
		})
	}
}

func TestSendWebhookWithConfig_Non2xxResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErrContain string
	}{
		{
			name:           "404 response returns error with status code",
			statusCode:     http.StatusNotFound,
			wantErrContain: "404",
		},
		{
			name:           "500 response with body includes body in error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "internal server error details",
			wantErrContain: "internal server error details",
		},
		{
			name:           "500 response without body has status code in error",
			statusCode:     http.StatusInternalServerError,
			wantErrContain: "500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				if tc.responseBody != "" {
					_, _ = io.WriteString(w, tc.responseBody)
				}
			}))
			defer ts.Close()

			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = ts.Client()

			cfg := &WebhookConfig{URL: ts.URL}
			err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContain)
		})
	}
}

func TestSendWebhookWithConfig_ConnectionRefused(t *testing.T) {
	// Use an address that will refuse connections
	cfg := &WebhookConfig{URL: "http://127.0.0.1:1/webhook"}
	err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send webhook request")
}

func TestSendWebhookWithConfig_ErrorsLeaveOutTheURL(t *testing.T) {
	const (
		secretPath  = "/services/T000/B000/s3cr3t-token"
		secretQuery = "key=s3cr3t-query"
	)

	tests := []struct {
		name           string
		url            string
		wantErrContain string
	}{
		{
			name:           "connection refused",
			url:            "http://127.0.0.1:1" + secretPath + "?" + secretQuery,
			wantErrContain: "failed to send webhook request to http://127.0.0.1:1/<redacted>",
		},
		{
			name:           "URL that does not parse",
			url:            "https://example.com" + secretPath + "\n",
			wantErrContain: "webhook URL is not a valid URL",
		},
		{
			name:           "secret value that is not a URL",
			url:            "s3cr3t-value",
			wantErrContain: "webhook URL must be an http or https URL with a host",
		},
		{
			name:           "URL with another scheme",
			url:            "ftp://example.com" + secretPath,
			wantErrContain: "webhook URL must be an http or https URL with a host",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: tc.url}, map[string]interface{}{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContain)
			assert.NotContains(t, err.Error(), "s3cr3t")
		})
	}

	// What a receiver may put in an error response, given the request it received.
	echoes := []struct {
		name string
		body func(r *http.Request) string
	}{
		{name: "request URI", body: func(r *http.Request) string {
			return "The requested URL " + r.URL.RequestURI() + " was not found on this server."
		}},
		{name: "last path segment", body: func(r *http.Request) string {
			return "invalid token " + r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		}},
		{name: "query value", body: func(r *http.Request) string {
			return "invalid key " + r.URL.Query().Get("key")
		}},
		{name: "path without its first segment", body: func(r *http.Request) string {
			return "no route " + strings.TrimPrefix(r.URL.Path, "/services")
		}},
		{name: "JSON-escaped path", body: func(r *http.Request) string {
			return `{"path":"` + strings.ReplaceAll(r.URL.Path, "/", `\/`) + `"}`
		}},
		{name: "HTML-escaped query", body: func(r *http.Request) string {
			return "bad parameters: team=a&amp;" + r.URL.RawQuery
		}},
		{name: "URL-encoded request URI", body: func(r *http.Request) string {
			return "sign in at /login?next=" + url.QueryEscape(r.URL.RequestURI())
		}},
	}
	for _, tc := range echoes {
		t.Run("receiver echoes the "+tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, tc.body(r))
			}))
			defer ts.Close()

			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = ts.Client()

			cfg := &WebhookConfig{URL: ts.URL + secretPath + "?" + secretQuery}
			err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "404")
			assert.Contains(t, err.Error(), redactedPlaceholder)
			assert.NotContains(t, err.Error(), "s3cr3t")
		})
	}

	// A receiver can quote a credential in an escaping that differs from the URL's own. Such text
	// cannot be scrubbed reliably, so the response is left out.
	escaped := []struct {
		name string
		body string
	}{
		{name: "JSON unicode escape", body: `{"error":"invalid credential ab\u0026cdef12345"}`},
		{name: "HTML numeric entity", body: "invalid credential ab&#38;cdef12345"},
		{name: "percent-encoded twice", body: "invalid credential ab%2526cdef12345"},
		{name: "backslash-escaped", body: `invalid credential ab\&cdef12345`},
	}

	t.Run("receiver quotes a credential with a JSON newline escape", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"invalid credential ab\ncdef12345"}`)
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		cfg := &WebhookConfig{URL: ts.URL + "/hook?key=ab%0Acdef12345"}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response omitted because it quotes the webhook URL")
		assert.NotContains(t, err.Error(), "cdef12345")
	})

	t.Run("receiver quotes a username-only credential", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _, _ := r.BasicAuth()
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "invalid credential "+user)
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		cfg := &WebhookConfig{URL: strings.Replace(ts.URL, "http://", "http://token-s3cr3t-123@", 1) + "/hook"}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
		assert.Contains(t, err.Error(), "invalid credential "+redactedPlaceholder)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})

	t.Run("receiver's response is never shown for a URL from a secret", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "unrelated receiver text")
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		cfg := &WebhookConfig{URL: ts.URL + secretPath, URLIsSecret: true}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "400")
		assert.Contains(t, err.Error(), "response omitted because the webhook URL is a secret")
		assert.NotContains(t, err.Error(), "unrelated receiver text")
	})
	for _, tc := range escaped {
		t.Run("receiver quotes a credential with a "+tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer ts.Close()

			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = ts.Client()

			cfg := &WebhookConfig{URL: ts.URL + "/hook?key=ab%26cdef12345"}
			err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "403")
			assert.Contains(t, err.Error(), "response omitted because it quotes the webhook URL")
			assert.NotContains(t, err.Error(), "cdef12345")
		})
	}

	t.Run("receiver redirects to a malformed Location that quotes the path", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", r.URL.Path+"/%zz")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		cfg := &WebhookConfig{URL: ts.URL + secretPath + "?" + secretQuery}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errCauseOmitted)
		assert.NotContains(t, err.Error(), "Location")
		assert.NotContains(t, err.Error(), "s3cr3t")
	})

	t.Run("receiver redirects to a malformed Location that quotes a credential in another escaping", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/ab%2526cdef12345/%zz")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		cfg := &WebhookConfig{URL: ts.URL + "/hook?key=ab%26cdef12345"}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errCauseOmitted)
		assert.NotContains(t, err.Error(), "cdef12345")
	})

	t.Run("receiver redirects to a host built from a credential", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "http://"+r.URL.Query().Get("key")+".invalid/")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer ts.Close()

		// Fail the lookup of the redirect target the way the resolver would, without using the network.
		dialer := &net.Dialer{}
		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if host, _, _ := net.SplitHostPort(addr); strings.HasSuffix(host, ".invalid") {
					return nil, &net.OpError{Op: "dial", Net: network, Err: &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}}
				}
				return dialer.DialContext(ctx, network, addr)
			},
		}}

		cfg := &WebhookConfig{URL: ts.URL + "/hook?" + secretQuery, URLIsSecret: true}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errNetworkAfterContact)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})

	t.Run("receiver that does not answer in time", func(t *testing.T) {
		release := make(chan struct{})
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
		}))
		defer ts.Close()
		defer close(release) // lets the handler return, so that the server can close

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = &http.Client{Timeout: 100 * time.Millisecond}

		cfg := &WebhookConfig{URL: ts.URL + secretPath + "?" + secretQuery}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errReceiverTimeout)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})
}

func TestSendWebhookWithConfig_TLSFailuresLeaveOutTheCertificate(t *testing.T) {
	const secretQuery = "key=s3cr3t-token"
	noContent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	t.Run("receiver presents a certificate named after a credential", func(t *testing.T) {
		cert := certificateNamedAfterACredential(t)
		ts := httptest.NewUnstartedServer(noContent)
		ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		ts.Config.ErrorLog = log.New(io.Discard, "", 0)
		ts.StartTLS()
		defer ts.Close()

		// Trust the certificate and send hooks.example.com to the test server, so that only the
		// certificate's names fail verification.
		roots := x509.NewCertPool()
		roots.AddCert(cert.Leaf)
		dialer := &net.Dialer{}
		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, ts.Listener.Addr().String())
			},
		}}

		cfg := &WebhookConfig{URL: "https://hooks.example.com/hook?" + secretQuery, URLIsSecret: true}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errCertificateWrongHost)
		assert.Contains(t, err.Error(), "https://hooks.example.com/"+redactedPlaceholder)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})

	t.Run("receiver presents a certificate from an unknown authority", func(t *testing.T) {
		ts := httptest.NewUnstartedServer(noContent)
		ts.Config.ErrorLog = log.New(io.Discard, "", 0)
		ts.StartTLS()
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12},
		}}

		err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: ts.URL + "/hook?" + secretQuery}, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errCertificateUnknownAuthority)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})

	t.Run("receiver does not speak TLS", func(t *testing.T) {
		ts := httptest.NewServer(noContent)
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = &http.Client{}

		cfg := &WebhookConfig{URL: strings.Replace(ts.URL, "http://", "https://", 1) + "/hook?" + secretQuery}
		err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errTLSHandshake)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})
}

// certificateNamedAfterACredential returns a self-signed certificate whose only name is built from
// the credential in the tests' webhook URL.
func certificateNamedAfterACredential(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		DNSNames:              []string{"s3cr3t-token.invalid"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func TestSafeCause(t *testing.T) {
	const webhookURL = "https://hooks.example.com/services/s3cr3t-token"
	dialErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}
	readErr := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}
	quoting := errors.New(`failed to parse Location header "/services/s3cr3t-token/%zz"`)

	certificateNaming := &x509.Certificate{DNSNames: []string{"s3cr3t-token.invalid"}}
	contacted := requestProgress{contacted: true}
	handshakeFailed := requestProgress{contacted: true, handshakeFailed: true}

	tests := []struct {
		name     string
		err      error
		progress requestProgress
		want     string
	}{
		{
			name: "before contact: the cause without the URL",
			err:  &url.Error{Op: "Post", URL: webhookURL, Err: dialErr},
			want: "dial tcp: connect: connection refused",
		},
		{
			name:     "after contact: a network error of this side",
			err:      &url.Error{Op: "Post", URL: webhookURL, Err: fmt.Errorf("transport connection broken: %w", readErr)},
			progress: contacted,
			want:     errNetworkAfterContact.Error(),
		},
		{
			name: "after contact: a failed lookup of a redirect target the receiver chose",
			err: &url.Error{Op: "Post", URL: webhookURL, Err: &net.OpError{Op: "dial", Net: "tcp",
				Err: &net.DNSError{Err: "no such host", Name: "s3cr3t-token.invalid", IsNotFound: true}}},
			progress: contacted,
			want:     errNetworkAfterContact.Error(),
		},
		{
			name:     "after contact: the receiver closed the connection",
			err:      &url.Error{Op: "Post", URL: webhookURL, Err: io.EOF},
			progress: contacted,
			want:     errNetworkAfterContact.Error(),
		},
		{
			name:     "after contact: a deadline",
			err:      &url.Error{Op: "Post", URL: webhookURL, Err: context.DeadlineExceeded},
			progress: contacted,
			want:     errReceiverTimeout.Error(),
		},
		{
			name:     "after contact: a canceled request",
			err:      &url.Error{Op: "Post", URL: webhookURL, Err: context.Canceled},
			progress: contacted,
			want:     errRequestCanceled.Error(),
		},
		{
			name: "after contact: a certificate whose names do not cover the host",
			err: &url.Error{Op: "Post", URL: webhookURL, Err: &tls.CertificateVerificationError{
				Err: x509.HostnameError{Certificate: certificateNaming, Host: "hooks.example.com"}}},
			progress: handshakeFailed,
			want:     errCertificateWrongHost.Error(),
		},
		{
			name: "after contact: a certificate from an unknown authority",
			err: &url.Error{Op: "Post", URL: webhookURL, Err: &tls.CertificateVerificationError{
				Err: x509.UnknownAuthorityError{Cert: certificateNaming}}},
			progress: handshakeFailed,
			want:     errCertificateUnknownAuthority.Error(),
		},
		{
			name: "after contact: an expired certificate",
			err: &url.Error{Op: "Post", URL: webhookURL, Err: &tls.CertificateVerificationError{
				Err: x509.CertificateInvalidError{Cert: certificateNaming, Reason: x509.Expired}}},
			progress: handshakeFailed,
			want:     errCertificateExpired.Error(),
		},
		{
			name: "after contact: a certificate that fails verification otherwise",
			err: &url.Error{Op: "Post", URL: webhookURL, Err: &tls.CertificateVerificationError{
				Err: x509.CertificateInvalidError{Cert: certificateNaming, Reason: x509.NameMismatch}}},
			progress: handshakeFailed,
			want:     errCertificateInvalid.Error(),
		},
		{
			name:     "after contact: a TLS handshake that failed otherwise",
			err:      &url.Error{Op: "Post", URL: webhookURL, Err: tls.RecordHeaderError{Msg: "tls: first record does not look like a TLS handshake"}},
			progress: handshakeFailed,
			want:     errTLSHandshake.Error(),
		},
		{
			name:     "after contact: anything else is left out",
			err:      &url.Error{Op: "Post", URL: webhookURL, Err: quoting},
			progress: contacted,
			want:     errCauseOmitted.Error(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeCause(tc.err, tc.progress)
			assert.Equal(t, tc.want, got.Error())
			assert.NotContains(t, got.Error(), "s3cr3t")
		})
	}
}

func TestRedactWebhookURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "path and query are replaced", url: "https://hooks.slack.com/services/T000/B000/XXXX?token=abc", want: "https://hooks.slack.com/<redacted>"},
		{name: "port is kept", url: "http://127.0.0.1:8080/hook", want: "http://127.0.0.1:8080/<redacted>"},
		{name: "userinfo is dropped", url: "https://user:pass@example.com/hook", want: "https://example.com/<redacted>"},
		{name: "URL without a path", url: "https://example.com", want: "https://example.com/<redacted>"},
		{name: "value that is not a URL", url: "hunter2", want: "<redacted>"},
		{name: "URL with another scheme", url: "smtp://mail.example.com/secret", want: "<redacted>"},
		{name: "URL that does not parse", url: "https://example.com/hook\n", want: "<redacted>"},
		{name: "empty", url: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RedactWebhookURL(tc.url))
		})
	}
}

func TestScrubWebhookURL(t *testing.T) {
	const webhookURL = "https://hooks.example.com/services/T000/B000/tok3n-abc?key=s3cret-value"

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "full URL", text: "cannot POST " + webhookURL, want: "cannot POST <redacted>"},
		{name: "request URI", text: "no route for /services/T000/B000/tok3n-abc?key=s3cret-value", want: "no route for <redacted>"},
		{name: "path", text: "unknown hook /services/T000/B000/tok3n-abc", want: "unknown hook <redacted>"},
		{name: "query", text: "bad parameters: key=s3cret-value", want: "bad parameters: <redacted>"},
		{name: "last path segment", text: "invalid token tok3n-abc", want: "invalid token <redacted>"},
		{name: "query value", text: "invalid key s3cret-value", want: "invalid key <redacted>"},
		{name: "path without its first segment", text: "no route /T000/B000/tok3n-abc", want: "no route /<redacted>/<redacted>/<redacted>"},
		{name: "JSON-escaped path", text: `{"path":"\/T000\/B000\/tok3n-abc"}`, want: `{"path":"\/<redacted>\/<redacted>\/<redacted>"}`},
		{name: "URL-encoded path", text: "next=%2FT000%2FB000%2Ftok3n-abc", want: "next=%2F<redacted>%2F<redacted>%2F<redacted>"},
		{name: "HTML-escaped query", text: "team=a&amp;key=s3cret-value", want: "team=a&amp;<redacted>"},
		{name: "unrelated text", text: "invalid_payload", want: "invalid_payload"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, scrubWebhookURL(tc.text, webhookURL))
		})
	}

	t.Run("escaped piece is removed in its unescaped form too", func(t *testing.T) {
		got := scrubWebhookURL("bad token s3cr+t/token", "https://hooks.example.com/hooks/s3cr%2Bt%2Ftoken")
		assert.Equal(t, "bad token <redacted>", got)
	})

	t.Run("userinfo is removed", func(t *testing.T) {
		got := scrubWebhookURL("bad credentials for alerts: p4ssw0rd", "https://alerts:p4ssw0rd@hooks.example.com/hook")
		assert.Equal(t, "bad credentials for <redacted>: <redacted>", got)
	})

	t.Run("username without a password is removed", func(t *testing.T) {
		got := scrubWebhookURL("invalid credential token-s3cr3t-123", "https://token-s3cr3t-123@hooks.example.com/hook")
		assert.Equal(t, "invalid credential <redacted>", got)
	})

	t.Run("short pieces are left alone", func(t *testing.T) {
		got := scrubWebhookURL("invalid id in v1 request", "https://hooks.example.com/v1/hook-abcdef?id=7")
		assert.Equal(t, "invalid id in v1 request", got)
	})

	t.Run("URL without a path leaves slashes alone", func(t *testing.T) {
		got := scrubWebhookURL("see /docs/errors", "https://hooks.example.com/")
		assert.Equal(t, "see /docs/errors", got)
		assert.False(t, strings.Contains(got, redactedPlaceholder))
	})
}

func TestQuotesWebhookURL(t *testing.T) {
	const webhookURL = "https://hooks.example.com/services/tok3n-abc?key=ab%26cdef12345"

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "JSON unicode escape", text: `invalid credential ab\u0026cdef12345`, want: true},
		{name: "HTML named entity", text: "invalid credential ab&amp;cdef12345", want: true},
		{name: "HTML numeric entity", text: "invalid credential ab&#x26;cdef12345", want: true},
		{name: "percent-encoded twice", text: "invalid credential ab%2526cdef12345", want: true},
		{name: "piece split by punctuation", text: "unknown hook tok3n_abc", want: true},
		{name: "JSON simple escape", text: `unknown hook tok3n\tabc`, want: true},
		{name: "unrelated text", text: "invalid_payload: no text field", want: false},
		{name: "short pieces are ignored", text: "missing key", want: false},
		{name: "scrubbed text", text: "unknown hook <redacted>", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, quotesWebhookURL(tc.text, webhookURL))
		})
	}

	t.Run("URL that does not parse counts as quoted", func(t *testing.T) {
		assert.True(t, quotesWebhookURL("anything", "https://example.com/hook\n"))
	})
}

func TestSendWebhookWithConfig_RequestPayload(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	origClient := httpClient
	t.Cleanup(func() { httpClient = origClient })
	httpClient = ts.Client()

	input := map[string]interface{}{
		"alertName": "HighCPU",
		"severity":  "critical",
		"threshold": 90.0,
	}
	cfg := &WebhookConfig{URL: ts.URL}
	err := SendWebhookWithConfig(context.Background(), cfg, input)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &got))
	assert.Equal(t, "HighCPU", got["alertName"])
	assert.Equal(t, "critical", got["severity"])
	assert.Equal(t, 90.0, got["threshold"])
}
