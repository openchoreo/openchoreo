// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateClusterComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cct := testutil.NewClusterComponentType("test-cct")

		result, err := svc.CreateClusterComponentType(ctx, cct)
		require.NoError(t, err)
		assert.Equal(t, clusterComponentTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cct", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterComponentTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterComponentType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterComponentType("dup-cct")
		svc := newService(t, existing)
		dup := testutil.NewClusterComponentType("dup-cct")

		_, err := svc.CreateClusterComponentType(ctx, dup)
		require.ErrorIs(t, err, ErrClusterComponentTypeAlreadyExists)
	})
}

func TestUpdateClusterComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterComponentType("test-cct")
		svc := newService(t, existing)

		update := testutil.NewClusterComponentType("test-cct")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterComponentType(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterComponentTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterComponentType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		cct := testutil.NewClusterComponentType("nonexistent")

		_, err := svc.UpdateClusterComponentType(ctx, cct)
		require.ErrorIs(t, err, ErrClusterComponentTypeNotFound)
	})
}

func TestListClusterComponentTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		cct1 := testutil.NewClusterComponentType("cct-1")
		cct2 := testutil.NewClusterComponentType("cct-2")
		svc := newService(t, cct1, cct2)

		result, err := svc.ListClusterComponentTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterComponentTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterComponentTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterComponentTypes(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cct := testutil.NewClusterComponentType("test-cct")
		svc := newService(t, cct)

		result, err := svc.GetClusterComponentType(ctx, "test-cct")
		require.NoError(t, err)
		assert.Equal(t, clusterComponentTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cct", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterComponentType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterComponentTypeNotFound)
	})
}

func TestDeleteClusterComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cct := testutil.NewClusterComponentType("test-cct")
		svc := newService(t, cct)

		err := svc.DeleteClusterComponentType(ctx, "test-cct")
		require.NoError(t, err)

		_, err = svc.GetClusterComponentType(ctx, "test-cct")
		require.ErrorIs(t, err, ErrClusterComponentTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterComponentType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterComponentTypeNotFound)
	})
}

func TestGetClusterComponentTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		cct := testutil.NewClusterComponentType("no-params")
		svc := newService(t, cct)

		result, err := svc.GetClusterComponentTypeSchema(ctx, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with shorthand schema params", func(t *testing.T) {
		cct := testutil.NewClusterComponentType("with-schema")
		cct.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"replicas":"integer"}`)},
		}
		svc := newService(t, cct)

		result, err := svc.GetClusterComponentTypeSchema(ctx, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "replicas")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterComponentTypeSchema(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterComponentTypeNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		cct := testutil.NewClusterComponentType("bad-schema")
		cct.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, cct)

		_, err := svc.GetClusterComponentTypeSchema(ctx, "bad-schema")
		require.Error(t, err)
	})
}
