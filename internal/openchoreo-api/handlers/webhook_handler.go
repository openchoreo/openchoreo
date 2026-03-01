// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	autobuildsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/autobuild"
)

const (
	// Provider detection headers — each git provider sends a unique header that
	// identifies it as the event source.
	headerGitHubSignature   = "X-Hub-Signature-256"
	headerGitLabToken       = "X-Gitlab-Token"
	headerBitbucketEventKey = "X-Event-Key"

	// Secret key names inside the webhook Kubernetes Secret for each provider.
	secretKeyGitHub    = "github-secret"
	secretKeyGitLab    = "gitlab-secret"
	secretKeyBitbucket = "bitbucket-secret"
)

// HandleAutoBuild processes incoming webhook events from any supported git provider.
// The git provider is detected from the request headers.
func (h *Handler) HandleAutoBuild(w http.ResponseWriter, r *http.Request) {
	providerType, signatureHeader, secretKey, ok := detectGitProvider(r)
	if !ok {
		h.logger.Error("Unable to detect git provider from webhook headers")
		respondJSON(w, http.StatusBadRequest, models.ErrorResponse("Unable to detect git provider from request headers", "UNKNOWN_GIT_PROVIDER"))
		return
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read webhook payload", "error", err, "provider", providerType)
		respondJSON(w, http.StatusBadRequest, models.ErrorResponse("Failed to read payload", "WEBHOOK_READ_ERROR"))
		return
	}

	result, err := h.autoBuildService.ProcessWebhook(r.Context(), &autobuildsvc.ProcessWebhookParams{
		ProviderType:    providerType,
		SignatureHeader: signatureHeader,
		Signature:       r.Header.Get(signatureHeader),
		SecretKey:       secretKey,
		Payload:         payload,
	})
	if err != nil {
		switch {
		case errors.Is(err, autobuildsvc.ErrInvalidSignature):
			h.logger.Error("Invalid webhook signature", "provider", providerType)
			respondJSON(w, http.StatusUnauthorized, models.ErrorResponse("Invalid webhook signature", "INVALID_SIGNATURE"))
		case errors.Is(err, autobuildsvc.ErrSecretNotConfigured):
			h.logger.Error("Webhook secret not configured", "provider", providerType, "error", err)
			respondJSON(w, http.StatusInternalServerError, models.ErrorResponse("Webhook secret not configured", "SECRET_NOT_CONFIGURED"))
		default:
			h.logger.Error("Failed to process webhook", "provider", providerType, "error", err)
			respondJSON(w, http.StatusInternalServerError, models.ErrorResponse(err.Error(), "WEBHOOK_PROCESS_ERROR"))
		}
		return
	}

	respondJSON(w, http.StatusOK, models.SuccessResponse(models.WebhookEventResponse{
		Success:            true,
		Message:            "Webhook processed successfully",
		AffectedComponents: result.AffectedComponents,
		TriggeredBuilds:    len(result.AffectedComponents),
	}))
}

// detectGitProvider identifies the git provider from the request headers.
// Returns the provider type, signature header name, secret key, and whether detection succeeded.
func detectGitProvider(r *http.Request) (git.ProviderType, string, string, bool) {
	switch {
	case r.Header.Get(headerGitHubSignature) != "":
		return git.ProviderGitHub, headerGitHubSignature, secretKeyGitHub, true
	case r.Header.Get(headerGitLabToken) != "":
		return git.ProviderGitLab, headerGitLabToken, secretKeyGitLab, true
	case r.Header.Get(headerBitbucketEventKey) != "":
		// Bitbucket does not send a standard secret token header (X-Hook-UUID is a
		// webhook identity UUID, not a token). Validation relies on the stored secret
		// being empty (open) for the MVP.
		return git.ProviderBitbucket, "", secretKeyBitbucket, true
	default:
		return "", "", "", false
	}
}

// respondJSON is a helper function to write JSON responses
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
