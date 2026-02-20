// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (t *Toolsets) RegisterListComponents(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_components",
		Description: "List all components in a project. Components are deployable units (services, jobs, etc.) " +
			"with independent build and deployment lifecycles.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
		}, []string{"namespace_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponents(ctx, args.NamespaceName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component",
		Description: "Get detailed information about a component including configuration, deployment status, " +
			"and builds. Use additional_resources to include 'bindings', 'workloads', 'builds', or 'endpoints'.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"additional_resources": arrayProperty(
				"Additional data to include: 'bindings', 'workloads', 'builds', 'endpoints'", "string"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName       string   `json:"namespace_name"`
		ProjectName         string   `json:"project_name"`
		ComponentName       string   `json:"component_name"`
		AdditionalResources []string `json:"additional_resources"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponent(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.AdditionalResources,
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
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentWorkloads(ctx, args.NamespaceName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_component",
		Description: "Create a new component in a project. Components are deployable units (services, jobs, etc.) " +
			"with independent build and deployment lifecycles. ",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"componentType": stringProperty("Component type identifier in {workloadType}/{componentTypeName} format." +
				"Use list_component_types to discover valid types"),
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
				"type":        "object",
				"description": "Optional: Component workflow configuration with name, systemParameters, and parameters",
			},
		}, []string{"namespace_name", "project_name", "name", "componentType"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ProjectName   string                 `json:"project_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		ComponentType string                 `json:"componentType"`
		AutoDeploy    *bool                  `json:"autoDeploy,omitempty"`
		Parameters    map[string]interface{} `json:"parameters"`
		Workflow      map[string]interface{} `json:"workflow"`
	}) (*mcp.CallToolResult, any, error) {
		var componentTypeRef *models.ComponentTypeRef
		if args.ComponentType != "" {
			componentTypeRef = &models.ComponentTypeRef{
				Kind: "ComponentType",
				Name: args.ComponentType,
			}
		}

		componentReq := &models.CreateComponentRequest{
			Name:          args.Name,
			DisplayName:   args.DisplayName,
			Description:   args.Description,
			ComponentType: componentTypeRef,
		}

		// Set the component to auto deploy by default
		if args.AutoDeploy == nil {
			autoDeploy := true
			componentReq.AutoDeploy = &autoDeploy
		}

		// Convert parameters if provided
		if args.Parameters != nil {
			rawParams, err := json.Marshal(args.Parameters)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: "Failed to marshal parameters: " + err.Error()},
					},
					IsError: true,
				}, nil, nil
			}
			componentReq.Parameters = &runtime.RawExtension{Raw: rawParams}
		}

		// Convert workflow if provided
		if args.Workflow != nil {
			workflow := &models.ComponentWorkflow{}
			if name, ok := args.Workflow["name"].(string); ok {
				workflow.Name = name
			}

			// Convert systemParameters if provided
			if systemParams, ok := args.Workflow["systemParameters"].(map[string]interface{}); ok {
				systemParamsModel := &models.ComponentWorkflowSystemParams{}
				if repo, ok := systemParams["repository"].(map[string]interface{}); ok {
					repoParams := models.ComponentWorkflowRepository{}
					if url, ok := repo["url"].(string); ok {
						repoParams.URL = url
					}
					if appPath, ok := repo["appPath"].(string); ok {
						repoParams.AppPath = appPath
					}
					if revision, ok := repo["revision"].(map[string]interface{}); ok {
						revParams := models.ComponentWorkflowRepositoryRevision{}
						if branch, ok := revision["branch"].(string); ok {
							revParams.Branch = branch
						}
						if commit, ok := revision["commit"].(string); ok {
							revParams.Commit = commit
						}
						repoParams.Revision = revParams
					}
					systemParamsModel.Repository = repoParams
				}
				workflow.SystemParameters = systemParamsModel
			}

			// Convert parameters if provided
			if params, ok := args.Workflow["parameters"].(map[string]interface{}); ok {
				rawParams, err := json.Marshal(params)
				if err != nil {
					return &mcp.CallToolResult{
						Content: []mcp.Content{
							&mcp.TextContent{Text: "Failed to marshal workflow parameters: " + err.Error()},
						},
						IsError: true,
					}, nil, nil
				}
				workflow.Parameters = &runtime.RawExtension{Raw: rawParams}
			}

			componentReq.ComponentWorkflow = workflow
		}

		result, err := t.ComponentToolset.CreateComponent(ctx, args.NamespaceName, args.ProjectName, componentReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentReleases(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_releases",
		Description: "List all releases for a component. Releases are immutable snapshots of a component at a " +
			"specific build, ready for deployment to environments.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponentReleases(ctx, args.NamespaceName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateComponentRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_component_release",
		Description: "Create a new release from the latest build of a component. Releases are immutable " +
			"snapshots that can be deployed to environments. The component must have at least one successful build.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Optional release name. If omitted, a name will be auto-generated"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.CreateComponentRelease(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.ReleaseName)
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
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentRelease(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListReleaseBindings(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_release_bindings",
		Description: "List release bindings for a component. Release bindings associate releases with " +
			"environments and define deployment configurations. Optionally filter by environment names.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"environments": arrayProperty(
				"Optional: filter by environment names (e.g., ['dev', 'staging'])", "string"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string   `json:"namespace_name"`
		ProjectName   string   `json:"project_name"`
		ComponentName string   `json:"component_name"`
		Environments  []string `json:"environments"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListReleaseBindings(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.Environments)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPatchReleaseBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "patch_release_binding",
		Description: "Patch (update) a release binding's configuration. Can update the associated release, environment " +
			"overrides, trait configurations, and workload settings.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
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
		}, []string{"namespace_name", "project_name", "component_name", "binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName             string                 `json:"namespace_name"`
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
				patchReq.WorkloadOverrides = &models.WorkloadOverrides{
					Container: &containerOverride,
				}
			}
		}
		result, err := t.ComponentToolset.PatchReleaseBinding(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.BindingName, patchReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeployRelease(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "deploy_release",
		Description: "Deploy a component release to the lowest environment in the deployment pipeline. " +
			"This creates or updates a release binding in the first environment of the pipeline.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("The release to deploy. Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		deployReq := &models.DeployReleaseRequest{
			ReleaseName: args.ReleaseName,
		}
		result, err := t.ComponentToolset.DeployRelease(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, deployReq,
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
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"source_env":     stringProperty("Source environment name (e.g., 'dev')"),
			"target_env":     stringProperty("Target environment name (e.g., 'staging')"),
		}, []string{"namespace_name", "project_name", "component_name", "source_env", "target_env"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
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
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, promoteReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateWorkload(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_workload",
		Description: "Create or update a workload for a component. Workloads define the runtime specification " +
			"including container images, resource limits, and environment variables.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"workload_spec": map[string]any{
				"type":        "object",
				"description": "Workload specification (containers, resources, env vars, etc.)",
			},
		}, []string{"namespace_name", "project_name", "component_name", "workload_spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ProjectName   string                 `json:"project_name"`
		ComponentName string                 `json:"component_name"`
		WorkloadSpec  map[string]interface{} `json:"workload_spec"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.CreateWorkload(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.WorkloadSpec)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateComponentBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_component_binding",
		Description: "Update a component binding's release state. Component bindings define how a component " +
			"behaves in a particular environment. Valid releaseState values: 'Active', 'Suspend', 'Undeploy'.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"binding_name":   defaultStringProperty(),
			"release_state":  stringProperty("Release state: 'Active', 'Suspend', or 'Undeploy'"),
		}, []string{"namespace_name", "project_name", "component_name", "binding_name", "release_state"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		BindingName   string `json:"binding_name"`
		ReleaseState  string `json:"release_state"`
	}) (*mcp.CallToolResult, any, error) {
		updateReq := &models.UpdateBindingRequest{
			ReleaseState: models.BindingReleaseState(args.ReleaseState),
		}
		result, err := t.ComponentToolset.UpdateComponentBinding(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.BindingName, updateReq)
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
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Component name. Use list_components to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentSchema(ctx, args.NamespaceName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentReleaseSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_release_schema",
		Description: "Get the schema definition for a component release. Returns the JSON schema showing release " +
			"configuration options, required fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Release name. Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentReleaseSchema(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_traits",
		Description: "List all trait instances attached to a component. Traits add capabilities to components " +
			"(e.g., autoscaling, ingress, service mesh). Returns the trait name, instance name, and parameter values.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponentTraits(ctx, args.NamespaceName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateComponentTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_component_traits",
		Description: "Update (replace) all trait instances on a component. This operation replaces the entire set of " +
			"traits, so include all desired traits in the request. Each trait needs a name (trait type), instanceName " +
			"(unique identifier), and optional parameters.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"traits": arrayProperty(
				"Array of trait configurations. Each trait must have 'name', 'instanceName', and optional 'parameters'",
				"object",
			),
		}, []string{"namespace_name", "project_name", "component_name", "traits"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                         `json:"namespace_name"`
		ProjectName   string                         `json:"project_name"`
		ComponentName string                         `json:"component_name"`
		Traits        []models.ComponentTraitRequest `json:"traits"`
	}) (*mcp.CallToolResult, any, error) {
		updateReq := &models.UpdateComponentTraitsRequest{
			Traits: args.Traits,
		}
		result, err := t.ComponentToolset.UpdateComponentTraits(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, updateReq)
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
			"project_name":     defaultStringProperty(),
			"component_name":   stringProperty("Use list_components to discover valid names"),
			"environment_name": stringProperty("Use list_environments to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name", "environment_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName   string `json:"namespace_name"`
		ProjectName     string `json:"project_name"`
		ComponentName   string `json:"component_name"`
		EnvironmentName string `json:"environment_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetEnvironmentRelease(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.EnvironmentName)
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
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"auto_deploy": map[string]any{
				"type":        "boolean",
				"description": "Optional: Whether the component should automatically deploy to the default environment",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Component type parameters (port, replicas, exposed, etc.)",
			},
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ProjectName   string                 `json:"project_name"`
		ComponentName string                 `json:"component_name"`
		AutoDeploy    *bool                  `json:"auto_deploy"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		patchReq := &models.PatchComponentRequest{}
		if args.AutoDeploy != nil {
			patchReq.AutoDeploy = args.AutoDeploy
		}
		if args.Parameters != nil {
			rawParams, err := json.Marshal(args.Parameters)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: "Failed to marshal parameters: " + err.Error()},
					},
					IsError: true,
				}, nil, nil
			}
			patchReq.Parameters = &runtime.RawExtension{Raw: rawParams}
		}
		result, err := t.ComponentToolset.PatchComponent(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, patchReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentWorkflows(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_workflows",
		Description: "List all available ComponentWorkflow templates in an namespace. ComponentWorkflows are " +
			"reusable workflow definitions (like CI/CD pipelines, build processes) that can be triggered for components.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponentWorkflows(ctx, args.NamespaceName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_workflow_schema",
		Description: "Get the schema definition for a ComponentWorkflow template. Returns the JSON schema showing " +
			"workflow configuration options, required fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"cwName":         stringProperty("ComponentWorkflow name. Use list_component_workflows to discover valid names"),
		}, []string{"namespace_name", "cwName"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		CwName        string `json:"cwName"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentWorkflowSchema(ctx, args.NamespaceName, args.CwName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterTriggerComponentWorkflow(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "trigger_component_workflow",
		Description: "Trigger a new workflow run for a component (e.g., build, test, deploy pipeline). " +
			"Optionally specify a git commit SHA to build from a specific commit. If no commit is provided, " +
			"the latest commit from the default branch will be used.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"commit":         stringProperty("Optional: Git commit SHA (7-40 hex characters) to build from"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Commit        string `json:"commit"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.TriggerComponentWorkflow(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, args.Commit)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentWorkflowRuns(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_workflow_runs",
		Description: "List all workflow runs (executions) for a specific component. Shows the history of builds, " +
			"tests, and other workflow executions with their status, timestamps, and results.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponentWorkflowRuns(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateComponentWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_component_workflow_schema",
		Description: "Update or initialize the workflow schema configuration for a specific component. " +
			"This allows customizing workflow behavior, build settings, and other component-specific workflow " +
			"parameters. If the component doesn't have a workflow, provide workflow_name to initialize it.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"workflow_name": map[string]any{
				"type":        "string",
				"description": "Optional: Workflow name (required when initializing workflow on component that doesn't have one)",
			},
			"system_parameters": map[string]any{
				"type":        "object",
				"description": "Optional: System parameters including repository URL, revision (branch/commit), and app path",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Developer-defined workflow parameters (must match ComponentWorkflow schema)",
			},
		}, []string{"namespace_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName    string                 `json:"namespace_name"`
		ProjectName      string                 `json:"project_name"`
		ComponentName    string                 `json:"component_name"`
		WorkflowName     string                 `json:"workflow_name"`
		SystemParameters map[string]interface{} `json:"system_parameters"`
		Parameters       map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		updateReq := &models.UpdateComponentWorkflowRequest{
			WorkflowName: args.WorkflowName,
		}

		// Convert system_parameters if provided
		if args.SystemParameters != nil {
			systemParams := &models.ComponentWorkflowSystemParams{}
			if repo, ok := args.SystemParameters["repository"].(map[string]interface{}); ok {
				repoParams := models.ComponentWorkflowRepository{}
				if url, ok := repo["url"].(string); ok {
					repoParams.URL = url
				}
				if appPath, ok := repo["appPath"].(string); ok {
					repoParams.AppPath = appPath
				}
				if revision, ok := repo["revision"].(map[string]interface{}); ok {
					revParams := models.ComponentWorkflowRepositoryRevision{}
					if branch, ok := revision["branch"].(string); ok {
						revParams.Branch = branch
					}
					if commit, ok := revision["commit"].(string); ok {
						revParams.Commit = commit
					}
					repoParams.Revision = revParams
				}
				systemParams.Repository = repoParams
			}
			updateReq.SystemParameters = systemParams
		}

		// Convert parameters if provided (as RawExtension)
		if args.Parameters != nil {
			rawParams, err := json.Marshal(args.Parameters)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: "Failed to marshal parameters: " + err.Error()},
					},
					IsError: true,
				}, nil, nil
			}
			updateReq.Parameters = &runtime.RawExtension{Raw: rawParams}
		}

		result, err := t.ComponentToolset.UpdateComponentWorkflowSchema(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, updateReq)
		return handleToolResult(result, err)
	})
}
