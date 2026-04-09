// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/agent"
	cpgen "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// NewCPTools creates the control-plane tools that call the OpenChoreo API
// using the generated client.
func NewCPTools(baseURL string, httpClient *http.Client) ([]agent.Tool, error) {
	client, err := cpgen.NewClient(baseURL, cpgen.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("creating openchoreo-api client: %w", err)
	}

	return []agent.Tool{
		{
			Name:        "list_components",
			Description: "List all components in a project. Components are deployable units (services, jobs, etc.) with independent build and deployment lifecycles.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "project"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"project":   map[string]any{"type": "string", "description": "Project name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Project   string `json:"project"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				resp, err := client.ListComponents(ctx, p.Namespace, &cpgen.ListComponentsParams{
					Project: &p.Project,
				})
				if err != nil {
					return "", err
				}
				return readResponse(resp)
			},
		},
		{
			Name:        "list_release_bindings",
			Description: "List release bindings for a component. Release bindings associate releases with environments and define deployment configurations including workloadOverrides, traitEnvironmentConfigs, and componentTypeEnvironmentConfigs.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name to filter by"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				resp, err := client.ListReleaseBindings(ctx, p.Namespace, &cpgen.ListReleaseBindingsParams{
					Component: &p.Component,
				})
				if err != nil {
					return "", err
				}
				return readResponse(resp)
			},
		},
		{
			Name:        "list_component_releases",
			Description: "List releases for a component. Returns release versions, status, and associated metadata.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				resp, err := client.ListComponentReleases(ctx, p.Namespace, &cpgen.ListComponentReleasesParams{
					Component: &p.Component,
				})
				if err != nil {
					return "", err
				}
				return readResponse(resp)
			},
		},
		{
			Name:        "get_component_workloads",
			Description: "List workloads for a component. Shows workload names, container specs, images, environment variables, and endpoint names.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				resp, err := client.ListWorkloads(ctx, p.Namespace, &cpgen.ListWorkloadsParams{
					Component: &p.Component,
				})
				if err != nil {
					return "", err
				}
				return readResponse(resp)
			},
		},
		{
			Name:        "get_component_release_schema",
			Description: "Get the JSON Schema for a component's configuration options. Returns the schema showing valid override fields for traits and componentType settings. Source of truth for valid fields when building release bindings.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				resp, err := client.GetComponentSchema(ctx, p.Namespace, p.Component)
				if err != nil {
					return "", err
				}
				return readResponse(resp)
			},
		},
		{
			Name:        "list_component_traits",
			Description: "List traits attached to a component with their base parameter values.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				resp, err := client.GetComponent(ctx, p.Namespace, p.Component)
				if err != nil {
					return "", err
				}
				body, err := readResponse(resp)
				if err != nil {
					return "", err
				}
				var comp struct {
					Spec struct {
						Traits json.RawMessage `json:"traits"`
					} `json:"spec"`
				}
				if err := json.Unmarshal([]byte(body), &comp); err != nil {
					return "", fmt.Errorf("decoding component response: %w", err)
				}
				if comp.Spec.Traits == nil || string(comp.Spec.Traits) == "null" {
					return "[]", nil
				}
				return string(comp.Spec.Traits), nil
			},
		},
	}, nil
}

// readResponse reads the HTTP response body and returns it as a string.
// Returns an error for non-2xx status codes.
func readResponse(resp *http.Response) (string, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
