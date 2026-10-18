// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

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

func TestCreateEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		rt := testutil.NewEndpointType("ns-a", "test-rt")

		result, err := svc.CreateEndpointType(ctx, "ns-a", rt)
		require.NoError(t, err)
		assert.Equal(t, endpointTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-rt", result.Name)
		assert.Equal(t, "ns-a", result.Namespace)
		assert.Equal(t, openchoreov1alpha1.EndpointTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateEndpointType(ctx, "ns-a", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewEndpointType("ns-a", "dup-rt")
		svc := newService(t, existing)
		dup := testutil.NewEndpointType("ns-a", "dup-rt")

		_, err := svc.CreateEndpointType(ctx, "ns-a", dup)
		require.ErrorIs(t, err, ErrEndpointTypeAlreadyExists)
	})

	t.Run("namespace overrides body namespace", func(t *testing.T) {
		svc := newService(t)
		rt := testutil.NewEndpointType("ignored", "test-rt")

		result, err := svc.CreateEndpointType(ctx, "ns-a", rt)
		require.NoError(t, err)
		assert.Equal(t, "ns-a", result.Namespace)
	})
}

func TestUpdateEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewEndpointType("ns-a", "test-rt")
		svc := newService(t, existing)

		update := testutil.NewEndpointType("ns-a", "test-rt")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateEndpointType(ctx, "ns-a", update)
		require.NoError(t, err)
		assert.Equal(t, endpointTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateEndpointType(ctx, "ns-a", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		rt := testutil.NewEndpointType("ns-a", "nonexistent")

		_, err := svc.UpdateEndpointType(ctx, "ns-a", rt)
		require.ErrorIs(t, err, ErrEndpointTypeNotFound)
	})
}

func TestListEndpointTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		rt1 := testutil.NewEndpointType("ns-a", "rt-1")
		rt2 := testutil.NewEndpointType("ns-a", "rt-2")
		svc := newService(t, rt1, rt2)

		result, err := svc.ListEndpointTypes(ctx, "ns-a", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, endpointTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("filters by namespace", func(t *testing.T) {
		rt1 := testutil.NewEndpointType("ns-a", "rt-a")
		rt2 := testutil.NewEndpointType("ns-b", "rt-b")
		svc := newService(t, rt1, rt2)

		result, err := svc.ListEndpointTypes(ctx, "ns-a", services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "rt-a", result.Items[0].Name)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListEndpointTypes(ctx, "ns-a", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpointTypes(ctx, "ns-a", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rt := testutil.NewEndpointType("ns-a", "test-rt")
		svc := newService(t, rt)

		result, err := svc.GetEndpointType(ctx, "ns-a", "test-rt")
		require.NoError(t, err)
		assert.Equal(t, endpointTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-rt", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEndpointType(ctx, "ns-a", "nonexistent")
		require.ErrorIs(t, err, ErrEndpointTypeNotFound)
	})
}

func TestDeleteEndpointType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rt := testutil.NewEndpointType("ns-a", "test-rt")
		svc := newService(t, rt)

		err := svc.DeleteEndpointType(ctx, "ns-a", "test-rt")
		require.NoError(t, err)

		_, err = svc.GetEndpointType(ctx, "ns-a", "test-rt")
		require.ErrorIs(t, err, ErrEndpointTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteEndpointType(ctx, "ns-a", "nonexistent")
		require.ErrorIs(t, err, ErrEndpointTypeNotFound)
	})
}

func TestGetEndpointTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		rt := testutil.NewEndpointType("ns-a", "no-params")
		svc := newService(t, rt)

		result, err := svc.GetEndpointTypeSchema(ctx, "ns-a", "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		rt := testutil.NewEndpointType("ns-a", "with-schema")
		rt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"version":{"type":"string"}}}`)},
		}
		svc := newService(t, rt)

		result, err := svc.GetEndpointTypeSchema(ctx, "ns-a", "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "version")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEndpointTypeSchema(ctx, "ns-a", "nonexistent")
		require.ErrorIs(t, err, ErrEndpointTypeNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		rt := testutil.NewEndpointType("ns-a", "bad-schema")
		rt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, rt)

		_, err := svc.GetEndpointTypeSchema(ctx, "ns-a", "bad-schema")
		require.Error(t, err)
	})
}
