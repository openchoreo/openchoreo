// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/list/output"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	scaffold "github.com/openchoreo/openchoreo/internal/scaffold/component"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CompImpl struct {
	config constants.CRDConfig
}

func NewCompImpl(config constants.CRDConfig) *CompImpl {
	return &CompImpl{
		config: config,
	}
}

// ListComponents lists all components in a project
func (l *CompImpl) ListComponents(params api.ListComponentsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponent, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponents(ctx, params.Namespace, params.Project, &gen.ListComponentsParams{})
	if err != nil {
		return fmt.Errorf("failed to list components: %w", err)
	}

	return output.PrintComponents(result)
}

// ScaffoldComponent generates a scaffold YAML for a component based on its ComponentType and optional Traits and Workflow
func (i *CompImpl) ScaffoldComponent(params api.ScaffoldComponentParams) error {
	return scaffoldComponent(params)
}

// DeployComponent deploys or promotes a component
func (d *CompImpl) DeployComponent(params api.DeployComponentParams) error {
	// Validate required params
	if err := validation.ValidateParams(validation.CmdDeploy, validation.ResourceComponent, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	var binding *gen.ReleaseBinding
	var bindingName string

	// Check if this is a promotion or initial deployment
	if params.To != "" {
		// Promotion flow
		binding, bindingName, err = d.promoteComponent(ctx, c, params)
		if err != nil {
			return err
		}
	} else {
		// Deploy to lowest environment in the pipeline
		binding, bindingName, err = d.deployComponent(ctx, c, params)
		if err != nil {
			return err
		}
	}

	// Apply overrides if provided
	// TODO: Update the deploy and promote API to accept overrides directly so we don't have to do a separate PATCH call here
	if len(params.Set) > 0 {
		binding, err = d.applyOverrides(ctx, c, params, bindingName, binding)
		if err != nil {
			return err
		}
	}

	environment := ""
	if binding.Spec != nil {
		environment = binding.Spec.Environment
	}
	fmt.Printf("Successfully deployed component '%s' to environment '%s'\n", params.ComponentName, environment)
	if binding.Spec != nil && binding.Spec.ReleaseName != nil {
		fmt.Printf("  Release: %s\n", *binding.Spec.ReleaseName)
	}
	fmt.Printf("  Binding: %s\n", binding.Metadata.Name)

	return nil
}

// deployComponent deploys a component to the lowest environment in the pipeline
func (d *CompImpl) deployComponent(ctx context.Context, c *client.Client, params api.DeployComponentParams) (*gen.ReleaseBinding, string, error) {
	releaseName := params.Release

	// If no release specified, generate a new one
	if releaseName == "" {
		release, err := c.GenerateRelease(ctx, params.Namespace, params.ComponentName, gen.GenerateReleaseRequest{})
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate component release: %w", err)
		}
		releaseName = release.Metadata.Name
		fmt.Printf("Created release: %s\n", releaseName)
	}

	binding, err := c.DeployRelease(ctx, params.Namespace, params.ComponentName, gen.DeployReleaseRequest{
		ReleaseName: releaseName,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to deploy release: %w", err)
	}

	return binding, binding.Metadata.Name, nil
}

// promoteComponent promotes a component to the target environment
func (d *CompImpl) promoteComponent(ctx context.Context, c *client.Client, params api.DeployComponentParams) (*gen.ReleaseBinding, string, error) {
	project, err := c.GetProject(ctx, params.Namespace, params.Project)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get project: %w", err)
	}

	if project.Spec == nil || project.Spec.DeploymentPipelineRef == nil {
		return nil, "", fmt.Errorf("project does not have a deployment pipeline configured")
	}

	pipeline, err := c.GetProjectDeploymentPipeline(ctx, params.Namespace, params.Project)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	sourceEnv, err := d.findSourceEnvironment(pipeline, params.To)
	if err != nil {
		return nil, "", err
	}

	binding, err := c.PromoteComponent(ctx, params.Namespace, params.ComponentName, gen.PromoteComponentRequest{
		SourceEnv: sourceEnv,
		TargetEnv: params.To,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to promote component: %w", err)
	}

	return binding, binding.Metadata.Name, nil
}

// findSourceEnvironment finds the source environment for a given target environment in the pipeline
func (d *CompImpl) findSourceEnvironment(pipeline *gen.DeploymentPipeline, targetEnv string) (string, error) {
	if pipeline.PromotionPaths == nil || len(*pipeline.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	// Search through promotion paths to find source for target
	for _, path := range *pipeline.PromotionPaths {
		for _, targetRef := range path.TargetEnvironmentRefs {
			if targetRef.Name == targetEnv {
				return path.SourceEnvironmentRef, nil
			}
		}
	}

	return "", fmt.Errorf("no promotion path found for target environment '%s'", targetEnv)
}

// applyOverrides applies override values to the release binding by merging with existing values
func (d *CompImpl) applyOverrides(ctx context.Context, c *client.Client, params api.DeployComponentParams, bindingName string, existingBinding *gen.ReleaseBinding) (*gen.ReleaseBinding, error) {
	// Merge --set values with existing binding using sjson
	merged, err := mergeOverridesWithBinding(existingBinding, params.Set)
	if err != nil {
		return nil, fmt.Errorf("failed to merge overrides: %w", err)
	}

	// Apply patch
	binding, err := c.PatchReleaseBinding(ctx, params.Namespace, params.Project, params.ComponentName, bindingName, *merged)
	if err != nil {
		return nil, fmt.Errorf("failed to patch release binding: %w", err)
	}

	return binding, nil
}

func scaffoldComponent(params api.ScaffoldComponentParams) error {
	// Validate required parameters
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required (--name)")
	}
	if params.ComponentType == "" {
		return fmt.Errorf("component type is required (--type)")
	}
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required (--namespace or set via context)")
	}
	if params.ProjectName == "" {
		return fmt.Errorf("project is required (--project or set via context)")
	}

	// Parse component type (format: workloadType/componentTypeName)
	workloadType, componentTypeName, err := parseComponentType(params.ComponentType)
	if err != nil {
		return err
	}

	// Create API client
	apiClient, err := client.NewAPIClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Create context with timeout for all API requests (ComponentType, Traits, Workflow)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch ComponentType schema
	componentTypeSchemaRaw, err := apiClient.GetComponentTypeSchema(ctx, params.Namespace, componentTypeName)
	if err != nil {
		return err
	}
	componentTypeSchema, err := unmarshalSchema(componentTypeSchemaRaw)
	if err != nil {
		return fmt.Errorf("invalid ComponentType schema: %w", err)
	}

	// Fetch Trait schemas if specified
	traitSchemas := make(map[string]*extv1.JSONSchemaProps)
	for _, traitName := range params.Traits {
		traitSchemaRaw, err := apiClient.GetTraitSchema(ctx, params.Namespace, traitName)
		if err != nil {
			return err
		}
		traitSchema, err := unmarshalSchema(traitSchemaRaw)
		if err != nil {
			return fmt.Errorf("invalid Trait schema for %q: %w", traitName, err)
		}
		traitSchemas[traitName] = traitSchema
	}

	// Fetch Workflow schema if specified
	var workflowSchema *extv1.JSONSchemaProps
	if params.WorkflowName != "" {
		workflowSchemaRaw, err := apiClient.GetComponentWorkflowSchema(ctx, params.Namespace, params.WorkflowName)
		if err != nil {
			return err
		}
		workflowSchema, err = unmarshalSchema(workflowSchemaRaw)
		if err != nil {
			return fmt.Errorf("invalid ComponentWorkflow schema: %w", err)
		}
	}

	// Create generator options
	// Default behavior: include all comments and optional fields
	// --skip-comments disables both structural comments and field descriptions
	// --skip-optional disables optional fields without defaults
	opts := &scaffold.Options{
		ComponentName:             params.ComponentName,
		Namespace:                 params.Namespace, // namespace name = k8s namespace
		ProjectName:               params.ProjectName,
		IncludeAllFields:          !params.SkipOptional,
		IncludeFieldDescriptions:  !params.SkipComments,
		IncludeStructuralComments: !params.SkipComments,
		IncludeWorkflow:           params.WorkflowName != "",
	}

	// Create generator from schemas
	generator, err := scaffold.NewGeneratorFromSchemas(
		componentTypeName,
		workloadType,
		componentTypeSchema,
		traitSchemas,
		params.WorkflowName,
		workflowSchema,
		opts,
	)
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}

	// Generate YAML
	yamlContent, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate Component YAML: %w", err)
	}

	// Output
	if params.OutputPath != "" {
		if err := os.WriteFile(params.OutputPath, []byte(yamlContent), 0600); err != nil {
			return fmt.Errorf("failed to write output file %s: %w", params.OutputPath, err)
		}
		fmt.Printf("Component YAML written to %s\n", params.OutputPath)
	} else {
		fmt.Print(yamlContent)
	}

	return nil
}

// parseComponentType parses "workloadType/componentTypeName" format
func parseComponentType(typeStr string) (workloadType, componentTypeName string, err error) {
	if typeStr == "" {
		return "", "", fmt.Errorf("--type is required (format: workloadType/componentTypeName, e.g., deployment/web-app)")
	}

	parts := strings.SplitN(typeStr, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid --type format: expected 'workloadType/componentTypeName' (e.g., deployment/web-app), got %q", typeStr)
	}

	return parts[0], parts[1], nil
}

// unmarshalSchema unmarshals a JSON RawMessage to JSONSchemaProps
func unmarshalSchema(raw *json.RawMessage) (*extv1.JSONSchemaProps, error) {
	var schema extv1.JSONSchemaProps
	if err := json.Unmarshal(*raw, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}
	return &schema, nil
}
