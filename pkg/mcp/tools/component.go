// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListComponents(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_components"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponent}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all components in a project. Components are deployable units (services, jobs, etc.) " +
			"with independent build and deployment lifecycles. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
		}), []string{"namespace_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponents(
			ctx, args.NamespaceName, args.ProjectName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponent(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponent}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a component including configuration, deployment status, " +
			"and builds.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponent(ctx, args.NamespaceName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListWorkloads(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_workloads"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkload}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List workloads for a component. Shows workload names, images, and endpoint names. " +
			"For Kubernetes users: Similar to 'kubectl get pods'.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListWorkloads(ctx, args.NamespaceName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkload(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workload"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkload}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a specific workload including container " +
			"configuration, endpoints, and connections.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workload_name":  stringProperty("Use list_workloads to discover valid names"),
		}, []string{"namespace_name", "workload_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkloadName  string `json:"workload_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetWorkload(ctx, args.NamespaceName, args.WorkloadName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a specific release binding including environment, " +
			"release name, state, overrides, endpoints, and deployment status.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"binding_name":   stringProperty("Use list_release_bindings to discover valid names"),
		}, []string{"namespace_name", "binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		BindingName   string `json:"binding_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetReleaseBinding(ctx, args.NamespaceName, args.BindingName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponent(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_component"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateComponent}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new component in a project. Components are deployable units (services, jobs, etc.) " +
			"with independent build and deployment lifecycles. For components using the from-image approach " +
			"(no workflow to build from source), use create_workload after creating the component to define " +
			"the runtime specification. For components that use workflows to build from source, the workload " +
			"is generated automatically; use update_workload to modify it if the source repository does not " +
			"contain a workload descriptor (workload.yaml).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"component_type": stringProperty("Component type in {workloadType}/{componentTypeName} format. " +
				"Use list_component_types or list_cluster_component_types to discover valid values."),
			"auto_deploy": map[string]any{
				"type": "boolean",
				"description": "Optional: Automatically triggers the component deployment if the component or" +
					" related resources such as build, configs are updated. Defaults to true.",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Component type parameters (port, replicas, exposed, etc.)",
			},
			"workflow": map[string]any{
				"type": "object",
				"description": "Optional: Component workflow configuration. " +
					"Set 'name' to the workflow name, 'kind' to 'ClusterWorkflow' or 'Workflow' " +
					"to match the workflow resource type, and 'parameters' to the workflow parameters " +
					"that strictly adhere to the workflow schema. " +
					"Use list_cluster_workflows or list_workflows to discover available workflow names " +
					"for cluster-scoped (ClusterWorkflow kind, used with ClusterComponentType) or " +
					"namespace-scoped (Workflow kind) workflows respectively. " +
					"Use get_cluster_workflow_schema or get_workflow_schema to inspect the " +
					"parameter schema for a given workflow.",
			},
		}, []string{"namespace_name", "project_name", "name", "component_type"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ProjectName   string                 `json:"project_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		ComponentType string                 `json:"component_type"`
		AutoDeploy    *bool                  `json:"auto_deploy,omitempty"`
		Parameters    map[string]interface{} `json:"parameters"`
		Workflow      map[string]interface{} `json:"workflow"`
	}) (*mcp.CallToolResult, any, error) {
		var componentType *string
		if args.ComponentType != "" {
			ct := args.ComponentType
			componentType = &ct
		}

		componentReq := &gen.CreateComponentRequest{
			Name: args.Name,
		}

		if args.DisplayName != "" {
			componentReq.DisplayName = &args.DisplayName
		}
		if args.Description != "" {
			componentReq.Description = &args.Description
		}
		if componentType != nil {
			componentReq.ComponentType = componentType
		}

		// Set the component to auto deploy by default
		if args.AutoDeploy == nil {
			autoDeploy := true
			componentReq.AutoDeploy = &autoDeploy
		} else {
			componentReq.AutoDeploy = args.AutoDeploy
		}

		// Convert parameters if provided
		if args.Parameters != nil {
			componentReq.Parameters = &args.Parameters
		}

		if args.Workflow != nil {
			workflow, err := parseComponentWorkflowInput(args.Workflow)
			if err != nil {
				return nil, nil, err
			}
			componentReq.Workflow = workflow
		}

		result, err := t.ComponentToolset.CreateComponent(ctx, args.NamespaceName, args.ProjectName, componentReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListReleaseBindings(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_release_bindings"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List release bindings for a component. Release bindings associate releases with " +
			"environments and define deployment configurations. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}), []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListReleaseBindings(
			ctx, args.NamespaceName, args.ComponentName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new release binding to deploy a component release to a specific " +
			"environment. Fails if a binding already exists for the component in that environment, " +
			"use update_release_binding to deploy a new release to an environment that already has " +
			"one. To promote a component to a new environment, create(or update) the release binding " +
			"in the target environment with the desired component release.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"environment":    stringProperty("Target environment name"),
			"release_name":   stringProperty("Name of the component release to bind"),
			"component_type_environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: environment-specific overrides for component type parameters. " +
					"Use get_component_type_schema or get_cluster_component_type_schema to discover available parameters.",
			},
			"trait_environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: environment-specific trait configuration overrides. " +
					"Use get_trait_schema or get_cluster_trait_schema to discover available parameters.",
			},
			"workload_overrides": map[string]any{
				"type": "object",
				"description": "Optional: workload configuration overrides. " +
					"Use get_workload_schema to see the full structure.",
			},
		}, []string{"namespace_name", "project_name", "component_name", "environment", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName                   string                 `json:"namespace_name"`
		ProjectName                     string                 `json:"project_name"`
		ComponentName                   string                 `json:"component_name"`
		Environment                     string                 `json:"environment"`
		ReleaseName                     string                 `json:"release_name"`
		ComponentTypeEnvironmentConfigs map[string]interface{} `json:"component_type_environment_configs"`
		TraitEnvironmentConfigs         map[string]interface{} `json:"trait_environment_configs"`
		WorkloadOverrides               map[string]interface{} `json:"workload_overrides"`
	}) (*mcp.CallToolResult, any, error) {
		createReq := &gen.ReleaseBindingSpec{
			Environment: args.Environment,
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: args.ComponentName,
				ProjectName:   args.ProjectName,
			},
		}
		if args.ReleaseName != "" {
			createReq.ReleaseName = &args.ReleaseName
		}
		if args.ComponentTypeEnvironmentConfigs != nil {
			createReq.ComponentTypeEnvironmentConfigs = &args.ComponentTypeEnvironmentConfigs
		}
		if args.TraitEnvironmentConfigs != nil {
			traitEnvironmentConfigs := make(map[string]interface{}, len(args.TraitEnvironmentConfigs))
			for k, v := range args.TraitEnvironmentConfigs {
				traitEnvironmentConfigs[k] = v
			}
			createReq.TraitEnvironmentConfigs = &traitEnvironmentConfigs
		}
		if args.WorkloadOverrides != nil {
			workloadOverrides, err := parseWorkloadOverrides(args.WorkloadOverrides)
			if err != nil {
				return nil, nil, err
			}
			createReq.WorkloadOverrides = workloadOverrides
		}
		result, err := t.DeploymentToolset.CreateReleaseBinding(
			ctx, args.NamespaceName, createReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing release binding's configuration (partial update). Only provided fields are " +
			"updated; omitted fields remain unchanged. Use this to deploy a new component release to an " +
			"environment, modify environment configs and workload overrides, or change the binding's " +
			"release state (Active to deploy, Undeploy to remove from data plane).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"binding_name":   defaultStringProperty(),
			"release_name":   stringProperty("Optional: update the release associated with this binding"),
			"release_state": stringProperty("Optional: target state — 'Active' to deploy or 'Undeploy' " +
				"to remove the deployment from the data plane while keeping the binding"),
			"component_type_environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: environment-specific overrides for component type parameters. " +
					"Use get_component_type_schema or get_cluster_component_type_schema to discover available parameters.",
			},
			"trait_environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: environment-specific trait configuration overrides. " +
					"Use get_trait_schema or get_cluster_trait_schema to discover available parameters.",
			},
			"workload_overrides": map[string]any{
				"type": "object",
				"description": "Optional: workload configuration overrides. " +
					"Use get_workload_schema to see the full structure.",
			},
		}, []string{"namespace_name", "binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName                   string                 `json:"namespace_name"`
		BindingName                     string                 `json:"binding_name"`
		ReleaseName                     string                 `json:"release_name"`
		ReleaseState                    string                 `json:"release_state"`
		ComponentTypeEnvironmentConfigs map[string]interface{} `json:"component_type_environment_configs"`
		TraitEnvironmentConfigs         map[string]interface{} `json:"trait_environment_configs"`
		WorkloadOverrides               map[string]interface{} `json:"workload_overrides"`
	}) (*mcp.CallToolResult, any, error) {
		patchReq := &gen.ReleaseBindingSpec{}
		if args.ReleaseName != "" {
			patchReq.ReleaseName = &args.ReleaseName
		}
		if args.ReleaseState != "" {
			if args.ReleaseState != string(gen.ReleaseBindingSpecStateActive) &&
				args.ReleaseState != string(gen.ReleaseBindingSpecStateUndeploy) {
				return nil, nil, fmt.Errorf("release_state must be one of: Active, Undeploy")
			}
			state := gen.ReleaseBindingSpecState(args.ReleaseState)
			patchReq.State = &state
		}
		if args.ComponentTypeEnvironmentConfigs != nil {
			patchReq.ComponentTypeEnvironmentConfigs = &args.ComponentTypeEnvironmentConfigs
		}
		if args.TraitEnvironmentConfigs != nil {
			traitEnvironmentConfigs := make(map[string]interface{}, len(args.TraitEnvironmentConfigs))
			for k, v := range args.TraitEnvironmentConfigs {
				traitEnvironmentConfigs[k] = v
			}
			patchReq.TraitEnvironmentConfigs = &traitEnvironmentConfigs
		}
		if args.WorkloadOverrides != nil {
			workloadOverrides, err := parseWorkloadOverrides(args.WorkloadOverrides)
			if err != nil {
				return nil, nil, err
			}
			patchReq.WorkloadOverrides = workloadOverrides
		}
		result, err := t.DeploymentToolset.UpdateReleaseBinding(
			ctx, args.NamespaceName, args.BindingName, patchReq)
		return handleToolResult(result, err)
	})
}

func parseWorkloadOverrides(overrides map[string]interface{}) (*gen.WorkloadOverrides, error) {
	for k := range overrides {
		if k != "container" {
			return nil, fmt.Errorf("unknown field %q in workload_overrides, allowed fields: [container]", k)
		}
	}
	containerRaw, exists := overrides["container"]
	if !exists {
		return nil, nil
	}
	container, ok := containerRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("workload_overrides.container must be an object")
	}
	for k := range container {
		if k != "env" && k != "files" {
			return nil, fmt.Errorf("unknown field %q in workload_overrides.container, allowed fields: [env, files]", k)
		}
	}
	envVars, err := parseEnvVars(container)
	if err != nil {
		return nil, err
	}
	fileVars, err := parseFileVars(container)
	if err != nil {
		return nil, err
	}
	if len(envVars) == 0 && len(fileVars) == 0 {
		return nil, nil
	}
	containerOverride := &gen.ContainerOverride{}
	if len(envVars) > 0 {
		containerOverride.Env = &envVars
	}
	if len(fileVars) > 0 {
		containerOverride.Files = &fileVars
	}
	return &gen.WorkloadOverrides{Container: containerOverride}, nil
}

func parseEnvVars(container map[string]interface{}) ([]gen.EnvVar, error) {
	envRaw, exists := container["env"]
	if !exists {
		return nil, nil
	}
	envs, ok := envRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("workload_overrides.container.env must be an array")
	}
	envVars := make([]gen.EnvVar, 0, len(envs))
	for i, ev := range envs {
		evMap, ok := ev.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("workload_overrides.container.env[%d] must be an object", i)
		}
		for k := range evMap {
			switch k {
			case "key", "value":
			case "valueFrom":
				return nil, fmt.Errorf("workload_overrides.container.env[%d]: valueFrom is not supported via MCP", i)
			default:
				return nil, fmt.Errorf(
					"unknown field %q in workload_overrides.container.env[%d], allowed fields: [key, value]", k, i)
			}
		}
		key, _ := evMap["key"].(string)
		if key == "" {
			return nil, fmt.Errorf("workload_overrides.container.env[%d]: \"key\" is required and must be non-empty", i)
		}
		value, _ := evMap["value"].(string)
		envVars = append(envVars, gen.EnvVar{Key: key, Value: &value})
	}
	return envVars, nil
}

func parseFileVars(container map[string]interface{}) ([]gen.FileVar, error) {
	filesRaw, exists := container["files"]
	if !exists {
		return nil, nil
	}
	files, ok := filesRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("workload_overrides.container.files must be an array")
	}
	fileVars := make([]gen.FileVar, 0, len(files))
	for i, f := range files {
		fMap, ok := f.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("workload_overrides.container.files[%d] must be an object", i)
		}
		for k := range fMap {
			switch k {
			case "key", "mountPath", "value":
			case "valueFrom":
				return nil, fmt.Errorf("workload_overrides.container.files[%d]: valueFrom is not supported via MCP", i)
			default:
				return nil, fmt.Errorf(
					"unknown field %q in workload_overrides.container.files[%d],"+
						"allowed fields: [key, mountPath, value]", k, i)
			}
		}
		key, _ := fMap["key"].(string)
		if key == "" {
			return nil, fmt.Errorf("workload_overrides.container.files[%d]: \"key\" is required and must be non-empty", i)
		}
		mountPath, _ := fMap["mountPath"].(string)
		if mountPath == "" {
			return nil, fmt.Errorf("workload_overrides.container.files[%d]: \"mountPath\" is required and must be non-empty", i)
		}
		value, _ := fMap["value"].(string)
		fileVars = append(fileVars, gen.FileVar{Key: key, MountPath: mountPath, Value: &value})
	}
	return fileVars, nil
}

func (t *Toolsets) RegisterCreateWorkload(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_workload"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateWorkload}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new workload for a component. Workloads define the runtime specification " +
			"including container images, resource limits, and environment variables. " +
			"Use this for components that follow the from-image approach (i.e., they do not use workflows to " +
			"build images from source). For components that use workflows to build from source, the workload " +
			"is generated automatically; use update_workload to modify it if the source repository does not " +
			"contain a workload descriptor (workload.yaml). " +
			"Use get_workload_schema to see the full workload_spec structure before calling this tool.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"workload_spec": map[string]any{
				"type":        "object",
				"description": "Workload specification (containers, resources, env vars, etc.)",
			},
		}, []string{"namespace_name", "component_name", "workload_spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ComponentName string                 `json:"component_name"`
		WorkloadSpec  map[string]interface{} `json:"workload_spec"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.CreateWorkload(
			ctx, args.NamespaceName, args.ComponentName, args.WorkloadSpec)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateWorkload(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_workload"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateWorkload}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing workload's specification for a component. Use this for components " +
			"that use workflows to build images from source, when the source repository does not contain a " +
			"workload descriptor (workload.yaml) and you need to modify the workload generated from the build " +
			"workflow. Allows updating container configuration, environment variables, file mounts, port " +
			"mappings, resource limits, and other runtime settings. For components that follow the from-image " +
			"approach, use create_workload instead. " +
			"Use get_workload_schema to see the full workload_spec structure, and " +
			"get_workload to retrieve the current workload name and spec before updating.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workload_name":  stringProperty("Use list_workloads to discover valid names"),
			"workload_spec": map[string]any{
				"type":        "object",
				"description": "Updated workload specification (containers, resources, env vars, port mappings, file mounts, etc.)",
			},
		}, []string{"namespace_name", "workload_name", "workload_spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		WorkloadName  string                 `json:"workload_name"`
		WorkloadSpec  map[string]interface{} `json:"workload_spec"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.UpdateWorkload(
			ctx, args.NamespaceName, args.WorkloadName, args.WorkloadSpec)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkloadSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workload_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkload}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the JSON schema for the workload specification. Returns the full schema showing " +
			"all available fields (container, endpoints, connections), their types, required fields, and " +
			"valid values. Use this before calling create_workload or update_workload to understand the " +
			"expected workload_spec structure.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetWorkloadSchema(ctx)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponent}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a component. Returns the JSON schema showing component " +
			"configuration options, required fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentSchema(ctx, args.NamespaceName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterTriggerWorkflowRun(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "trigger_workflow_run"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Trigger a workflow run for a component using the component's configured workflow and " +
			"parameters. Optionally override commit SHA when the workflow supports a commit parameter mapping.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"commit":         stringProperty("Optional: Git commit SHA to use for the workflow run"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Commit        string `json:"commit"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.TriggerWorkflowRun(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.Commit)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPatchComponent(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "patch_component"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateComponent}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Patch (partially update) a component's configuration. Only the fields provided " +
			"in the request are updated; omitted fields remain unchanged. Supports updating " +
			"display_name, description, auto_deploy, parameters, traits, and workflow. " +
			"Pass an empty array for traits to clear all traits. Component owner and componentType " +
			"are immutable and cannot be patched.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"display_name": stringProperty(
				"Optional: Updated human-readable display name. Empty string is treated as no-change."),
			"description": stringProperty(
				"Optional: Updated human-readable description. Empty string is treated as no-change."),
			"auto_deploy": map[string]any{
				"type":        "boolean",
				"description": "Optional: Whether the component should automatically deploy to the default environment",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Component type parameters (port, replicas, exposed, etc.). Replaces existing parameters.",
			},
			"traits": map[string]any{
				"type": "array",
				"description": "Optional: Replace the entire traits list. Pass an empty array to clear all traits. " +
					"Each entry: 'name' (required), 'instanceName' (required, unique per component), " +
					"'kind' (optional, 'Trait' or 'ClusterTrait', default 'Trait'), 'parameters' (optional object). " +
					"Use list_cluster_traits or list_traits to discover trait names; " +
					"use get_cluster_trait_schema or get_trait_schema to inspect parameters.",
				"items": map[string]any{
					"type": "object",
				},
			},
			"workflow": map[string]any{
				"type": "object",
				"description": "Optional: Replace the workflow configuration. Set 'name' (required), " +
					"'kind' (optional, 'Workflow' or 'ClusterWorkflow', default 'ClusterWorkflow'), " +
					"and 'parameters' (optional object) that strictly adhere to the workflow schema. " +
					"Use list_cluster_workflows or list_workflows to discover names; " +
					"use get_cluster_workflow_schema or get_workflow_schema to inspect parameters.",
			},
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                    `json:"namespace_name"`
		ComponentName string                    `json:"component_name"`
		DisplayName   *string                   `json:"display_name,omitempty"`
		Description   *string                   `json:"description,omitempty"`
		AutoDeploy    *bool                     `json:"auto_deploy,omitempty"`
		Parameters    map[string]interface{}    `json:"parameters,omitempty"`
		Traits        *[]map[string]interface{} `json:"traits,omitempty"`
		Workflow      map[string]interface{}    `json:"workflow,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		patchReq := &gen.PatchComponentRequest{
			AutoDeploy:  args.AutoDeploy,
			DisplayName: args.DisplayName,
			Description: args.Description,
		}
		if args.Parameters != nil {
			patchReq.Parameters = &args.Parameters
		}
		if args.Traits != nil {
			traits := make([]gen.ComponentTraitInput, 0, len(*args.Traits))
			for i, raw := range *args.Traits {
				ti, err := parseComponentTraitInput(raw)
				if err != nil {
					return nil, nil, fmt.Errorf("traits[%d]: %w", i, err)
				}
				traits = append(traits, ti)
			}
			patchReq.Traits = &traits
		}
		if args.Workflow != nil {
			wf, err := parseComponentWorkflowInput(args.Workflow)
			if err != nil {
				return nil, nil, err
			}
			patchReq.Workflow = wf
		}
		result, err := t.ComponentToolset.PatchComponent(
			ctx, args.NamespaceName, args.ComponentName, patchReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteComponent(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_component"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteComponent}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a component. Destructive: removes the Component resource and triggers cleanup of " +
			"its workload, releases, and release bindings via owner references.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.DeleteComponent(ctx, args.NamespaceName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteWorkload(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_workload"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteWorkload}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a workload. Destructive: removes the Workload resource. " +
			"Use update_release_binding with release_state: Undeploy first if you want to remove the running " +
			"deployment from the data plane while keeping the workload definition.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workload_name":  stringProperty("Use list_workloads to discover valid names"),
		}, []string{"namespace_name", "workload_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkloadName  string `json:"workload_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.DeleteWorkload(ctx, args.NamespaceName, args.WorkloadName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a release binding. Destructive: removes the binding record entirely. For a " +
			"reversible removal that keeps the binding but tears down data-plane resources, use " +
			"update_release_binding with release_state: Undeploy instead.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"binding_name":   stringProperty("Use list_release_bindings to discover valid names"),
		}, []string{"namespace_name", "binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		BindingName   string `json:"binding_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteReleaseBinding(ctx, args.NamespaceName, args.BindingName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteComponentRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_component_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteComponentRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a component release. Destructive: removes the immutable release record. " +
			"Useful for pruning old releases. Will not affect running deployments unless a binding still references it.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_name":   stringProperty("Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteComponentRelease(ctx, args.NamespaceName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

// parseComponentTraitInput converts an untyped MCP arg map into a gen.ComponentTraitInput,
// validating required fields (name, instanceName) and the optional kind enum.
func parseComponentTraitInput(raw map[string]interface{}) (gen.ComponentTraitInput, error) {
	var ti gen.ComponentTraitInput
	name, _ := raw["name"].(string)
	if name == "" {
		return ti, fmt.Errorf("trait 'name' is required and must be a non-empty string")
	}
	instanceName, _ := raw["instanceName"].(string)
	if instanceName == "" {
		return ti, fmt.Errorf("trait 'instanceName' is required and must be a non-empty string")
	}
	ti.Name = name
	ti.InstanceName = instanceName
	if kind, ok := raw["kind"].(string); ok && kind != "" {
		if kind != string(gen.ComponentTraitInputKindTrait) &&
			kind != string(gen.ComponentTraitInputKindClusterTrait) {
			return ti, fmt.Errorf("invalid trait 'kind' %q: must be one of [Trait, ClusterTrait]", kind)
		}
		k := gen.ComponentTraitInputKind(kind)
		ti.Kind = &k
	}
	if rawParams, exists := raw["parameters"]; exists && rawParams != nil {
		params, ok := rawParams.(map[string]interface{})
		if !ok {
			return ti, fmt.Errorf("trait 'parameters' must be an object")
		}
		ti.Parameters = &params
	}
	return ti, nil
}

// parseComponentWorkflowInput converts an untyped MCP arg map into a *gen.ComponentWorkflowInput,
// validating the required name field and the optional kind enum.
func parseComponentWorkflowInput(raw map[string]interface{}) (*gen.ComponentWorkflowInput, error) {
	wf := &gen.ComponentWorkflowInput{}
	name, _ := raw["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("workflow 'name' is required and must be a non-empty string")
	}
	wf.Name = name
	if kind, ok := raw["kind"].(string); ok && kind != "" {
		if kind != string(gen.ComponentWorkflowInputKindClusterWorkflow) &&
			kind != string(gen.ComponentWorkflowInputKindWorkflow) {
			return nil, fmt.Errorf("invalid workflow 'kind' %q: must be one of [ClusterWorkflow, Workflow]", kind)
		}
		k := gen.ComponentWorkflowInputKind(kind)
		wf.Kind = &k
	}
	if rawParams, exists := raw["parameters"]; exists && rawParams != nil {
		params, ok := rawParams.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("workflow 'parameters' must be an object")
		}
		wf.Parameters = &params
	}
	return wf, nil
}
