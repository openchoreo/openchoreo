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
	assert.Contains(t, err.Error(), "no dataPlaneRef specified and default DataPlane 'default' not found in namespace 'test-namespace'")
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

func TestGetObservabilityPlaneOfBuildPlane_WithExplicitRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ObservabilityPlane
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-observability",
			Namespace: "test-namespace",
		},
	}

	// Create test BuildPlane with explicit observabilityPlaneRef
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-buildplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(observabilityPlane, buildPlane).
		Build()

	result, err := GetObservabilityPlaneOfBuildPlane(context.Background(), fakeClient, buildPlane)

	require.NoError(t, err)
	assert.Equal(t, "my-observability", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetObservabilityPlaneOfBuildPlane_WithEmptyRef_DefaultExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create default ObservabilityPlane
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
	}

	// Create BuildPlane without observabilityPlaneRef
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-buildplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(observabilityPlane, buildPlane).
		Build()

	result, err := GetObservabilityPlaneOfBuildPlane(context.Background(), fakeClient, buildPlane)

	require.NoError(t, err)
	assert.Equal(t, "default", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestGetObservabilityPlaneOfBuildPlane_WithEmptyRef_DefaultNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create BuildPlane without observabilityPlaneRef, but no "default" ObservabilityPlane exists
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-buildplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buildPlane).
		Build()

	result, err := GetObservabilityPlaneOfBuildPlane(context.Background(), fakeClient, buildPlane)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no observabilityPlaneRef specified and default ObservabilityPlane 'default' not found in namespace 'test-namespace'")
}

func TestGetObservabilityPlaneOfBuildPlane_WithExplicitRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create BuildPlane with explicit ref that doesn't exist
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-buildplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "nonexistent-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buildPlane).
		Build()

	result, err := GetObservabilityPlaneOfBuildPlane(context.Background(), fakeClient, buildPlane)

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

func TestGetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane_WithClusterRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-observability",
		},
	}

	// Create test BuildPlane with ClusterObservabilityPlane ref
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-buildplane",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, buildPlane).
		Build()

	result, err := GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane(context.Background(), fakeClient, buildPlane)

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

func TestGetClusterObservabilityPlaneOfClusterBuildPlane_WithExplicitRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterObservabilityPlane
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-observability",
		},
	}

	// Create test ClusterBuildPlane with explicit ref
	clusterBuildPlane := &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterbuildplane",
		},
		Spec: openchoreov1alpha1.ClusterBuildPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-observability",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObservabilityPlane, clusterBuildPlane).
		Build()

	result, err := GetClusterObservabilityPlaneOfClusterBuildPlane(context.Background(), fakeClient, clusterBuildPlane)

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
// Tests for GetBuildPlaneOrClusterBuildPlaneOfProject
// ============================================================================

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithExplicitBuildPlaneRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test BuildPlane
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-buildplane",
			Namespace: "test-namespace",
		},
	}

	// Create test Project with explicit buildPlaneRef
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef: &openchoreov1alpha1.BuildPlaneRef{
				Kind: openchoreov1alpha1.BuildPlaneRefKindBuildPlane,
				Name: "my-buildplane",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buildPlane, project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.BuildPlane)
	assert.Nil(t, result.ClusterBuildPlane)
	assert.Equal(t, "my-buildplane", result.GetName())
	assert.Equal(t, "test-namespace", result.GetNamespace())
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithExplicitClusterBuildPlaneRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create test ClusterBuildPlane
	clusterBuildPlane := &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-buildplane",
		},
	}

	// Create test Project with explicit ClusterBuildPlane ref
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef: &openchoreov1alpha1.BuildPlaneRef{
				Kind: openchoreov1alpha1.BuildPlaneRefKindClusterBuildPlane,
				Name: "shared-buildplane",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterBuildPlane, project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.BuildPlane)
	assert.NotNil(t, result.ClusterBuildPlane)
	assert.Equal(t, "shared-buildplane", result.GetName())
	assert.Equal(t, "", result.GetNamespace()) // ClusterBuildPlane is cluster-scoped
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithNoRef_DefaultExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create default BuildPlane
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
	}

	// Create Project without buildPlaneRef
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef:         nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buildPlane, project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.BuildPlane)
	assert.Equal(t, "default", result.GetName())
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithNoRef_DefaultClusterBuildPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Project without buildPlaneRef
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef:         nil,
		},
	}

	// Create "default" ClusterBuildPlane (no namespace BuildPlane)
	clusterBuildPlane := &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: openchoreov1alpha1.ClusterBuildPlaneSpec{
			PlaneID: "test-plane",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project, clusterBuildPlane).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.BuildPlane)
	assert.NotNil(t, result.ClusterBuildPlane)
	assert.Equal(t, "default", result.ClusterBuildPlane.Name)
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithNoRef_FallbackToFirst(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create a non-default BuildPlane
	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-buildplane",
			Namespace: "test-namespace",
		},
	}

	// Create Project without buildPlaneRef
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef:         nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buildPlane, project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.BuildPlane)
	assert.Equal(t, "other-buildplane", result.GetName())
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithNoRef_NoBuildPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Project without buildPlaneRef, and no BuildPlane exists
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef:         nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	// Should return nil without error (BuildPlane is optional for Projects)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithExplicitRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Project with explicit ref that doesn't exist
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef: &openchoreov1alpha1.BuildPlaneRef{
				Kind: openchoreov1alpha1.BuildPlaneRefKindBuildPlane,
				Name: "nonexistent-buildplane",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "buildPlane 'nonexistent-buildplane' not found in namespace 'test-namespace'")
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithExplicitClusterRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Project with explicit ClusterBuildPlane ref that doesn't exist
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef: &openchoreov1alpha1.BuildPlaneRef{
				Kind: openchoreov1alpha1.BuildPlaneRefKindClusterBuildPlane,
				Name: "nonexistent-clusterbuildplane",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "clusterBuildPlane 'nonexistent-clusterbuildplane' not found")
}

func TestGetBuildPlaneOrClusterBuildPlaneOfProject_WithUnsupportedKind(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Create Project with unsupported Kind
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef: &openchoreov1alpha1.BuildPlaneRef{
				Kind: openchoreov1alpha1.BuildPlaneRefKind("UnsupportedKind"),
				Name: "some-buildplane",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	result, err := GetBuildPlaneOrClusterBuildPlaneOfProject(context.Background(), fakeClient, project)

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
			Gateway: openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost: "public.example.com",
			},
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
				PublicVirtualHost:       "public.cluster.example.com",
				OrganizationVirtualHost: "org.cluster.example.com",
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
	assert.Equal(t, "public.cluster.example.com", got.Spec.Gateway.PublicVirtualHost)
	assert.Equal(t, "org.cluster.example.com", got.Spec.Gateway.OrganizationVirtualHost)

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
// Tests for FindProjectByName
// ============================================================================

func TestFindProjectByName_Found(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"openchoreo.dev/name": "test-project",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	result, err := FindProjectByName(context.Background(), fakeClient, "test-namespace", "test-project")

	require.NoError(t, err)
	assert.Equal(t, "test-project", result.Name)
	assert.Equal(t, "test-namespace", result.Namespace)
}

func TestFindProjectByName_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result, err := FindProjectByName(context.Background(), fakeClient, "test-namespace", "nonexistent")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "project 'nonexistent' not found in namespace 'test-namespace'")
}

func TestFindProjectByName_WrongNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "other-namespace",
			Labels: map[string]string{
				"openchoreo.dev/name": "test-project",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	result, err := FindProjectByName(context.Background(), fakeClient, "test-namespace", "test-project")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "project 'test-project' not found in namespace 'test-namespace'")
}

// ============================================================================
// Tests for ResolveBuildPlane
// ============================================================================

func TestResolveBuildPlane_WithProjectLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	buildPlane := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-bp",
			Namespace: "test-namespace",
		},
	}

	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-project",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"openchoreo.dev/name":      "test-project",
				"openchoreo.dev/namespace": "test-namespace",
			},
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default",
			BuildPlaneRef: &openchoreov1alpha1.BuildPlaneRef{
				Kind: openchoreov1alpha1.BuildPlaneRefKindBuildPlane,
				Name: "my-bp",
			},
		},
	}

	// Object with hierarchy labels that GetProject can use
	obj := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"openchoreo.dev/namespace": "test-namespace",
				"openchoreo.dev/project":   "test-project",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buildPlane, project, obj).
		Build()

	result, err := ResolveBuildPlane(context.Background(), fakeClient, obj)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.BuildPlane)
	assert.Equal(t, "my-bp", result.GetName())
}

func TestResolveBuildPlane_WithoutProjectLabels_FallsBackToDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	defaultBP := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test-namespace",
		},
	}

	// Object without hierarchy labels
	obj := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test-namespace",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultBP, obj).
		Build()

	result, err := ResolveBuildPlane(context.Background(), fakeClient, obj)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.BuildPlane)
	assert.Equal(t, "default", result.GetName())
}

func TestResolveBuildPlane_WithoutProjectLabels_FallsBackToClusterBuildPlane(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	clusterBP := &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}

	// Object without hierarchy labels and no namespace-scoped BuildPlane
	obj := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test-namespace",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterBP, obj).
		Build()

	result, err := ResolveBuildPlane(context.Background(), fakeClient, obj)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.ClusterBuildPlane)
	assert.Equal(t, "default", result.GetName())
}

func TestResolveBuildPlane_NoBuildPlaneExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	obj := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test-namespace",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	result, err := ResolveBuildPlane(context.Background(), fakeClient, obj)

	require.NoError(t, err)
	assert.Nil(t, result)
}

// ============================================================================
// Tests for BuildPlaneResult.GetObservabilityPlane
// ============================================================================

func TestBuildPlaneResult_GetObservabilityPlane_WithBuildPlane(t *testing.T) {
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

	bp := &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-bp",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-obs",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obsPlane, bp).
		Build()

	result := &BuildPlaneResult{BuildPlane: bp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.NotNil(t, obsResult.ObservabilityPlane)
	assert.Nil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "my-obs", obsResult.GetName())
	assert.Equal(t, "http://observer.example.com", obsResult.GetObserverURL())
}

func TestBuildPlaneResult_GetObservabilityPlane_WithClusterBuildPlane(t *testing.T) {
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

	cbp := &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-bp",
		},
		Spec: openchoreov1alpha1.ClusterBuildPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-obs",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterObsPlane, cbp).
		Build()

	result := &BuildPlaneResult{ClusterBuildPlane: cbp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.Nil(t, obsResult.ObservabilityPlane)
	assert.NotNil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "shared-obs", obsResult.GetName())
	assert.Equal(t, "http://cluster-observer.example.com", obsResult.GetObserverURL())
}

func TestBuildPlaneResult_GetObservabilityPlane_WithClusterBuildPlane_DefaultObs(t *testing.T) {
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

	// ClusterBuildPlane without explicit obs ref — should default to "default"
	cbp := &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-bp-no-ref",
		},
		Spec: openchoreov1alpha1.ClusterBuildPlaneSpec{
			ObservabilityPlaneRef: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultClusterObs, cbp).
		Build()

	result := &BuildPlaneResult{ClusterBuildPlane: cbp}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.NoError(t, err)
	require.NotNil(t, obsResult)
	assert.NotNil(t, obsResult.ClusterObservabilityPlane)
	assert.Equal(t, "default", obsResult.GetName())
	assert.Equal(t, "http://default-observer.example.com", obsResult.GetObserverURL())
}

func TestBuildPlaneResult_GetObservabilityPlane_NeitherSet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	result := &BuildPlaneResult{}
	obsResult, err := result.GetObservabilityPlane(context.Background(), fakeClient)

	require.Error(t, err)
	assert.Nil(t, obsResult)
	assert.Contains(t, err.Error(), "no build plane set in result")
}
