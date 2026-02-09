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

// DataPlaneResult contains either a DataPlane or ClusterDataPlane
type DataPlaneResult struct {
	DataPlane        *openchoreov1alpha1.DataPlane
	ClusterDataPlane *openchoreov1alpha1.ClusterDataPlane
}

// GetName returns the name of the data plane (either DataPlane or ClusterDataPlane)
func (r *DataPlaneResult) GetName() string {
	if r.DataPlane != nil {
		return r.DataPlane.Name
	}
	if r.ClusterDataPlane != nil {
		return r.ClusterDataPlane.Name
	}
	return ""
}

// GetNamespace returns the namespace (empty for ClusterDataPlane)
func (r *DataPlaneResult) GetNamespace() string {
	if r.DataPlane != nil {
		return r.DataPlane.Namespace
	}
	return ""
}

// GetDataplaneOfEnv retrieves the DataPlane for the given Environment.
// If DataPlaneRef is not specified, it defaults to a DataPlane named "default" in the same namespace.
// If DataPlaneRef specifies ClusterDataPlane kind, it looks up a cluster-scoped ClusterDataPlane.
func GetDataplaneOfEnv(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.DataPlane, error) {
	result, err := GetDataPlaneOrClusterDataPlaneOfEnv(ctx, c, env)
	if err != nil {
		return nil, err
	}
	if result.DataPlane != nil {
		return result.DataPlane, nil
	}
	// ClusterDataPlane was found but caller expects DataPlane
	// Return nil with descriptive error since this function returns *DataPlane
	return nil, fmt.Errorf("environment '%s' references ClusterDataPlane '%s', use GetDataPlaneOrClusterDataPlaneOfEnv instead", env.Name, result.ClusterDataPlane.Name)
}

// GetDataPlaneOrClusterDataPlaneOfEnv retrieves either a DataPlane or ClusterDataPlane for the given Environment.
// If DataPlaneRef is not specified, it defaults to a DataPlane named "default" in the same namespace.
func GetDataPlaneOrClusterDataPlaneOfEnv(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment) (*DataPlaneResult, error) {
	ref := env.Spec.DataPlaneRef

	// If no DataPlaneRef is specified, default to DataPlane named "default" in the same namespace
	if ref == nil {
		dataPlane := &openchoreov1alpha1.DataPlane{}
		key := client.ObjectKey{Namespace: env.Namespace, Name: DefaultPlaneName}

		if err := c.Get(ctx, key, dataPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no dataPlaneRef specified and default DataPlane '%s' not found in namespace '%s': %w", DefaultPlaneName, env.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get default dataPlane: %w", err)
		}
		return &DataPlaneResult{DataPlane: dataPlane}, nil
	}

	// Handle based on Kind
	switch ref.Kind {
	case openchoreov1alpha1.DataPlaneRefKindDataPlane:
		dataPlane := &openchoreov1alpha1.DataPlane{}
		key := client.ObjectKey{Namespace: env.Namespace, Name: ref.Name}

		if err := c.Get(ctx, key, dataPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("dataPlane '%s' not found in namespace '%s': %w", ref.Name, env.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get dataPlane '%s': %w", ref.Name, err)
		}
		return &DataPlaneResult{DataPlane: dataPlane}, nil

	case openchoreov1alpha1.DataPlaneRefKindClusterDataPlane:
		clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{}
		key := client.ObjectKey{Name: ref.Name}

		if err := c.Get(ctx, key, clusterDataPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("clusterDataPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get clusterDataPlane '%s': %w", ref.Name, err)
		}
		return &DataPlaneResult{ClusterDataPlane: clusterDataPlane}, nil

	default:
		return nil, fmt.Errorf("unsupported dataPlaneRef kind '%s'", ref.Kind)
	}
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
