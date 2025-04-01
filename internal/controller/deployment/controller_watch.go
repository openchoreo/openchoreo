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

package deployment

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// All the watch handlers for the deployment controller are defined in this file.

const (
	// deploymentArtifactRefIndexKey is the field index key in the deployment that
	// points to a deployable artifact.
	deploymentArtifactRefIndexKey = "spec.deploymentArtifactRef"
	// configurationGroupRefIndexKey is the field index key which points to the configuration group
	// by mapping it via deployment artifacts.
	configurationGroupRefIndexKey = "spec.configuration.application.configurationGroupRef"
)

// setupDeploymentArtifactRefIndex creates a field index for the deployment artifact reference in the deployments.
func (r *Reconciler) setupDeploymentArtifactRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&choreov1.Deployment{},
		deploymentArtifactRefIndexKey,
		func(obj client.Object) []string {
			// Convert the object to the appropriate type
			deployment, ok := obj.(*choreov1.Deployment)
			if !ok {
				return nil
			}
			// Return the value of the deploymentArtifactRef field
			return []string{deployment.Spec.DeploymentArtifactRef}
		},
	)
}

func (r *Reconciler) setupEndpointsOwnerRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&choreov1.Endpoint{},
		"metadata.ownerReferences",
		func(rawObj client.Object) []string {
			endpoint := rawObj.(*choreov1.Endpoint)
			var owners []string
			for _, ownerRef := range endpoint.OwnerReferences {
				owners = append(owners, string(ownerRef.UID))
			}
			return owners
		},
	)
}

// listDeploymentsForDeployableArtifact is a watch handler that lists all the deployments
// that refers to a given deployable artifact and makes reconcile.Request for reconciliation.
func (r *Reconciler) listDeploymentsForDeployableArtifact(ctx context.Context, obj client.Object) []reconcile.Request {
	deployableArtifact, ok := obj.(*choreov1.DeployableArtifact)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a DeployableArtifact from the Watch
		return nil
	}

	// List all the deployments that have .spec.deploymentArtifactRef equal to the name of the deployable artifact
	deploymentList := &choreov1.DeploymentList{}
	if err := r.List(
		ctx,
		deploymentList,
		client.MatchingFields{deploymentArtifactRefIndexKey: deployableArtifact.Name},
	); err != nil {
		return nil
	}

	// Enqueue all the deployments that have the deployable artifact as the deployment artifact
	requests := make([]reconcile.Request, len(deploymentList.Items))
	for i, deployment := range deploymentList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: deployment.Namespace,
				Name:      deployment.Name,
			},
		}
	}

	// Enqueue the deployment if the deployable artifact is updated
	return requests
}

// setupConfigurationGroupRefIndex creates a field index for the configuration groups that are mapped via deployment artifacts.
func (r *Reconciler) setupConfigurationGroupRefIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&choreov1.DeployableArtifact{},
		configurationGroupRefIndexKey,
		func(obj client.Object) []string {
			// Convert the object to the appropriate type
			da, ok := obj.(*choreov1.DeployableArtifact)
			if !ok || da.Spec.Configuration == nil || da.Spec.Configuration.Application == nil {
				return nil
			}

			// Store the configuration group names in a map to avoid duplicates
			configurationGroupNameSet := make(map[string]struct{})
			appConfig := da.Spec.Configuration.Application

			// Find the configuration group references in the env section
			for _, env := range appConfig.Env {
				if env.ValueFrom == nil || env.ValueFrom.ConfigurationGroupRef == nil {
					continue
				}
				configurationGroupNameSet[env.ValueFrom.ConfigurationGroupRef.Name] = struct{}{}
			}

			// Find the configuration group references in the envFrom section
			for _, envFrom := range appConfig.EnvFrom {
				if envFrom.ConfigurationGroupRef == nil {
					continue
				}
				configurationGroupNameSet[envFrom.ConfigurationGroupRef.Name] = struct{}{}
			}

			// Convert the map to a slice
			configurationGroupNames := make([]string, 0, len(configurationGroupNameSet))
			for name := range configurationGroupNameSet {
				configurationGroupNames = append(configurationGroupNames, name)
			}

			// Return the value of the mapped configuration group names
			return configurationGroupNames
		},
	)
}

// listDeploymentsForConfigurationGroup is a watch handler that queues all the deployments
// that refers to a configuration group via a deployable artifact.
func (r *Reconciler) listDeploymentsForConfigurationGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	cg, ok := obj.(*choreov1.ConfigurationGroup)
	if !ok {
		// Ideally, this should not happen as obj is always expected to be a ConfigurationGroup from the Watch
		return nil
	}

	// List all the deployable artifacts that refers to this configuration group
	deployableArtifactList := &choreov1.DeployableArtifactList{}
	if err := r.List(
		ctx,
		deployableArtifactList,
		client.MatchingFields{configurationGroupRefIndexKey: controller.GetName(cg)},
	); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0)

	// For each deployable artifact, list all the deployments that refers to it
	for _, da := range deployableArtifactList.Items {
		deploymentList := &choreov1.DeploymentList{}
		if err := r.List(
			ctx,
			deploymentList,
			client.MatchingFields{deploymentArtifactRefIndexKey: da.Name},
		); err != nil {
			return nil
		}

		// Enqueue all the deployments that have the deployable artifact as the deployment artifact
		for _, deployment := range deploymentList.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: deployment.Namespace,
					Name:      deployment.Name,
				},
			})
		}
	}
	return requests
}
