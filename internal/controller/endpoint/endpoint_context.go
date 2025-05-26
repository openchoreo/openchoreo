// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// makeEndpointContext creates a endpoint context for the given deployment by retrieving the
// parent objects that this deployment is associated with.
func (r *Reconciler) makeEndpointContext(ctx context.Context, ep *choreov1.Endpoint) (*dataplane.EndpointContext, error) {
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

func getDataplane(ctx context.Context, c client.Client, env *choreov1.Environment) (*choreov1.DataPlane, error) {
	dp := &choreov1.DataPlane{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.GetNamespace(), Name: env.Spec.DataPlaneRef}, dp); err != nil {
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}
	return dp, nil
}
