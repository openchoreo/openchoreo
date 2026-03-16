// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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

func TestCreateClusterTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		ct := testutil.NewClusterTrait("test-trait")

		result, err := svc.CreateClusterTrait(ctx, ct)
		require.NoError(t, err)
		assert.Equal(t, clusterTraitTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-trait", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterTraitStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterTrait(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterTrait("dup-trait")
		svc := newService(t, existing)
		dup := testutil.NewClusterTrait("dup-trait")

		_, err := svc.CreateClusterTrait(ctx, dup)
		require.ErrorIs(t, err, ErrClusterTraitAlreadyExists)
	})
}

func TestUpdateClusterTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterTrait("test-trait")
		svc := newService(t, existing)

		update := testutil.NewClusterTrait("test-trait")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterTrait(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterTraitTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterTrait(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		ct := testutil.NewClusterTrait("nonexistent")

		_, err := svc.UpdateClusterTrait(ctx, ct)
		require.ErrorIs(t, err, ErrClusterTraitNotFound)
	})
}

func TestListClusterTraits(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		ct1 := testutil.NewClusterTrait("trait-1")
		ct2 := testutil.NewClusterTrait("trait-2")
		svc := newService(t, ct1, ct2)

		result, err := svc.ListClusterTraits(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterTraitTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterTraits(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterTraits(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		ct := testutil.NewClusterTrait("test-trait")
		svc := newService(t, ct)

		result, err := svc.GetClusterTrait(ctx, "test-trait")
		require.NoError(t, err)
		assert.Equal(t, clusterTraitTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-trait", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterTrait(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterTraitNotFound)
	})
}

func TestDeleteClusterTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		ct := testutil.NewClusterTrait("test-trait")
		svc := newService(t, ct)

		err := svc.DeleteClusterTrait(ctx, "test-trait")
		require.NoError(t, err)

		_, err = svc.GetClusterTrait(ctx, "test-trait")
		require.ErrorIs(t, err, ErrClusterTraitNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterTrait(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterTraitNotFound)
	})
}

func TestGetClusterTraitSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		ct := testutil.NewClusterTrait("no-params")
		svc := newService(t, ct)

		result, err := svc.GetClusterTraitSchema(ctx, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		ct := testutil.NewClusterTrait("with-schema")
		ct.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer"}}}`)},
		}
		svc := newService(t, ct)

		result, err := svc.GetClusterTraitSchema(ctx, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "replicas")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterTraitSchema(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterTraitNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		ct := testutil.NewClusterTrait("bad-schema")
		ct.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, ct)

		_, err := svc.GetClusterTraitSchema(ctx, "bad-schema")
		require.Error(t, err)
	})
}
