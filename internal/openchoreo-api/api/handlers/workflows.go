// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListWorkflows returns a list of generic workflows
func (h *Handler) ListWorkflows(
	ctx context.Context,
	request gen.ListWorkflowsRequestObject,
) (gen.ListWorkflowsResponseObject, error) {
	return nil, errNotImplemented
}

// GetWorkflowSchema returns the parameter schema for a workflow
func (h *Handler) GetWorkflowSchema(
	ctx context.Context,
	request gen.GetWorkflowSchemaRequestObject,
) (gen.GetWorkflowSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// ListWorkflowRuns returns a list of workflow runs
func (h *Handler) ListWorkflowRuns(
	ctx context.Context,
	request gen.ListWorkflowRunsRequestObject,
) (gen.ListWorkflowRunsResponseObject, error) {
	return nil, errNotImplemented
}

// CreateWorkflowRun creates a new workflow run
func (h *Handler) CreateWorkflowRun(
	ctx context.Context,
	request gen.CreateWorkflowRunRequestObject,
) (gen.CreateWorkflowRunResponseObject, error) {
	return nil, errNotImplemented
}

// GetWorkflowRun returns a specific workflow run
func (h *Handler) GetWorkflowRun(
	ctx context.Context,
	request gen.GetWorkflowRunRequestObject,
) (gen.GetWorkflowRunResponseObject, error) {
	return nil, errNotImplemented
}
