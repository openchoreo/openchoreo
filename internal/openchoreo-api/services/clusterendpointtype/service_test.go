// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

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

func TestCreateClusterEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		crt := testutil.NewClusterEndpointType("test-crt")

		result, err := svc.CreateClusterEndpointType(ctx, crt)
		require.NoError(t, err)
		assert.Equal(t, clusterEndpointTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-crt", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterEndpointTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterEndpointType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterEndpointType("dup-crt")
		svc := newService(t, existing)
		dup := testutil.NewClusterEndpointType("dup-crt")

		_, err := svc.CreateClusterEndpointType(ctx, dup)
		require.ErrorIs(t, err, ErrClusterEndpointTypeAlreadyExists)
	})
}

func TestUpdateClusterEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterEndpointType("test-crt")
		svc := newService(t, existing)

		update := testutil.NewClusterEndpointType("test-crt")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterEndpointType(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterEndpointTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterEndpointType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		crt := testutil.NewClusterEndpointType("nonexistent")

		_, err := svc.UpdateClusterEndpointType(ctx, crt)
		require.ErrorIs(t, err, ErrClusterEndpointTypeNotFound)
	})
}

func TestListClusterEndpointTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		crt1 := testutil.NewClusterEndpointType("crt-1")
		crt2 := testutil.NewClusterEndpointType("crt-2")
		svc := newService(t, crt1, crt2)

		result, err := svc.ListClusterEndpointTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterEndpointTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterEndpointTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterEndpointTypes(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		crt := testutil.NewClusterEndpointType("test-crt")
		svc := newService(t, crt)

		result, err := svc.GetClusterEndpointType(ctx, "test-crt")
		require.NoError(t, err)
		assert.Equal(t, clusterEndpointTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-crt", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterEndpointType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterEndpointTypeNotFound)
	})
}

func TestDeleteClusterEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		crt := testutil.NewClusterEndpointType("test-crt")
		svc := newService(t, crt)

		err := svc.DeleteClusterEndpointType(ctx, "test-crt")
		require.NoError(t, err)

		_, err = svc.GetClusterEndpointType(ctx, "test-crt")
		require.ErrorIs(t, err, ErrClusterEndpointTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterEndpointType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterEndpointTypeNotFound)
	})
}

func TestGetClusterEndpointTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		crt := testutil.NewClusterEndpointType("no-params")
		svc := newService(t, crt)

		result, err := svc.GetClusterEndpointTypeSchema(ctx, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		crt := testutil.NewClusterEndpointType("with-schema")
		crt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"version":{"type":"string"}}}`)},
		}
		svc := newService(t, crt)

		result, err := svc.GetClusterEndpointTypeSchema(ctx, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "version")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterEndpointTypeSchema(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterEndpointTypeNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		crt := testutil.NewClusterEndpointType("bad-schema")
		crt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, crt)

		_, err := svc.GetClusterEndpointTypeSchema(ctx, "bad-schema")
		require.Error(t, err)
	})
}
