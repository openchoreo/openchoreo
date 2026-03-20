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
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
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
// If DataPlaneRef is not specified, it defaults to a DataPlane named "default" in the same namespace,
// falling back to a ClusterDataPlane named "default" if the namespace-scoped one is not found.
func GetDataPlaneOrClusterDataPlaneOfEnv(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment) (*DataPlaneResult, error) {
	ref := env.Spec.DataPlaneRef

	if ref == nil {
		dataPlane := &openchoreov1alpha1.DataPlane{}
		key := client.ObjectKey{Namespace: env.Namespace, Name: DefaultPlaneName}

		if err := c.Get(ctx, key, dataPlane); err == nil {
			return &DataPlaneResult{DataPlane: dataPlane}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default dataPlane: %w", err)
		}

		clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{}
		clusterKey := client.ObjectKey{Name: DefaultPlaneName}

		if err := c.Get(ctx, clusterKey, clusterDataPlane); err == nil {
			return &DataPlaneResult{ClusterDataPlane: clusterDataPlane}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default clusterDataPlane: %w", err)
		}

		return nil, fmt.Errorf("no dataPlaneRef specified and neither default DataPlane nor ClusterDataPlane '%s' found", DefaultPlaneName)
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

// WorkflowPlaneResult contains either a WorkflowPlane or ClusterWorkflowPlane
type WorkflowPlaneResult struct {
	WorkflowPlane        *openchoreov1alpha1.WorkflowPlane
	ClusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane
}

// GetName returns the name of the workflow plane (either WorkflowPlane or ClusterWorkflowPlane)
func (r *WorkflowPlaneResult) GetName() string {
	if r.WorkflowPlane != nil {
		return r.WorkflowPlane.Name
	}
	if r.ClusterWorkflowPlane != nil {
		return r.ClusterWorkflowPlane.Name
	}
	return ""
}

// GetNamespace returns the namespace (empty for ClusterWorkflowPlane)
func (r *WorkflowPlaneResult) GetNamespace() string {
	if r.WorkflowPlane != nil {
		return r.WorkflowPlane.Namespace
	}
	return ""
}

// GetK8sClient returns a Kubernetes client for this workflow plane result.
// It dispatches to the correct client constructor based on whether this is a WorkflowPlane or ClusterWorkflowPlane.
func (r *WorkflowPlaneResult) GetK8sClient(
	clientMgr *kubernetesClient.KubeMultiClientManager,
	gatewayURL string,
) (client.Client, error) {
	if r.WorkflowPlane != nil {
		return kubernetesClient.GetK8sClientFromWorkflowPlane(clientMgr, r.WorkflowPlane, gatewayURL)
	}
	if r.ClusterWorkflowPlane != nil {
		return kubernetesClient.GetK8sClientFromClusterWorkflowPlane(clientMgr, r.ClusterWorkflowPlane, gatewayURL)
	}
	return nil, fmt.Errorf("no workflow plane set in result")
}

// GetSecretStoreName returns the secret store name from the workflow plane (either WorkflowPlane or ClusterWorkflowPlane).
// Returns empty string if no secret store ref is configured.
func (r *WorkflowPlaneResult) GetSecretStoreName() string {
	if r.WorkflowPlane != nil && r.WorkflowPlane.Spec.SecretStoreRef != nil {
		return r.WorkflowPlane.Spec.SecretStoreRef.Name
	}
	if r.ClusterWorkflowPlane != nil && r.ClusterWorkflowPlane.Spec.SecretStoreRef != nil {
		return r.ClusterWorkflowPlane.Spec.SecretStoreRef.Name
	}
	return ""
}

// GetObservabilityPlane resolves the observability plane for this workflow plane result.
func (r *WorkflowPlaneResult) GetObservabilityPlane(ctx context.Context, c client.Client) (*ObservabilityPlaneResult, error) {
	if r.WorkflowPlane != nil {
		return GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane(ctx, c, r.WorkflowPlane)
	}
	if r.ClusterWorkflowPlane != nil {
		cop, err := GetClusterObservabilityPlaneOfClusterWorkflowPlane(ctx, c, r.ClusterWorkflowPlane)
		if err != nil {
			return nil, err
		}
		return &ObservabilityPlaneResult{ClusterObservabilityPlane: cop}, nil
	}
	return nil, fmt.Errorf("no workflow plane set in result")
}

// ResolveWorkflowPlane resolves the WorkflowPlane or ClusterWorkflowPlane using the given WorkflowPlaneRef.
// The ref is always expected to be non-nil since defaulting webhooks ensure workflowPlaneRef is always set.
func ResolveWorkflowPlane(ctx context.Context, c client.Client, namespace string, ref *openchoreov1alpha1.WorkflowPlaneRef) (*WorkflowPlaneResult, error) {
	if ref == nil {
		return nil, fmt.Errorf("workflowPlaneRef must not be nil: CRD defaulting should have set it")
	}

	switch ref.Kind {
	case openchoreov1alpha1.WorkflowPlaneRefKindWorkflowPlane:
		workflowPlane := &openchoreov1alpha1.WorkflowPlane{}
		key := client.ObjectKey{Namespace: namespace, Name: ref.Name}

		if err := c.Get(ctx, key, workflowPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("workflowPlane '%s' not found in namespace '%s': %w", ref.Name, namespace, err)
			}
			return nil, fmt.Errorf("failed to get workflowPlane '%s': %w", ref.Name, err)
		}
		return &WorkflowPlaneResult{WorkflowPlane: workflowPlane}, nil

	case openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane:
		clusterWorkflowPlane := &openchoreov1alpha1.ClusterWorkflowPlane{}
		key := client.ObjectKey{Name: ref.Name}

		if err := c.Get(ctx, key, clusterWorkflowPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("clusterWorkflowPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get clusterWorkflowPlane '%s': %w", ref.Name, err)
		}
		return &WorkflowPlaneResult{ClusterWorkflowPlane: clusterWorkflowPlane}, nil

	default:
		return nil, fmt.Errorf("unsupported workflowPlaneRef kind '%s'", ref.Kind)
	}
}

// GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane retrieves either an ObservabilityPlane or
// ClusterObservabilityPlane for the given WorkflowPlane.
// If ObservabilityPlaneRef is not specified, it defaults to an ObservabilityPlane named "default" in the same namespace.
func GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane(ctx context.Context, c client.Client, workflowPlane *openchoreov1alpha1.WorkflowPlane) (*ObservabilityPlaneResult, error) {
	ref := workflowPlane.Spec.ObservabilityPlaneRef

	// If no ObservabilityPlaneRef is specified, default to ObservabilityPlane named "default" in the same namespace
	if ref == nil {
		observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
		key := client.ObjectKey{Namespace: workflowPlane.Namespace, Name: DefaultPlaneName}

		if err := c.Get(ctx, key, observabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("no observabilityPlaneRef specified and default ObservabilityPlane '%s' not found in namespace '%s': %w", DefaultPlaneName, workflowPlane.Namespace, err)
			}
			return nil, fmt.Errorf("failed to get default observabilityPlane: %w", err)
		}
		return &ObservabilityPlaneResult{ObservabilityPlane: observabilityPlane}, nil
	}

	// Handle based on Kind
	switch ref.Kind {
	case openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane:
		observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
		key := client.ObjectKey{Namespace: workflowPlane.Namespace, Name: ref.Name}

		if err := c.Get(ctx, key, observabilityPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("observabilityPlane '%s' not found in namespace '%s': %w", ref.Name, workflowPlane.Namespace, err)
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

// GetClusterObservabilityPlaneOfClusterWorkflowPlane retrieves the ClusterObservabilityPlane for a ClusterWorkflowPlane.
// If ObservabilityPlaneRef is not specified, it defaults to a ClusterObservabilityPlane named "default".
func GetClusterObservabilityPlaneOfClusterWorkflowPlane(ctx context.Context, c client.Client, clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	ref := clusterWorkflowPlane.Spec.ObservabilityPlaneRef

	// Validate that the ref kind is ClusterObservabilityPlane (the only allowed kind for cluster-scoped resources)
	if ref != nil && openchoreov1alpha1.ObservabilityPlaneRefKind(ref.Kind) != openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane {
		return nil, fmt.Errorf("clusterWorkflowPlane '%s' only supports ClusterObservabilityPlane ref, got '%s'", clusterWorkflowPlane.Name, ref.Kind)
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

// GetObservabilityPlaneOfWorkflowPlane retrieves the ObservabilityPlane for the given WorkflowPlane.
// If ObservabilityPlaneRef is not specified, it defaults to an ObservabilityPlane named "default" in the same namespace.
// This function returns only the ObservabilityPlane; use GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane
// if the ref may point to a ClusterObservabilityPlane.
func GetObservabilityPlaneOfWorkflowPlane(ctx context.Context, c client.Client, workflowPlane *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	result, err := GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane(ctx, c, workflowPlane)
	if err != nil {
		return nil, err
	}
	if result.ObservabilityPlane != nil {
		return result.ObservabilityPlane, nil
	}
	// ClusterObservabilityPlane was found but caller expects ObservabilityPlane
	return nil, fmt.Errorf("workflowPlane '%s' references ClusterObservabilityPlane '%s', use GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane instead", workflowPlane.Name, result.ClusterObservabilityPlane.Name)
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

// WorkflowResult contains either a Workflow or ClusterWorkflow
type WorkflowResult struct {
	Workflow        *openchoreov1alpha1.Workflow
	ClusterWorkflow *openchoreov1alpha1.ClusterWorkflow
}

// GetName returns the name of the workflow
func (r *WorkflowResult) GetName() string {
	if r.Workflow != nil {
		return r.Workflow.Name
	}
	if r.ClusterWorkflow != nil {
		return r.ClusterWorkflow.Name
	}
	return ""
}

// GetNamespace returns the namespace (empty for ClusterWorkflow)
func (r *WorkflowResult) GetNamespace() string {
	if r.Workflow != nil {
		return r.Workflow.Namespace
	}
	return ""
}

// GetWorkflowSpec converts the resolved workflow (either kind) to a unified WorkflowSpec.
// For ClusterWorkflow, ClusterWorkflowPlaneRef is mapped to WorkflowPlaneRef with kind ClusterWorkflowPlane.
// When ClusterWorkflow omits WorkflowPlaneRef, it defaults to ClusterWorkflowPlane "default" so that
// downstream resolution never falls back to a namespace-scoped WorkflowPlane.
func (r *WorkflowResult) GetWorkflowSpec() openchoreov1alpha1.WorkflowSpec {
	if r.Workflow != nil {
		return r.Workflow.Spec
	}
	if r.ClusterWorkflow != nil {
		spec := openchoreov1alpha1.WorkflowSpec{
			Parameters:         r.ClusterWorkflow.Spec.Parameters,
			RunTemplate:        r.ClusterWorkflow.Spec.RunTemplate,
			Resources:          r.ClusterWorkflow.Spec.Resources,
			ExternalRefs:       r.ClusterWorkflow.Spec.ExternalRefs,
			TTLAfterCompletion: r.ClusterWorkflow.Spec.TTLAfterCompletion,
		}
		// Map ClusterWorkflowPlaneRef to WorkflowPlaneRef, defaulting to ClusterWorkflowPlane "default"
		// when the field is omitted (CRD defaulting webhook may not have run).
		if r.ClusterWorkflow.Spec.WorkflowPlaneRef != nil {
			spec.WorkflowPlaneRef = &openchoreov1alpha1.WorkflowPlaneRef{
				Kind: openchoreov1alpha1.WorkflowPlaneRefKind(r.ClusterWorkflow.Spec.WorkflowPlaneRef.Kind),
				Name: r.ClusterWorkflow.Spec.WorkflowPlaneRef.Name,
			}
		} else {
			// Default to the cluster-scoped WorkflowPlane named "default"
			spec.WorkflowPlaneRef = &openchoreov1alpha1.WorkflowPlaneRef{
				Kind: openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane,
				Name: DefaultPlaneName,
			}
		}
		return spec
	}
	return openchoreov1alpha1.WorkflowSpec{}
}

// ResolveWorkflow resolves a Workflow or ClusterWorkflow by kind and name.
func ResolveWorkflow(ctx context.Context, c client.Client, namespace string, kind openchoreov1alpha1.WorkflowRefKind, name string) (*WorkflowResult, error) {
	switch kind {
	case openchoreov1alpha1.WorkflowRefKindClusterWorkflow, "":
		// Cluster-scoped ClusterWorkflow (empty kind defaults to ClusterWorkflow via CRD defaulting)
		cw := &openchoreov1alpha1.ClusterWorkflow{}
		key := client.ObjectKey{Name: name}

		if err := c.Get(ctx, key, cw); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("clusterWorkflow '%s' not found: %w", name, err)
			}
			return nil, fmt.Errorf("failed to get clusterWorkflow '%s': %w", name, err)
		}
		return &WorkflowResult{ClusterWorkflow: cw}, nil

	case openchoreov1alpha1.WorkflowRefKindWorkflow:
		// Namespace-scoped Workflow
		wf := &openchoreov1alpha1.Workflow{}
		key := client.ObjectKey{Namespace: namespace, Name: name}

		if err := c.Get(ctx, key, wf); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("workflow '%s' not found in namespace '%s': %w", name, namespace, err)
			}
			return nil, fmt.Errorf("failed to get workflow '%s': %w", name, err)
		}
		return &WorkflowResult{Workflow: wf}, nil

	default:
		return nil, fmt.Errorf("unsupported workflowRef kind '%s' for workflow '%s' in namespace '%s'", kind, name, namespace)
	}
}
