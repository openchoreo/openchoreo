// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestGetDataplaneOfEnv_WithExplicitRef(t *testing.T) {
	// Create a scheme with our API types
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test DataPlane
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-dataplane",
			Namespace: "test-namespace",
		},
	}

	// Create test Environment with explicit dataPlaneRef
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-env",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "my-dataplane",
			},
		},
	}

	// Create fake client with objects
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dataPlane, environment).
		Build()

	// Test the function
	result, err := GetDataplaneOfEnv(context.Background(), fakeClient, environment)

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, "my-dataplane", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetDataplaneOfEnv_WithEmptyRef_DefaultExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create default DataPlane
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
	}

	// Create Environment without dataPlaneRef
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-env",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: nil, // nil ref - should fallback to "default"
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dataPlane, environment).
		Build()

	result, err := GetDataplaneOfEnv(context.Background(), fakeClient, environment)

	require.NoError(t, err)
	assert.Equal(t, "default", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetDataplaneOfEnv_WithEmptyRef_DefaultNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Environment without dataPlaneRef, but no "default" DataPlane exists
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-env",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(environment).
		Build()

	result, err := GetDataplaneOfEnv(context.Background(), fakeClient, environment)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no dataPlaneRef specified and neither default DataPlane nor ClusterDataPlane 'default' found")
}

func TestGetDataplaneOfEnv_WithExplicitRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Environment with explicit ref that doesn't exist
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-env",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "nonexistent-dataplane",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(environment).
		Build()

	result, err := GetDataplaneOfEnv(context.Background(), fakeClient, environment)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "dataPlane 'nonexistent-dataplane' not found in namespace 'test-namespace'")
}

func TestGetObservabilityPlaneOfWorkflowPlane_WithExplicitRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ObservabilityPlane
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-observability",
			Namespace: "test-namespace",
		},
	}

	// Create test WorkflowPlane with explicit observabilityPlaneRef
	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflowplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(observabilityPlane, workflowPlane).
		Build()

	result, err := GetObservabilityPlaneOfWorkflowPlane(context.Background(), fakeClient, workflowPlane)

	require.NoError(t, err)
	assert.Equal(t, "my-observability", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetObservabilityPlaneOfWorkflowPlane_WithEmptyRef_DefaultExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create default ObservabilityPlane
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
	}

	// Create WorkflowPlane without observabilityPlaneRef
	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflowplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(observabilityPlane, workflowPlane).
		Build()

	result, err := GetObservabilityPlaneOfWorkflowPlane(context.Background(), fakeClient, workflowPlane)

	require.NoError(t, err)
	assert.Equal(t, "default", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetObservabilityPlaneOfWorkflowPlane_WithEmptyRef_DefaultNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create WorkflowPlane without observabilityPlaneRef, but no "default" ObservabilityPlane exists
	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflowplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workflowPlane).
		Build()

	result, err := GetObservabilityPlaneOfWorkflowPlane(context.Background(), fakeClient, workflowPlane)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no observabilityPlaneRef specified and default ObservabilityPlane 'default' not found in namespace 'test-namespace'")
}

func TestGetObservabilityPlaneOfWorkflowPlane_WithExplicitRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create WorkflowPlane with explicit ref that doesn't exist
	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflowplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "nonexistent-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workflowPlane).
		Build()

	result, err := GetObservabilityPlaneOfWorkflowPlane(context.Background(), fakeClient, workflowPlane)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "observabilityPlane 'nonexistent-observability' not found in namespace 'test-namespace'")
}

func TestGetObservabilityPlaneOfDataPlane_WithExplicitRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ObservabilityPlane
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-observability",
			Namespace: "test-namespace",
		},
	}

	// Create test DataPlane with explicit observabilityPlaneRef
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dataplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(observabilityPlane, dataPlane).
		Build()

	result, err := GetObservabilityPlaneOfDataPlane(context.Background(), fakeClient, dataPlane)

	require.NoError(t, err)
	assert.Equal(t, "my-observability", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetObservabilityPlaneOfDataPlane_WithEmptyRef_DefaultExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create default ObservabilityPlane
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
	}

	// Create DataPlane without observabilityPlaneRef
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dataplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(observabilityPlane, dataPlane).
		Build()

	result, err := GetObservabilityPlaneOfDataPlane(context.Background(), fakeClient, dataPlane)

	require.NoError(t, err)
	assert.Equal(t, "default", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetObservabilityPlaneOfDataPlane_WithEmptyRef_DefaultNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create DataPlane without observabilityPlaneRef, but no "default" ObservabilityPlane exists
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dataplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dataPlane).
		Build()

	result, err := GetObservabilityPlaneOfDataPlane(context.Background(), fakeClient, dataPlane)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no observabilityPlaneRef specified and default ObservabilityPlane 'default' not found in namespace 'test-namespace'")
}

func TestGetObservabilityPlaneOfDataPlane_WithExplicitRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create DataPlane with explicit ref that doesn't exist
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dataplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "nonexistent-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dataPlane).
		Build()

	result, err := GetObservabilityPlaneOfDataPlane(context.Background(), fakeClient, dataPlane)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "observabilityPlane 'nonexistent-observability' not found in namespace 'test-namespace'")
}

func TestGetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane_WithClusterRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-observability",
		},
	}

	// Create test WorkflowPlane with ClusterObservabilityPlane ref
	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflowplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, workflowPlane).
		Build()

	result, err := GetObservabilityPlaneOrClusterObservabilityPlaneOfWorkflowPlane(context.Background(), fakeClient, workflowPlane)

	require.NoError(t, err)
	assert.Nil(t, result.ObservabilityPlane)
	assert.NotNil(t, result.ClusterObservabilityPlane)
	assert.Equal(t, "shared-observability", result.ClusterObservabilityPlane.Name)
}

func TestGetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane_WithClusterRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-observability",
		},
	}

	// Create test DataPlane with ClusterObservabilityPlane ref
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dataplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, dataPlane).
		Build()

	result, err := GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(context.Background(), fakeClient, dataPlane)

	require.NoError(t, err)
	assert.Nil(t, result.ObservabilityPlane)
	assert.NotNil(t, result.ClusterObservabilityPlane)
	assert.Equal(t, "shared-observability", result.ClusterObservabilityPlane.Name)
}

func TestGetClusterObservabilityPlaneOfClusterDataPlane_WithExplicitRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-observability",
		},
	}

	// Create test ClusterDataPlane with explicit ref
	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterdataplane",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, clusterDataPlane).
		Build()

	result, err := GetClusterObservabilityPlaneOfClusterDataPlane(context.Background(), fakeClient, clusterDataPlane)

	require.NoError(t, err)
	assert.Equal(t, "shared-observability", result.Name)
}

func TestGetClusterObservabilityPlaneOfClusterDataPlane_WithNilRef_DefaultExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create default ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}

	// Create test ClusterDataPlane without ref
	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterdataplane",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, clusterDataPlane).
		Build()

	result, err := GetClusterObservabilityPlaneOfClusterDataPlane(context.Background(), fakeClient, clusterDataPlane)

	require.NoError(t, err)
	assert.Equal(t, "default", result.Name)
}

func TestGetClusterObservabilityPlaneOfClusterWorkflowPlane_WithExplicitRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-observability",
		},
	}

	// Create test ClusterWorkflowPlane with explicit ref
	clusterWorkflowPlane := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterworkflowplane",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, clusterWorkflowPlane).
		Build()

	result, err := GetClusterObservabilityPlaneOfClusterWorkflowPlane(context.Background(), fakeClient, clusterWorkflowPlane)

	require.NoError(t, err)
	assert.Equal(t, "shared-observability", result.Name)
}

func TestObservabilityPlaneResult_Methods(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Test with ObservabilityPlane
	opResult := &ObservabilityPlaneResult{
		ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-op",
				Namespace: "test-namespace",
			},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				ObserverURL: "http://observer.example.com",
				PlaneID:     "plane-123",
			},
		},
	}

	assert.Equal(t, "test-op", opResult.GetName())
	assert.Equal(t, "test-namespace", opResult.GetNamespace())
	assert.Equal(t, "http://observer.example.com", opResult.GetObserverURL())
	assert.Equal(t, "plane-123", opResult.GetPlaneID())

	// Test with ClusterObservabilityPlane
	copResult := &ObservabilityPlaneResult{
		ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cop",
			},
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				ObserverURL: "http://cluster-observer.example.com",
				PlaneID:     "cluster-plane-456",
			},
		},
	}

	assert.Equal(t, "test-cop", copResult.GetName())
	assert.Equal(t, "", copResult.GetNamespace())
	assert.Equal(t, "http://cluster-observer.example.com", copResult.GetObserverURL())
	assert.Equal(t, "cluster-plane-456", copResult.GetPlaneID())

	// Test with empty result
	emptyResult := &ObservabilityPlaneResult{}
	assert.Equal(t, "", emptyResult.GetName())
	assert.Equal(t, "", emptyResult.GetNamespace())
	assert.Equal(t, "", emptyResult.GetObserverURL())
	assert.Equal(t, "", emptyResult.GetPlaneID())
}

// ============================================================================
// Tests for ResolveWorkflowPlane with explicit WorkflowPlaneRef
// ============================================================================

func TestResolveWorkflowPlane_WithExplicitWorkflowPlaneRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-workflowplane",
			Namespace: "test-namespace",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workflowPlane).
		Build()

	ref := &openchoreov1alpha1.WorkflowPlaneRef{
		Kind: openchoreov1alpha1.WorkflowPlaneRefKindWorkflowPlane,
		Name: "my-workflowplane",
	}
	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", ref)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.WorkflowPlane)
	assert.Nil(t, result.ClusterWorkflowPlane)
	assert.Equal(t, "my-workflowplane", result.GetName())
	assert.Equal(t, "test-namespace", result.GetNamespace())
}

func TestResolveWorkflowPlane_WithExplicitClusterWorkflowPlaneRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	clusterWorkflowPlane := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-workflowplane",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterWorkflowPlane).
		Build()

	ref := &openchoreov1alpha1.WorkflowPlaneRef{
		Kind: openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane,
		Name: "shared-workflowplane",
	}
	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", ref)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.WorkflowPlane)
	assert.NotNil(t, result.ClusterWorkflowPlane)
	assert.Equal(t, "shared-workflowplane", result.GetName())
	assert.Equal(t, "", result.GetNamespace()) // ClusterWorkflowPlane is cluster-scoped
}

func TestResolveWorkflowPlane_WithNilRef_ReturnsError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", nil)

	// Should return an error since nil ref is no longer valid after webhook defaulting
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "workflowPlaneRef must not be nil: CRD defaulting should have set it")
}

func TestResolveWorkflowPlane_WithExplicitRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ref := &openchoreov1alpha1.WorkflowPlaneRef{
		Kind: openchoreov1alpha1.WorkflowPlaneRefKindWorkflowPlane,
		Name: "nonexistent-workflowplane",
	}
	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", ref)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "workflowPlane 'nonexistent-workflowplane' not found in namespace 'test-namespace'")
}

func TestResolveWorkflowPlane_WithExplicitClusterRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ref := &openchoreov1alpha1.WorkflowPlaneRef{
		Kind: openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane,
		Name: "nonexistent-clusterworkflowplane",
	}
	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", ref)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "clusterWorkflowPlane 'nonexistent-clusterworkflowplane' not found")
}

func TestResolveWorkflowPlane_WithUnsupportedKind(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ref := &openchoreov1alpha1.WorkflowPlaneRef{
		Kind: openchoreov1alpha1.WorkflowPlaneRefKind("UnsupportedKind"),
		Name: "some-workflowplane",
	}
	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", ref)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported")
}

// ============================================================================
// Tests for DataPlaneResult.ToDataPlane
// ============================================================================

func TestDataPlaneResult_ToDataPlane_WithDataPlane(t *testing.T) {
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-dp",
			Namespace: "test-ns",
			UID:       "dp-uid-123",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "plane-1",
		},
	}

	result := &DataPlaneResult{DataPlane: dp}
	got := result.ToDataPlane()

	// Should return the exact same pointer
	assert.Same(t, dp, got)
}

func TestDataPlaneResult_ToDataPlane_WithClusterDataPlane(t *testing.T) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp",
			UID:  "cdp-uid-456",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "shared-plane",
			Gateway: openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						Name:      "public-gw",
						Namespace: "gw-ns",
					},
				},
			},
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-obs",
			},
		},
	}

	result := &DataPlaneResult{ClusterDataPlane: cdp}
	got := result.ToDataPlane()

	require.NotNil(t, got)
	// Verify ObjectMeta fields are mapped
	assert.Equal(t, "cluster-dp", got.Name)
	assert.Equal(t, "cdp-uid-456", string(got.UID))
	assert.Equal(t, "", got.Namespace) // Cluster-scoped has no namespace

	// Verify Spec fields are mapped
	assert.Equal(t, "shared-plane", got.Spec.PlaneID)
	require.NotNil(t, got.Spec.Gateway.Ingress)
	require.NotNil(t, got.Spec.Gateway.Ingress.External)
	assert.Equal(t, "public-gw", got.Spec.Gateway.Ingress.External.Name)
	assert.Equal(t, "gw-ns", got.Spec.Gateway.Ingress.External.Namespace)

	// Verify ObservabilityPlaneRef is mapped from ClusterObservabilityPlaneRef
	require.NotNil(t, got.Spec.ObservabilityPlaneRef)
	assert.Equal(t, openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane, got.Spec.ObservabilityPlaneRef.Kind)
	assert.Equal(t, "shared-obs", got.Spec.ObservabilityPlaneRef.Name)
}

func TestDataPlaneResult_ToDataPlane_WithClusterDataPlane_NoObsRef(t *testing.T) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp-no-obs",
			UID:  "cdp-uid-789",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID:               "plane-no-obs",
			ObservabilityPlaneRef: nil,
		},
	}

	result := &DataPlaneResult{ClusterDataPlane: cdp}
	got := result.ToDataPlane()

	require.NotNil(t, got)
	assert.Equal(t, "cluster-dp-no-obs", got.Name)
	assert.Nil(t, got.Spec.ObservabilityPlaneRef)
}

func TestDataPlaneResult_ToDataPlane_NeitherSet(t *testing.T) {
	result := &DataPlaneResult{}
	got := result.ToDataPlane()

	assert.Nil(t, got)
}

// ============================================================================
// Tests for DataPlaneResult.GetObservabilityPlane
// ============================================================================

func TestDataPlaneResult_GetObservabilityPlane_WithDataPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create ObservabilityPlane that the DataPlane references
	obsPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-obs",
			Namespace: "test-ns",
		},
	}

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-dp",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-obs",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obsPlane, dp).
		Build()

	result := &DataPlaneResult{DataPlane: dp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.NotNil(t, obsResult.ObservabilityPlane)
	assert.Nil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "my-obs", obsResult.GetName())
}

func TestDataPlaneResult_GetObservabilityPlane_WithClusterDataPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create ClusterObservabilityPlane that the ClusterDataPlane references
	clusterObsPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-obs",
		},
	}

	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-obs",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObsPlane, cdp).
		Build()

	result := &DataPlaneResult{ClusterDataPlane: cdp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.Nil(t, obsResult.ObservabilityPlane)
	assert.NotNil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "shared-obs", obsResult.GetName())
}

func TestDataPlaneResult_GetObservabilityPlane_WithClusterDataPlane_DefaultObs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create "default" ClusterObservabilityPlane
	defaultClusterObs := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}

	// ClusterDataPlane without explicit obs ref — should default to "default"
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp-no-ref",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultClusterObs, cdp).
		Build()

	result := &DataPlaneResult{ClusterDataPlane: cdp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.NotNil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "default", obsResult.GetName())
}

func TestDataPlaneResult_GetObservabilityPlane_NeitherSet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result := &DataPlaneResult{}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.Error(t, err)
	assert.Nil(t, obsResult)
	assert.Contains(t, err.Error(), "no data plane set in result")
}

// ============================================================================
// Additional Tests for ResolveWorkflowPlane (nil ref returns error after CRD defaulting)
// ============================================================================

func TestResolveWorkflowPlane_NilRef_ReturnsError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result, err := ResolveWorkflowPlane(context.Background(), fakeClient, "test-namespace", nil)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "workflowPlaneRef must not be nil: CRD defaulting should have set it")
}

// ============================================================================
// Tests for WorkflowPlaneResult.GetObservabilityPlane
// ============================================================================

func TestWorkflowPlaneResult_GetObservabilityPlane_WithWorkflowPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	obsPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-obs",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			ObserverURL: "http://observer.example.com",
		},
	}

	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-wp",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-obs",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obsPlane, wp).
		Build()

	result := &WorkflowPlaneResult{WorkflowPlane: wp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.NotNil(t, obsResult.ObservabilityPlane)
	assert.Nil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "my-obs", obsResult.GetName())
	assert.Equal(t, "http://observer.example.com", obsResult.GetObserverURL())
}

func TestWorkflowPlaneResult_GetObservabilityPlane_WithClusterWorkflowPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	clusterObsPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-obs",
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			ObserverURL: "http://cluster-observer.example.com",
		},
	}

	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-wp",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-obs",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObsPlane, cwp).
		Build()

	result := &WorkflowPlaneResult{ClusterWorkflowPlane: cwp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.Nil(t, obsResult.ObservabilityPlane)
	assert.NotNil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "shared-obs", obsResult.GetName())
	assert.Equal(t, "http://cluster-observer.example.com", obsResult.GetObserverURL())
}

func TestWorkflowPlaneResult_GetObservabilityPlane_WithClusterWorkflowPlane_DefaultObs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	defaultClusterObs := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			ObserverURL: "http://default-observer.example.com",
		},
	}

	// ClusterWorkflowPlane without explicit obs ref — should default to "default"
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-wp-no-ref",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultClusterObs, cwp).
		Build()

	result := &WorkflowPlaneResult{ClusterWorkflowPlane: cwp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.NotNil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "default", obsResult.GetName())
	assert.Equal(t, "http://default-observer.example.com", obsResult.GetObserverURL())
}

func TestWorkflowPlaneResult_GetObservabilityPlane_NeitherSet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result := &WorkflowPlaneResult{}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.Error(t, err)
	assert.Nil(t, obsResult)
	assert.Contains(t, err.Error(), "no workflow plane set in result")
}

// ============================================================================
// Tests for WorkflowPlaneResult.GetSecretStoreName
// ============================================================================

func TestWorkflowPlaneResult_GetSecretStoreName_WithWorkflowPlane(t *testing.T) {
	result := &WorkflowPlaneResult{
		WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-wp",
				Namespace: "test-ns",
			},
			Spec: openchoreov1alpha1.WorkflowPlaneSpec{
				SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
					Name: "my-secret-store",
				},
			},
		},
	}

	assert.Equal(t, "my-secret-store", result.GetSecretStoreName())
}

func TestWorkflowPlaneResult_GetSecretStoreName_WithClusterWorkflowPlane(t *testing.T) {
	result := &WorkflowPlaneResult{
		ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-wp",
			},
			Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
				SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
					Name: "cluster-secret-store",
				},
			},
		},
	}

	assert.Equal(t, "cluster-secret-store", result.GetSecretStoreName())
}

func TestWorkflowPlaneResult_GetSecretStoreName_WithWorkflowPlane_NilRef(t *testing.T) {
	result := &WorkflowPlaneResult{
		WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-wp",
				Namespace: "test-ns",
			},
			Spec: openchoreov1alpha1.WorkflowPlaneSpec{
				SecretStoreRef: nil,
			},
		},
	}

	assert.Equal(t, "", result.GetSecretStoreName())
}

func TestWorkflowPlaneResult_GetSecretStoreName_WithClusterWorkflowPlane_NilRef(t *testing.T) {
	result := &WorkflowPlaneResult{
		ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-wp",
			},
			Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
				SecretStoreRef: nil,
			},
		},
	}

	assert.Equal(t, "", result.GetSecretStoreName())
}

func TestWorkflowPlaneResult_GetSecretStoreName_NeitherSet(t *testing.T) {
	result := &WorkflowPlaneResult{}
	assert.Equal(t, "", result.GetSecretStoreName())
}

// ============================================================================
// Tests for ResolveWorkflow
// ============================================================================

func TestResolveWorkflow_WithEmptyKind_ResolvesClusterScopedWorkflow(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	clusterWorkflow := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name: "docker",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterWorkflow).
		Build()

	// Empty kind should default to ClusterWorkflow
	result, err := ResolveWorkflow(context.Background(), fakeClient, "test-namespace", "", "docker")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Workflow)
	assert.NotNil(t, result.ClusterWorkflow)
	assert.Equal(t, "docker", result.GetName())
	assert.Equal(t, "", result.GetNamespace()) // Cluster-scoped has no namespace
}

func TestResolveWorkflow_WithExplicitWorkflowKind(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	workflow := &openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker",
			Namespace: "test-namespace",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workflow).
		Build()

	result, err := ResolveWorkflow(context.Background(), fakeClient, "test-namespace", openchoreov1alpha1.WorkflowRefKindWorkflow, "docker")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Workflow)
	assert.Nil(t, result.ClusterWorkflow)
	assert.Equal(t, "docker", result.GetName())
	assert.Equal(t, "test-namespace", result.GetNamespace())
}

func TestResolveWorkflow_WithClusterWorkflowKind(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	clusterWorkflow := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name: "docker",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterWorkflow).
		Build()

	result, err := ResolveWorkflow(context.Background(), fakeClient, "test-namespace", openchoreov1alpha1.WorkflowRefKindClusterWorkflow, "docker")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Workflow)
	assert.NotNil(t, result.ClusterWorkflow)
	assert.Equal(t, "docker", result.GetName())
	assert.Equal(t, "", result.GetNamespace()) // Cluster-scoped has no namespace
}

func TestResolveWorkflow_WorkflowNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result, err := ResolveWorkflow(context.Background(), fakeClient, "test-namespace", openchoreov1alpha1.WorkflowRefKindWorkflow, "nonexistent")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "workflow 'nonexistent' not found in namespace 'test-namespace'")
}

func TestResolveWorkflow_ClusterWorkflowNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result, err := ResolveWorkflow(context.Background(), fakeClient, "test-namespace", openchoreov1alpha1.WorkflowRefKindClusterWorkflow, "nonexistent")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "clusterWorkflow 'nonexistent' not found")
}

func TestResolveWorkflow_WithUnsupportedKind(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result, err := ResolveWorkflow(context.Background(), fakeClient, "test-namespace", openchoreov1alpha1.WorkflowRefKind("UnsupportedKind"), "some-workflow")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported workflowRef kind 'UnsupportedKind'")
}

// ============================================================================
// Tests for WorkflowResult methods
// ============================================================================

func TestWorkflowResult_GetWorkflowSpec_FromWorkflow(t *testing.T) {
	result := &WorkflowResult{
		Workflow: &openchoreov1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf"},
			Spec: openchoreov1alpha1.WorkflowSpec{
				TTLAfterCompletion: "1h",
			},
		},
	}

	spec := result.GetWorkflowSpec()
	assert.Equal(t, "1h", spec.TTLAfterCompletion)
}

func TestWorkflowResult_GetWorkflowSpec_FromClusterWorkflow(t *testing.T) {
	result := &WorkflowResult{
		ClusterWorkflow: &openchoreov1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "cwf"},
			Spec: openchoreov1alpha1.ClusterWorkflowSpec{
				TTLAfterCompletion: "2h",
				WorkflowPlaneRef: &openchoreov1alpha1.ClusterWorkflowPlaneRef{
					Kind: openchoreov1alpha1.ClusterWorkflowPlaneRefKindClusterWorkflowPlane,
					Name: "shared-wp",
				},
			},
		},
	}

	spec := result.GetWorkflowSpec()
	assert.Equal(t, "2h", spec.TTLAfterCompletion)
	require.NotNil(t, spec.WorkflowPlaneRef)
	assert.Equal(t, openchoreov1alpha1.WorkflowPlaneRefKind("ClusterWorkflowPlane"), spec.WorkflowPlaneRef.Kind)
	assert.Equal(t, "shared-wp", spec.WorkflowPlaneRef.Name)
}

func TestWorkflowResult_GetWorkflowSpec_FromClusterWorkflow_NilWorkflowPlaneRef(t *testing.T) {
	// When ClusterWorkflow omits WorkflowPlaneRef (e.g., legacy or before defaulting webhook),
	// GetWorkflowSpec defaults to ClusterWorkflowPlane "default" so ResolveWorkflowPlane
	// always receives a non-nil ref.
	result := &WorkflowResult{
		ClusterWorkflow: &openchoreov1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "cwf-no-wp"},
			Spec: openchoreov1alpha1.ClusterWorkflowSpec{
				TTLAfterCompletion: "1h",
			},
		},
	}

	spec := result.GetWorkflowSpec()
	assert.Equal(t, "1h", spec.TTLAfterCompletion)
	require.NotNil(t, spec.WorkflowPlaneRef)
	assert.Equal(t, openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane, spec.WorkflowPlaneRef.Kind)
	assert.Equal(t, "default", spec.WorkflowPlaneRef.Name)
}

func TestWorkflowResult_GetWorkflowSpec_Empty(t *testing.T) {
	result := &WorkflowResult{}
	spec := result.GetWorkflowSpec()
	assert.Equal(t, openchoreov1alpha1.WorkflowSpec{}, spec)
}

func TestWorkflowResult_GetName_Empty(t *testing.T) {
	result := &WorkflowResult{}
	assert.Equal(t, "", result.GetName())
	assert.Equal(t, "", result.GetNamespace())
}
