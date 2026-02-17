// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"log/slog"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// CheckRequest represents a resource authorization check.
type CheckRequest struct {
	Action       string
	ResourceType string
	ResourceID   string
	Hierarchy    authz.ResourceHierarchy
}

// AuthzChecker provides authorization checking for service authz wrappers.
type AuthzChecker struct {
	pdp    authz.PDP
	logger *slog.Logger
}

// NewAuthzChecker creates a new AuthzChecker.
func NewAuthzChecker(pdp authz.PDP, logger *slog.Logger) *AuthzChecker {
	return &AuthzChecker{pdp: pdp, logger: logger}
}

// Check performs a single authorization check.
func (c *AuthzChecker) Check(ctx context.Context, req CheckRequest) error {
	authSubjectCtx, _ := auth.GetSubjectContextFromContext(ctx)
	authzSubjectCtx := authz.GetAuthzSubjectContext(authSubjectCtx)

	evalReq := &authz.EvaluateRequest{
		SubjectContext: authzSubjectCtx,
		Action:         req.Action,
		Resource: authz.Resource{
			Type:      req.ResourceType,
			ID:        req.ResourceID,
			Hierarchy: req.Hierarchy,
		},
		Context: authz.Context{},
	}

	decision, err := c.pdp.Evaluate(ctx, evalReq)
	if err != nil {
		c.logger.Error("Failed to evaluate authorization", "error", err, "action", req.Action, "resourceType", req.ResourceType, "resourceID", req.ResourceID)
		return fmt.Errorf("authorization evaluation failed: %w", err)
	}

	c.logger.Debug("authorization decision received",
		"decision", decision.Decision,
		"reason", decision.Context.Reason,
	)

	if !decision.Decision {
		return ErrForbidden
	}

	return nil
}

// BatchCheck performs a batch authorization check and returns a boolean slice
func (c *AuthzChecker) BatchCheck(ctx context.Context, requests []CheckRequest) ([]bool, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	authSubjectCtx, _ := auth.GetSubjectContextFromContext(ctx)
	authzSubjectCtx := authz.GetAuthzSubjectContext(authSubjectCtx)

	evalRequests := make([]authz.EvaluateRequest, len(requests))
	for i, r := range requests {
		evalRequests[i] = authz.EvaluateRequest{
			SubjectContext: authzSubjectCtx,
			Action:         r.Action,
			Resource: authz.Resource{
				Type:      r.ResourceType,
				ID:        r.ResourceID,
				Hierarchy: r.Hierarchy,
			},
			Context: authz.Context{},
		}
	}

	resp, err := c.pdp.BatchEvaluate(ctx, &authz.BatchEvaluateRequest{Requests: evalRequests})
	if err != nil {
		c.logger.Error("Failed to batch evaluate authorization", "error", err)
		return nil, fmt.Errorf("batch authorization evaluation failed: %w", err)
	}

	results := make([]bool, len(resp.Decisions))
	for i, d := range resp.Decisions {
		results[i] = d.Decision
	}

	return results, nil
}
