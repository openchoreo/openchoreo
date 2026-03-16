// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

type Services struct {
	WorkflowRunService   WorkflowRunServiceInterface
	WorkflowPlaneService *WorkflowPlaneService
	WebhookService       *WebhookService
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, k8sClientMgr *kubernetesClient.KubeMultiClientManager, authzPDP authz.PDP,
	logger *slog.Logger, gwClient *gatewayClient.Client) *Services {
	// Create workflow plane service with client manager for multi-cluster support
	workflowPlaneService := NewWorkflowPlaneService(k8sClient, k8sClientMgr, logger.With("service", "workflowplane"), authzPDP)

	// Create WorkflowRun service
	workflowRunService := NewWorkflowRunService(k8sClient, logger.With("service", "workflowrun"), authzPDP, workflowPlaneService, gwClient)

	// Create webhook service (handles all git providers)
	webhookService := NewWebhookService(k8sClient, workflowRunService, logger.With("service", "webhook"))

	return &Services{
		WorkflowRunService:   workflowRunService,
		WorkflowPlaneService: workflowPlaneService,
		WebhookService:       webhookService,
	}
}
