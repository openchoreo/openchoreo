// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

import (
	"context"
	"fmt"
	"log/slog"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// constructAuthzCheckRequest builds an authorization evaluation request from context and resource details
func constructAuthzCheckRequest(ctx context.Context, action, resourceType, resourceID string, hierarchy authz.ResourceHierarchy) *authz.EvaluateRequest {
	// Extract SubjectContext from context (set by authentication middleware)
	authSubjectCtx, _ := auth.GetSubjectContextFromContext(ctx)

	// Convert auth.SubjectContext to authz.SubjectContext
	authzSubjectCtx := authz.GetAuthzSubjectContext(authSubjectCtx)

	return &authz.EvaluateRequest{
		SubjectContext: authzSubjectCtx,
		Action:         action,
		Resource: authz.Resource{
			Type:      resourceType,
			ID:        resourceID,
			Hierarchy: hierarchy,
		},
		Context: authz.Context{},
	}
}

// checkAuthorization performs a complete authorization check including request construction and evaluation
func checkAuthorization(ctx context.Context, logger *slog.Logger, pdp authz.PDP, action systemAction, resourceType ResourceType, resourceID string, hierarchy authz.ResourceHierarchy) error {
	authzReq := constructAuthzCheckRequest(ctx, string(action), string(resourceType), resourceID, hierarchy)

	decision, err := pdp.Evaluate(ctx, authzReq)
	if err != nil {
		logger.Error("Failed to evaluate authorization", "error", err, "action", action, "resourceType", resourceType, "resourceID", resourceID)
		return fmt.Errorf("authorization evaluation failed: %w", err)
	}

	logger.Debug("authorization decision received",
		"decision", decision.Decision,
		"reason", decision.Context.Reason,
	)

	if !decision.Decision {
		return ErrForbidden
	}

	return nil
}

// toAgentConnectionStatusResponse converts a CRD AgentConnectionStatus to an API response
func toAgentConnectionStatusResponse(ac *openchoreov1alpha1.AgentConnectionStatus) *models.AgentConnectionStatusResponse {
	if ac == nil {
		return nil
	}
	resp := &models.AgentConnectionStatusResponse{
		Connected:       ac.Connected,
		ConnectedAgents: ac.ConnectedAgents,
		Message:         ac.Message,
	}
	if ac.LastConnectedTime != nil {
		t := ac.LastConnectedTime.Time
		resp.LastConnectedTime = &t
	}
	if ac.LastDisconnectedTime != nil {
		t := ac.LastDisconnectedTime.Time
		resp.LastDisconnectedTime = &t
	}
	if ac.LastHeartbeatTime != nil {
		t := ac.LastHeartbeatTime.Time
		resp.LastHeartbeatTime = &t
	}
	return resp
}
