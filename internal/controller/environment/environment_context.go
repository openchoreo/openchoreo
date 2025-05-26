// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"fmt"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

func (r *Reconciler) makeEnvironmentContext(ctx context.Context, environment *choreov1.Environment) (*dataplane.EnvironmentContext, error) {
	dataPlane, err := controller.GetDataPlaneByEnvironment(ctx, r.Client, environment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the dataplane: %w", err)
	}

	return &dataplane.EnvironmentContext{
		DataPlane:   dataPlane,
		Environment: environment,
	}, nil
}
