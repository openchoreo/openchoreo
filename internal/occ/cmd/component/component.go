// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/setoverride"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	scaffold "github.com/openchoreo/openchoreo/internal/scaffold/component"
)

type Component struct{}

func New() *Component {
	return &Component{}
}

// List lists all components in a project
func (cp *Component) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponent, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Component, string, error) {
		p := &gen.ListComponentsParams{}
		if params.Project != "" {
			p.Project = &params.Project
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListComponents(ctx, params.Namespace, "", p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}

	return printList(items, params.Project == "")
}

// StartWorkflow gets the component, resolves its workflow name, and starts a workflow run.
func (cp *Component) StartWorkflow(params StartWorkflowParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required")
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	comp, err := c.GetComponent(ctx, params.Namespace, params.ComponentName)
	if err != nil {
		return err
	}

	if comp.Spec == nil || comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
		return fmt.Errorf("component %q has no workflow configured", params.ComponentName)
	}

	wfConfig := comp.Spec.Workflow
	var baseParams map[string]interface{}
	if wfConfig.Parameters != nil {
		baseParams = *wfConfig.Parameters
	}

	var workflowKind string
	if wfConfig.Kind != nil {
		workflowKind = string(*wfConfig.Kind)
	}

	return workflow.New().StartRun(workflow.StartRunParams{
		Namespace:    params.Namespace,
		WorkflowName: wfConfig.Name,
		WorkflowKind: workflowKind,
		RunName:      fmt.Sprintf("%s-build-%d", params.ComponentName, time.Now().Unix()),
		Parameters:   baseParams,
		Set:          params.Set,
		Labels: map[string]string{
			"openchoreo.dev/component": params.ComponentName,
			"openchoreo.dev/project":   params.Project,
		},
	})
}

// ListWorkflowRuns lists workflow runs filtered by component name.
func (cp *Component) ListWorkflowRuns(params ListWorkflowRunsParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required")
	}

	items, err := workflowrun.FetchAll(params.Namespace, "")
	if err != nil {
		return err
	}

	filtered := workflowrun.FilterByComponent(items, params.ComponentName)
	return workflowrun.PrintList(filtered)
}

// Get retrieves a single component and outputs it as YAML
func (cp *Component) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceComponent, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetComponent(ctx, params.Namespace, params.ComponentName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal component to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single component
func (cp *Component) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceComponent, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteComponent(ctx, params.Namespace, params.ComponentName); err != nil {
		return err
	}

	fmt.Printf("Component '%s' deleted\n", params.ComponentName)
	return nil
}

// Scaffold generates a scaffold YAML for a component based on its ComponentType and optional Traits and Workflow
func (cp *Component) Scaffold(params ScaffoldParams) error {
	return scaffoldComponent(params)
}

// Deploy deploys or promotes a component
func (cp *Component) Deploy(params DeployParams) error {
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

	// Check if this is a promotion or initial deployment
	if params.To != "" {
		// Promotion flow
		binding, err = cp.promoteComponent(ctx, c, params)
		if err != nil {
			return err
		}
	} else {
		// Deploy to lowest environment in the pipeline
		binding, err = cp.deployComponent(ctx, c, params)
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
func (cp *Component) deployComponent(ctx context.Context, c *client.Client, params DeployParams) (*gen.ReleaseBinding, error) {
	releaseName := params.Release

	// If no release specified, generate a new one
	if releaseName == "" {
		release, err := c.GenerateRelease(ctx, params.Namespace, params.ComponentName, gen.GenerateReleaseRequest{})
		if err != nil {
			return nil, err
		}
		releaseName = release.Metadata.Name
		fmt.Printf("Created release: %s\n", releaseName)
	}

	// Resolve the lowest environment from the deployment pipeline
	pipeline, err := c.GetProjectDeploymentPipeline(ctx, params.Namespace, params.Project)
	if err != nil {
		return nil, err
	}

	lowestEnv, err := findLowestEnvironment(pipeline)
	if err != nil {
		return nil, err
	}

	// Build the ReleaseBinding
	bindingName := fmt.Sprintf("%s-%s", params.ComponentName, lowestEnv)
	rb := gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{
			Name: bindingName,
		},
		Spec: &gen.ReleaseBindingSpec{
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: params.ComponentName,
				ProjectName:   params.Project,
			},
			Environment: lowestEnv,
			ReleaseName: &releaseName,
		},
	}

	// Apply overrides if provided
	if len(params.Set) > 0 {
		merged, err := mergeOverridesWithBinding(&rb, params.Set)
		if err != nil {
			return nil, fmt.Errorf("failed to merge overrides: %w", err)
		}
		rb = *merged
	}

	binding, err := c.CreateReleaseBinding(ctx, params.Namespace, rb)
	if err != nil {
		return nil, err
	}

	return binding, nil
}

// promoteComponent promotes a component to the target environment
func (cp *Component) promoteComponent(ctx context.Context, c *client.Client, params DeployParams) (*gen.ReleaseBinding, error) {
	pipeline, err := c.GetProjectDeploymentPipeline(ctx, params.Namespace, params.Project)
	if err != nil {
		return nil, err
	}

	sourceEnv, err := findSourceEnvironment(pipeline, params.To)
	if err != nil {
		return nil, err
	}

	// Get the source release binding to find the release name
	sourceBindings, err := c.ListReleaseBindings(ctx, params.Namespace, "", params.ComponentName)
	if err != nil {
		return nil, err
	}

	var releaseName string
	for _, b := range sourceBindings.Items {
		if b.Spec != nil && b.Spec.Environment == sourceEnv &&
			b.Spec.Owner.ComponentName == params.ComponentName {
			if b.Spec.ReleaseName != nil {
				releaseName = *b.Spec.ReleaseName
			}
			break
		}
	}
	if releaseName == "" {
		return nil, fmt.Errorf("no release binding found for source environment '%s'", sourceEnv)
	}

	// Check if a binding already exists for the target environment
	bindingName := fmt.Sprintf("%s-%s", params.ComponentName, params.To)
	existing, err := c.GetReleaseBinding(ctx, params.Namespace, bindingName)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Update existing binding with the new release
		existing.Spec.ReleaseName = &releaseName

		// Apply overrides if provided
		if len(params.Set) > 0 {
			merged, err := mergeOverridesWithBinding(existing, params.Set)
			if err != nil {
				return nil, fmt.Errorf("failed to merge overrides: %w", err)
			}
			existing = merged
		}

		return c.UpdateReleaseBinding(ctx, params.Namespace, bindingName, *existing)
	}

	// Create new binding
	rb := gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{
			Name: bindingName,
		},
		Spec: &gen.ReleaseBindingSpec{
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: params.ComponentName,
				ProjectName:   params.Project,
			},
			Environment: params.To,
			ReleaseName: &releaseName,
		},
	}

	// Apply overrides if provided
	if len(params.Set) > 0 {
		merged, err := mergeOverridesWithBinding(&rb, params.Set)
		if err != nil {
			return nil, fmt.Errorf("failed to merge overrides: %w", err)
		}
		rb = *merged
	}

	return c.CreateReleaseBinding(ctx, params.Namespace, rb)
}

// findLowestEnvironment finds the environment that is not a target in any promotion path.
func findLowestEnvironment(pipeline *gen.DeploymentPipeline) (string, error) {
	if pipeline.Spec == nil || pipeline.Spec.PromotionPaths == nil || len(*pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	targets := make(map[string]bool)
	for _, path := range *pipeline.Spec.PromotionPaths {
		for _, targetRef := range path.TargetEnvironmentRefs {
			targets[targetRef.Name] = true
		}
	}

	for _, path := range *pipeline.Spec.PromotionPaths {
		if !targets[path.SourceEnvironmentRef.Name] {
			return path.SourceEnvironmentRef.Name, nil
		}
	}

	// Fallback: return the first source
	return (*pipeline.Spec.PromotionPaths)[0].SourceEnvironmentRef.Name, nil
}

// findSourceEnvironment finds the source environment for a given target environment in the pipeline
func findSourceEnvironment(pipeline *gen.DeploymentPipeline, targetEnv string) (string, error) {
	if pipeline.Spec == nil || pipeline.Spec.PromotionPaths == nil || len(*pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	// Search through promotion paths to find source for target
	for _, path := range *pipeline.Spec.PromotionPaths {
		for _, targetRef := range path.TargetEnvironmentRefs {
			if targetRef.Name == targetEnv {
				return path.SourceEnvironmentRef.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no promotion path found for target environment '%s'", targetEnv)
}

func scaffoldComponent(params ScaffoldParams) error {
	// Validate required parameters
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required")
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
	apiClient, err := client.NewClient()
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
		workflowSchemaRaw, err := apiClient.GetWorkflowSchema(ctx, params.Namespace, params.WorkflowName)
		if err != nil {
			return err
		}
		workflowSchema, err = unmarshalSchema(workflowSchemaRaw)
		if err != nil {
			return fmt.Errorf("invalid Workflow schema: %w", err)
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

// mergeOverridesWithBinding merges --set override values with existing ReleaseBinding.
func mergeOverridesWithBinding(existingBinding *gen.ReleaseBinding, setValues []string) (*gen.ReleaseBinding, error) {
	existingJSON, err := json.Marshal(existingBinding)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal existing binding: %w", err)
	}

	jsonStr, err := setoverride.Apply(string(existingJSON), setValues)
	if err != nil {
		return nil, fmt.Errorf("failed to merge overrides: %w", err)
	}

	var rb gen.ReleaseBinding
	if err := json.Unmarshal([]byte(jsonStr), &rb); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged result: %w", err)
	}

	return &rb, nil
}

func printList(items []gen.Component, showProject bool) error {
	if len(items) == 0 {
		fmt.Println("No components found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if showProject {
		fmt.Fprintln(w, "NAME\tPROJECT\tTYPE\tAGE")
	} else {
		fmt.Fprintln(w, "NAME\tTYPE\tAGE")
	}

	for _, comp := range items {
		projectName := ""
		componentType := ""
		if comp.Spec != nil {
			projectName = comp.Spec.Owner.ProjectName
			componentType = comp.Spec.ComponentType.Name
		}
		age := ""
		if comp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*comp.Metadata.CreationTimestamp)
		}
		if showProject {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				comp.Metadata.Name,
				projectName,
				componentType,
				age)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				comp.Metadata.Name,
				componentType,
				age)
		}
	}

	return w.Flush()
}
