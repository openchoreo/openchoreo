// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// apiError extracts a human-readable error message from a raw API response body.
// The OpenChoreo API returns structured ErrorResponse JSON on failures; this
// function parses it and falls back to the raw body when parsing fails.
func apiError(statusCode int, body []byte) error {
	var errResp gen.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		msg := errResp.Error
		if errResp.Details != nil {
			for _, d := range *errResp.Details {
				if d.Field != nil && d.Message != nil {
					msg += fmt.Sprintf("; %s: %s", *d.Field, *d.Message)
				}
			}
		}
		return fmt.Errorf("%s", msg)
	}
	if len(body) > 0 {
		return fmt.Errorf("unexpected response (HTTP %d): %s", statusCode, string(body))
	}
	return fmt.Errorf("unexpected response status: %d", statusCode)
}

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

// GetClient returns the underlying generated OpenAPI client.
func (c *Client) GetClient() *gen.ClientWithResponses {
	return c.client
}

// ListNamespaces retrieves all namespaces
func (c *Client) ListNamespaces(ctx context.Context, params *gen.ListNamespacesParams) (*gen.NamespaceList, error) {
	resp, err := c.client.ListNamespacesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
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
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteProject deletes a project
func (c *Client) DeleteProject(ctx context.Context, namespaceName, projectName string) error {
	resp, err := c.client.DeleteProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
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
		return nil, apiError(resp.StatusCode(), resp.Body)
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
		return nil, apiError(resp.StatusCode(), resp.Body)
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
		return nil, apiError(resp.StatusCode(), resp.Body)
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
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListWorkflowPlanes retrieves all workflow planes for a namespace
func (c *Client) ListWorkflowPlanes(ctx context.Context, namespaceName string, params *gen.ListWorkflowPlanesParams) (*gen.WorkflowPlaneList, error) {
	resp, err := c.client.ListWorkflowPlanesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListObservabilityPlanes retrieves all observability planes for a namespace
func (c *Client) ListObservabilityPlanes(ctx context.Context, namespaceName string, params *gen.ListObservabilityPlanesParams) (*gen.ObservabilityPlaneList, error) {
	if params == nil {
		params = &gen.ListObservabilityPlanesParams{}
	}
	resp, err := c.client.ListObservabilityPlanesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list observability planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListComponentTypes retrieves all component types for a namespace
func (c *Client) ListComponentTypes(ctx context.Context, namespaceName string, params *gen.ListComponentTypesParams) (*gen.ComponentTypeList, error) {
	if params == nil {
		params = &gen.ListComponentTypesParams{}
	}
	resp, err := c.client.ListComponentTypesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list component types: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetComponentType retrieves a specific component type
func (c *Client) GetComponentType(ctx context.Context, namespaceName, ctName string) (*gen.ComponentType, error) {
	resp, err := c.client.GetComponentTypeWithResponse(ctx, namespaceName, ctName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component type: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// CreateComponentType creates a new component type
func (c *Client) CreateComponentType(ctx context.Context, namespaceName string, ct gen.ComponentType) (*gen.ComponentType, error) {
	resp, err := c.client.CreateComponentTypeWithResponse(ctx, namespaceName, ct)
	if err != nil {
		return nil, fmt.Errorf("failed to create component type: %w", err)
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// UpdateComponentType updates an existing component type
func (c *Client) UpdateComponentType(ctx context.Context, namespaceName, ctName string, ct gen.ComponentType) (*gen.ComponentType, error) {
	resp, err := c.client.UpdateComponentTypeWithResponse(ctx, namespaceName, ctName, ct)
	if err != nil {
		return nil, fmt.Errorf("failed to update component type: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteComponent deletes a component
func (c *Client) DeleteComponent(ctx context.Context, namespaceName, componentName string) error {
	resp, err := c.client.DeleteComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to delete component: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// DeleteComponentType deletes a component type
func (c *Client) DeleteComponentType(ctx context.Context, namespaceName, ctName string) error {
	resp, err := c.client.DeleteComponentTypeWithResponse(ctx, namespaceName, ctName)
	if err != nil {
		return fmt.Errorf("failed to delete component type: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListClusterComponentTypes retrieves all cluster-scoped component types
func (c *Client) ListClusterComponentTypes(ctx context.Context, params *gen.ListClusterComponentTypesParams) (*gen.ClusterComponentTypeList, error) {
	if params == nil {
		params = &gen.ListClusterComponentTypesParams{}
	}
	resp, err := c.client.ListClusterComponentTypesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster component types: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListClusterTraits retrieves all cluster-scoped traits
func (c *Client) ListClusterTraits(ctx context.Context, params *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error) {
	if params == nil {
		params = &gen.ListClusterTraitsParams{}
	}
	resp, err := c.client.ListClusterTraitsWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster traits: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListTraits retrieves all traits for a namespace
func (c *Client) ListTraits(ctx context.Context, namespaceName string, params *gen.ListTraitsParams) (*gen.TraitList, error) {
	if params == nil {
		params = &gen.ListTraitsParams{}
	}
	resp, err := c.client.ListTraitsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list traits: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetTrait retrieves a specific trait
func (c *Client) GetTrait(ctx context.Context, namespaceName, traitName string) (*gen.Trait, error) {
	resp, err := c.client.GetTraitWithResponse(ctx, namespaceName, traitName)
	if err != nil {
		return nil, fmt.Errorf("failed to get trait: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// CreateTrait creates a new trait
func (c *Client) CreateTrait(ctx context.Context, namespaceName string, t gen.Trait) (*gen.Trait, error) {
	resp, err := c.client.CreateTraitWithResponse(ctx, namespaceName, t)
	if err != nil {
		return nil, fmt.Errorf("failed to create trait: %w", err)
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// UpdateTrait updates an existing trait
func (c *Client) UpdateTrait(ctx context.Context, namespaceName, traitName string, t gen.Trait) (*gen.Trait, error) {
	resp, err := c.client.UpdateTraitWithResponse(ctx, namespaceName, traitName, t)
	if err != nil {
		return nil, fmt.Errorf("failed to update trait: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteTrait deletes a trait
func (c *Client) DeleteTrait(ctx context.Context, namespaceName, traitName string) error {
	resp, err := c.client.DeleteTraitWithResponse(ctx, namespaceName, traitName)
	if err != nil {
		return fmt.Errorf("failed to delete trait: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListWorkflows retrieves all workflows for a namespace
func (c *Client) ListWorkflows(ctx context.Context, namespaceName string, params *gen.ListWorkflowsParams) (*gen.WorkflowList, error) {
	resp, err := c.client.ListWorkflowsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetWorkflow retrieves a specific workflow by name
func (c *Client) GetWorkflow(ctx context.Context, namespaceName, workflowName string) (*gen.Workflow, error) {
	resp, err := c.client.GetWorkflowWithResponse(ctx, namespaceName, workflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteWorkflow deletes a workflow
func (c *Client) DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) error {
	resp, err := c.client.DeleteWorkflowWithResponse(ctx, namespaceName, workflowName)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListSecretReferences retrieves all secret references for a namespace
func (c *Client) ListSecretReferences(ctx context.Context, namespaceName string, params *gen.ListSecretReferencesParams) (*gen.SecretReferenceList, error) {
	if params == nil {
		params = &gen.ListSecretReferencesParams{}
	}
	resp, err := c.client.ListSecretReferencesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret references: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
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
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// ListReleaseBindings retrieves all release bindings for a component
func (c *Client) ListReleaseBindings(ctx context.Context, namespaceName string, params *gen.ListReleaseBindingsParams) (*gen.ReleaseBindingList, error) {
	if params == nil {
		params = &gen.ListReleaseBindingsParams{}
	}
	resp, err := c.client.ListReleaseBindingsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListComponentReleases retrieves all component releases for a component
func (c *Client) ListComponentReleases(ctx context.Context, namespaceName string, params *gen.ListComponentReleasesParams) (*gen.ComponentReleaseList, error) {
	if params == nil {
		params = &gen.ListComponentReleasesParams{}
	}
	resp, err := c.client.ListComponentReleasesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list component releases: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListWorkflowRuns retrieves all workflow runs for a namespace
func (c *Client) ListWorkflowRuns(ctx context.Context, namespaceName string, params *gen.ListWorkflowRunsParams) (*gen.WorkflowRunList, error) {
	if params == nil {
		params = &gen.ListWorkflowRunsParams{}
	}
	resp, err := c.client.ListWorkflowRunsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// UpdateReleaseBinding updates a release binding
func (c *Client) UpdateReleaseBinding(ctx context.Context, namespaceName, bindingName string, req gen.ReleaseBinding) (*gen.ReleaseBinding, error) {
	resp, err := c.client.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update release binding: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// CreateReleaseBinding creates a new release binding
func (c *Client) CreateReleaseBinding(ctx context.Context, namespaceName string, req gen.ReleaseBinding) (*gen.ReleaseBinding, error) {
	resp, err := c.client.CreateReleaseBindingWithResponse(ctx, namespaceName, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create release binding: %w", err)
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// GetProject retrieves a project by name
func (c *Client) GetProject(ctx context.Context, namespaceName, projectName string) (*gen.Project, error) {
	resp, err := c.client.GetProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetProjectDeploymentPipeline retrieves a project's deployment pipeline by
// first resolving the pipeline name from the project, then fetching the pipeline.
func (c *Client) GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*gen.DeploymentPipeline, error) {
	project, err := c.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if project.Spec == nil || project.Spec.DeploymentPipelineRef == nil || strings.TrimSpace(project.Spec.DeploymentPipelineRef.Name) == "" {
		return nil, fmt.Errorf("project %q does not have a deployment pipeline configured", projectName)
	}
	pipelineName := strings.TrimSpace(project.Spec.DeploymentPipelineRef.Name)

	resp, err := c.client.GetDeploymentPipelineWithResponse(ctx, namespaceName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// CreateWorkflowRun creates a new workflow run
func (c *Client) CreateWorkflowRun(
	ctx context.Context,
	namespace string,
	body gen.CreateWorkflowRunJSONRequestBody,
) (*gen.WorkflowRun, error) {
	resp, err := c.client.CreateWorkflowRunWithResponse(ctx, namespace, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
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
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetNamespace retrieves a specific namespace
func (c *Client) GetNamespace(ctx context.Context, namespaceName string) (*gen.Namespace, error) {
	resp, err := c.client.GetNamespaceWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteNamespace deletes a namespace
func (c *Client) DeleteNamespace(ctx context.Context, namespaceName string) error {
	resp, err := c.client.DeleteNamespaceWithResponse(ctx, namespaceName)
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// DeleteEnvironment deletes an environment
func (c *Client) DeleteEnvironment(ctx context.Context, namespaceName, envName string) error {
	resp, err := c.client.DeleteEnvironmentWithResponse(ctx, namespaceName, envName)
	if err != nil {
		return fmt.Errorf("failed to delete environment: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetDataPlane retrieves a specific data plane
func (c *Client) GetDataPlane(ctx context.Context, namespaceName, dpName string) (*gen.DataPlane, error) {
	resp, err := c.client.GetDataPlaneWithResponse(ctx, namespaceName, dpName)
	if err != nil {
		return nil, fmt.Errorf("failed to get data plane: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListClusterDataPlanes retrieves all cluster-scoped data planes
func (c *Client) ListClusterDataPlanes(ctx context.Context, params *gen.ListClusterDataPlanesParams) (*gen.ClusterDataPlaneList, error) {
	if params == nil {
		params = &gen.ListClusterDataPlanesParams{}
	}
	resp, err := c.client.ListClusterDataPlanesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster data planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetClusterDataPlane retrieves a specific cluster data plane
func (c *Client) GetClusterDataPlane(ctx context.Context, cdpName string) (*gen.ClusterDataPlane, error) {
	resp, err := c.client.GetClusterDataPlaneWithResponse(ctx, cdpName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster data plane: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterDataPlane deletes a cluster data plane
func (c *Client) DeleteClusterDataPlane(ctx context.Context, cdpName string) error {
	resp, err := c.client.DeleteClusterDataPlaneWithResponse(ctx, cdpName)
	if err != nil {
		return fmt.Errorf("failed to delete cluster data plane: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// DeleteDataPlane deletes a data plane
func (c *Client) DeleteDataPlane(ctx context.Context, namespaceName, dpName string) error {
	resp, err := c.client.DeleteDataPlaneWithResponse(ctx, namespaceName, dpName)
	if err != nil {
		return fmt.Errorf("failed to delete data plane: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetWorkflowPlane retrieves a specific workflow plane
func (c *Client) GetWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) (*gen.WorkflowPlane, error) {
	resp, err := c.client.GetWorkflowPlaneWithResponse(ctx, namespaceName, workflowPlaneName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteWorkflowPlane deletes a workflow plane
func (c *Client) DeleteWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) error {
	resp, err := c.client.DeleteWorkflowPlaneWithResponse(ctx, namespaceName, workflowPlaneName)
	if err != nil {
		return fmt.Errorf("failed to delete workflow plane: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetObservabilityPlane retrieves a specific observability plane
func (c *Client) GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (*gen.ObservabilityPlane, error) {
	resp, err := c.client.GetObservabilityPlaneWithResponse(ctx, namespaceName, observabilityPlaneName)
	if err != nil {
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteObservabilityPlane deletes an observability plane
func (c *Client) DeleteObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) error {
	resp, err := c.client.DeleteObservabilityPlaneWithResponse(ctx, namespaceName, observabilityPlaneName)
	if err != nil {
		return fmt.Errorf("failed to delete observability plane: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetClusterComponentType retrieves a specific cluster component type
func (c *Client) GetClusterComponentType(ctx context.Context, cctName string) (*gen.ClusterComponentType, error) {
	resp, err := c.client.GetClusterComponentTypeWithResponse(ctx, cctName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster component type: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterComponentType deletes a cluster component type
func (c *Client) DeleteClusterComponentType(ctx context.Context, cctName string) error {
	resp, err := c.client.DeleteClusterComponentTypeWithResponse(ctx, cctName)
	if err != nil {
		return fmt.Errorf("failed to delete cluster component type: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetClusterTrait retrieves a specific cluster trait
func (c *Client) GetClusterTrait(ctx context.Context, clusterTraitName string) (*gen.ClusterTrait, error) {
	resp, err := c.client.GetClusterTraitWithResponse(ctx, clusterTraitName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster trait: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterTrait deletes a cluster trait
func (c *Client) DeleteClusterTrait(ctx context.Context, clusterTraitName string) error {
	resp, err := c.client.DeleteClusterTraitWithResponse(ctx, clusterTraitName)
	if err != nil {
		return fmt.Errorf("failed to delete cluster trait: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListClusterWorkflows retrieves all cluster-scoped workflows
func (c *Client) ListClusterWorkflows(ctx context.Context, params *gen.ListClusterWorkflowsParams) (*gen.ClusterWorkflowList, error) {
	if params == nil {
		params = &gen.ListClusterWorkflowsParams{}
	}
	resp, err := c.client.ListClusterWorkflowsWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster workflows: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetClusterWorkflow retrieves a specific cluster workflow
func (c *Client) GetClusterWorkflow(ctx context.Context, clusterWorkflowName string) (*gen.ClusterWorkflow, error) {
	resp, err := c.client.GetClusterWorkflowWithResponse(ctx, clusterWorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster workflow: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterWorkflow deletes a cluster workflow
func (c *Client) DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) error {
	resp, err := c.client.DeleteClusterWorkflowWithResponse(ctx, clusterWorkflowName)
	if err != nil {
		return fmt.Errorf("failed to delete cluster workflow: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetSecretReference retrieves a specific secret reference
func (c *Client) GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (*gen.SecretReference, error) {
	resp, err := c.client.GetSecretReferenceWithResponse(ctx, namespaceName, secretReferenceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteSecretReference deletes a secret reference
func (c *Client) DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) error {
	resp, err := c.client.DeleteSecretReferenceWithResponse(ctx, namespaceName, secretReferenceName)
	if err != nil {
		return fmt.Errorf("failed to delete secret reference: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetWorkflowRun retrieves a specific workflow run
func (c *Client) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*gen.WorkflowRun, error) {
	resp, err := c.client.GetWorkflowRunWithResponse(ctx, namespaceName, runName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetWorkload retrieves a specific workload
func (c *Client) GetWorkload(ctx context.Context, namespaceName, workloadName string) (*gen.Workload, error) {
	resp, err := c.client.GetWorkloadWithResponse(ctx, namespaceName, workloadName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListWorkloads retrieves all workloads for a namespace
func (c *Client) ListWorkloads(ctx context.Context, namespaceName string, params *gen.ListWorkloadsParams) (*gen.WorkloadList, error) {
	if params == nil {
		params = &gen.ListWorkloadsParams{}
	}
	resp, err := c.client.ListWorkloadsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteWorkload deletes a workload
func (c *Client) DeleteWorkload(ctx context.Context, namespaceName, workloadName string) error {
	resp, err := c.client.DeleteWorkloadWithResponse(ctx, namespaceName, workloadName)
	if err != nil {
		return fmt.Errorf("failed to delete workload: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetDeploymentPipeline retrieves a specific deployment pipeline
func (c *Client) GetDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) (*gen.DeploymentPipeline, error) {
	resp, err := c.client.GetDeploymentPipelineWithResponse(ctx, namespaceName, deploymentPipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListDeploymentPipelines retrieves all deployment pipelines for a namespace
func (c *Client) ListDeploymentPipelines(ctx context.Context, namespaceName string, params *gen.ListDeploymentPipelinesParams) (*gen.DeploymentPipelineList, error) {
	if params == nil {
		params = &gen.ListDeploymentPipelinesParams{}
	}
	resp, err := c.client.ListDeploymentPipelinesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment pipelines: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteDeploymentPipeline deletes a deployment pipeline
func (c *Client) DeleteDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) error {
	resp, err := c.client.DeleteDeploymentPipelineWithResponse(ctx, namespaceName, deploymentPipelineName)
	if err != nil {
		return fmt.Errorf("failed to delete deployment pipeline: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetReleaseBinding retrieves a specific release binding
func (c *Client) GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*gen.ReleaseBinding, error) {
	resp, err := c.client.GetReleaseBindingWithResponse(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, nil
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteReleaseBinding deletes a release binding
func (c *Client) DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error {
	resp, err := c.client.DeleteReleaseBindingWithResponse(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return fmt.Errorf("failed to delete release binding: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetComponentRelease retrieves a specific component release
func (c *Client) GetComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) (*gen.ComponentRelease, error) {
	resp, err := c.client.GetComponentReleaseWithResponse(ctx, namespaceName, componentReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// CreateComponentRelease creates a new component release
func (c *Client) CreateComponentRelease(ctx context.Context, namespaceName string, cr gen.ComponentRelease) (*gen.ComponentRelease, error) {
	resp, err := c.client.CreateComponentReleaseWithResponse(ctx, namespaceName, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to create component release: %w", err)
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// DeleteComponentRelease deletes a component release
func (c *Client) DeleteComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) error {
	resp, err := c.client.DeleteComponentReleaseWithResponse(ctx, namespaceName, componentReleaseName)
	if err != nil {
		return fmt.Errorf("failed to delete component release: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListObservabilityAlertsNotificationChannels retrieves all observability alerts notification channels for a namespace
func (c *Client) ListObservabilityAlertsNotificationChannels(ctx context.Context, namespaceName string, params *gen.ListObservabilityAlertsNotificationChannelsParams) (*gen.ObservabilityAlertsNotificationChannelList, error) {
	if params == nil {
		params = &gen.ListObservabilityAlertsNotificationChannelsParams{}
	}
	resp, err := c.client.ListObservabilityAlertsNotificationChannelsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list observability alerts notification channels: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetObservabilityAlertsNotificationChannel retrieves a specific observability alerts notification channel
func (c *Client) GetObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) (*gen.ObservabilityAlertsNotificationChannel, error) {
	resp, err := c.client.GetObservabilityAlertsNotificationChannelWithResponse(ctx, namespaceName, channelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get observability alerts notification channel: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteObservabilityAlertsNotificationChannel deletes an observability alerts notification channel
func (c *Client) DeleteObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) error {
	resp, err := c.client.DeleteObservabilityAlertsNotificationChannelWithResponse(ctx, namespaceName, channelName)
	if err != nil {
		return fmt.Errorf("failed to delete observability alerts notification channel: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListClusterRoles retrieves all cluster-scoped authorization roles
func (c *Client) ListClusterRoles(ctx context.Context, params *gen.ListClusterRolesParams) (*gen.ClusterAuthzRoleList, error) {
	if params == nil {
		params = &gen.ListClusterRolesParams{}
	}
	resp, err := c.client.ListClusterRolesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster roles: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetClusterRole retrieves a specific cluster-scoped authorization role
func (c *Client) GetClusterRole(ctx context.Context, name string) (*gen.ClusterAuthzRole, error) {
	resp, err := c.client.GetClusterRoleWithResponse(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster role: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListClusterRoleBindings retrieves all cluster-scoped role bindings
func (c *Client) ListClusterRoleBindings(ctx context.Context, params *gen.ListClusterRoleBindingsParams) (*gen.ClusterAuthzRoleBindingList, error) {
	if params == nil {
		params = &gen.ListClusterRoleBindingsParams{}
	}
	resp, err := c.client.ListClusterRoleBindingsWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster role bindings: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetClusterRoleBinding retrieves a specific cluster-scoped role binding
func (c *Client) GetClusterRoleBinding(ctx context.Context, name string) (*gen.ClusterAuthzRoleBinding, error) {
	resp, err := c.client.GetClusterRoleBindingWithResponse(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster role binding: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListNamespaceRoles retrieves all namespace-scoped authorization roles
func (c *Client) ListNamespaceRoles(ctx context.Context, namespaceName string, params *gen.ListNamespaceRolesParams) (*gen.AuthzRoleList, error) {
	if params == nil {
		params = &gen.ListNamespaceRolesParams{}
	}
	resp, err := c.client.ListNamespaceRolesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetNamespaceRole retrieves a specific namespace-scoped authorization role
func (c *Client) GetNamespaceRole(ctx context.Context, namespaceName, name string) (*gen.AuthzRole, error) {
	resp, err := c.client.GetNamespaceRoleWithResponse(ctx, namespaceName, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListNamespaceRoleBindings retrieves all namespace-scoped role bindings
func (c *Client) ListNamespaceRoleBindings(ctx context.Context, namespaceName string, params *gen.ListNamespaceRoleBindingsParams) (*gen.AuthzRoleBindingList, error) {
	if params == nil {
		params = &gen.ListNamespaceRoleBindingsParams{}
	}
	resp, err := c.client.ListNamespaceRoleBindingsWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetNamespaceRoleBinding retrieves a specific namespace-scoped role binding
func (c *Client) GetNamespaceRoleBinding(ctx context.Context, namespaceName, name string) (*gen.AuthzRoleBinding, error) {
	resp, err := c.client.GetNamespaceRoleBindingWithResponse(ctx, namespaceName, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get role binding: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterRole deletes a cluster-scoped authorization role
func (c *Client) DeleteClusterRole(ctx context.Context, name string) error {
	resp, err := c.client.DeleteClusterRoleWithResponse(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to delete cluster role: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// DeleteClusterRoleBinding deletes a cluster-scoped role binding
func (c *Client) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	resp, err := c.client.DeleteClusterRoleBindingWithResponse(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to delete cluster role binding: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// DeleteNamespaceRole deletes a namespace-scoped authorization role
func (c *Client) DeleteNamespaceRole(ctx context.Context, namespaceName, name string) error {
	resp, err := c.client.DeleteNamespaceRoleWithResponse(ctx, namespaceName, name)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// DeleteNamespaceRoleBinding deletes a namespace-scoped role binding
func (c *Client) DeleteNamespaceRoleBinding(ctx context.Context, namespaceName, name string) error {
	resp, err := c.client.DeleteNamespaceRoleBindingWithResponse(ctx, namespaceName, name)
	if err != nil {
		return fmt.Errorf("failed to delete role binding: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetWorkflowRunLogs retrieves live logs for a workflow run from the workflow plane
func (c *Client) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName string, params *gen.GetWorkflowRunLogsParams) ([]gen.WorkflowRunLogEntry, error) {
	if params == nil {
		params = &gen.GetWorkflowRunLogsParams{}
	}
	resp, err := c.client.GetWorkflowRunLogsWithResponse(ctx, namespaceName, runName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run logs: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return *resp.JSON200, nil
}

// GetWorkflowRunStatus retrieves the status of a workflow run including live observability info
func (c *Client) GetWorkflowRunStatus(ctx context.Context, namespaceName, runName string) (*gen.WorkflowRunStatusResponse, error) {
	resp, err := c.client.GetWorkflowRunStatusWithResponse(ctx, namespaceName, runName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run status: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetClusterWorkflowPlane retrieves a specific cluster workflow plane
func (c *Client) GetClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) (*gen.ClusterWorkflowPlane, error) {
	resp, err := c.client.GetClusterWorkflowPlaneWithResponse(ctx, clusterWorkflowPlaneName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster workflow plane: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ListClusterWorkflowPlanes retrieves all cluster-scoped workflow planes
func (c *Client) ListClusterWorkflowPlanes(ctx context.Context, params *gen.ListClusterWorkflowPlanesParams) (*gen.ClusterWorkflowPlaneList, error) {
	if params == nil {
		params = &gen.ListClusterWorkflowPlanesParams{}
	}
	resp, err := c.client.ListClusterWorkflowPlanesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster workflow planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterWorkflowPlane deletes a cluster workflow plane
func (c *Client) DeleteClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) error {
	resp, err := c.client.DeleteClusterWorkflowPlaneWithResponse(ctx, clusterWorkflowPlaneName)
	if err != nil {
		return fmt.Errorf("failed to delete cluster workflow plane: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// ListClusterObservabilityPlanes retrieves all cluster-scoped observability planes
func (c *Client) ListClusterObservabilityPlanes(ctx context.Context, params *gen.ListClusterObservabilityPlanesParams) (*gen.ClusterObservabilityPlaneList, error) {
	if params == nil {
		params = &gen.ListClusterObservabilityPlanesParams{}
	}
	resp, err := c.client.ListClusterObservabilityPlanesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster observability planes: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// GetClusterObservabilityPlane retrieves a specific cluster observability plane
func (c *Client) GetClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) (*gen.ClusterObservabilityPlane, error) {
	resp, err := c.client.GetClusterObservabilityPlaneWithResponse(ctx, clusterObservabilityPlaneName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster observability plane: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DeleteClusterObservabilityPlane deletes a cluster observability plane
func (c *Client) DeleteClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) error {
	resp, err := c.client.DeleteClusterObservabilityPlaneWithResponse(ctx, clusterObservabilityPlaneName)
	if err != nil {
		return fmt.Errorf("failed to delete cluster observability plane: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetComponentTypeSchema retrieves the parameter schema for a component type
func (c *Client) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (*json.RawMessage, error) {
	resp, err := c.client.GetComponentTypeSchemaWithResponse(ctx, namespaceName, ctName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component type schema: %w", err)
	}
	if resp.JSON404 != nil {
		return nil, fmt.Errorf("component type %q not found", ctName)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return schemaResponseToRaw(resp.JSON200)
}

// GetTraitSchema retrieves the parameter schema for a trait
func (c *Client) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (*json.RawMessage, error) {
	resp, err := c.client.GetTraitSchemaWithResponse(ctx, namespaceName, traitName)
	if err != nil {
		return nil, fmt.Errorf("failed to get trait schema: %w", err)
	}
	if resp.JSON404 != nil {
		return nil, fmt.Errorf("trait %q not found", traitName)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return schemaResponseToRaw(resp.JSON200)
}

// GetWorkflowSchema retrieves the parameter schema for a workflow
func (c *Client) GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (*json.RawMessage, error) {
	resp, err := c.client.GetWorkflowSchemaWithResponse(ctx, namespaceName, workflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow schema: %w", err)
	}
	if resp.JSON404 != nil {
		return nil, fmt.Errorf("workflow %q not found", workflowName)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return schemaResponseToRaw(resp.JSON200)
}

// GetClusterComponentTypeSchema retrieves the parameter schema for a cluster-scoped component type
func (c *Client) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (*json.RawMessage, error) {
	resp, err := c.client.GetClusterComponentTypeSchemaWithResponse(ctx, cctName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster component type schema: %w", err)
	}
	if resp.JSON404 != nil {
		return nil, fmt.Errorf("cluster component type %q not found", cctName)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return schemaResponseToRaw(resp.JSON200)
}

// GetClusterTraitSchema retrieves the parameter schema for a cluster-scoped trait
func (c *Client) GetClusterTraitSchema(ctx context.Context, clusterTraitName string) (*json.RawMessage, error) {
	resp, err := c.client.GetClusterTraitSchemaWithResponse(ctx, clusterTraitName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster trait schema: %w", err)
	}
	if resp.JSON404 != nil {
		return nil, fmt.Errorf("cluster trait %q not found", clusterTraitName)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return schemaResponseToRaw(resp.JSON200)
}

// GetClusterWorkflowSchema retrieves the parameter schema for a cluster-scoped workflow
func (c *Client) GetClusterWorkflowSchema(ctx context.Context, clusterWorkflowName string) (*json.RawMessage, error) {
	resp, err := c.client.GetClusterWorkflowSchemaWithResponse(ctx, clusterWorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster workflow schema: %w", err)
	}
	if resp.JSON404 != nil {
		return nil, fmt.Errorf("cluster workflow %q not found", clusterWorkflowName)
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return schemaResponseToRaw(resp.JSON200)
}

func schemaResponseToRaw(schema *gen.SchemaResponse) (*json.RawMessage, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}
	raw := json.RawMessage(data)
	return &raw, nil
}
