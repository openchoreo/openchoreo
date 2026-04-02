// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"log/slog"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Exported constants for the external test package.
var (
	ExportActionCreate  = actionCreateWorkflowRun
	ExportActionUpdate  = actionUpdateWorkflowRun
	ExportActionDelete  = authz.ActionDeleteWorkflowRun
	ExportActionView    = actionViewWorkflowRun
	ExportResourceType  = resourceTypeWorkflowRun
	ExportStatusPending = workflowRunStatusPending
)

// ExportConstructHierarchy exposes constructHierarchyForAuthzCheck for testing.
var ExportConstructHierarchy = constructHierarchyForAuthzCheck

// NewTestServiceWithAuthz creates a workflowRunServiceWithAuthz with injectable dependencies for testing.
func NewTestServiceWithAuthz(internal Service, pdp authz.PDP, logger *slog.Logger) Service {
	return &workflowRunServiceWithAuthz{
		internal: internal,
		authz:    services.NewAuthzChecker(pdp, logger),
	}
}
