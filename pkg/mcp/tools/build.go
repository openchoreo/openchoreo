// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t *Toolsets) RegisterListBuildTemplates(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_build_templates",
		Description: "List available build templates in an organization. Build templates define how source code " +
			"is transformed into container images (Docker, Buildpacks, Kaniko, etc.).",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.ListBuildTemplates(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterTriggerBuild(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "trigger_build",
		Description: "Trigger a new build for a component at a specific commit. Creates a container image that " +
			"can be deployed to environments. Builds run asynchronously; use list_builds to monitor progress.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"commit":         stringProperty("Git commit SHA (full or short) or tag"),
		}, []string{"org_name", "project_name", "component_name", "commit"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Commit        string `json:"commit"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.TriggerBuild(ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Commit)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListBuilds(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_builds",
		Description: "List all builds for a component showing build history, status (queued, running, " +
			"succeeded, failed), commit information, and generated image tags.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.ListBuilds(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetBuildObserverURL(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_build_observer_url",
		Description: "Get the observability dashboard URL for component builds. Provides access to real-time " +
			"build logs, pipeline stages, and build history.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetBuildObserverURL(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListBuildPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_buildplanes",
		Description: "List all build planes in an organization. Build planes are dedicated infrastructure where " +
			"component builds execute (isolated from runtime workloads).",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.ListBuildPlanes(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}
