// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logsadapterclientgen

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertRuleRequestBuilders(t *testing.T) {
	// Build sample body via JSON unmarshal to avoid repeating anonymous struct types.
	var sampleBody AlertRuleRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"condition": {"enabled": true, "threshold": 100, "window": "5m"},
		"metadata":  {"name": "my-rule", "namespace": "test-ns"}
	}`), &sampleBody))

	tests := []struct {
		name           string
		serverStatus   int
		serverBody     string
		buildRequest   func(serverURL string) (*http.Request, error)
		wantMethod     string
		wantPath       string
		wantHeader     string // expected Content-Type header; empty means no check
		wantBodySubstr string // substring expected in the request body; empty means no body
	}{
		// ---- CreateAlertRule ----
		{
			name:         "CreateAlertRule/happy",
			serverStatus: http.StatusCreated,
			serverBody:   `{"metadata":{"name":"my-rule"}}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewCreateAlertRuleRequest(u, sampleBody)
			},
			wantMethod:     http.MethodPost,
			wantPath:       "/api/v1alpha1/alerts/rules",
			wantHeader:     "application/json",
			wantBodySubstr: `"name":"my-rule"`,
		},
		{
			name:         "CreateAlertRule/server_error",
			serverStatus: http.StatusInternalServerError,
			serverBody:   `{"error":"internal"}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewCreateAlertRuleRequest(u, sampleBody)
			},
			wantMethod:     http.MethodPost,
			wantPath:       "/api/v1alpha1/alerts/rules",
			wantHeader:     "application/json",
			wantBodySubstr: `"name":"my-rule"`,
		},
		// ---- GetAlertRule ----
		{
			name:         "GetAlertRule/happy",
			serverStatus: http.StatusOK,
			serverBody:   `{"metadata":{"name":"my-rule"}}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewGetAlertRuleRequest(u, "my-rule")
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1alpha1/alerts/rules/my-rule",
		},
		{
			name:         "GetAlertRule/not_found",
			serverStatus: http.StatusNotFound,
			serverBody:   `{"error":"not found"}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewGetAlertRuleRequest(u, "missing-rule")
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1alpha1/alerts/rules/missing-rule",
		},
		{
			name:         "GetAlertRule/encoded_path_param",
			serverStatus: http.StatusOK,
			serverBody:   `{}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewGetAlertRuleRequest(u, "rule/with/slash")
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1alpha1/alerts/rules/rule%2Fwith%2Fslash",
		},
		// ---- UpdateAlertRule ----
		{
			name:         "UpdateAlertRule/happy",
			serverStatus: http.StatusOK,
			serverBody:   `{"metadata":{"name":"my-rule"}}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewUpdateAlertRuleRequest(u, "my-rule", sampleBody)
			},
			wantMethod:     http.MethodPut,
			wantPath:       "/api/v1alpha1/alerts/rules/my-rule",
			wantHeader:     "application/json",
			wantBodySubstr: `"name":"my-rule"`,
		},
		{
			name:         "UpdateAlertRule/server_error",
			serverStatus: http.StatusInternalServerError,
			serverBody:   `{"error":"boom"}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewUpdateAlertRuleRequest(u, "my-rule", sampleBody)
			},
			wantMethod:     http.MethodPut,
			wantPath:       "/api/v1alpha1/alerts/rules/my-rule",
			wantHeader:     "application/json",
			wantBodySubstr: `"name":"my-rule"`,
		},
		// ---- DeleteAlertRule ----
		{
			name:         "DeleteAlertRule/happy",
			serverStatus: http.StatusNoContent,
			serverBody:   "",
			buildRequest: func(u string) (*http.Request, error) {
				return NewDeleteAlertRuleRequest(u, "my-rule")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1alpha1/alerts/rules/my-rule",
		},
		{
			name:         "DeleteAlertRule/not_found",
			serverStatus: http.StatusNotFound,
			serverBody:   `{"error":"not found"}`,
			buildRequest: func(u string) (*http.Request, error) {
				return NewDeleteAlertRuleRequest(u, "no-such-rule")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1alpha1/alerts/rules/no-such-rule",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Start a test server that asserts the incoming request.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Assert method
				assert.Equal(t, tc.wantMethod, r.Method, "unexpected HTTP method")

				// Assert path (use RawPath when available for encoded segments)
				gotPath := r.URL.Path
				if r.URL.RawPath != "" {
					gotPath = r.URL.RawPath
				}
				assert.Equal(t, tc.wantPath, gotPath, "unexpected URL path")

				// Assert Content-Type header when expected
				if tc.wantHeader != "" {
					assert.Equal(t, tc.wantHeader, r.Header.Get("Content-Type"), "unexpected Content-Type")
				}

				// Assert request body when expected
				if tc.wantBodySubstr != "" {
					bodyBytes, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(bodyBytes), tc.wantBodySubstr, "request body mismatch")

					// Verify valid JSON
					assert.True(t, json.Valid(bodyBytes), "request body is not valid JSON")
				}

				// Return the controlled response
				w.WriteHeader(tc.serverStatus)
				if tc.serverBody != "" {
					_, _ = w.Write([]byte(tc.serverBody))
				}
			}))
			defer srv.Close()

			// Build the request using the generated function.
			req, err := tc.buildRequest(srv.URL)
			require.NoError(t, err, "request builder should not fail")

			// Execute the request against the test server.
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err, "HTTP request should not fail")
			defer resp.Body.Close()

			// Assert response status code.
			assert.Equal(t, tc.serverStatus, resp.StatusCode, "unexpected response status")

			// If the server returned a body, verify we can read it.
			if tc.serverBody != "" {
				respBytes, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.JSONEq(t, tc.serverBody, string(respBytes), "response body mismatch")
			}
		})
	}
}
