// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

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

func TestCreateClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cpt := testutil.NewClusterProjectType("test-cpt")

		result, err := svc.CreateClusterProjectType(ctx, cpt)
		require.NoError(t, err)
		assert.Equal(t, clusterProjectTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cpt", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterProjectTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterProjectType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterProjectType("dup-cpt")
		svc := newService(t, existing)
		dup := testutil.NewClusterProjectType("dup-cpt")

		_, err := svc.CreateClusterProjectType(ctx, dup)
		require.ErrorIs(t, err, ErrClusterProjectTypeAlreadyExists)
	})
}

func TestUpdateClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterProjectType("test-cpt")
		svc := newService(t, existing)

		update := testutil.NewClusterProjectType("test-cpt")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterProjectType(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterProjectTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterProjectType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		cpt := testutil.NewClusterProjectType("nonexistent")

		_, err := svc.UpdateClusterProjectType(ctx, cpt)
		require.ErrorIs(t, err, ErrClusterProjectTypeNotFound)
	})
}

func TestListClusterProjectTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		cpt1 := testutil.NewClusterProjectType("cpt-1")
		cpt2 := testutil.NewClusterProjectType("cpt-2")
		svc := newService(t, cpt1, cpt2)

		result, err := svc.ListClusterProjectTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterProjectTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterProjectTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterProjectTypes(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cpt := testutil.NewClusterProjectType("test-cpt")
		svc := newService(t, cpt)

		result, err := svc.GetClusterProjectType(ctx, "test-cpt")
		require.NoError(t, err)
		assert.Equal(t, clusterProjectTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cpt", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterProjectType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterProjectTypeNotFound)
	})
}

func TestDeleteClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cpt := testutil.NewClusterProjectType("test-cpt")
		svc := newService(t, cpt)

		err := svc.DeleteClusterProjectType(ctx, "test-cpt")
		require.NoError(t, err)

		_, err = svc.GetClusterProjectType(ctx, "test-cpt")
		require.ErrorIs(t, err, ErrClusterProjectTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterProjectType(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterProjectTypeNotFound)
	})
}

func TestGetClusterProjectTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		cpt := testutil.NewClusterProjectType("no-params")
		svc := newService(t, cpt)

		result, err := svc.GetClusterProjectTypeSchema(ctx, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		cpt := testutil.NewClusterProjectType("with-schema")
		cpt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"tier":{"type":"string"}}}`)},
		}
		svc := newService(t, cpt)

		result, err := svc.GetClusterProjectTypeSchema(ctx, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "tier")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterProjectTypeSchema(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterProjectTypeNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		cpt := testutil.NewClusterProjectType("bad-schema")
		cpt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, cpt)

		_, err := svc.GetClusterProjectTypeSchema(ctx, "bad-schema")
		require.Error(t, err)
	})
}
