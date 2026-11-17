// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
		assert.Contains(t, err.Error(), "failed to parse Location header")
		assert.NotContains(t, err.Error(), "s3cr3t")
	})
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
