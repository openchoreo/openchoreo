// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/list/output"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
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
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceProject, params); err != nil {
		return err
	}

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

// ListEnvironments lists all environments in a namespace
func (l *ListImpl) ListEnvironments(params api.ListEnvironmentsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceEnvironment, params); err != nil {
		return err
	}

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
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceDataPlane, params); err != nil {
		return err
	}

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
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceBuildPlane, params); err != nil {
		return err
	}

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
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceObservabilityPlane, params); err != nil {
		return err
	}

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
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponentType, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentTypes(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list component types: %w", err)
	}

	return output.PrintComponentTypes(result)
}

// ListTraits lists all traits in a namespace
func (l *ListImpl) ListTraits(params api.ListTraitsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceTrait, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListTraits(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list traits: %w", err)
	}

	return output.PrintTraits(result)
}

// ListWorkflows lists all workflows in a namespace
func (l *ListImpl) ListWorkflows(params api.ListWorkflowsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflow, params); err != nil {
		return err
	}

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

// ListSecretReferences lists all secret references in a namespace
func (l *ListImpl) ListSecretReferences(params api.ListSecretReferencesParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceSecretReference, params); err != nil {
		return err
	}

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

// ListWorkflowRuns lists all workflow runs in a namespace
func (l *ListImpl) ListWorkflowRuns(params api.ListWorkflowRunsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflowRun, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListWorkflowRuns(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list workflow runs: %w", err)
	}

	return output.PrintWorkflowRuns(result)
}

// ListComponentWorkflowRuns lists all component workflow runs for a component
func (l *ListImpl) ListComponentWorkflowRuns(params api.ListComponentWorkflowRunsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponentWorkflowRun, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentWorkflowRuns(ctx, params.Namespace, params.Project, params.Component)
	if err != nil {
		return fmt.Errorf("failed to list component workflow runs: %w", err)
	}

	return output.PrintComponentWorkflowRuns(result)
}
