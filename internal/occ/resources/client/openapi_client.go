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

// ListComponents retrieves all components for a namespace and project
func (c *Client) ListComponents(ctx context.Context, namespaceName, projectName string, params *gen.ListComponentsParams) (*gen.ComponentList, error) {
	resp, err := c.client.ListComponentsWithResponse(ctx, namespaceName, projectName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
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
func (c *Client) ListComponentTypes(ctx context.Context, namespaceName string, params *gen.ListComponentTypesParams) (*gen.ComponentTypeList, error) {
	resp, err := c.client.ListComponentTypesWithResponse(ctx, namespaceName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list component types: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListTraits retrieves all traits for a namespace
func (c *Client) ListTraits(ctx context.Context, namespaceName string, params *gen.ListTraitsParams) (*gen.TraitList, error) {
	resp, err := c.client.ListTraitsWithResponse(ctx, namespaceName, params)
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
