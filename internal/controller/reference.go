// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// ToDataPlane returns a *DataPlane - either the real one or a facade built from ClusterDataPlane.
// This allows downstream code (e.g. rendering pipeline) to remain unchanged.
func (r *DataPlaneResult) ToDataPlane() *openchoreov1alpha1.DataPlane {
	if r.DataPlane != nil {
		return r.DataPlane
	}
	if r.ClusterDataPlane != nil {
		var obsRef *openchoreov1alpha1.ObservabilityPlaneRef
		if r.ClusterDataPlane.Spec.ObservabilityPlaneRef != nil {
			obsRef = &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: r.ClusterDataPlane.Spec.ObservabilityPlaneRef.Name,
			}
		}
		return &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: r.ClusterDataPlane.Name,
				UID:  r.ClusterDataPlane.UID,
			},
			Spec: openchoreov1alpha1.DataPlaneSpec{
				PlaneID:               r.ClusterDataPlane.Spec.PlaneID,
				ClusterAgent:          r.ClusterDataPlane.Spec.ClusterAgent,
				Gateway:               r.ClusterDataPlane.Spec.Gateway,
				ImagePullSecretRefs:   r.ClusterDataPlane.Spec.ImagePullSecretRefs,
				SecretStoreRef:        r.ClusterDataPlane.Spec.SecretStoreRef,
				ObservabilityPlaneRef: obsRef,
			},
		}
	}
	return nil
}

// GetObservabilityPlane resolves the observability plane for this data plane result.
func (r *DataPlaneResult) GetObservabilityPlane(ctx context.Context, c client.Client) (*ObservabilityPlaneResult, error) {
	if r.DataPlane != nil {
		return GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, c, r.DataPlane)
	}
	if r.ClusterDataPlane != nil {
		cop, err := GetClusterObservabilityPlaneOfClusterDataPlane(ctx, c, r.ClusterDataPlane)
		if err != nil {
			return nil, err
		}
		return &ObservabilityPlaneResult{ClusterObservabilityPlane: cop}, nil
	}
	return nil, fmt.Errorf("no data plane set in result")
}

// ObservabilityPlaneResult contains either an ObservabilityPlane or ClusterObservabilityPlane
type ObservabilityPlaneResult struct {
	ObservabilityPlane        *openchoreov1alpha1.ObservabilityPlane
	ClusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane
}

// GetName returns the name of the observability plane
func (r *ObservabilityPlaneResult) GetName() string {
	if r.ObservabilityPlane != nil {
		return r.ObservabilityPlane.Name
	}
	if r.ClusterObservabilityPlane != nil {
		return r.ClusterObservabilityPlane.Name
	}
	return ""
}

// GetNamespace returns the namespace (empty for ClusterObservabilityPlane)
func (r *ObservabilityPlaneResult) GetNamespace() string {
	if r.ObservabilityPlane != nil {
		return r.ObservabilityPlane.Namespace
	}
	return ""
}

// GetObserverURL returns the observer URL from either ObservabilityPlane or ClusterObservabilityPlane
func (r *ObservabilityPlaneResult) GetObserverURL() string {
	if r.ObservabilityPlane != nil {
		return r.ObservabilityPlane.Spec.ObserverURL
	}
	if r.ClusterObservabilityPlane != nil {
		return r.ClusterObservabilityPlane.Spec.ObserverURL
	}
	return ""
}

// GetPlaneID returns the plane ID from either ObservabilityPlane or ClusterObservabilityPlane
func (r *ObservabilityPlaneResult) GetPlaneID() string {
	if r.ObservabilityPlane != nil {
		return r.ObservabilityPlane.Spec.PlaneID
	}
	if r.ClusterObservabilityPlane != nil {
		return r.ClusterObservabilityPlane.Spec.PlaneID
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

// BuildPlaneResult contains either a BuildPlane or ClusterBuildPlane
type BuildPlaneResult struct {
	BuildPlane        *openchoreov1alpha1.BuildPlane
	ClusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane
}

// GetName returns the name of the build plane (either BuildPlane or ClusterBuildPlane)
func (r *BuildPlaneResult) GetName() string {
	if r.BuildPlane != nil {
		return r.BuildPlane.Name
	}
	if r.ClusterBuildPlane != nil {
		return r.ClusterBuildPlane.Name
	}
	return ""
}

// GetNamespace returns the namespace (empty for ClusterBuildPlane)
func (r *BuildPlaneResult) GetNamespace() string {
	if r.BuildPlane != nil {
		return r.BuildPlane.Namespace
	}
	return ""
}

// GetBuildPlaneOrClusterBuildPlaneOfProject retrieves the BuildPlane or ClusterBuildPlane for the given Project.
// Resolution order:
// 1. If Project.Spec.BuildPlaneRef is set, use that by Kind and Name
// 2. If not set, try BuildPlane named "default" in the same namespace
// 3. If "default" BuildPlane not found, try ClusterBuildPlane named "default"
// 4. If neither found, fall back to first available BuildPlane in namespace
// Returns nil without error if no BuildPlane exists (BuildPlane is optional for Projects)
func GetBuildPlaneOrClusterBuildPlaneOfProject(ctx context.Context, c client.Client, project *openchoreov1alpha1.Project) (*BuildPlaneResult, error) {
	ref := project.Spec.BuildPlaneRef

	// If no ref specified, try resolution chain
	if ref == nil {
		// Step 1: Try "default" BuildPlane in namespace
		buildPlane := &openchoreov1alpha1.BuildPlane{}
		key := client.ObjectKey{Namespace: project.Namespace, Name: DefaultPlaneName}

		if err := c.Get(ctx, key, buildPlane); err == nil {
			return &BuildPlaneResult{BuildPlane: buildPlane}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default buildPlane: %w", err)
		}

		// Step 2: Try "default" ClusterBuildPlane
		clusterBuildPlane := &openchoreov1alpha1.ClusterBuildPlane{}
		clusterKey := client.ObjectKey{Name: DefaultPlaneName}

		if err := c.Get(ctx, clusterKey, clusterBuildPlane); err == nil {
			return &BuildPlaneResult{ClusterBuildPlane: clusterBuildPlane}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default clusterBuildPlane: %w", err)
		}

		// Step 3: Fall back to first available BuildPlane in namespace
		return getFirstBuildPlaneInNamespace(ctx, c, project.Namespace)
	}

	switch ref.Kind {
	case openchoreov1alpha1.BuildPlaneRefKindBuildPlane:
		buildPlane := &openchoreov1alpha1.BuildPlane{}
		key := client.ObjectKey{Namespace: project.Namespace, Name: ref.Name}

		if err := c.Get(ctx, key, buildPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("buildPlane '%s' not found in namespace '%s': %w", ref.Name, project.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get buildPlane '%s': %w", ref.Name, err)
		}
		return &BuildPlaneResult{BuildPlane: buildPlane}, nil

	case openchoreov1alpha1.BuildPlaneRefKindClusterBuildPlane:
		clusterBuildPlane := &openchoreov1alpha1.ClusterBuildPlane{}
		key := client.ObjectKey{Name: ref.Name}

		if err := c.Get(ctx, key, clusterBuildPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("clusterBuildPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get clusterBuildPlane '%s': %w", ref.Name, err)
		}
		return &BuildPlaneResult{ClusterBuildPlane: clusterBuildPlane}, nil

	default:
		return nil, fmt.Errorf("unsupported buildPlaneRef kind '%s'", ref.Kind)
	}
}

// getFirstBuildPlaneInNamespace returns the first BuildPlane found in the namespace.
// Returns nil without error if no BuildPlane exists.
func getFirstBuildPlaneInNamespace(ctx context.Context, c client.Client, namespace string) (*BuildPlaneResult, error) {
	buildPlaneList := &openchoreov1alpha1.BuildPlaneList{}
	if err := c.List(ctx, buildPlaneList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	if len(buildPlaneList.Items) == 0 {
		return nil, nil // No BuildPlane available - this is OK for Projects
	}

	return &BuildPlaneResult{BuildPlane: &buildPlaneList.Items[0]}, nil
}

// GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane retrieves either an ObservabilityPlane or
// ClusterObservabilityPlane for the given BuildPlane.
// If ObservabilityPlaneRef is not specified, it defaults to an ObservabilityPlane named "default" in the same namespace.
func GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane(ctx context.Context, c client.Client, buildPlane *openchoreov1alpha1.BuildPlane) (*ObservabilityPlaneResult, error) {
	ref := buildPlane.Spec.ObservabilityPlaneRef

	// If no ObservabilityPlaneRef is specified, default to ObservabilityPlane named "default" in the same namespace
	if ref == nil {
		observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
		key := client.ObjectKey{Namespace: buildPlane.Namespace, Name: DefaultPlaneName}

		if err := c.Get(ctx, key, observabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ObservabilityPlane '%s' not found in namespace '%s': %w", DefaultPlaneName, buildPlane.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get default observabilityPlane: %w", err)
		}
		return &ObservabilityPlaneResult{ObservabilityPlane: observabilityPlane}, nil
	}

	// Handle based on Kind
	switch ref.Kind {
	case openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane:
		observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
		key := client.ObjectKey{Namespace: buildPlane.Namespace, Name: ref.Name}

		if err := c.Get(ctx, key, observabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("observabilityPlane '%s' not found in namespace '%s': %w", ref.Name, buildPlane.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get observabilityPlane '%s': %w", ref.Name, err)
		}
		return &ObservabilityPlaneResult{ObservabilityPlane: observabilityPlane}, nil

	case openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane:
		clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{}
		key := client.ObjectKey{Name: ref.Name}

		if err := c.Get(ctx, key, clusterObservabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("clusterObservabilityPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get clusterObservabilityPlane '%s': %w", ref.Name, err)
		}
		return &ObservabilityPlaneResult{ClusterObservabilityPlane: clusterObservabilityPlane}, nil

	default:
		return nil, fmt.Errorf("unsupported observabilityPlaneRef kind '%s'", ref.Kind)
	}
}

// GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane retrieves either an ObservabilityPlane or
// ClusterObservabilityPlane for the given DataPlane.
// If ObservabilityPlaneRef is not specified, it defaults to an ObservabilityPlane named "default" in the same namespace.
func GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx context.Context, c client.Client, dataPlane *openchoreov1alpha1.DataPlane) (*ObservabilityPlaneResult, error) {
	ref := dataPlane.Spec.ObservabilityPlaneRef

	// If no ObservabilityPlaneRef is specified, default to ObservabilityPlane named "default" in the same namespace
	if ref == nil {
		observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
		key := client.ObjectKey{Namespace: dataPlane.Namespace, Name: DefaultPlaneName}

		if err := c.Get(ctx, key, observabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ObservabilityPlane '%s' not found in namespace '%s': %w", DefaultPlaneName, dataPlane.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get default observabilityPlane: %w", err)
		}
		return &ObservabilityPlaneResult{ObservabilityPlane: observabilityPlane}, nil
	}

	// Handle based on Kind
	switch ref.Kind {
	case openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane:
		observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
		key := client.ObjectKey{Namespace: dataPlane.Namespace, Name: ref.Name}

		if err := c.Get(ctx, key, observabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("observabilityPlane '%s' not found in namespace '%s': %w", ref.Name, dataPlane.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get observabilityPlane '%s': %w", ref.Name, err)
		}
		return &ObservabilityPlaneResult{ObservabilityPlane: observabilityPlane}, nil

	case openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane:
		clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{}
		key := client.ObjectKey{Name: ref.Name}

		if err := c.Get(ctx, key, clusterObservabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("clusterObservabilityPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get clusterObservabilityPlane '%s': %w", ref.Name, err)
		}
		return &ObservabilityPlaneResult{ClusterObservabilityPlane: clusterObservabilityPlane}, nil

	default:
		return nil, fmt.Errorf("unsupported observabilityPlaneRef kind '%s'", ref.Kind)
	}
}

// GetClusterObservabilityPlaneOfClusterDataPlane retrieves the ClusterObservabilityPlane for a ClusterDataPlane.
// If ObservabilityPlaneRef is not specified, it defaults to a ClusterObservabilityPlane named "default".
func GetClusterObservabilityPlaneOfClusterDataPlane(ctx context.Context, c client.Client, clusterDataPlane *openchoreov1alpha1.ClusterDataPlane) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	ref := clusterDataPlane.Spec.ObservabilityPlaneRef

	// Validate that the ref kind is ClusterObservabilityPlane (the only allowed kind for cluster-scoped resources)
	if ref != nil && openchoreov1alpha1.ObservabilityPlaneRefKind(ref.Kind) != openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane {
		return nil, fmt.Errorf("clusterDataPlane '%s' only supports ClusterObservabilityPlane ref, got '%s'", clusterDataPlane.Name, ref.Kind)
	}

	// Determine the plane name to look for
	planeName := DefaultPlaneName
	if ref != nil {
		planeName = ref.Name
	}

	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{}
	key := client.ObjectKey{Name: planeName}

	if err := c.Get(ctx, key, clusterObservabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			if ref == nil {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ClusterObservabilityPlane '%s' not found: %w", DefaultPlaneName, err)
			}
			return nil, fmt.Errorf("clusterObservabilityPlane '%s' not found: %w", planeName, err)
		}
		return nil, fmt.Errorf("failed to get clusterObservabilityPlane '%s': %w", planeName, err)
	}

	return clusterObservabilityPlane, nil
}

// GetClusterObservabilityPlaneOfClusterBuildPlane retrieves the ClusterObservabilityPlane for a ClusterBuildPlane.
// If ObservabilityPlaneRef is not specified, it defaults to a ClusterObservabilityPlane named "default".
func GetClusterObservabilityPlaneOfClusterBuildPlane(ctx context.Context, c client.Client, clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	ref := clusterBuildPlane.Spec.ObservabilityPlaneRef

	// Validate that the ref kind is ClusterObservabilityPlane (the only allowed kind for cluster-scoped resources)
	if ref != nil && openchoreov1alpha1.ObservabilityPlaneRefKind(ref.Kind) != openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane {
		return nil, fmt.Errorf("clusterBuildPlane '%s' only supports ClusterObservabilityPlane ref, got '%s'", clusterBuildPlane.Name, ref.Kind)
	}

	// Determine the plane name to look for
	planeName := DefaultPlaneName
	if ref != nil {
		planeName = ref.Name
	}

	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{}
	key := client.ObjectKey{Name: planeName}

	if err := c.Get(ctx, key, clusterObservabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			if ref == nil {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ClusterObservabilityPlane '%s' not found: %w", DefaultPlaneName, err)
			}
			return nil, fmt.Errorf("clusterObservabilityPlane '%s' not found: %w", planeName, err)
		}
		return nil, fmt.Errorf("failed to get clusterObservabilityPlane '%s': %w", planeName, err)
	}

	return clusterObservabilityPlane, nil
}

// GetObservabilityPlaneOfBuildPlane retrieves the ObservabilityPlane for the given BuildPlane.
// If ObservabilityPlaneRef is not specified, it defaults to an ObservabilityPlane named "default" in the same namespace.
// This function returns only the ObservabilityPlane; use GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane
// if the ref may point to a ClusterObservabilityPlane.
func GetObservabilityPlaneOfBuildPlane(ctx context.Context, c client.Client, buildPlane *openchoreov1alpha1.BuildPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	result, err := GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane(ctx, c, buildPlane)
	if err != nil {
		return nil, err
	}
	if result.ObservabilityPlane != nil {
		return result.ObservabilityPlane, nil
	}
	// ClusterObservabilityPlane was found but caller expects ObservabilityPlane
	return nil, fmt.Errorf("buildPlane '%s' references ClusterObservabilityPlane '%s', use GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane instead", buildPlane.Name, result.ClusterObservabilityPlane.Name)
}

// GetObservabilityPlaneOfDataPlane retrieves the ObservabilityPlane for the given DataPlane.
// If ObservabilityPlaneRef is not specified, it defaults to an ObservabilityPlane named "default" in the same namespace.
// This function returns only the ObservabilityPlane; use GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane
// if the ref may point to a ClusterObservabilityPlane.
func GetObservabilityPlaneOfDataPlane(ctx context.Context, c client.Client, dataPlane *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	result, err := GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, c, dataPlane)
	if err != nil {
		return nil, err
	}
	if result.ObservabilityPlane != nil {
		return result.ObservabilityPlane, nil
	}
	// ClusterObservabilityPlane was found but caller expects ObservabilityPlane
	return nil, fmt.Errorf("dataPlane '%s' references ClusterObservabilityPlane '%s', use GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane instead", dataPlane.Name, result.ClusterObservabilityPlane.Name)
}
