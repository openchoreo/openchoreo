// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// DefaultPlaneName is the default name for plane resources when no explicit reference is provided
	DefaultPlaneName = "default"
)

func GetDataplaneOfEnv(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.DataPlane, error) {
	// Determine the plane name to look for
	planeName := env.Spec.DataPlaneRef
	if planeName == "" {
		planeName = DefaultPlaneName
	}

	// Try to find the DataPlane in the same namespace
	dataPlane := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{Namespace: env.Namespace, Name: planeName}

	if err := c.Get(ctx, key, dataPlane); err != nil {
		if apierrors.IsNotFound(err) {
			if env.Spec.DataPlaneRef == "" {
				return nil, fmt.Errorf("no dataPlaneRef specified and default DataPlane '%s' not found in namespace '%s'. Error is: %w", DefaultPlaneName, env.Namespace, err)
			}
			return nil, fmt.Errorf("dataPlane '%s' not found in namespace '%s'. Error is: %w", planeName, env.Namespace, err)
		}
		return nil, fmt.Errorf("failed to get dataPlane. Error is: %w", err)
	}

	return dataPlane, nil
}

func GetObservabilityPlaneOfBuildPlane(ctx context.Context, c client.Client, buildPlane *openchoreov1alpha1.BuildPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	// Determine the plane name to look for
	planeName := buildPlane.Spec.ObservabilityPlaneRef
	if planeName == "" {
		planeName = DefaultPlaneName
	}

	// Try to find the ObservabilityPlane in the same namespace
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
	key := client.ObjectKey{Namespace: buildPlane.Namespace, Name: planeName}

	if err := c.Get(ctx, key, observabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			if buildPlane.Spec.ObservabilityPlaneRef == "" {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ObservabilityPlane '%s' not found in namespace '%s'. Error is: %w", DefaultPlaneName, buildPlane.Namespace, err)
			}
			return nil, fmt.Errorf("observabilityPlane '%s' not found in namespace '%s'. Error is: %w", planeName, buildPlane.Namespace, err)
		}
		return nil, fmt.Errorf("failed to get observabilityPlane. Error is: %w", err)
	}

	return observabilityPlane, nil
}

func GetObservabilityPlaneOfDataPlane(ctx context.Context, c client.Client, dataPlane *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	// Determine the plane name to look for
	planeName := dataPlane.Spec.ObservabilityPlaneRef
	if planeName == "" {
		planeName = DefaultPlaneName
	}

	// Try to find the ObservabilityPlane in the same namespace
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
	key := client.ObjectKey{Namespace: dataPlane.Namespace, Name: planeName}

	if err := c.Get(ctx, key, observabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			if dataPlane.Spec.ObservabilityPlaneRef == "" {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ObservabilityPlane '%s' not found in namespace '%s'", DefaultPlaneName, dataPlane.Namespace)
			}
			return nil, fmt.Errorf("observabilityPlane '%s' not found in namespace '%s'", planeName, dataPlane.Namespace)
		}
		return nil, fmt.Errorf("failed to get observabilityPlane: %w", err)
	}

	return observabilityPlane, nil
}
