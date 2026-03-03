// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func (t *Toolsets) RegisterListComponents(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_components",
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

func (t *Toolsets) RegisterGetComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component",
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

func (t *Toolsets) RegisterListWorkloads(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_workloads",
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

func (t *Toolsets) RegisterGetWorkload(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workload",
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

func (t *Toolsets) RegisterGetReleaseBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_release_binding",
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
		result, err := t.ComponentToolset.GetReleaseBinding(ctx, args.NamespaceName, args.BindingName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_component",
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
			"componentType": stringProperty("Component type identifier in {workloadType}/{componentTypeName} format. " +
				"Use list_component_types or list_cluster_component_types to discover valid types"),
			"componentTypeKind": stringProperty("Optional: Kind of component type reference. " +
				"Use 'ComponentType' for namespace-scoped (default) or 'ClusterComponentType' for cluster-scoped"),
			"autoDeploy": map[string]any{
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
				"description": "Optional: Component workflow configuration. Use list_workflows to discover available " +
					"workflow names, and get_workflow_schema to inspect the parameter schema a workflow accepts.",
			},
		}, []string{"namespace_name", "project_name", "name", "componentType"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName     string                 `json:"namespace_name"`
		ProjectName       string                 `json:"project_name"`
		Name              string                 `json:"name"`
		DisplayName       string                 `json:"display_name"`
		Description       string                 `json:"description"`
		ComponentType     string                 `json:"componentType"`
		ComponentTypeKind string                 `json:"componentTypeKind"`
		AutoDeploy        *bool                  `json:"autoDeploy,omitempty"`
		Parameters        map[string]interface{} `json:"parameters"`
		Workflow          map[string]interface{} `json:"workflow"`
	}) (*mcp.CallToolResult, any, error) {
		var componentType *string
		if args.ComponentType != "" {
			// ComponentType in gen is a string in format: {workloadType}/{componentTypeName}
			// If ComponentTypeKind is provided, we construct it, otherwise use ComponentType as-is
			ct := args.ComponentType
			if args.ComponentTypeKind != "" {
				// If kind is provided, assume ComponentType is just the name
				ct = args.ComponentType
			}
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

		// Convert workflow if provided
		if args.Workflow != nil {
			workflow := &gen.ComponentWorkflowInput{}
			if name, ok := args.Workflow["name"].(string); ok {
				workflow.Name = name
			}

			// Convert parameters if provided
			if params, ok := args.Workflow["parameters"].(map[string]interface{}); ok {
				workflow.Parameters = &params
			}

			componentReq.Workflow = workflow
		}

		result, err := t.ComponentToolset.CreateComponent(ctx, args.NamespaceName, args.ProjectName, componentReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentReleases(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_releases",
		Description: "List all releases for a component. Releases are immutable snapshots of a component at a " +
			"specific build, ready for deployment to environments. Supports pagination via limit and cursor.",
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
		result, err := t.ComponentToolset.ListComponentReleases(
			ctx, args.NamespaceName, args.ComponentName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponentRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_component_release",
		Description: "Create a new release from the latest build of a component. Releases are immutable " +
			"snapshots that can be deployed to environments. The component must have at least one successful build. " +
			"If the source repository does not contain a workload descriptor (workload.yaml), use update_workload " +
			"to configure the workload before creating a release.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Optional release name. If omitted, a name will be auto-generated"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.CreateComponentRelease(
			ctx, args.NamespaceName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_release",
		Description: "Get detailed information about a specific component release including build information, " +
			"image tags, and deployment status.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_name":   stringProperty("Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentRelease(ctx, args.NamespaceName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListReleaseBindings(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_release_bindings",
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
		result, err := t.ComponentToolset.ListReleaseBindings(
			ctx, args.NamespaceName, args.ComponentName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPatchReleaseBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "patch_release_binding",
		Description: "Patch (update) a release binding's configuration. Can update the associated release, environment " +
			"overrides, trait configurations, and workload settings. " +
			"WARNING: Override fields are destructive — they fully replace the existing values, not merge. ",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"binding_name":   defaultStringProperty(),
			"release_name":   stringProperty("Optional: update the release associated with this binding"),
			"environment":    stringProperty("Optional: update the target environment"),
			"component_type_env_overrides": map[string]any{
				"type":        "object",
				"description": "Optional: environment-specific overrides for component type parameters",
			},
			"trait_overrides": map[string]any{
				"type":        "object",
				"description": "Optional: environment-specific trait configuration overrides",
			},
			"workload_overrides": map[string]any{
				"type":        "object",
				"description": "Optional: workload configuration overrides",
			},
		}, []string{"namespace_name", "binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName             string                 `json:"namespace_name"`
		BindingName               string                 `json:"binding_name"`
		ReleaseName               string                 `json:"release_name"`
		Environment               string                 `json:"environment"`
		ComponentTypeEnvOverrides map[string]interface{} `json:"component_type_env_overrides"`
		TraitOverrides            map[string]interface{} `json:"trait_overrides"`
		WorkloadOverrides         map[string]interface{} `json:"workload_overrides"`
	}) (*mcp.CallToolResult, any, error) {
		patchReq := &gen.ReleaseBindingSpec{}
		if args.Environment != "" {
			patchReq.Environment = args.Environment
		}
		if args.ReleaseName != "" {
			patchReq.ReleaseName = &args.ReleaseName
		}
		if args.ComponentTypeEnvOverrides != nil {
			patchReq.ComponentTypeEnvOverrides = &args.ComponentTypeEnvOverrides
		}
		if args.TraitOverrides != nil {
			traitOverrides := make(map[string]interface{}, len(args.TraitOverrides))
			for k, v := range args.TraitOverrides {
				traitOverrides[k] = v
			}
			patchReq.TraitOverrides = &traitOverrides
		}
		if args.WorkloadOverrides != nil {
			workloadOverrides, err := parseWorkloadOverrides(args.WorkloadOverrides)
			if err != nil {
				return nil, nil, err
			}
			patchReq.WorkloadOverrides = workloadOverrides
		}
		result, err := t.ComponentToolset.PatchReleaseBinding(
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

func (t *Toolsets) RegisterDeployRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "deploy_release",
		Description: "Deploy a component release to the lowest environment in the deployment pipeline. " +
			"This creates or updates a release binding in the first environment of the pipeline. " +
			"If the source repository does not contain a workload descriptor (workload.yaml), use update_workload " +
			"to configure the workload before deploying.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("The release to deploy. Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		deployReq := &gen.DeployReleaseRequest{
			ReleaseName: args.ReleaseName,
		}
		result, err := t.ComponentToolset.DeployRelease(
			ctx, args.NamespaceName, args.ComponentName, deployReq,
		)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPromoteComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "promote_component",
		Description: "Promote a component release from one environment to another following the deployment " +
			"pipeline. Validates that the promotion path exists in the pipeline configuration.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"source_env":     stringProperty("Source environment name (e.g., 'dev')"),
			"target_env":     stringProperty("Target environment name (e.g., 'staging')"),
		}, []string{"namespace_name", "component_name", "source_env", "target_env"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		SourceEnv     string `json:"source_env"`
		TargetEnv     string `json:"target_env"`
	}) (*mcp.CallToolResult, any, error) {
		promoteReq := &gen.PromoteComponentRequest{
			SourceEnv: args.SourceEnv,
			TargetEnv: args.TargetEnv,
		}
		result, err := t.ComponentToolset.PromoteComponent(
			ctx, args.NamespaceName, args.ComponentName, promoteReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateWorkload(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_workload",
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

func (t *Toolsets) RegisterUpdateWorkload(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_workload",
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
			"workload_name":  stringProperty("Use get_component_workloads to discover valid names"),
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

func (t *Toolsets) RegisterGetWorkloadSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workload_schema",
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

func (t *Toolsets) RegisterGetComponentSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_schema",
		Description: "Get the schema definition for a component. Returns the JSON schema showing component " +
			"configuration options, required fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Component name. Use list_components to discover valid names"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentSchema(ctx, args.NamespaceName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetEnvironmentRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_environment_release",
		Description: "Get the Release spec and status for a component deployed in a specific environment. " +
			"Returns the complete Release resource including all Kubernetes manifests and deployment status.",
		InputSchema: createSchema(map[string]any{
			"namespace_name":   defaultStringProperty(),
			"component_name":   stringProperty("Use list_components to discover valid names"),
			"environment_name": stringProperty("Use list_environments to discover valid names"),
		}, []string{"namespace_name", "component_name", "environment_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName   string `json:"namespace_name"`
		ComponentName   string `json:"component_name"`
		EnvironmentName string `json:"environment_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetEnvironmentRelease(
			ctx, args.NamespaceName, args.ComponentName, args.EnvironmentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateReleaseBindingState(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_release_binding_state",
		Description: "Update the state of a release binding. Use this to activate, suspend, or undeploy a " +
			"component in a specific environment. Valid states: Active, Undeploy.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"binding_name":   stringProperty("Use list_release_bindings to discover valid names"),
			"release_state":  stringProperty("Target state: 'Active' or 'Undeploy'"),
		}, []string{"namespace_name", "binding_name", "release_state"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		BindingName   string `json:"binding_name"`
		ReleaseState  string `json:"release_state"`
	}) (*mcp.CallToolResult, any, error) {
		// Validate releaseState
		validStates := map[string]bool{
			string(gen.ReleaseBindingSpecStateActive):   true,
			string(gen.ReleaseBindingSpecStateUndeploy): true,
		}
		if !validStates[args.ReleaseState] {
			return nil, nil, fmt.Errorf("releaseState must be one of: Active, Undeploy")
		}
		state := gen.ReleaseBindingSpecState(args.ReleaseState)
		result, err := t.ComponentToolset.UpdateReleaseBindingState(
			ctx, args.NamespaceName, args.BindingName, &state)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentReleaseSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_release_schema",
		Description: "Get the release schema for a component. Returns the JSON schema showing the configuration " +
			"options available when creating or deploying releases for this component.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"release_name":   stringProperty("Optional: specific release name for release-specific schema"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentReleaseSchema(
			ctx, args.NamespaceName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterTriggerWorkflowRun(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "trigger_workflow_run",
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
		result, err := t.ComponentToolset.TriggerWorkflowRun(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.Commit)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPatchComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "patch_component",
		Description: "Patch (partially update) a component's configuration. Only the fields provided in the request " +
			"will be updated; omitted fields remain unchanged. Supports updating autoDeploy and parameters.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"auto_deploy": map[string]any{
				"type":        "boolean",
				"description": "Optional: Whether the component should automatically deploy to the default environment",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Component type parameters (port, replicas, exposed, etc.)",
			},
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ComponentName string                 `json:"component_name"`
		AutoDeploy    *bool                  `json:"auto_deploy,omitempty"`
		Parameters    map[string]interface{} `json:"parameters,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		patchReq := &gen.PatchComponentRequest{
			AutoDeploy: args.AutoDeploy,
		}
		if args.Parameters != nil {
			patchReq.Parameters = &args.Parameters
		}
		result, err := t.ComponentToolset.PatchComponent(
			ctx, args.NamespaceName, args.ComponentName, patchReq)
		return handleToolResult(result, err)
	})
}
