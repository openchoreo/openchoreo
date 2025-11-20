// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t *Toolsets) RegisterGetDeploymentPipeline(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_deployment_pipeline",
		Description: "Get the deployment pipeline configuration for a project. Shows the progression path for " +
			"builds through environments (e.g., dev → staging → production) and promotion policies.",
		InputSchema: createSchema(map[string]any{
			"org_name":     defaultStringProperty(),
			"project_name": defaultStringProperty(),
		}, []string{"org_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetProjectDeploymentPipeline(ctx, args.OrgName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentObserverURL(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_observer_url",
		Description: "Get the observability dashboard URL for a deployed component in a specific environment. " +
			"Provides access to real-time logs, metrics, traces, and debugging tools.",
		InputSchema: createSchema(map[string]any{
			"org_name":         defaultStringProperty(),
			"project_name":     defaultStringProperty(),
			"component_name":   defaultStringProperty(),
			"environment_name": defaultStringProperty(),
		}, []string{"org_name", "project_name", "component_name", "environment_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName         string `json:"org_name"`
		ProjectName     string `json:"project_name"`
		ComponentName   string `json:"component_name"`
		EnvironmentName string `json:"environment_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetComponentObserverURL(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.EnvironmentName,
		)
		return handleToolResult(result, err)
	})
}
