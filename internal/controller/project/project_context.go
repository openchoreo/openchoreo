/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package project

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func (r *Reconciler) makeProjectContext(ctx context.Context, project *choreov1.Project) (*dataplane.ProjectContext, error) {
	deploymentPipeline, err := r.findDeploymentPipeline(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment pipeline: %w", err)
	}

	environmentNames := r.findEnvironmentNamesFromDeploymentPipeline(&deploymentPipeline)
	if len(environmentNames) == 0 {
		return nil, fmt.Errorf("no environments found for deployment pipeline %s", project.Spec.DeploymentPipelineRef)
	}

	namespaceNames := r.makeNamespaceNames(environmentNames, *project)

	return &dataplane.ProjectContext{
		DeploymentPipeline: &deploymentPipeline,
		EnvironmentNames:   environmentNames,
		Project:            project,
		NamespaceNames:     namespaceNames,
	}, nil
}

func (r *Reconciler) findDeploymentPipeline(ctx context.Context, project *choreov1.Project) (choreov1.DeploymentPipeline, error) {
	logger := log.FromContext(ctx).WithValues("project", project.Name)

	// Get deployment pipeline
	var deploymentPipeline choreov1.DeploymentPipeline
	err := r.Get(ctx, types.NamespacedName{
		Namespace: project.Namespace,
		Name:      project.Spec.DeploymentPipelineRef,
	}, &deploymentPipeline)

	if err != nil {
		logger.Error(err, "Failed to get deployment pipeline",
			"pipelineRef", project.Spec.DeploymentPipelineRef,
			"namespace", project.Namespace)
		return choreov1.DeploymentPipeline{}, err
	}

	return deploymentPipeline, nil
}

func (r *Reconciler) findEnvironmentNamesFromDeploymentPipeline(deploymentPipeline *choreov1.DeploymentPipeline) []string {
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

// NamespaceName has the format dp-<organization-name>-<project-name>-<environment-name>-<hash>
func (r *Reconciler) makeNamespaceNames(environmentNames []string, project choreov1.Project) []string {
	namespaceNames := make([]string, 0, len(environmentNames))

	organizationName := controller.GetOrganizationName(&project)
	projectName := controller.GetName(&project)
	for _, env := range environmentNames {
		environmentName := env
		// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
		namespaceName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
			"dp", organizationName, projectName, environmentName)
		namespaceNames = append(namespaceNames, namespaceName)
	}

	return namespaceNames
}
