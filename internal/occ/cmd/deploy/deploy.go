// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// DeployImpl implements deploy operations
type DeployImpl struct{}

// NewDeployImpl creates a new deploy implementation
func NewDeployImpl() *DeployImpl {
	return &DeployImpl{}
}

// DeployComponent deploys or promotes a component
func (d *DeployImpl) DeployComponent(params api.DeployComponentParams) error {
	// Validate required params
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required")
	}
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.Project == "" {
		return fmt.Errorf("project is required")
	}

	ctx := context.Background()

	// Create API client
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	var binding *gen.ReleaseBinding
	var bindingName string

	// Check if this is a promotion or initial deployment
	if params.To != "" {
		// Promotion flow: need to determine source environment
		binding, bindingName, err = d.promoteComponent(ctx, c, params)
		if err != nil {
			return err
		}
	} else {
		// Deploy to root environment
		binding, bindingName, err = d.deployToRoot(ctx, c, params)
		if err != nil {
			return err
		}
	}

	// Apply overrides if provided
	if len(params.Set) > 0 {
		binding, err = d.applyOverrides(ctx, c, params, bindingName)
		if err != nil {
			return err
		}
	}

	// Print result
	fmt.Printf("Successfully deployed component '%s' to environment '%s'\n", params.ComponentName, binding.Environment)
	if binding.ReleaseName != nil {
		fmt.Printf("  Release: %s\n", *binding.ReleaseName)
	}
	fmt.Printf("  Binding: %s\n", binding.Name)

	return nil
}

// deployToRoot deploys a component to the root environment
func (d *DeployImpl) deployToRoot(ctx context.Context, c *client.Client, params api.DeployComponentParams) (*gen.ReleaseBinding, string, error) {
	releaseName := params.Release

	// If no release specified, create a new one
	if releaseName == "" {
		release, err := c.CreateComponentRelease(ctx, params.Namespace, params.Project, params.ComponentName, gen.CreateComponentReleaseRequest{})
		if err != nil {
			return nil, "", fmt.Errorf("failed to create component release: %w", err)
		}
		releaseName = release.Name
		fmt.Printf("Created release: %s\n", releaseName)
	}

	// Deploy the release
	binding, err := c.DeployRelease(ctx, params.Namespace, params.Project, params.ComponentName, gen.DeployReleaseRequest{
		ReleaseName: releaseName,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to deploy release: %w", err)
	}

	return binding, binding.Name, nil
}

// promoteComponent promotes a component to the target environment
func (d *DeployImpl) promoteComponent(ctx context.Context, c *client.Client, params api.DeployComponentParams) (*gen.ReleaseBinding, string, error) {
	// Get project to find deployment pipeline
	project, err := c.GetProject(ctx, params.Namespace, params.Project)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get project: %w", err)
	}

	if project.DeploymentPipeline == nil {
		return nil, "", fmt.Errorf("project does not have a deployment pipeline configured")
	}

	// Get deployment pipeline to determine source environment
	pipeline, err := c.GetProjectDeploymentPipeline(ctx, params.Namespace, params.Project)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Find source environment from pipeline
	sourceEnv, err := d.findSourceEnvironment(pipeline, params.To)
	if err != nil {
		return nil, "", err
	}

	// Promote component
	binding, err := c.PromoteComponent(ctx, params.Namespace, params.Project, params.ComponentName, gen.PromoteComponentRequest{
		SourceEnv: sourceEnv,
		TargetEnv: params.To,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to promote component: %w", err)
	}

	return binding, binding.Name, nil
}

// findSourceEnvironment finds the source environment for a given target environment in the pipeline
func (d *DeployImpl) findSourceEnvironment(pipeline *gen.DeploymentPipeline, targetEnv string) (string, error) {
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

// applyOverrides applies override values to the release binding
func (d *DeployImpl) applyOverrides(ctx context.Context, c *client.Client, params api.DeployComponentParams, bindingName string) (*gen.ReleaseBinding, error) {
	// Parse overrides from --set flags
	overrides, err := ParseOverrides(params.Set)
	if err != nil {
		return nil, fmt.Errorf("failed to parse overrides: %w", err)
	}

	// Create patch request
	patchReq := gen.PatchReleaseBindingRequest{}

	if len(overrides.ComponentTypeEnvOverrides) > 0 {
		patchReq.ComponentTypeEnvOverrides = &overrides.ComponentTypeEnvOverrides
	}

	if len(overrides.TraitOverrides) > 0 {
		patchReq.TraitOverrides = &overrides.TraitOverrides
	}

	if overrides.WorkloadOverrides != nil {
		patchReq.WorkloadOverrides = overrides.WorkloadOverrides
	}

	// Apply patch
	binding, err := c.PatchReleaseBinding(ctx, params.Namespace, params.Project, params.ComponentName, bindingName, patchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to patch release binding: %w", err)
	}

	fmt.Println("Applied overrides successfully")
	return binding, nil
}
