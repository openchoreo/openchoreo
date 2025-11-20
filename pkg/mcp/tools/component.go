// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (t *Toolsets) RegisterListComponents(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_components",
		Description: "List all components in a project. Components are deployable units (services, jobs, etc.) " +
			"with independent build and deployment lifecycles.",
		InputSchema: createSchema(map[string]any{
			"org_name":     defaultStringProperty(),
			"project_name": defaultStringProperty(),
		}, []string{"org_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponents(ctx, args.OrgName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component",
		Description: "Get detailed information about a component including configuration, deployment status, " +
			"and builds. Use additional_resources to include 'bindings', 'workloads', 'builds', or 'endpoints'.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"additional_resources": arrayProperty(
				"Additional data to include: 'bindings', 'workloads', 'builds', 'endpoints'", "string"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName             string   `json:"org_name"`
		ProjectName         string   `json:"project_name"`
		ComponentName       string   `json:"component_name"`
		AdditionalResources []string `json:"additional_resources"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponent(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.AdditionalResources,
		)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterComponentBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_binding",
		Description: "Get environment-specific configuration for a component. Bindings define how a component " +
			"behaves in a particular environment (replicas, env vars, resource limits, etc.).",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"environment": stringProperty(
				"E.g., 'dev', 'staging', 'production'. Use list_environments to discover"),
		}, []string{"org_name", "project_name", "component_name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Environment   string `json:"environment"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentBinding(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Environment,
		)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentWorkloads(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_workloads",
		Description: "Get real-time workload information for a component across all environments. Shows " +
			"running pods, their status, resource usage, and container details. For Kubernetes users: Similar " +
			"to 'kubectl get pods'.",
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
		result, err := t.ComponentToolset.GetComponentWorkloads(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_component",
		Description: "Create a new component in a project. Components are deployable units (services, jobs, etc.) " +
			"with independent build and deployment lifecycles. Component names must be DNS-compatible.",
		InputSchema: createSchema(map[string]any{
			"org_name":     defaultStringProperty(),
			"project_name": defaultStringProperty(),
			"name":         stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"type":         stringProperty("Component type identifier. Use list_component_types to discover valid types"),
		}, []string{"org_name", "project_name", "name", "type"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}) (*mcp.CallToolResult, any, error) {
		componentReq := &models.CreateComponentRequest{
			Name:        args.Name,
			DisplayName: args.DisplayName,
			Description: args.Description,
			Type:        args.Type,
		}
		result, err := t.ComponentToolset.CreateComponent(ctx, args.OrgName, args.ProjectName, componentReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentReleases(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_releases",
		Description: "List all releases for a component. Releases are immutable snapshots of a component at a " +
			"specific build, ready for deployment to environments.",
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
		result, err := t.ComponentToolset.ListComponentReleases(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponentRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_component_release",
		Description: "Create a new release from the latest build of a component. Releases are immutable " +
			"snapshots that can be deployed to environments. The component must have at least one successful build.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Optional release name. If omitted, a name will be auto-generated"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.CreateComponentRelease(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_release",
		Description: "Get detailed information about a specific component release including build information, " +
			"image tags, and deployment status.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Use list_component_releases to discover valid names"),
		}, []string{"org_name", "project_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentRelease(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListReleaseBindings(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_release_bindings",
		Description: "List release bindings for a component. Release bindings associate releases with " +
			"environments and define deployment configurations. Optionally filter by environment names.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"environments": arrayProperty(
				"Optional: filter by environment names (e.g., ['dev', 'staging'])", "string"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string   `json:"org_name"`
		ProjectName   string   `json:"project_name"`
		ComponentName string   `json:"component_name"`
		Environments  []string `json:"environments"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListReleaseBindings(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Environments)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPatchReleaseBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "patch_release_binding",
		Description: "Patch (update) a release binding's configuration. Can update the associated release, environment " +
			"overrides, trait configurations, and workload settings.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
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
			"configuration_overrides": map[string]any{
				"type":        "object",
				"description": "Optional: workload configuration overrides (env vars, files, etc.)",
			},
		}, []string{"org_name", "project_name", "component_name", "binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName                   string                 `json:"org_name"`
		ProjectName               string                 `json:"project_name"`
		ComponentName             string                 `json:"component_name"`
		BindingName               string                 `json:"binding_name"`
		ReleaseName               string                 `json:"release_name"`
		Environment               string                 `json:"environment"`
		ComponentTypeEnvOverrides map[string]interface{} `json:"component_type_env_overrides"`
		TraitOverrides            map[string]interface{} `json:"trait_overrides"`
		ConfigurationOverrides    map[string]interface{} `json:"configuration_overrides"`
	}) (*mcp.CallToolResult, any, error) {
		// Convert trait overrides to the correct type
		var traitOverrides map[string]map[string]interface{}
		if args.TraitOverrides != nil {
			traitOverrides = make(map[string]map[string]interface{})
			for k, v := range args.TraitOverrides {
				if vMap, ok := v.(map[string]interface{}); ok {
					traitOverrides[k] = vMap
				}
			}
		}

		patchReq := &models.PatchReleaseBindingRequest{
			ReleaseName:               args.ReleaseName,
			Environment:               args.Environment,
			ComponentTypeEnvOverrides: args.ComponentTypeEnvOverrides,
			TraitOverrides:            traitOverrides,
		}
		if args.ConfigurationOverrides != nil {
			// Convert map to WorkloadOverrides struct
			workloadOverrides := &models.WorkloadOverrides{
				Containers: make(map[string]models.ContainerOverride),
			}
			// Default container name if not specified
			containerName := "default"
			if name, ok := args.ConfigurationOverrides["container_name"].(string); ok && name != "" {
				containerName = name
			}
			containerOverride := models.ContainerOverride{}
			if envVars, ok := args.ConfigurationOverrides["env"].([]interface{}); ok {
				for _, ev := range envVars {
					if evMap, ok := ev.(map[string]interface{}); ok {
						containerOverride.Env = append(containerOverride.Env, models.EnvVar{
							Key:   evMap["key"].(string),
							Value: evMap["value"].(string),
						})
					}
				}
			}
			if files, ok := args.ConfigurationOverrides["files"].([]interface{}); ok {
				for _, f := range files {
					if fMap, ok := f.(map[string]interface{}); ok {
						containerOverride.Files = append(containerOverride.Files, models.FileVar{
							Key:       fMap["key"].(string),
							MountPath: fMap["mount_path"].(string),
							Value:     fMap["value"].(string),
						})
					}
				}
			}
			if len(containerOverride.Env) > 0 || len(containerOverride.Files) > 0 {
				workloadOverrides.Containers[containerName] = containerOverride
				patchReq.WorkloadOverrides = workloadOverrides
			}
		}
		result, err := t.ComponentToolset.PatchReleaseBinding(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.BindingName, patchReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeployRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "deploy_release",
		Description: "Deploy a component release to the lowest environment in the deployment pipeline. " +
			"This creates or updates a release binding in the first environment of the pipeline.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("The release to deploy. Use list_component_releases to discover valid names"),
		}, []string{"org_name", "project_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		deployReq := &models.DeployReleaseRequest{
			ReleaseName: args.ReleaseName,
		}
		result, err := t.ComponentToolset.DeployRelease(ctx, args.OrgName, args.ProjectName, args.ComponentName, deployReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPromoteComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "promote_component",
		Description: "Promote a component release from one environment to another following the deployment " +
			"pipeline. Validates that the promotion path exists in the pipeline configuration.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"source_env":     stringProperty("Source environment name (e.g., 'dev')"),
			"target_env":     stringProperty("Target environment name (e.g., 'staging')"),
		}, []string{"org_name", "project_name", "component_name", "source_env", "target_env"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		SourceEnv     string `json:"source_env"`
		TargetEnv     string `json:"target_env"`
	}) (*mcp.CallToolResult, any, error) {
		promoteReq := &models.PromoteComponentRequest{
			SourceEnvironment: args.SourceEnv,
			TargetEnvironment: args.TargetEnv,
		}
		result, err := t.ComponentToolset.PromoteComponent(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, promoteReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateWorkload(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_workload",
		Description: "Create or update a workload for a component. Workloads define the runtime specification " +
			"including container images, resource limits, and environment variables.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"workload_spec": map[string]any{
				"type":        "object",
				"description": "Workload specification (containers, resources, env vars, etc.)",
			},
		}, []string{"org_name", "project_name", "component_name", "workload_spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string                 `json:"org_name"`
		ProjectName   string                 `json:"project_name"`
		ComponentName string                 `json:"component_name"`
		WorkloadSpec  map[string]interface{} `json:"workload_spec"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.CreateWorkload(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.WorkloadSpec)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateComponentBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_component_binding",
		Description: "Update a component binding's release state. Component bindings define how a component " +
			"behaves in a particular environment. Valid releaseState values: 'Active', 'Suspend', 'Undeploy'.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"binding_name":   defaultStringProperty(),
			"release_state":  stringProperty("Release state: 'Active', 'Suspend', or 'Undeploy'"),
		}, []string{"org_name", "project_name", "component_name", "binding_name", "release_state"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		BindingName   string `json:"binding_name"`
		ReleaseState  string `json:"release_state"`
	}) (*mcp.CallToolResult, any, error) {
		updateReq := &models.UpdateBindingRequest{
			ReleaseState: models.BindingReleaseState(args.ReleaseState),
		}
		result, err := t.ComponentToolset.UpdateComponentBinding(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.BindingName, updateReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_schema",
		Description: "Get the schema definition for a component. Returns the JSON schema showing component " +
			"configuration options, required fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Component name. Use list_components to discover valid names"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentSchema(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentReleaseSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_release_schema",
		Description: "Get the schema definition for a component release. Returns the JSON schema showing release " +
			"configuration options, required fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Release name. Use list_component_releases to discover valid names"),
		}, []string{"org_name", "project_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentReleaseSchema(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}
