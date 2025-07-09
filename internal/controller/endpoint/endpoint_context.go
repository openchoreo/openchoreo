// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// makeEndpointContext creates a endpoint context for the given deployment by retrieving the
// parent objects that this deployment is associated with.
func (r *Reconciler) makeEndpointContext(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (*dataplane.EndpointContext, error) {
	project, err := controller.GetProject(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the project: %w", err)
	}

	component, err := controller.GetComponent(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the component: %w", err)
	}

	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment track: %w", err)
	}

	environment, err := controller.GetEnvironment(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the environment: %w", err)
	}

	deployment, err := controller.GetDeployment(ctx, r.Client, ep)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment: %w", err)
	}
	dp, err := getDataplane(ctx, r.Client, environment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the dataplane: %w", err)
	}
	return &dataplane.EndpointContext{
		DataPlane:       dp,
		Project:         project,
		Component:       component,
		DeploymentTrack: deploymentTrack,
		Deployment:      deployment,
		Environment:     environment,
		Endpoint:        ep,
	}, nil
}

func getDataplane(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.DataPlane, error) {
	dp := &openchoreov1alpha1.DataPlane{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.GetNamespace(), Name: env.Spec.DataPlaneRef}, dp); err != nil {
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}
	return dp, nil
}
