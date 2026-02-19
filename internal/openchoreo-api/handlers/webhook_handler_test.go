// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
)

func TestDetectGitProvider(t *testing.T) {
	tests := []struct {
		name                string
		headers             map[string]string
		wantProviderType    git.ProviderType
		wantSignatureHeader string
		wantSecretKey       string
		wantOK              bool
	}{
		{
			name:                "GitHub detected via X-Hub-Signature-256",
			headers:             map[string]string{"X-Hub-Signature-256": "sha256=abc123"},
			wantProviderType:    git.ProviderGitHub,
			wantSignatureHeader: "X-Hub-Signature-256",
			wantSecretKey:       "github-secret",
			wantOK:              true,
		},
		{
			name:                "GitLab detected via X-Gitlab-Token",
			headers:             map[string]string{"X-Gitlab-Token": "my-token"},
			wantProviderType:    git.ProviderGitLab,
			wantSignatureHeader: "X-Gitlab-Token",
			wantSecretKey:       "gitlab-secret",
			wantOK:              true,
		},
		{
			name:                "Bitbucket detected via X-Event-Key",
			headers:             map[string]string{"X-Event-Key": "repo:push"},
			wantProviderType:    git.ProviderBitbucket,
			wantSignatureHeader: "",
			wantSecretKey:       "bitbucket-secret",
			wantOK:              true,
		},
		{
			name:    "Unknown provider with no recognized headers",
			headers: map[string]string{"X-Custom-Header": "value"},
			wantOK:  false,
		},
		{
			name:    "No headers at all",
			headers: map[string]string{},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader("{}"))
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			providerType, signatureHeader, secretKey, ok := detectGitProvider(req)

			if ok != tt.wantOK {
				t.Fatalf("detectGitProvider() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if providerType != tt.wantProviderType {
				t.Errorf("providerType = %q, want %q", providerType, tt.wantProviderType)
			}
			if signatureHeader != tt.wantSignatureHeader {
				t.Errorf("signatureHeader = %q, want %q", signatureHeader, tt.wantSignatureHeader)
			}
			if secretKey != tt.wantSecretKey {
				t.Errorf("secretKey = %q, want %q", secretKey, tt.wantSecretKey)
			}
		})
	}
}

func TestHandleWebhook_UnknownProvider(t *testing.T) {
	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader("{}"))
	// No provider-identifying headers set
	rw := httptest.NewRecorder()

	h.HandleWebhook(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rw.Code, http.StatusBadRequest)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "UNKNOWN_GIT_PROVIDER") {
		t.Errorf("response body missing UNKNOWN_GIT_PROVIDER error code, got: %s", body)
	}
}
