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
			ObservabilityPlaneRef: "my-observability",
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
			ObservabilityPlaneRef: "",
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
			ObservabilityPlaneRef: "",
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
			ObservabilityPlaneRef: "nonexistent-observability",
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
			ObservabilityPlaneRef: "my-observability",
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
			ObservabilityPlaneRef: "",
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
			ObservabilityPlaneRef: "",
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
			ObservabilityPlaneRef: "nonexistent-observability",
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
