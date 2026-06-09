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

// ============================================================================
// DataPlane resolution
// ============================================================================

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

// GetK8sClient returns a Kubernetes client for this data plane result.
// It dispatches to the correct client constructor based on whether this is a DataPlane or ClusterDataPlane.
func (r *DataPlaneResult) GetK8sClient(provider kubernetesClient.DataPlaneClientProvider) (client.Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("no DataPlaneClientProvider configured")
	}
	if r.DataPlane != nil {
		return provider.DataPlaneClient(r.DataPlane)
	}
	if r.ClusterDataPlane != nil {
		return provider.ClusterDataPlaneClient(r.ClusterDataPlane)
	}
	return nil, fmt.Errorf("no data plane set in result")
}

// GetObservabilityPlane resolves the observability plane for this data plane result.
func (r *DataPlaneResult) GetObservabilityPlane(ctx context.Context, c client.Client) (*ObservabilityPlaneResult, error) {
	if r.DataPlane != nil {
		return GetObservabilityPlaneFromRef(ctx, c, r.DataPlane.Namespace, r.DataPlane.Spec.ObservabilityPlaneRef)
	}
	if r.ClusterDataPlane != nil {
		return GetObservabilityPlaneFromRef(ctx, c, "", clusterObsRefToObsRef(r.ClusterDataPlane.Spec.ObservabilityPlaneRef))
	}
	return nil, fmt.Errorf("no data plane set in result")
}

// GetDataPlaneFromRef resolves a DataPlane or ClusterDataPlane from a ref.
// The namespace parameter is the namespace of the referrer (the resource holding the ref).
// If namespace is empty, only cluster-scoped resources are considered (cluster-scoped referrer).
//
// When ref is nil, the function applies a default resolution strategy:
//   - Namespace-scoped referrer (namespace != ""): tries DataPlane "default" in the namespace,
//     then falls back to ClusterDataPlane "default".
//   - Cluster-scoped referrer (namespace == ""): tries ClusterDataPlane "default" only.
//
// When ref is set, it fetches exactly what the ref specifies based on Kind.
func GetDataPlaneFromRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref *openchoreov1alpha1.DataPlaneRef,
) (*DataPlaneResult, error) {
	if ref == nil {
		return getDefaultDataPlane(ctx, c, namespace)
	}

	switch ref.Kind {
	case openchoreov1alpha1.DataPlaneRefKindDataPlane:
		dp := &openchoreov1alpha1.DataPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, dp); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("DataPlane '%s' not found in namespace '%s': %w", ref.Name, namespace, err)
			}
			return nil, fmt.Errorf("failed to get DataPlane '%s': %w", ref.Name, err)
		}
		return &DataPlaneResult{DataPlane: dp}, nil

	case openchoreov1alpha1.DataPlaneRefKindClusterDataPlane:
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name}, cdp); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ClusterDataPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get ClusterDataPlane '%s': %w", ref.Name, err)
		}
		return &DataPlaneResult{ClusterDataPlane: cdp}, nil

	default:
		return nil, fmt.Errorf("unsupported DataPlaneRef kind '%s'", ref.Kind)
	}
}

func getDefaultDataPlane(ctx context.Context, c client.Client, namespace string) (*DataPlaneResult, error) {
	if namespace != "" {
		dp := &openchoreov1alpha1.DataPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: DefaultPlaneName}, dp); err == nil {
			return &DataPlaneResult{DataPlane: dp}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default DataPlane: %w", err)
		}
	}

	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: DefaultPlaneName}, cdp); err == nil {
		return &DataPlaneResult{ClusterDataPlane: cdp}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get default ClusterDataPlane: %w", err)
	}

	if namespace != "" {
		return nil, fmt.Errorf("no DataPlaneRef specified and neither default DataPlane nor ClusterDataPlane '%s' found in namespace '%s'", DefaultPlaneName, namespace)
	}
	return nil, fmt.Errorf("no DataPlaneRef specified and default ClusterDataPlane '%s' not found", DefaultPlaneName)
}

// ============================================================================
// ObservabilityPlane resolution
// ============================================================================

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

// GetK8sClient returns a Kubernetes client for this observability plane result.
// It dispatches to the correct client constructor based on whether this is an ObservabilityPlane or ClusterObservabilityPlane.
func (r *ObservabilityPlaneResult) GetK8sClient(provider kubernetesClient.ObservabilityPlaneClientProvider) (client.Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("no ObservabilityPlaneClientProvider configured")
	}
	if r.ObservabilityPlane != nil {
		return provider.ObservabilityPlaneClient(r.ObservabilityPlane)
	}
	if r.ClusterObservabilityPlane != nil {
		return provider.ClusterObservabilityPlaneClient(r.ClusterObservabilityPlane)
	}
	return nil, fmt.Errorf("no observability plane set in result")
}

// GetObservabilityPlaneFromRef resolves an ObservabilityPlane or ClusterObservabilityPlane from a ref.
// The namespace parameter is the namespace of the referrer (the resource holding the ref).
// If namespace is empty, only cluster-scoped resources are considered (cluster-scoped referrer).
//
// When ref is nil, the function applies a default resolution strategy:
//   - Namespace-scoped referrer (namespace != ""): tries ObservabilityPlane "default" in the namespace,
//     then falls back to ClusterObservabilityPlane "default".
//   - Cluster-scoped referrer (namespace == ""): tries ClusterObservabilityPlane "default" only.
//
// When ref is set, it fetches exactly what the ref specifies based on Kind.
func GetObservabilityPlaneFromRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref *openchoreov1alpha1.ObservabilityPlaneRef,
) (*ObservabilityPlaneResult, error) {
	if ref == nil {
		return getDefaultObservabilityPlane(ctx, c, namespace)
	}

	switch ref.Kind {
	case openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane:
		op := &openchoreov1alpha1.ObservabilityPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, op); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ObservabilityPlane '%s' not found in namespace '%s': %w", ref.Name, namespace, err)
			}
			return nil, fmt.Errorf("failed to get ObservabilityPlane '%s': %w", ref.Name, err)
		}
		return &ObservabilityPlaneResult{ObservabilityPlane: op}, nil

	case openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane:
		cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name}, cop); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ClusterObservabilityPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get ClusterObservabilityPlane '%s': %w", ref.Name, err)
		}
		return &ObservabilityPlaneResult{ClusterObservabilityPlane: cop}, nil

	default:
		return nil, fmt.Errorf("unsupported ObservabilityPlaneRef kind '%s'", ref.Kind)
	}
}

func getDefaultObservabilityPlane(ctx context.Context, c client.Client, namespace string) (*ObservabilityPlaneResult, error) {
	if namespace != "" {
		op := &openchoreov1alpha1.ObservabilityPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: DefaultPlaneName}, op); err == nil {
			return &ObservabilityPlaneResult{ObservabilityPlane: op}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default ObservabilityPlane: %w", err)
		}
	}

	cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: DefaultPlaneName}, cop); err == nil {
		return &ObservabilityPlaneResult{ClusterObservabilityPlane: cop}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get default ClusterObservabilityPlane: %w", err)
	}

	if namespace != "" {
		return nil, fmt.Errorf("no ObservabilityPlaneRef specified and neither default ObservabilityPlane nor ClusterObservabilityPlane '%s' found in namespace '%s'", DefaultPlaneName, namespace)
	}
	return nil, fmt.Errorf("no ObservabilityPlaneRef specified and default ClusterObservabilityPlane '%s' not found", DefaultPlaneName)
}

// clusterObsRefToObsRef converts a ClusterObservabilityPlaneRef to an ObservabilityPlaneRef.
// Returns nil if the input is nil, preserving the "no ref specified" semantics.
func clusterObsRefToObsRef(ref *openchoreov1alpha1.ClusterObservabilityPlaneRef) *openchoreov1alpha1.ObservabilityPlaneRef {
	if ref == nil {
		return nil
	}
	return &openchoreov1alpha1.ObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ObservabilityPlaneRefKind(ref.Kind),
		Name: ref.Name,
	}
}

// ============================================================================
// WorkflowPlane resolution
// ============================================================================

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
func (r *WorkflowPlaneResult) GetK8sClient(provider kubernetesClient.WorkflowPlaneClientProvider) (client.Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("no WorkflowPlaneClientProvider configured")
	}
	if r.WorkflowPlane != nil {
		return provider.WorkflowPlaneClient(r.WorkflowPlane)
	}
	if r.ClusterWorkflowPlane != nil {
		return provider.ClusterWorkflowPlaneClient(r.ClusterWorkflowPlane)
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
		return GetObservabilityPlaneFromRef(ctx, c, r.WorkflowPlane.Namespace, r.WorkflowPlane.Spec.ObservabilityPlaneRef)
	}
	if r.ClusterWorkflowPlane != nil {
		return GetObservabilityPlaneFromRef(ctx, c, "", clusterObsRefToObsRef(r.ClusterWorkflowPlane.Spec.ObservabilityPlaneRef))
	}
	return nil, fmt.Errorf("no workflow plane set in result")
}

// GetWorkflowPlaneFromRef resolves a WorkflowPlane or ClusterWorkflowPlane from a ref.
// The namespace parameter is the namespace of the referrer (the resource holding the ref).
// If namespace is empty, only cluster-scoped resources are considered (cluster-scoped referrer).
//
// When ref is nil, the function applies a default resolution strategy:
//   - Namespace-scoped referrer (namespace != ""): tries WorkflowPlane "default" in the namespace,
//     then falls back to ClusterWorkflowPlane "default".
//   - Cluster-scoped referrer (namespace == ""): tries ClusterWorkflowPlane "default" only.
//
// When ref is set, it fetches exactly what the ref specifies based on Kind.
func GetWorkflowPlaneFromRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	ref *openchoreov1alpha1.WorkflowPlaneRef,
) (*WorkflowPlaneResult, error) {
	if ref == nil {
		return getDefaultWorkflowPlane(ctx, c, namespace)
	}

	switch ref.Kind {
	case openchoreov1alpha1.WorkflowPlaneRefKindWorkflowPlane:
		wp := &openchoreov1alpha1.WorkflowPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, wp); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("WorkflowPlane '%s' not found in namespace '%s': %w", ref.Name, namespace, err)
			}
			return nil, fmt.Errorf("failed to get WorkflowPlane '%s': %w", ref.Name, err)
		}
		return &WorkflowPlaneResult{WorkflowPlane: wp}, nil

	case openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane:
		cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name}, cwp); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ClusterWorkflowPlane '%s' not found: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("failed to get ClusterWorkflowPlane '%s': %w", ref.Name, err)
		}
		return &WorkflowPlaneResult{ClusterWorkflowPlane: cwp}, nil

	default:
		return nil, fmt.Errorf("unsupported WorkflowPlaneRef kind '%s'", ref.Kind)
	}
}

func getDefaultWorkflowPlane(ctx context.Context, c client.Client, namespace string) (*WorkflowPlaneResult, error) {
	if namespace != "" {
		wp := &openchoreov1alpha1.WorkflowPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: DefaultPlaneName}, wp); err == nil {
			return &WorkflowPlaneResult{WorkflowPlane: wp}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get default WorkflowPlane: %w", err)
		}
	}

	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: DefaultPlaneName}, cwp); err == nil {
		return &WorkflowPlaneResult{ClusterWorkflowPlane: cwp}, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get default ClusterWorkflowPlane: %w", err)
	}

	if namespace != "" {
		return nil, fmt.Errorf("no WorkflowPlaneRef specified and neither default WorkflowPlane nor ClusterWorkflowPlane '%s' found in namespace '%s'", DefaultPlaneName, namespace)
	}
	return nil, fmt.Errorf("no WorkflowPlaneRef specified and default ClusterWorkflowPlane '%s' not found", DefaultPlaneName)
}

// ============================================================================
// Workflow resolution (Workflow/ClusterWorkflow, not a plane type)
// ============================================================================

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
