/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package deployment

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// reconcileChoreoEndpoints reconciles the Choreo endpoints in the Control Plane based on the deployment context so that
// the endpoint controller will take care of the reconciliation of the external resources for the endpoints.
func (r *Reconciler) reconcileChoreoEndpoints(ctx context.Context, deploymentCtx *dataplane.DeploymentContext) error {
	// Make the desired endpoints
	desiredEndpoints, err := r.makeEndpoints(deploymentCtx)
	if err != nil {
		return fmt.Errorf("failed to make desired endpoints: %w", err)
	}

	// Get the current endpoints owned by this deployment
	var currentEndpoints choreov1.EndpointList
	listOpts := []client.ListOption{
		client.InNamespace(deploymentCtx.Deployment.Namespace),
		client.MatchingFields{"metadata.ownerReferences": string(deploymentCtx.Deployment.UID)},
	}
	if err := r.List(ctx, &currentEndpoints, listOpts...); err != nil {
		return fmt.Errorf("failed to list current endpoints: %w", err)
	}

	// Reconcile each desired endpoint
	for _, desiredEndpoint := range desiredEndpoints {
		existingEndpoint := &choreov1.Endpoint{}
		err := r.Get(ctx, client.ObjectKeyFromObject(desiredEndpoint), existingEndpoint)
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredEndpoint); err != nil {
				return fmt.Errorf("failed to create the desired endpoint: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to get the endpoint: %w", err)
		} else {
			// Update the existing endpoint
			existingEndpoint.Spec = desiredEndpoint.Spec
			// TODO: Check the possibility of updating the endpoint only if the spec is changed
			if err := r.Update(ctx, existingEndpoint); err != nil {
				return fmt.Errorf("failed to update endpoint: %w", err)
			}
		}
	}

	// Delete the endpoints that are not in the desired state by comparing the names
	desiredEndpointNames := make(map[string]bool, len(desiredEndpoints))
	for _, desiredEndpoint := range desiredEndpoints {
		desiredEndpointNames[desiredEndpoint.Name] = true
	}
	for _, currentEndpoint := range currentEndpoints.Items {
		if !desiredEndpointNames[currentEndpoint.Name] {
			if err := r.Delete(ctx, &currentEndpoint); err != nil {
				return fmt.Errorf("failed to delete the endpoint: %w", err)
			}
		}
	}

	return nil
}

func (r *Reconciler) makeEndpoints(deployCtx *dataplane.DeploymentContext) ([]*choreov1.Endpoint, error) {
	if deployCtx.DeployableArtifact.Spec.Configuration == nil {
		return nil, nil
	}
	endpointTemplates := deployCtx.DeployableArtifact.Spec.Configuration.EndpointTemplates
	endpoints := make([]*choreov1.Endpoint, 0, len(endpointTemplates))
	for _, endpointTemplate := range endpointTemplates {
		endpoint := makeEndpoint(deployCtx, &endpointTemplate)
		if err := ctrl.SetControllerReference(deployCtx.Deployment, endpoint, r.Scheme); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

func makeEndpoint(deployCtx *dataplane.DeploymentContext, endpointTemplate *choreov1.EndpointTemplate) *choreov1.Endpoint {
	endpoint := &choreov1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:        makeEndpointName(deployCtx, endpointTemplate),
			Namespace:   deployCtx.Deployment.Namespace,
			Annotations: makeEndpointAnnotations(deployCtx, endpointTemplate),
			Labels:      makeEndpointLabels(deployCtx, endpointTemplate),
		},
		Spec: *endpointTemplate.Spec.DeepCopy(),
	}
	return endpoint
}

// makeEndpointName generates a unique name for the endpoint using the deployment and endpoint names.
// Format: <deployment-name>-<endpoint-name>-<random-suffix> if .metadata.name is set
//
//	<deployment-name>-endpoint-<random-suffix> if .metadata.name is not set
func makeEndpointName(deployCtx *dataplane.DeploymentContext, endpointTemplate *choreov1.EndpointTemplate) string {
	if endpointTemplate.Name != "" {
		return kubernetes.GenerateK8sName(deployCtx.Deployment.Name, endpointTemplate.Name)
	}
	return kubernetes.GenerateK8sName(deployCtx.Deployment.Name, "endpoint")
}

func makeEndpointLabels(deployCtx *dataplane.DeploymentContext, endpointTemplate *choreov1.EndpointTemplate) map[string]string {
	l := make(map[string]string)
	// Add all the labels from the deployment
	for key, value := range deployCtx.Deployment.Labels {
		l[key] = value
	}

	// Add the parent deployment name
	l[labels.LabelKeyDeploymentName] = controller.GetName(deployCtx.Deployment)
	// Set the endpoint name as the value for the label name
	l[labels.LabelKeyName] = endpointTemplate.Name

	return l
}

func makeEndpointAnnotations(deployCtx *dataplane.DeploymentContext, endpointTemplate *choreov1.EndpointTemplate) map[string]string {
	annotations := make(map[string]string)
	// Add all the annotations from the deployment
	for key, value := range deployCtx.Deployment.Annotations {
		annotations[key] = value
	}

	if endpointTemplate.Annotations != nil {
		annotations[controller.AnnotationKeyDisplayName] = endpointTemplate.Annotations[controller.AnnotationKeyDisplayName]
		annotations[controller.AnnotationKeyDescription] = endpointTemplate.Annotations[controller.AnnotationKeyDescription]
	}
	return annotations
}
