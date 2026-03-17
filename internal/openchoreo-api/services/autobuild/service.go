// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
)

const (
	// webhookSecretName is the name of the Kubernetes Secret containing webhook secrets.
	webhookSecretName = "git-webhook-secrets" // #nosec G101 -- This is a secret name, not a hardcoded credential
	// webhookSecretNamespace is the namespace where the webhook secret is stored.
	webhookSecretNamespace = "openchoreo-control-plane" // #nosec G101 -- This is a namespace name, not a credential
)

// WebhookProcessor handles the core webhook processing: finding affected components and
// triggering builds. webhookProcessor (in this package) satisfies this interface.
type WebhookProcessor interface {
	ProcessWebhook(ctx context.Context, provider git.Provider, payload []byte) ([]string, error)
}

type autobuildService struct {
	k8sClient client.Client
	processor WebhookProcessor
	logger    *slog.Logger
}

var _ Service = (*autobuildService)(nil)

// NewService creates a new autobuild service.
func NewService(k8sClient client.Client, processor WebhookProcessor, logger *slog.Logger) Service {
	return &autobuildService{
		k8sClient: k8sClient,
		processor: processor,
		logger:    logger,
	}
}

// ProcessWebhook retrieves the appropriate git provider, validates the payload signature
// against the stored Kubernetes secret, and delegates processing to the WebhookProcessor.
func (s *autobuildService) ProcessWebhook(ctx context.Context, params *ProcessWebhookParams) (*WebhookResult, error) {
	provider, err := git.GetProvider(params.ProviderType)
	if err != nil {
		s.logger.Error("Failed to get git provider", "error", err, "provider", params.ProviderType)
		return nil, fmt.Errorf("failed to get git provider: %w", err)
	}

	allowEmpty := params.SignatureHeader == ""
	webhookSecret, err := s.getWebhookSecret(ctx, params.SecretKey, allowEmpty)
	if err != nil {
		s.logger.Error("Failed to get webhook secret", "error", err, "provider", params.ProviderType)
		return nil, ErrSecretNotConfigured
	}

	if err := provider.ValidateWebhookPayload(params.Payload, params.Signature, webhookSecret); err != nil {
		s.logger.Error("Invalid webhook signature", "error", err, "provider", params.ProviderType)
		return nil, ErrInvalidSignature
	}

	affectedComponents, err := s.processor.ProcessWebhook(ctx, provider, params.Payload)
	if err != nil {
		s.logger.Error("Failed to process webhook", "error", err, "provider", params.ProviderType)
		return nil, fmt.Errorf("failed to process webhook: %w", err)
	}

	s.logger.Info("Webhook processed successfully",
		"provider", params.ProviderType,
		"affectedComponents", len(affectedComponents),
	)
	return &WebhookResult{AffectedComponents: affectedComponents}, nil
}

// getWebhookSecret retrieves the webhook secret value for the given key from the Kubernetes Secret.
// When allowEmpty is true (e.g. Bitbucket, which has no HMAC header), a missing or empty key is not an error.
func (s *autobuildService) getWebhookSecret(ctx context.Context, secretKey string, allowEmpty bool) (string, error) {
	secret := &corev1.Secret{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      webhookSecretName,
		Namespace: webhookSecretNamespace,
	}, secret); err != nil {
		return "", fmt.Errorf("failed to get webhook secret %s/%s: %w",
			webhookSecretNamespace, webhookSecretName, err)
	}

	secretData, ok := secret.Data[secretKey]
	if !ok {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("secret %s/%s does not contain '%s' key",
			webhookSecretNamespace, webhookSecretName, secretKey)
	}

	if len(secretData) == 0 {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("secret %s/%s has empty '%s' value",
			webhookSecretNamespace, webhookSecretName, secretKey)
	}

	return string(secretData), nil
}
