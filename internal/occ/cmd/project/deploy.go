// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Deploy deploys or promotes a project by managing its ProjectReleaseBinding.
func (p *Project) Deploy(params DeployParams) error {
	if err := cmdutil.RequireFields("deploy", "project", map[string]string{
		"namespace": params.Namespace,
		"name":      params.ProjectName,
	}); err != nil {
		return err
	}

	ctx := context.Background()

	var binding *gen.ProjectReleaseBinding
	var err error
	if params.To != "" {
		binding, err = p.promoteProject(ctx, params)
	} else {
		binding, err = p.deployProject(ctx, params)
	}
	if err != nil {
		return err
	}

	env := ""
	if binding.Spec != nil {
		env = binding.Spec.Environment
	}
	fmt.Printf("Successfully deployed project '%s' to environment '%s'\n", params.ProjectName, env)
	if binding.Spec != nil && binding.Spec.ProjectRelease != nil {
		fmt.Printf("  Release: %s\n", *binding.Spec.ProjectRelease)
	}
	fmt.Printf("  Binding: %s\n", binding.Metadata.Name)
	return nil
}

// deployProject ensures a ProjectReleaseBinding exists for the lowest environment
// in the project's pipeline. When no --release is given, spec.projectRelease is
// left unset so the Project controller seeds it with the latest release.
func (p *Project) deployProject(ctx context.Context, params DeployParams) (*gen.ProjectReleaseBinding, error) {
	pipeline, err := p.client.GetProjectDeploymentPipeline(ctx, params.Namespace, params.ProjectName)
	if err != nil {
		return nil, err
	}

	lowestEnv, err := utils.FindLowestEnvironment(pipeline)
	if err != nil {
		return nil, err
	}

	var releasePtr *string
	if params.Release != "" {
		releasePtr = &params.Release
	}

	existing, err := p.findBinding(ctx, params.Namespace, params.ProjectName, lowestEnv)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Binding already exists. Only advance an explicit release pin; otherwise
		// leave the controller-seeded value untouched.
		if releasePtr == nil {
			fmt.Printf("Project '%s' is already deployed to environment '%s'\n", params.ProjectName, lowestEnv)
			return existing, nil
		}
		existing.Spec.ProjectRelease = releasePtr
		return p.client.UpdateProjectReleaseBinding(ctx, params.Namespace, existing.Metadata.Name, *existing)
	}

	prb := newBinding(fmt.Sprintf("%s-%s", params.ProjectName, lowestEnv), params.ProjectName, lowestEnv, releasePtr)
	return p.client.CreateProjectReleaseBinding(ctx, params.Namespace, prb)
}

// promoteProject advances the target environment's ProjectReleaseBinding to the
// release pinned in the source environment (or an explicit --release).
func (p *Project) promoteProject(ctx context.Context, params DeployParams) (*gen.ProjectReleaseBinding, error) {
	pipeline, err := p.client.GetProjectDeploymentPipeline(ctx, params.Namespace, params.ProjectName)
	if err != nil {
		return nil, err
	}

	sourceEnv, err := utils.FindSourceEnvironment(pipeline, params.To)
	if err != nil {
		return nil, err
	}

	releaseName := params.Release
	if releaseName == "" {
		source, err := p.findBinding(ctx, params.Namespace, params.ProjectName, sourceEnv)
		if err != nil {
			return nil, err
		}
		if source == nil || source.Spec == nil || source.Spec.ProjectRelease == nil || *source.Spec.ProjectRelease == "" {
			return nil, fmt.Errorf("no release pinned for source environment '%s'", sourceEnv)
		}
		releaseName = *source.Spec.ProjectRelease
	}

	existing, err := p.findBinding(ctx, params.Namespace, params.ProjectName, params.To)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.Spec.ProjectRelease = &releaseName
		return p.client.UpdateProjectReleaseBinding(ctx, params.Namespace, existing.Metadata.Name, *existing)
	}

	prb := newBinding(fmt.Sprintf("%s-%s", params.ProjectName, params.To), params.ProjectName, params.To, &releaseName)
	return p.client.CreateProjectReleaseBinding(ctx, params.Namespace, prb)
}

// findBinding returns the ProjectReleaseBinding owned by the project for the
// given environment, or nil if none exists.
func (p *Project) findBinding(ctx context.Context, namespace, project, env string) (*gen.ProjectReleaseBinding, error) {
	list, err := p.client.ListProjectReleaseBindings(ctx, namespace, &gen.ListProjectReleaseBindingsParams{
		Project: &project,
	})
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		b := &list.Items[i]
		if b.Spec != nil && b.Spec.Environment == env && b.Spec.Owner.ProjectName == project {
			return b, nil
		}
	}
	return nil, nil
}

// newBinding builds a ProjectReleaseBinding for the given project and environment.
// A nil release leaves spec.projectRelease unset for the controller to seed.
func newBinding(name, project, env string, release *string) gen.ProjectReleaseBinding {
	prb := gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ProjectReleaseBindingSpec{
			Environment:    env,
			ProjectRelease: release,
		},
	}
	prb.Spec.Owner.ProjectName = project
	return prb
}
