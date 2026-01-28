// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/client"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/list/output"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// ListImpl implements list operations
type ListImpl struct{}

// NewListImpl creates a new list implementation
func NewListImpl() *ListImpl {
	return &ListImpl{}
}

// ListNamespaces lists all namespaces
func (l *ListImpl) ListNamespaces(params api.ListNamespacesParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListNamespaces(ctx, &gen.ListNamespacesParams{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	return output.PrintNamespaces(result)
}

// ListProjects lists all projects in a namespace
func (l *ListImpl) ListProjects(params api.ListProjectsParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListProjects(ctx, params.Namespace, &gen.ListProjectsParams{})
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	return output.PrintProjects(result)
}

// ListComponents lists all components in a project
func (l *ListImpl) ListComponents(params api.ListComponentsParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponents(ctx, params.Namespace, params.Project, &gen.ListComponentsParams{})
	if err != nil {
		return fmt.Errorf("failed to list components: %w", err)
	}

	return output.PrintComponents(result)
}

// ListEnvironments lists all environments in a namespace
func (l *ListImpl) ListEnvironments(params api.ListEnvironmentsParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListEnvironments(ctx, params.Namespace, &gen.ListEnvironmentsParams{})
	if err != nil {
		return fmt.Errorf("failed to list environments: %w", err)
	}

	return output.PrintEnvironments(result)
}

// ListDataPlanes lists all data planes in a namespace
func (l *ListImpl) ListDataPlanes(params api.ListDataPlanesParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListDataPlanes(ctx, params.Namespace, &gen.ListDataPlanesParams{})
	if err != nil {
		return fmt.Errorf("failed to list data planes: %w", err)
	}

	return output.PrintDataPlanes(result)
}

// ListBuildPlanes lists all build planes in a namespace
func (l *ListImpl) ListBuildPlanes(params api.ListBuildPlanesParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListBuildPlanes(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list build planes: %w", err)
	}

	return output.PrintBuildPlanes(result)
}

// ListObservabilityPlanes lists all observability planes in a namespace
func (l *ListImpl) ListObservabilityPlanes(params api.ListObservabilityPlanesParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListObservabilityPlanes(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list observability planes: %w", err)
	}

	return output.PrintObservabilityPlanes(result)
}

// ListComponentTypes lists all component types in a namespace
func (l *ListImpl) ListComponentTypes(params api.ListComponentTypesParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentTypes(ctx, params.Namespace, &gen.ListComponentTypesParams{})
	if err != nil {
		return fmt.Errorf("failed to list component types: %w", err)
	}

	return output.PrintComponentTypes(result)
}

// ListTraits lists all traits in a namespace
func (l *ListImpl) ListTraits(params api.ListTraitsParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListTraits(ctx, params.Namespace, &gen.ListTraitsParams{})
	if err != nil {
		return fmt.Errorf("failed to list traits: %w", err)
	}

	return output.PrintTraits(result)
}

// ListWorkflows lists all workflows in a namespace
func (l *ListImpl) ListWorkflows(params api.ListWorkflowsParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListWorkflows(ctx, params.Namespace, &gen.ListWorkflowsParams{})
	if err != nil {
		return fmt.Errorf("failed to list workflows: %w", err)
	}

	return output.PrintWorkflows(result)
}

// ListComponentWorkflows lists all component workflows in a namespace
func (l *ListImpl) ListComponentWorkflows(params api.ListComponentWorkflowsParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentWorkflows(ctx, params.Namespace, &gen.ListComponentWorkflowsParams{})
	if err != nil {
		return fmt.Errorf("failed to list component workflows: %w", err)
	}

	return output.PrintComponentWorkflows(result)
}

// ListSecretReferences lists all secret references in a namespace
func (l *ListImpl) ListSecretReferences(params api.ListSecretReferencesParams) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListSecretReferences(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list secret references: %w", err)
	}

	return output.PrintSecretReferences(result)
}
