// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListComponentWorkflows returns a list of component workflows
func (h *Handler) ListComponentWorkflows(
	ctx context.Context,
	request gen.ListComponentWorkflowsRequestObject,
) (gen.ListComponentWorkflowsResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentWorkflowSchema returns the parameter schema for a component workflow
func (h *Handler) GetComponentWorkflowSchema(
	ctx context.Context,
	request gen.GetComponentWorkflowSchemaRequestObject,
) (gen.GetComponentWorkflowSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// UpdateComponentWorkflowParameters updates the workflow parameters for a component
func (h *Handler) UpdateComponentWorkflowParameters(
	ctx context.Context,
	request gen.UpdateComponentWorkflowParametersRequestObject,
) (gen.UpdateComponentWorkflowParametersResponseObject, error) {
	return nil, errNotImplemented
}

// ListComponentWorkflowRuns returns a list of workflow runs for a component
func (h *Handler) ListComponentWorkflowRuns(
	ctx context.Context,
	request gen.ListComponentWorkflowRunsRequestObject,
) (gen.ListComponentWorkflowRunsResponseObject, error) {
	return nil, errNotImplemented
}

// CreateComponentWorkflowRun triggers a new workflow run for a component
func (h *Handler) CreateComponentWorkflowRun(
	ctx context.Context,
	request gen.CreateComponentWorkflowRunRequestObject,
) (gen.CreateComponentWorkflowRunResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentWorkflowRun returns details of a specific workflow run
func (h *Handler) GetComponentWorkflowRun(
	ctx context.Context,
	request gen.GetComponentWorkflowRunRequestObject,
) (gen.GetComponentWorkflowRunResponseObject, error) {
	return nil, errNotImplemented
}
