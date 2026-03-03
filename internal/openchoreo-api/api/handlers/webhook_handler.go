// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
	autobuildsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/autobuild"
)

// webhookRawBodyKey is the context key for raw webhook request body bytes.
// HMAC signature validation requires the original bytes exactly as sent by the git provider.
// The OpenAPI strict handler decodes the JSON body before our handler is called, so we must
// capture the raw bytes in a middleware before decoding occurs.
type webhookRawBodyKey struct{}

// maxWebhookPayloadSize is the maximum accepted webhook request body size (1 MB).
const maxWebhookPayloadSize = 1 << 20

// WebhookRawBodyMiddleware captures the raw request body for the autobuild webhook endpoint
// and stores it in the context before the OpenAPI strict handler decodes the body.
// This is necessary because HMAC signature validation requires the original raw payload bytes.
func WebhookRawBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1alpha1/autobuild" && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxWebhookPayloadSize)
			rawBody, err := io.ReadAll(r.Body)
			if err != nil {
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "failed to read request body", http.StatusBadRequest)
				}
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), webhookRawBodyKey{}, rawBody))
			r.Body = io.NopCloser(bytes.NewReader(rawBody))
		}
		next.ServeHTTP(w, r)
	})
}

// detectGitProviderFromParams identifies the git provider from parsed OpenAPI header parameters.
func detectGitProviderFromParams(params gen.HandleAutoBuildParams) (git.ProviderType, string, string, bool) {
	switch {
	case params.XHubSignature256 != nil && *params.XHubSignature256 != "":
		return git.ProviderGitHub, "X-Hub-Signature-256", "github-secret", true
	case params.XGitlabToken != nil && *params.XGitlabToken != "":
		return git.ProviderGitLab, "X-Gitlab-Token", "gitlab-secret", true
	case params.XEventKey != nil && *params.XEventKey != "":
		return git.ProviderBitbucket, "", "bitbucket-secret", true
	default:
		return "", "", "", false
	}
}

// signatureFromParams extracts the signature value from the parsed header parameters.
func signatureFromParams(params gen.HandleAutoBuildParams, signatureHeader string) string {
	switch signatureHeader {
	case "X-Hub-Signature-256":
		if params.XHubSignature256 != nil {
			return *params.XHubSignature256
		}
	case "X-Gitlab-Token":
		if params.XGitlabToken != nil {
			return *params.XGitlabToken
		}
	}
	return ""
}

// HandleAutoBuild processes incoming webhook events from any supported git provider.
// The provider is detected from the request headers (X-Hub-Signature-256, X-Gitlab-Token, X-Event-Key).
func (h *Handler) HandleAutoBuild(
	ctx context.Context,
	request gen.HandleAutoBuildRequestObject,
) (gen.HandleAutoBuildResponseObject, error) {
	providerType, signatureHeader, secretKey, ok := detectGitProviderFromParams(request.Params)
	if !ok {
		h.logger.Error("Unable to detect git provider from webhook headers")
		return gen.HandleAutoBuild400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse{
				Code:  gen.UNKNOWNGITPROVIDER,
				Error: "Unable to detect git provider from request headers",
			},
		}, nil
	}

	rawBody, ok := ctx.Value(webhookRawBodyKey{}).([]byte)
	if !ok {
		h.logger.Error("Raw webhook body not found in context; WebhookRawBodyMiddleware may not be configured")
		return gen.HandleAutoBuild500JSONResponse{
			InternalErrorJSONResponse: gen.InternalErrorJSONResponse{
				Code:  gen.INTERNALERROR,
				Error: "Internal server error",
			},
		}, nil
	}

	result, err := h.services.AutoBuildService.ProcessWebhook(ctx, &autobuildsvc.ProcessWebhookParams{
		ProviderType:    providerType,
		SignatureHeader: signatureHeader,
		Signature:       signatureFromParams(request.Params, signatureHeader),
		SecretKey:       secretKey,
		Payload:         rawBody,
	})
	if err != nil {
		switch {
		case errors.Is(err, autobuildsvc.ErrInvalidSignature):
			h.logger.Error("Invalid webhook signature", "provider", providerType)
			return gen.HandleAutoBuild401JSONResponse{
				UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse{
					Code:  gen.UNAUTHORIZED,
					Error: "Invalid webhook signature",
				},
			}, nil
		case errors.Is(err, autobuildsvc.ErrSecretNotConfigured):
			h.logger.Error("Webhook secret not configured", "provider", providerType, "error", err)
			return gen.HandleAutoBuild500JSONResponse{
				InternalErrorJSONResponse: gen.InternalErrorJSONResponse{
					Code:  gen.INTERNALERROR,
					Error: "Webhook secret not configured",
				},
			}, nil
		default:
			h.logger.Error("Failed to process webhook", "provider", providerType, "error", err)
			return gen.HandleAutoBuild500JSONResponse{
				InternalErrorJSONResponse: gen.InternalErrorJSONResponse{
					Code:  gen.INTERNALERROR,
					Error: "Internal server error",
				},
			}, nil
		}
	}

	return gen.HandleAutoBuild200JSONResponse{
		Success:            true,
		Message:            "Webhook processed successfully",
		AffectedComponents: &result.AffectedComponents,
		TriggeredBuilds:    len(result.AffectedComponents),
	}, nil
}
