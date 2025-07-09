// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentv2

// import (
//	"context"
//
//	ctrl "sigs.k8s.io/controller-runtime"
//
//	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
// )

// reconcileEndpoints reconciles the Choreo endpoints in the Control Plane based on the ComponentV2
/*
func (r *Reconciler) reconcileEndpoints(ctx context.Context, comp *openchoreov1alpha1.ComponentV2) (ctrl.Result, error) {
	// Make the desired endpoints
	// desiredEndpoints, err := r.makeEndpoints(comp)
	// if err != nil {
	//	return ctrl.Result{}, fmt.Errorf("failed to make desired endpoints: %w", err)
	// }
	//
	// // Get all endpoints in the namespace and filter by owner reference
	// var allEndpoints openchoreov1alpha1.EndpointV2List
	// if err := r.List(ctx, &allEndpoints, client.InNamespace(comp.Namespace)); err != nil {
	//	return ctrl.Result{}, fmt.Errorf("failed to list endpoints: %w", err)
	// }
	//
	// // Filter endpoints owned by this component
	// var currentEndpoints []openchoreov1alpha1.EndpointV2
	// for _, endpoint := range allEndpoints.Items {
	//	if metav1.IsControlledBy(&endpoint, comp) {
	//		currentEndpoints = append(currentEndpoints, endpoint)
	//	}
	// }
	//
	// // Reconcile each desired endpoint
	// for _, desiredEndpoint := range desiredEndpoints {
	//	existingEndpoint := &openchoreov1alpha1.EndpointV2{}
	//	err := r.Get(ctx, client.ObjectKeyFromObject(desiredEndpoint), existingEndpoint)
	//	if apierrors.IsNotFound(err) {
	//		if err := r.Create(ctx, desiredEndpoint); err != nil {
	//			return ctrl.Result{}, fmt.Errorf("failed to create the desired endpoint: %w", err)
	//		}
	//	} else if err != nil {
	//		return ctrl.Result{}, fmt.Errorf("failed to get the endpoint: %w", err)
	//	} else {
	//		// Update the existing endpoint
	//		existingEndpoint.Spec = desiredEndpoint.Spec
	//		// TODO: Check the possibility of updating the endpoint only if the spec is changed
	//		if err := r.Update(ctx, existingEndpoint); err != nil {
	//			return ctrl.Result{}, fmt.Errorf("failed to update endpoint: %w", err)
	//		}
	//	}
	// }
	//
	// // Delete the endpoints that are not in the desired state by comparing the names
	// desiredEndpointNames := make(map[string]bool, len(desiredEndpoints))
	// for _, desiredEndpoint := range desiredEndpoints {
	//	desiredEndpointNames[desiredEndpoint.Name] = true
	// }
	// for _, currentEndpoint := range currentEndpoints {
	//	if !desiredEndpointNames[currentEndpoint.Name] {
	//		if err := r.Delete(ctx, &currentEndpoint); err != nil {
	//			return ctrl.Result{}, fmt.Errorf("failed to delete the endpoint: %w", err)
	//		}
	//	}
	// }
	//
	return ctrl.Result{}, nil
}
*/
