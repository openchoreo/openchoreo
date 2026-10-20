// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// releaseLineArgs are the writable fields of a release line, shared by create_release_line and
// patch_release_line.
type releaseLineArgs struct {
	NamespaceName      string           `json:"namespace_name"`
	ComponentName      string           `json:"component_name"`
	Description        string           `json:"description"`
	AutoDeploy         *bool            `json:"auto_deploy,omitempty"`
	Parameters         map[string]any   `json:"parameters"`
	Traits             []map[string]any `json:"traits"`
	WorkflowParameters map[string]any   `json:"workflow_parameters"`
}

func releaseLineSpecProperties() map[string]any {
	return map[string]any{
		"description": stringProperty("Optional: Human-readable description, e.g. the branch the line builds"),
		"auto_deploy": map[string]any{
			"type":        "boolean",
			"description": "Optional: Overrides the component's auto_deploy for this line",
		},
		"parameters": map[string]any{
			"type": "object",
			"description": "Optional: Deep-merged onto the component's parameters for this line. " +
				"Maps merge key by key; scalars and lists replace.",
		},
		"traits": map[string]any{
			"type": "array",
			"description": "Optional: Merged onto the component's traits by kind, name and instanceName. " +
				"A matching trait has its parameters deep-merged; a new trait is added. " +
				"Each item has 'name', 'instanceName', optional 'kind' (Trait or ClusterTrait) and 'parameters'.",
			"items": map[string]any{"type": "object"},
		},
		"workflow_parameters": map[string]any{
			"type": "object",
			"description": "Optional: Deep-merged onto the component's workflow parameters for this line, " +
				"typically to build from a different branch.",
		},
	}
}

// toReleaseLine builds the API release line from tool arguments. The owner is filled in by the
// server from the component.
func (a releaseLineArgs) toReleaseLine(name string) (*gen.ReleaseLine, error) {
	spec := map[string]any{"owner": map[string]any{"componentName": a.ComponentName}}
	if a.AutoDeploy != nil {
		spec["autoDeploy"] = *a.AutoDeploy
	}
	if a.Parameters != nil {
		spec["parameters"] = a.Parameters
	}
	if a.Traits != nil {
		spec["traits"] = a.Traits
	}
	if a.WorkflowParameters != nil {
		spec["workflow"] = map[string]any{"parameters": a.WorkflowParameters}
	}
	metadata := map[string]any{"name": name}
	if a.Description != "" {
		metadata["annotations"] = map[string]any{"openchoreo.dev/description": a.Description}
	}
	raw, err := json.Marshal(map[string]any{"metadata": metadata, "spec": spec})
	if err != nil {
		return nil, err
	}
	line := &gen.ReleaseLine{}
	if err := json.Unmarshal(raw, line); err != nil {
		return nil, fmt.Errorf("invalid release line: %w", err)
	}
	return line, nil
}

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListReleaseLines(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_release_lines"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseLine}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List the release lines of a component with release lines enabled. A release line is a " +
			"named series of releases of the component (e.g. main, next), built and deployed on its own, " +
			"with its own workload, releases and release bindings. Shows each line's overrides, latest " +
			"release and the environments it runs in. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
		}), []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListReleaseLines(
			ctx, args.NamespaceName, args.ComponentName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetReleaseLine(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_release_line"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseLine}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get a release line of a component, including the overrides it applies on top of the " +
			"component, its conditions, latest release, and the release binding, release and readiness " +
			"per environment.",
		InputSchema: createSchema(map[string]any{
			"namespace_name":    defaultStringProperty(),
			"component_name":    stringProperty("Use list_components to discover valid names"),
			"release_line_name": stringProperty("Use list_release_lines to discover valid names"),
		}, []string{"namespace_name", "component_name", "release_line_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName   string `json:"namespace_name"`
		ComponentName   string `json:"component_name"`
		ReleaseLineName string `json:"release_line_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetReleaseLine(
			ctx, args.NamespaceName, args.ComponentName, args.ReleaseLineName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateReleaseLine(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_release_line"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateReleaseLine}
	props := releaseLineSpecProperties()
	props["namespace_name"] = defaultStringProperty()
	props["component_name"] = stringProperty(
		"Component with release lines enabled. Use list_components to discover valid names")
	props["name"] = stringProperty("Release line name (lowercase, alphanumeric, hyphens only, max 63 chars). " +
		"Unique within the namespace, across all components")
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a release line for a component with release lines enabled. The line starts from " +
			"the component's configuration and can override its parameters, traits, workflow parameters and " +
			"auto_deploy. After creating it, build the line with trigger_workflow_run (release_line) or create " +
			"its workload with create_workload (release_line).",
		InputSchema: createSchema(props, []string{"namespace_name", "component_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		releaseLineArgs
		Name string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		line, err := args.toReleaseLine(args.Name)
		if err != nil {
			return nil, nil, err
		}
		result, err := t.ComponentToolset.CreateReleaseLine(ctx, args.NamespaceName, args.ComponentName, line)
		return handleToolResult(result, err)
	})
}

// Overrides patch_release_line can remove so the line inherits them from the component again.
const (
	resetAutoDeploy         = "auto_deploy"
	resetParameters         = "parameters"
	resetTraits             = "traits"
	resetWorkflowParameters = "workflow_parameters"
)

func (t *Toolsets) RegisterPatchReleaseLine(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "patch_release_line"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateReleaseLine}
	props := releaseLineSpecProperties()
	props["namespace_name"] = defaultStringProperty()
	props["component_name"] = stringProperty("Use list_components to discover valid names")
	props["release_line_name"] = stringProperty("Use list_release_lines to discover valid names")
	props["description"] = stringProperty(
		"Optional: Updated human-readable description. Empty string is treated as no-change.")
	props["reset"] = map[string]any{
		"type": "array",
		"description": "Optional: Overrides to remove so the line inherits them from the component again. " +
			"A field can't be both set and reset.",
		"items": map[string]any{
			"type": "string",
			"enum": []string{resetAutoDeploy, resetParameters, resetTraits, resetWorkflowParameters},
		},
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Patch (partially update) a release line's overrides. Only the fields provided are " +
			"updated; omitted fields remain unchanged. A provided parameters, traits or workflow_parameters " +
			"value replaces the line's existing override for that field. Use reset to remove an override.",
		InputSchema: createSchema(props, []string{"namespace_name", "component_name", "release_line_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		releaseLineArgs
		ReleaseLineName string   `json:"release_line_name"`
		Reset           []string `json:"reset"`
	}) (*mcp.CallToolResult, any, error) {
		set := map[string]bool{
			resetAutoDeploy:         args.AutoDeploy != nil,
			resetParameters:         args.Parameters != nil,
			resetTraits:             args.Traits != nil,
			resetWorkflowParameters: args.WorkflowParameters != nil,
		}
		for _, field := range args.Reset {
			isSet, known := set[field]
			if !known {
				return nil, nil, fmt.Errorf("reset: unknown field %q", field)
			}
			if isSet {
				return nil, nil, fmt.Errorf("%s can't be both set and reset", field)
			}
		}
		patch, err := args.toReleaseLine(args.ReleaseLineName)
		if err != nil {
			return nil, nil, err
		}
		result, err := t.ComponentToolset.PatchReleaseLine(ctx, args.NamespaceName, args.ComponentName, patch, args.Reset)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteReleaseLine(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_release_line"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteReleaseLine}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a release line of a component. The line is undeployed from every environment, " +
			"and its releases and workload are removed. The component's other lines keep running.",
		InputSchema: createSchema(map[string]any{
			"namespace_name":    defaultStringProperty(),
			"component_name":    stringProperty("Use list_components to discover valid names"),
			"release_line_name": stringProperty("Use list_release_lines to discover valid names"),
		}, []string{"namespace_name", "component_name", "release_line_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName   string `json:"namespace_name"`
		ComponentName   string `json:"component_name"`
		ReleaseLineName string `json:"release_line_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.DeleteReleaseLine(
			ctx, args.NamespaceName, args.ComponentName, args.ReleaseLineName)
		return handleToolResult(result, err)
	})
}

// releaseLineProperty is the release_line argument of tools that act on one line of a component.
func releaseLineProperty() map[string]any {
	return stringProperty("Release line of the component. Required when the component has release lines " +
		"enabled, and must be omitted otherwise. Use list_release_lines to discover valid names")
}
