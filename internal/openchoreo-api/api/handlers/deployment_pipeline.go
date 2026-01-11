// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetProjectDeploymentPipeline returns the deployment pipeline for a project
func (h *Handler) GetProjectDeploymentPipeline(
	ctx context.Context,
	request gen.GetProjectDeploymentPipelineRequestObject,
) (gen.GetProjectDeploymentPipelineResponseObject, error) {
	return nil, errNotImplemented
}
