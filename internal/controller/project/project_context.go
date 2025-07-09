// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sintegrations "github.com/openchoreo/openchoreo/internal/controller/project/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

func (r *Reconciler) makeProjectContext(ctx context.Context, project *openchoreov1alpha1.Project) (*dataplane.ProjectContext, error) {
	deploymentPipeline, err := r.findDeploymentPipeline(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment pipeline: %w", err)
	}

	environmentNames := r.findEnvironmentNamesFromDeploymentPipeline(deploymentPipeline)
	if len(environmentNames) == 0 {
		return nil, fmt.Errorf("no environments found for deployment pipeline %s", project.Spec.DeploymentPipelineRef)
	}

	namespaceNames := k8sintegrations.MakeNamespaceNames(environmentNames, *project)

	return &dataplane.ProjectContext{
		DeploymentPipeline: deploymentPipeline,
		EnvironmentNames:   environmentNames,
		Project:            project,
		NamespaceNames:     namespaceNames,
	}, nil
}

func (r *Reconciler) findDeploymentPipeline(ctx context.Context, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.DeploymentPipeline, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	// Get deployment pipeline
	var deploymentPipeline openchoreov1alpha1.DeploymentPipeline
	err := r.Get(ctx, types.NamespacedName{
		Namespace: project.Namespace,
		Name:      project.Spec.DeploymentPipelineRef,
	}, &deploymentPipeline)

	if err != nil {
		logger.Error(err, "Failed to get deployment pipeline",
			"pipelineRef", project.Spec.DeploymentPipelineRef,
			"namespace", project.Namespace)
		return &openchoreov1alpha1.DeploymentPipeline{}, err
	}

	return &deploymentPipeline, nil
}

func (r *Reconciler) findEnvironmentNamesFromDeploymentPipeline(deploymentPipeline *openchoreov1alpha1.DeploymentPipeline) []string {
	// Use a map to track unique environments
	environmentMap := make(map[string]struct{})

	// Iterate through all promotion paths
	for _, path := range deploymentPipeline.Spec.PromotionPaths {
		// Add source environment
		environmentMap[path.SourceEnvironmentRef] = struct{}{}

		// Add target environments
		for _, target := range path.TargetEnvironmentRefs {
			environmentMap[target.Name] = struct{}{}
		}
	}

	// Convert the map keys to a slice
	environments := make([]string, 0, len(environmentMap))
	for env := range environmentMap {
		environments = append(environments, env)
	}

	return environments
}
