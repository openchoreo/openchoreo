// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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

func TestCreateClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		crt := testutil.NewClusterResourceType("test-crt")

		result, err := svc.CreateClusterResourceType(ctx, crt)
		require.NoError(t, err)
		assert.Equal(t, clusterResourceTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-crt", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterResourceTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterResourceType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterResourceType("dup-crt")
		svc := newService(t, existing)
		dup := testutil.NewClusterResourceType("dup-crt")

		_, err := svc.CreateClusterResourceType(ctx, dup)
		require.ErrorIs(t, err, ErrClusterResourceTypeAlreadyExists)
	})
}

func TestUpdateClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterResourceType("test-crt")
		svc := newService(t, existing)

		update := testutil.NewClusterResourceType("test-crt")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterResourceType(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterResourceTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterResourceType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		crt := testutil.NewClusterResourceType("nonexistent")

		_, err := svc.UpdateClusterResourceType(ctx, crt)
		require.ErrorIs(t, err, ErrClusterResourceTypeNotFound)
	})
}

func TestListClusterResourceTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		crt1 := testutil.NewClusterResourceType("crt-1")
		crt2 := testutil.NewClusterResourceType("crt-2")
		svc := newService(t, crt1, crt2)

		result, err := svc.ListClusterResourceTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterResourceTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterResourceTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterResourceTypes(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		crt := testutil.NewClusterResourceType("test-crt")
		svc := newService(t, crt)

		result, err := svc.GetClusterResourceType(ctx, "test-crt")
		require.NoError(t, err)
		assert.Equal(t, clusterResourceTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-crt", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterResourceType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterResourceTypeNotFound)
	})
}

func TestDeleteClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		crt := testutil.NewClusterResourceType("test-crt")
		svc := newService(t, crt)

		err := svc.DeleteClusterResourceType(ctx, "test-crt")
		require.NoError(t, err)

		_, err = svc.GetClusterResourceType(ctx, "test-crt")
		require.ErrorIs(t, err, ErrClusterResourceTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterResourceType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterResourceTypeNotFound)
	})
}

func TestGetClusterResourceTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		crt := testutil.NewClusterResourceType("no-params")
		svc := newService(t, crt)

		result, err := svc.GetClusterResourceTypeSchema(ctx, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		crt := testutil.NewClusterResourceType("with-schema")
		crt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"version":{"type":"string"}}}`)},
		}
		svc := newService(t, crt)

		result, err := svc.GetClusterResourceTypeSchema(ctx, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "version")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterResourceTypeSchema(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterResourceTypeNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		crt := testutil.NewClusterResourceType("bad-schema")
		crt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, crt)

		_, err := svc.GetClusterResourceTypeSchema(ctx, "bad-schema")
		require.Error(t, err)
	})
}
