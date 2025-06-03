// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentv2

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// reconcileEndpoints reconciles the Choreo endpoints in the Control Plane based on the ComponentV2
func (r *Reconciler) reconcileEndpoints(ctx context.Context, comp *choreov1.ComponentV2) (ctrl.Result, error) {
	// Make the desired endpoints
	desiredEndpoints, err := r.makeEndpoints(comp)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to make desired endpoints: %w", err)
	}

	// Get all endpoints in the namespace and filter by owner reference
	var allEndpoints choreov1.EndpointV2List
	if err := r.List(ctx, &allEndpoints, client.InNamespace(comp.Namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list endpoints: %w", err)
	}

	// Filter endpoints owned by this component
	var currentEndpoints []choreov1.EndpointV2
	for _, endpoint := range allEndpoints.Items {
		if metav1.IsControlledBy(&endpoint, comp) {
			currentEndpoints = append(currentEndpoints, endpoint)
		}
	}

	// Reconcile each desired endpoint
	for _, desiredEndpoint := range desiredEndpoints {
		existingEndpoint := &choreov1.EndpointV2{}
		err := r.Get(ctx, client.ObjectKeyFromObject(desiredEndpoint), existingEndpoint)
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desiredEndpoint); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create the desired endpoint: %w", err)
			}
		} else if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get the endpoint: %w", err)
		} else {
			// Update the existing endpoint
			existingEndpoint.Spec = desiredEndpoint.Spec
			// TODO: Check the possibility of updating the endpoint only if the spec is changed
			if err := r.Update(ctx, existingEndpoint); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update endpoint: %w", err)
			}
		}
	}

	// Delete the endpoints that are not in the desired state by comparing the names
	desiredEndpointNames := make(map[string]bool, len(desiredEndpoints))
	for _, desiredEndpoint := range desiredEndpoints {
		desiredEndpointNames[desiredEndpoint.Name] = true
	}
	for _, currentEndpoint := range currentEndpoints {
		if !desiredEndpointNames[currentEndpoint.Name] {
			if err := r.Delete(ctx, &currentEndpoint); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete the endpoint: %w", err)
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) makeEndpoints(comp *choreov1.ComponentV2) ([]*choreov1.EndpointV2, error) {
	if len(comp.Spec.Endpoints) == 0 {
		return nil, nil
	}
	endpointTemplates := comp.Spec.Endpoints
	endpoints := make([]*choreov1.EndpointV2, 0, len(endpointTemplates))
	for _, endpointTemplate := range endpointTemplates {
		endpoint := makeEndpoint(comp, &endpointTemplate)
		if err := ctrl.SetControllerReference(comp, endpoint, r.Scheme); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

func makeEndpoint(comp *choreov1.ComponentV2, endpointSpec *choreov1.ComponentEndpointSpec) *choreov1.EndpointV2 {
	endpoint := &choreov1.EndpointV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeEndpointName(comp, endpointSpec),
			Namespace: comp.Namespace,
			Labels:    makeEndpointLabels(comp, endpointSpec),
		},
		Spec: choreov1.EndpointV2Spec{
			Owner: choreov1.EndpointOwner{
				ProjectName:   comp.Spec.Owner.ProjectName,
				ComponentName: comp.Name,
			},
			EnvironmentName: "development", // TODO: Get from context or configuration
			EndpointTemplateSpec: choreov1.EndpointTemplateSpec{
				ClassName:    endpointSpec.ClassName,
				Type:         endpointSpec.Type,
				RESTEndpoint: makeRESTEndpoint(endpointSpec.RESTEndpoint),
			},
		},
	}
	return endpoint
}

// makeEndpointName generates a unique name for the endpoint using the component and endpoint names.
// Format: <component-name>-<endpoint-name>-<random-suffix>
func makeEndpointName(comp *choreov1.ComponentV2, endpointSpec *choreov1.ComponentEndpointSpec) string {
	return kubernetes.GenerateK8sName(comp.Name, endpointSpec.Name)
}

func makeEndpointLabels(comp *choreov1.ComponentV2, endpointSpec *choreov1.ComponentEndpointSpec) map[string]string {
	l := make(map[string]string)
	// Add all the labels from the component
	for key, value := range comp.Labels {
		l[key] = value
	}

	// Add the parent component name
	l[labels.LabelKeyComponentName] = controller.GetName(comp)
	// Set the endpoint name
	l[labels.LabelKeyName] = endpointSpec.Name

	return l
}

// makeRESTEndpoint converts ComponentRESTEndpoint to RESTEndpoint
func makeRESTEndpoint(componentREST *choreov1.ComponentRESTEndpoint) *choreov1.RESTEndpoint {
	return &choreov1.RESTEndpoint{
		Backend: choreov1.HTTPBackend{
			Port:     componentREST.Backend.Port,
			BasePath: componentREST.Backend.BasePath,
		},
		Operations: componentREST.Operations,
	}
}
