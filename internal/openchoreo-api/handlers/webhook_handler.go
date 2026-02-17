// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services/git"
)

const (
	// WebhookSecretName is the name of the Kubernetes Secret containing webhook secrets
	WebhookSecretName = "git-webhook-secrets" // #nosec G101 -- This is a secret name, not a hardcoded credential
	// WebhookSecretNamespace is the namespace where the webhook secret is stored
	WebhookSecretNamespace = "openchoreo-control-plane" // #nosec G101 -- This is a namespace name, not a credential
)

// HandleGitHubWebhook processes incoming GitHub webhook events
func (h *Handler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, git.ProviderGitHub, "X-Hub-Signature-256", "github-secret")
}

// HandleGitLabWebhook processes incoming GitLab webhook events
func (h *Handler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, git.ProviderGitLab, "X-Gitlab-Token", "gitlab-secret")
}

// HandleBitbucketWebhook processes incoming Bitbucket webhook events
func (h *Handler) HandleBitbucketWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, git.ProviderBitbucket, "X-Hook-UUID", "bitbucket-secret")
}

// handleWebhook is the common handler for all git provider webhooks
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request, providerType git.ProviderType, signatureHeader, secretKey string) {
	// Read the payload
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read webhook payload", "error", err, "provider", providerType)
		respondJSON(w, http.StatusBadRequest, models.ErrorResponse("Failed to read payload", "WEBHOOK_READ_ERROR"))
		return
	}

	// Get signature/token from header
	signature := r.Header.Get(signatureHeader)

	// Get the git provider
	provider, err := git.GetProvider(providerType)
	if err != nil {
		h.logger.Error("Failed to get git provider", "error", err, "provider", providerType)
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse("Provider not supported", "PROVIDER_ERROR"))
		return
	}

	// Get webhook secret from Kubernetes Secret
	webhookSecret, err := h.getWebhookSecret(r.Context(), secretKey)
	if err != nil {
		h.logger.Error("Failed to get webhook secret", "error", err, "provider", providerType)
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse("Webhook secret not configured", "SECRET_NOT_CONFIGURED"))
		return
	}

	// Validate signature
	if err := provider.ValidateWebhookPayload(payload, signature, webhookSecret); err != nil {
		h.logger.Error("Invalid webhook signature", "error", err, "provider", providerType)
		respondJSON(w, http.StatusUnauthorized, models.ErrorResponse("Invalid webhook signature", "INVALID_SIGNATURE"))
		return
	}

	// Process webhook through service
	affectedComponents, err := h.services.WebhookService.ProcessWebhook(
		r.Context(),
		provider,
		payload,
	)
	if err != nil {
		h.logger.Error("Failed to process webhook", "error", err, "provider", providerType)
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse(err.Error(), "WEBHOOK_PROCESS_ERROR"))
		return
	}

	h.logger.Info("Webhook processed successfully",
		"provider", providerType,
		"affectedComponents", len(affectedComponents),
	)

	respondJSON(w, http.StatusOK, models.SuccessResponse(models.WebhookEventResponse{
		Success:            true,
		Message:            "Webhook processed successfully",
		AffectedComponents: affectedComponents,
		TriggeredBuilds:    len(affectedComponents),
	}))
}

// getWebhookSecret retrieves the webhook secret from Kubernetes Secret
func (h *Handler) getWebhookSecret(ctx context.Context, secretKey string) (string, error) {
	// Get the Secret
	secret := &corev1.Secret{}
	if err := h.services.GetKubernetesClient().Get(ctx, client.ObjectKey{
		Name:      WebhookSecretName,
		Namespace: WebhookSecretNamespace,
	}, secret); err != nil {
		return "", fmt.Errorf("failed to get webhook secret %s/%s: %w",
			WebhookSecretNamespace, WebhookSecretName, err)
	}

	// Extract the secret value
	secretData, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain '%s' key",
			WebhookSecretNamespace, WebhookSecretName, secretKey)
	}

	if len(secretData) == 0 {
		return "", fmt.Errorf("secret %s/%s has empty '%s' value",
			WebhookSecretNamespace, WebhookSecretName, secretKey)
	}

	return string(secretData), nil
}

// respondJSON is a helper function to write JSON responses
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
