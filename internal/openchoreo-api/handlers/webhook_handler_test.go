// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
)

// TestDetectGitProvider verifies that the correct provider is identified from
// the incoming request headers and that the returned signature-header name,
// secret key, and boolean success flag match expectations.
func TestDetectGitProvider(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		wantProvider  git.ProviderType
		wantSigHeader string
		wantSecretKey string
		wantOK        bool
	}{
		{
			name:          "GitHub detected via X-Hub-Signature-256",
			headers:       map[string]string{headerGitHubSignature: "sha256=abc123"},
			wantProvider:  git.ProviderGitHub,
			wantSigHeader: headerGitHubSignature,
			wantSecretKey: secretKeyGitHub,
			wantOK:        true,
		},
		{
			name:          "GitLab detected via X-Gitlab-Token",
			headers:       map[string]string{headerGitLabToken: "some-token"},
			wantProvider:  git.ProviderGitLab,
			wantSigHeader: headerGitLabToken,
			wantSecretKey: secretKeyGitLab,
			wantOK:        true,
		},
		{
			name:          "Bitbucket detected via X-Event-Key",
			headers:       map[string]string{headerBitbucketEventKey: "repo:push"},
			wantProvider:  git.ProviderBitbucket,
			wantSigHeader: "",
			wantSecretKey: secretKeyBitbucket,
			wantOK:        true,
		},
		{
			// When both GitHub and GitLab headers are present the switch evaluates
			// top-to-bottom so GitHub must win.
			name: "ambiguous headers — GitHub header takes precedence over GitLab",
			headers: map[string]string{
				headerGitHubSignature: "sha256=abc123",
				headerGitLabToken:     "some-token",
			},
			wantProvider:  git.ProviderGitHub,
			wantSigHeader: headerGitHubSignature,
			wantSecretKey: secretKeyGitHub,
			wantOK:        true,
		},
		{
			name:          "missing headers — detection fails",
			headers:       map[string]string{},
			wantProvider:  "",
			wantSigHeader: "",
			wantSecretKey: "",
			wantOK:        false,
		},
		{
			// Header present but with an empty value must not trigger detection.
			name:          "empty header value — detection fails",
			headers:       map[string]string{headerGitHubSignature: ""},
			wantProvider:  "",
			wantSigHeader: "",
			wantSecretKey: "",
			wantOK:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/autobuild", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			gotProvider, gotSigHeader, gotSecretKey, gotOK := detectGitProvider(req)

			if gotProvider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", gotProvider, tt.wantProvider)
			}
			if gotSigHeader != tt.wantSigHeader {
				t.Errorf("signatureHeader = %q, want %q", gotSigHeader, tt.wantSigHeader)
			}
			if gotSecretKey != tt.wantSecretKey {
				t.Errorf("secretKey = %q, want %q", gotSecretKey, tt.wantSecretKey)
			}
			if gotOK != tt.wantOK {
				t.Errorf("ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

// TestHandleAutoBuild_UnknownProvider exercises the early-exit path in
// HandleAutoBuild when no recognized provider header is present.  It must
// respond with HTTP 400 and a body whose "code" field equals
// "UNKNOWN_GIT_PROVIDER" — no Kubernetes or service calls are made.
func TestHandleAutoBuild_UnknownProvider(t *testing.T) {
	h := &Handler{logger: slog.Default()}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/autobuild", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.HandleAutoBuild(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Code != "UNKNOWN_GIT_PROVIDER" {
		t.Errorf("code = %q, want %q", body.Code, "UNKNOWN_GIT_PROVIDER")
	}
}
