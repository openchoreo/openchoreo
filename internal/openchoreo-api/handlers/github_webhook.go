// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// HandleGitHubWebhook processes incoming GitHub webhook events
func (h *Handler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	// Read the payload
	payload, err := io.ReadAll(r.Body)
	h.logger.Info(string(payload))
	if err != nil {
		respondJSON(w, http.StatusBadRequest, models.ErrorResponse("Failed to read payload", "WEBHOOK_READ_ERROR"))
		return
	}

	// Get signature from header
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		respondJSON(w, http.StatusUnauthorized, models.ErrorResponse("Missing signature header", "WEBHOOK_SIGNATURE_MISSING"))
		return
	}

	// Extract repository URL from payload to find the right webhook secret
	repoURL, err := extractRepoURLFromPayload(payload)
	if err != nil {
		h.logger.Error("Failed to extract repository URL from payload", "error", err)
		respondJSON(w, http.StatusBadRequest, models.ErrorResponse("Failed to parse repository URL from payload", "INVALID_PAYLOAD"))
		return
	}

	// Get the webhook secret from Kubernetes Secret
	webhookSecret, err := h.getWebhookSecret(r.Context(), repoURL)
	if err != nil {
		h.logger.Error("Failed to get webhook secret", "error", err, "repository", repoURL)
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse("Failed to retrieve webhook secret", "SECRET_READ_ERROR"))
		return
	}

	// Process webhook through service
	affectedComponents, err := h.services.GitHubWebhookService.ProcessWebhook(
		r.Context(),
		payload,
		signature,
		webhookSecret,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse(err.Error(), "WEBHOOK_PROCESS_ERROR"))
		return
	}

	respondJSON(w, http.StatusOK, models.SuccessResponse(models.WebhookEventResponse{
		Success:            true,
		Message:            "Webhook processed successfully",
		AffectedComponents: affectedComponents,
		TriggeredBuilds:    len(affectedComponents),
	}))
}

// RegisterWebhook handles webhook registration requests
func (h *Handler) RegisterWebhook(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	webhookID, err := h.services.ComponentService.RegisterWebhook(
		r.Context(),
		orgName,
		projectName,
		componentName,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse(err.Error(), "WEBHOOK_REGISTER_ERROR"))
		return
	}

	respondJSON(w, http.StatusOK, models.SuccessResponse(models.WebhookResponse{
		Success:   true,
		Message:   "Webhook registered successfully",
		WebhookID: webhookID,
	}))
}

// DeregisterWebhook handles webhook deregistration requests
func (h *Handler) DeregisterWebhook(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	err := h.services.ComponentService.DeregisterWebhook(
		r.Context(),
		orgName,
		projectName,
		componentName,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse(err.Error(), "WEBHOOK_DEREGISTER_ERROR"))
		return
	}

	respondJSON(w, http.StatusOK, models.SuccessResponse(models.WebhookResponse{
		Success: true,
		Message: "Webhook deregistered successfully",
	}))
}

// GetWebhookStatus retrieves webhook registration status
func (h *Handler) GetWebhookStatus(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	status, err := h.services.ComponentService.GetWebhookStatus(
		r.Context(),
		orgName,
		projectName,
		componentName,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, models.ErrorResponse(err.Error(), "WEBHOOK_STATUS_ERROR"))
		return
	}

	respondJSON(w, http.StatusOK, models.SuccessResponse(status))
}

// respondJSON is a helper function to write JSON responses
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// extractRepoURLFromPayload extracts the repository URL from GitHub webhook payload
func extractRepoURLFromPayload(payload []byte) (string, error) {
	var event struct {
		Repository struct {
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(payload, &event); err != nil {
		return "", fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	if event.Repository.CloneURL == "" {
		return "", fmt.Errorf("repository clone_url not found in webhook payload")
	}

	return event.Repository.CloneURL, nil
}

// getWebhookSecret retrieves the webhook secret from Kubernetes Secret
func (h *Handler) getWebhookSecret(ctx context.Context, repoURL string) (string, error) {
	// Normalize repository URL to match the format used by the controller
	normalizedRepoURL := normalizeRepoURL(repoURL)

	// Generate the webhook resource name (same logic as controller)
	webhookResourceName := generateWebhookResourceName(normalizedRepoURL)

	// Get the GitRepositoryWebhook CR
	gitWebhook := &openchoreov1alpha1.GitRepositoryWebhook{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Name: webhookResourceName}, gitWebhook); err != nil {
		return "", fmt.Errorf("failed to get GitRepositoryWebhook for repository %s: %w", repoURL, err)
	}

	// Check if webhook has a secret reference
	if gitWebhook.Spec.WebhookSecretRef == nil {
		return "", fmt.Errorf("GitRepositoryWebhook %s has no secret reference", webhookResourceName)
	}

	// Get the Secret
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      gitWebhook.Spec.WebhookSecretRef.Name,
		Namespace: gitWebhook.Spec.WebhookSecretRef.Namespace,
	}

	if err := h.k8sClient.Get(ctx, secretKey, secret); err != nil {
		return "", fmt.Errorf("failed to get webhook secret %s/%s: %w",
			secretKey.Namespace, secretKey.Name, err)
	}

	// Extract the secret value
	secretData, ok := secret.Data["secret"]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain 'secret' key",
			secretKey.Namespace, secretKey.Name)
	}

	if len(secretData) == 0 {
		return "", fmt.Errorf("secret %s/%s has empty 'secret' value",
			secretKey.Namespace, secretKey.Name)
	}

	return string(secretData), nil
}

// normalizeRepoURL normalizes repository URLs for comparison
// Converts SSH URLs to HTTPS, removes .git suffix, and converts to lowercase
func normalizeRepoURL(repoURL string) string {
	// Convert SSH to HTTPS
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.Replace(repoURL, "git@", "https://", 1)
	}

	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Convert to lowercase for case-insensitive comparison
	repoURL = strings.ToLower(repoURL)

	return repoURL
}

// generateWebhookResourceName generates a Kubernetes resource name from a repository URL
// Uses the same logic as the controller to ensure consistency
func generateWebhookResourceName(normalizedRepoURL string) string {
	// Remove protocol prefix
	url := strings.TrimPrefix(normalizedRepoURL, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Replace invalid characters with hyphens
	url = strings.ReplaceAll(url, "/", "-")
	url = strings.ReplaceAll(url, ".", "-")
	url = strings.ReplaceAll(url, "_", "-")

	// Kubernetes resource names must be lowercase and no more than 253 characters
	// Prefix with "webhook-" for clarity
	resourceName := "webhook-" + url

	// If name is too long, truncate and add a hash
	if len(resourceName) > 253 {
		// Take first 240 characters and add a hash of the full URL
		hash := sha256.Sum256([]byte(strings.ToLower(normalizedRepoURL)))
		hashStr := hex.EncodeToString(hash[:])
		if len(hashStr) > 12 {
			hashStr = hashStr[:12]
		}
		resourceName = resourceName[:240] + "-" + hashStr
	}

	return resourceName
}
