// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// Service defines the project service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
type Service interface {
	CreateProject(ctx context.Context, namespaceName string, req *models.CreateProjectRequest) (*models.ProjectResponse, error)
	ListProjects(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error)
	GetProject(ctx context.Context, namespaceName, projectName string) (*models.ProjectResponse, error)
	DeleteProject(ctx context.Context, namespaceName, projectName string) error
	ProjectExists(ctx context.Context, namespaceName, projectName string) (bool, error)
}
