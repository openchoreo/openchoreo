// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
	autobuildsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/autobuild"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// mockAutoBuildService is a test stub for autobuildsvc.Service.
type mockAutoBuildService struct {
	result         *autobuildsvc.WebhookResult
	err            error
	capturedParams *autobuildsvc.ProcessWebhookParams
}

func (m *mockAutoBuildService) ProcessWebhook(_ context.Context, params *autobuildsvc.ProcessWebhookParams) (*autobuildsvc.WebhookResult, error) {
	m.capturedParams = params
	return m.result, m.err
}

func newTestHandler(svc autobuildsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{
			AutoBuildService: svc,
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestDetectGitProviderFromParams(t *testing.T) {
	sig256 := "sha256=abc"
	token := "some-token"
	eventKey := "repo:push"
	empty := ""

	tests := []struct {
		name          string
		params        gen.HandleAutoBuildParams
		wantProvider  git.ProviderType
		wantSigHeader string
		wantSecretKey string
		wantOK        bool
	}{
		{
			name:          "GitHub with X-Hub-Signature-256",
			params:        gen.HandleAutoBuildParams{XHubSignature256: &sig256},
			wantProvider:  git.ProviderGitHub,
			wantSigHeader: "X-Hub-Signature-256",
			wantSecretKey: "github-secret",
			wantOK:        true,
		},
		{
			name:          "GitLab with X-Gitlab-Token",
			params:        gen.HandleAutoBuildParams{XGitlabToken: &token},
			wantProvider:  git.ProviderGitLab,
			wantSigHeader: "X-Gitlab-Token",
			wantSecretKey: "gitlab-secret",
			wantOK:        true,
		},
		{
			name:          "Bitbucket with X-Event-Key",
			params:        gen.HandleAutoBuildParams{XEventKey: &eventKey},
			wantProvider:  git.ProviderBitbucket,
			wantSigHeader: "",
			wantSecretKey: "bitbucket-secret",
			wantOK:        true,
		},
		{
			name:   "no recognized header returns false",
			params: gen.HandleAutoBuildParams{},
			wantOK: false,
		},
		{
			name:   "empty X-Hub-Signature-256 not recognized",
			params: gen.HandleAutoBuildParams{XHubSignature256: &empty},
			wantOK: false,
		},
		{
			name:   "empty X-Gitlab-Token not recognized",
			params: gen.HandleAutoBuildParams{XGitlabToken: &empty},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, sigHeader, secretKey, ok := detectGitProviderFromParams(tt.params)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
			}
			if sigHeader != tt.wantSigHeader {
				t.Errorf("sigHeader = %q, want %q", sigHeader, tt.wantSigHeader)
			}
			if secretKey != tt.wantSecretKey {
				t.Errorf("secretKey = %q, want %q", secretKey, tt.wantSecretKey)
			}
		})
	}
}

func TestSignatureFromParams(t *testing.T) {
	sig := "sha256=abc123"
	tok := "my-token"

	tests := []struct {
		name            string
		params          gen.HandleAutoBuildParams
		signatureHeader string
		wantSig         string
	}{
		{
			name:            "GitHub signature header",
			params:          gen.HandleAutoBuildParams{XHubSignature256: &sig},
			signatureHeader: "X-Hub-Signature-256",
			wantSig:         sig,
		},
		{
			name:            "GitLab token header",
			params:          gen.HandleAutoBuildParams{XGitlabToken: &tok},
			signatureHeader: "X-Gitlab-Token",
			wantSig:         tok,
		},
		{
			name:            "Bitbucket has no signature header",
			params:          gen.HandleAutoBuildParams{},
			signatureHeader: "",
			wantSig:         "",
		},
		{
			name:            "unknown header returns empty",
			params:          gen.HandleAutoBuildParams{},
			signatureHeader: "X-Unknown",
			wantSig:         "",
		},
		{
			name:            "nil XHubSignature256 returns empty",
			params:          gen.HandleAutoBuildParams{},
			signatureHeader: "X-Hub-Signature-256",
			wantSig:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signatureFromParams(tt.params, tt.signatureHeader)
			if got != tt.wantSig {
				t.Errorf("signatureFromParams = %q, want %q", got, tt.wantSig)
			}
		})
	}
}

func TestHandleAutoBuild(t *testing.T) {
	rawPayload := []byte(`{"ref":"refs/heads/main","after":"abc123","repository":{"clone_url":"https://github.com/test/repo.git"},"commits":[]}`)
	sig := "sha256=validhmac"
	ctxWithBody := context.WithValue(context.Background(), webhookRawBodyKey{}, rawPayload)

	tests := []struct {
		name           string
		ctx            context.Context
		params         gen.HandleAutoBuildParams
		svcResult      *autobuildsvc.WebhookResult
		svcErr         error
		wantStatusCode int
		wantErrMsg     string
		wantSuccess    bool
		wantTriggered  int
		// wantParams asserts ProcessWebhook was called with these values (optional)
		wantProvider  git.ProviderType
		wantSigHeader string
		wantSignature string
	}{
		{
			name:           "no provider header returns 400",
			ctx:            ctxWithBody,
			params:         gen.HandleAutoBuildParams{},
			wantStatusCode: 400,
		},
		{
			name:           "missing raw body in context returns 500 with generic message",
			ctx:            context.Background(),
			params:         gen.HandleAutoBuildParams{XHubSignature256: &sig},
			wantStatusCode: 500,
			wantErrMsg:     "Internal server error",
		},
		{
			name:           "ErrInvalidSignature returns 401",
			ctx:            ctxWithBody,
			params:         gen.HandleAutoBuildParams{XHubSignature256: &sig},
			svcErr:         autobuildsvc.ErrInvalidSignature,
			wantStatusCode: 401,
		},
		{
			name:           "ErrSecretNotConfigured returns 500 with specific message",
			ctx:            ctxWithBody,
			params:         gen.HandleAutoBuildParams{XHubSignature256: &sig},
			svcErr:         autobuildsvc.ErrSecretNotConfigured,
			wantStatusCode: 500,
			wantErrMsg:     "Webhook secret not configured",
		},
		{
			name:           "generic error returns 500 with generic message not leaking details",
			ctx:            ctxWithBody,
			params:         gen.HandleAutoBuildParams{XHubSignature256: &sig},
			svcErr:         errors.New("database connection failed: sensitive details"),
			wantStatusCode: 500,
			wantErrMsg:     "Internal server error",
		},
		{
			name:           "success returns 200 with affected components and correct params forwarded",
			ctx:            ctxWithBody,
			params:         gen.HandleAutoBuildParams{XHubSignature256: &sig},
			svcResult:      &autobuildsvc.WebhookResult{AffectedComponents: []string{"comp-a", "comp-b"}},
			wantStatusCode: 200,
			wantSuccess:    true,
			wantTriggered:  2,
			wantProvider:   git.ProviderGitHub,
			wantSigHeader:  "X-Hub-Signature-256",
			wantSignature:  sig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAutoBuildService{result: tt.svcResult, err: tt.svcErr}
			h := newTestHandler(mock)

			resp, err := h.HandleAutoBuild(tt.ctx, gen.HandleAutoBuildRequestObject{
				Params: tt.params,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			switch r := resp.(type) {
			case gen.HandleAutoBuild400JSONResponse:
				if tt.wantStatusCode != 400 {
					t.Fatalf("got 400, want %d", tt.wantStatusCode)
				}
			case gen.HandleAutoBuild401JSONResponse:
				if tt.wantStatusCode != 401 {
					t.Fatalf("got 401, want %d", tt.wantStatusCode)
				}
			case gen.HandleAutoBuild500JSONResponse:
				if tt.wantStatusCode != 500 {
					t.Fatalf("got 500, want %d", tt.wantStatusCode)
				}
				if tt.wantErrMsg != "" && r.Error != tt.wantErrMsg {
					t.Errorf("error message = %q, want %q", r.Error, tt.wantErrMsg)
				}
				// Verify raw error details are never exposed in the response.
				if tt.svcErr != nil && r.Error == tt.svcErr.Error() {
					t.Errorf("response leaks internal error string: %q", r.Error)
				}
			case gen.HandleAutoBuild200JSONResponse:
				if tt.wantStatusCode != 200 {
					t.Fatalf("got 200, want %d", tt.wantStatusCode)
				}
				if r.Success != tt.wantSuccess {
					t.Errorf("success = %v, want %v", r.Success, tt.wantSuccess)
				}
				if r.TriggeredBuilds != tt.wantTriggered {
					t.Errorf("triggered = %d, want %d", r.TriggeredBuilds, tt.wantTriggered)
				}
				if tt.wantProvider != "" {
					if mock.capturedParams == nil {
						t.Fatal("ProcessWebhook was not called")
					}
					if mock.capturedParams.ProviderType != tt.wantProvider {
						t.Errorf("ProviderType = %q, want %q", mock.capturedParams.ProviderType, tt.wantProvider)
					}
					if mock.capturedParams.SignatureHeader != tt.wantSigHeader {
						t.Errorf("SignatureHeader = %q, want %q", mock.capturedParams.SignatureHeader, tt.wantSigHeader)
					}
					if mock.capturedParams.Signature != tt.wantSignature {
						t.Errorf("Signature = %q, want %q", mock.capturedParams.Signature, tt.wantSignature)
					}
					if !bytes.Equal(mock.capturedParams.Payload, rawPayload) {
						t.Errorf("Payload = %q, want %q", mock.capturedParams.Payload, rawPayload)
					}
				}
			default:
				t.Fatalf("unexpected response type: %T", resp)
			}
		})
	}
}
