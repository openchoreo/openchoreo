// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Client wraps the generated OpenAPI client with token refresh functionality
type Client struct {
	client *gen.ClientWithResponses
	token  string
}

func NewClient() (*Client, error) {
	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane: %w", err)
	}

	token := ""
	credential, err := config.GetCurrentCredential()
	if err == nil && credential != nil {
		token = credential.Token
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	client, err := gen.NewClientWithResponses(
		controlPlane.URL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			// Set X-Use-OpenAPI header to indicate OpenAPI client
			req.Header.Set("X-Use-OpenAPI", "true")

			// Token refresh logic
			currentToken := token
			if currentToken != "" && auth.IsTokenExpired(currentToken) {
				newToken, err := auth.RefreshToken()
				if err != nil {
					return fmt.Errorf("failed to refresh token: %w", err)
				}
				currentToken = newToken
				token = newToken
			}
			if currentToken != "" {
				req.Header.Set("Authorization", "Bearer "+currentToken)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return &Client{
		client: client,
		token:  token,
	}, nil
}

// ListNamespaces retrieves all namespaces
func (c *Client) ListNamespaces(ctx context.Context, params *gen.ListNamespacesParams) (*gen.NamespaceList, error) {
	resp, err := c.client.ListNamespacesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListProjects retrieves all projects for a namespace
func (c *Client) ListProjects(ctx context.Context, namespaceName string, params *gen.ListProjectsParams) (*gen.ProjectList, error) {
	resp, err := c.client.ListProjectsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListComponents retrieves all components for a namespace, optionally filtered by project
func (c *Client) ListComponents(ctx context.Context, namespaceName, projectName string, params *gen.ListComponentsParams) (*gen.ComponentList, error) {
	if params == nil {
		params = &gen.ListComponentsParams{}
	}
	if projectName != "" {
		params.Project = &projectName
	}
	resp, err := c.client.ListComponentsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GetComponent retrieves a specific component
func (c *Client) GetComponent(ctx context.Context, namespaceName, componentName string) (*gen.Component, error) {
	resp, err := c.client.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListEnvironments retrieves all environments for a namespace
func (c *Client) ListEnvironments(ctx context.Context, namespaceName string, params *gen.ListEnvironmentsParams) (*gen.EnvironmentList, error) {
	resp, err := c.client.ListEnvironmentsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListDataPlanes retrieves all data planes for a namespace
func (c *Client) ListDataPlanes(ctx context.Context, namespaceName string, params *gen.ListDataPlanesParams) (*gen.DataPlaneList, error) {
	resp, err := c.client.ListDataPlanesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list data planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListBuildPlanes retrieves all build planes for a namespace
func (c *Client) ListBuildPlanes(ctx context.Context, namespaceName string) (*gen.BuildPlaneList, error) {
	resp, err := c.client.ListBuildPlanesWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListObservabilityPlanes retrieves all observability planes for a namespace
func (c *Client) ListObservabilityPlanes(ctx context.Context, namespaceName string) (*gen.ObservabilityPlaneList, error) {
	resp, err := c.client.ListObservabilityPlanesWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list observability planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListComponentTypes retrieves all component types for a namespace
func (c *Client) ListComponentTypes(ctx context.Context, namespaceName string) (*gen.ComponentTypeList, error) {
	resp, err := c.client.ListComponentTypesWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list component types: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListTraits retrieves all traits for a namespace
func (c *Client) ListTraits(ctx context.Context, namespaceName string) (*gen.TraitList, error) {
	resp, err := c.client.ListTraitsWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list traits: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListWorkflows retrieves all workflows for a namespace
func (c *Client) ListWorkflows(ctx context.Context, namespaceName string, params *gen.ListWorkflowsParams) (*gen.WorkflowList, error) {
	resp, err := c.client.ListWorkflowsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListComponentWorkflows retrieves all component workflows for a namespace
func (c *Client) ListComponentWorkflows(ctx context.Context, namespaceName string, params *gen.ListComponentWorkflowsParams) (*gen.ComponentWorkflowTemplateList, error) {
	resp, err := c.client.ListComponentWorkflowsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list component workflows: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListSecretReferences retrieves all secret references for a namespace
func (c *Client) ListSecretReferences(ctx context.Context, namespaceName string) (*gen.SecretReferenceList, error) {
	resp, err := c.client.ListSecretReferencesWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret references: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GenerateRelease generates an immutable release snapshot via the flat K8s-native endpoint
func (c *Client) GenerateRelease(ctx context.Context, namespaceName, componentName string, req gen.GenerateReleaseRequest) (*gen.ComponentRelease, error) {
	resp, err := c.client.GenerateReleaseWithResponse(ctx, namespaceName, componentName, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate release: %w", err)
	}
	if resp.JSON201 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// ListReleaseBindings retrieves all release bindings for a component
func (c *Client) ListReleaseBindings(ctx context.Context, namespaceName, projectName, componentName string) (*gen.ReleaseBindingList, error) {
	resp, err := c.client.ListReleaseBindingsWithResponse(ctx, namespaceName, projectName, componentName, &gen.ListReleaseBindingsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListComponentReleases retrieves all component releases for a component
func (c *Client) ListComponentReleases(ctx context.Context, namespaceName, projectName, componentName string) (*gen.ComponentReleaseList, error) {
	resp, err := c.client.ListComponentReleasesWithResponse(ctx, namespaceName, projectName, componentName, &gen.ListComponentReleasesParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list component releases: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListWorkflowRuns retrieves all workflow runs for a namespace
func (c *Client) ListWorkflowRuns(ctx context.Context, namespaceName string) (*gen.WorkflowRunList, error) {
	resp, err := c.client.ListWorkflowRunsWithResponse(ctx, namespaceName, &gen.ListWorkflowRunsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListComponentWorkflowRuns retrieves all component workflow runs for a component
func (c *Client) ListComponentWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string) (*gen.ComponentWorkflowRunList, error) {
	resp, err := c.client.ListComponentWorkflowRunsWithResponse(ctx, namespaceName, projectName, componentName, &gen.ListComponentWorkflowRunsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list component workflow runs: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// PatchReleaseBinding updates a release binding
func (c *Client) PatchReleaseBinding(ctx context.Context, namespaceName, projectName, componentName, bindingName string, req gen.PatchReleaseBindingRequest) (*gen.ReleaseBinding, error) {
	resp, err := c.client.PatchReleaseBindingWithResponse(ctx, namespaceName, projectName, componentName, bindingName, req)
	if err != nil {
		return nil, fmt.Errorf("failed to patch release binding: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// DeployRelease deploys a component release to the root environment
func (c *Client) DeployRelease(ctx context.Context, namespaceName, componentName string, req gen.DeployReleaseRequest) (*gen.ReleaseBinding, error) {
	resp, err := c.client.DeployReleaseWithResponse(ctx, namespaceName, componentName, req)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy release: %w", err)
	}
	if resp.JSON201 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// PromoteComponent promotes a component from source to target environment
func (c *Client) PromoteComponent(ctx context.Context, namespaceName, componentName string, req gen.PromoteComponentRequest) (*gen.ReleaseBinding, error) {
	resp, err := c.client.PromoteComponentWithResponse(ctx, namespaceName, componentName, req)
	if err != nil {
		return nil, fmt.Errorf("failed to promote component: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GetProject retrieves a project by name
func (c *Client) GetProject(ctx context.Context, namespaceName, projectName string) (*gen.Project, error) {
	resp, err := c.client.GetProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GetProjectDeploymentPipeline retrieves a project's deployment pipeline
func (c *Client) GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*gen.DeploymentPipeline, error) {
	resp, err := c.client.GetProjectDeploymentPipelineWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project deployment pipeline: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// CreateWorkflowRun creates a new workflow run
func (c *Client) CreateWorkflowRun(
	ctx context.Context,
	namespace string,
	workflowName string,
	parameters map[string]interface{},
) (*gen.WorkflowRun, error) {
	req := gen.CreateWorkflowRunJSONRequestBody{
		WorkflowName: workflowName,
		Parameters:   parameters,
	}

	resp, err := c.client.CreateWorkflowRunWithResponse(ctx, namespace, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}

	return resp.JSON201, nil
}

// UpdateComponentWorkflowParameters updates workflow parameters for a component
func (c *Client) UpdateComponentWorkflowParameters(
	ctx context.Context,
	namespace, project, component string,
	body gen.UpdateComponentWorkflowParametersJSONRequestBody,
) error {
	resp, err := c.client.UpdateComponentWorkflowParametersWithResponse(
		ctx,
		namespace,
		project,
		component,
		body,
	)
	if err != nil {
		return fmt.Errorf("failed to update component workflow parameters: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}

	return nil
}

// CreateComponentWorkflowRun creates a new component workflow run
func (c *Client) CreateComponentWorkflowRun(
	ctx context.Context,
	namespace, project, component string,
	commit string,
) (*gen.ComponentWorkflowRun, error) {
	// Prepare query params
	params := &gen.CreateComponentWorkflowRunParams{}
	if commit != "" {
		params.Commit = &commit
	}

	resp, err := c.client.CreateComponentWorkflowRunWithResponse(
		ctx,
		namespace,
		project,
		component,
		params,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create component workflow run: %w", err)
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}

	return resp.JSON201, nil
}

// GetEnvironment retrieves an environment by name
func (c *Client) GetEnvironment(ctx context.Context, namespaceName, envName string) (*gen.Environment, error) {
	resp, err := c.client.GetEnvironmentWithResponse(ctx, namespaceName, envName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GetEnvironmentObserverURL retrieves the observer URL for an environment
func (c *Client) GetEnvironmentObserverURL(ctx context.Context, namespaceName, envName string) (string, error) {
	resp, err := c.client.GetEnvironmentObserverURLWithResponse(ctx, namespaceName, envName)
	if err != nil {
		return "", fmt.Errorf("failed to get environment observer URL: %w", err)
	}
	if resp.JSON200 == nil {
		return "", fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	if resp.JSON200.ObserverUrl == nil {
		return "", fmt.Errorf("observer URL not configured for environment")
	}
	return *resp.JSON200.ObserverUrl, nil
}

// GetComponentObserverURL retrieves the observer URL for a component in an environment
func (c *Client) GetComponentObserverURL(ctx context.Context, namespaceName, projectName, componentName, envName string) (string, error) {
	resp, err := c.client.GetComponentObserverURLWithResponse(ctx, namespaceName, projectName, componentName, envName)
	if err != nil {
		return "", fmt.Errorf("failed to get component observer URL for component %s/%s/%s in environment %s: %w",
			namespaceName, projectName, componentName, envName, err)
	}
	if resp.JSON200 == nil {
		return "", fmt.Errorf("unexpected response status %d for component %s/%s/%s in environment %s",
			resp.StatusCode(), namespaceName, projectName, componentName, envName)
	}
	if resp.JSON200.ObserverUrl == nil {
		return "", fmt.Errorf("observer URL not configured for component %s/%s/%s in environment %s",
			namespaceName, projectName, componentName, envName)
	}
	return *resp.JSON200.ObserverUrl, nil
}
