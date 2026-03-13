// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const testNamespace = "test-ns"

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		ct := testutil.NewComponentType(testNamespace, "web-app")

		result, err := svc.CreateComponentType(ctx, testNamespace, ct)
		require.NoError(t, err)
		assert.Equal(t, componentTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ComponentTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateComponentType(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewComponentType(testNamespace, "web-app")
		svc := newService(t, existing)
		ct := &openchoreov1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "web-app"},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateComponentType(ctx, testNamespace, ct)
		require.ErrorIs(t, err, ErrComponentTypeAlreadyExists)
	})

	t.Run("same name in other namespace succeeds", func(t *testing.T) {
		existing := testutil.NewComponentType("other-ns", "web-app")
		svc := newService(t, existing)
		ct := testutil.NewComponentType(testNamespace, "web-app")

		result, err := svc.CreateComponentType(ctx, testNamespace, ct)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewComponentType(testNamespace, "web-app")
		svc := newService(t, existing)

		update := &openchoreov1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "web-app",
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateComponentType(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, componentTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateComponentType(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		ct := &openchoreov1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateComponentType(ctx, testNamespace, ct)
		require.ErrorIs(t, err, ErrComponentTypeNotFound)
	})
}

func TestListComponentTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		ct1 := testutil.NewComponentType(testNamespace, "ct-1")
		ct2 := testutil.NewComponentType(testNamespace, "ct-2")
		svc := newService(t, ct1, ct2)

		result, err := svc.ListComponentTypes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, componentTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListComponentTypes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListComponentTypes(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		ctInNs := testutil.NewComponentType(testNamespace, "ct-in")
		ctOtherNs := testutil.NewComponentType("other-ns", "ct-out")
		svc := newService(t, ctInNs, ctOtherNs)

		result, err := svc.ListComponentTypes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "ct-in", result.Items[0].Name)
	})
}

func TestGetComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		ct := testutil.NewComponentType(testNamespace, "web-app")
		svc := newService(t, ct)

		result, err := svc.GetComponentType(ctx, testNamespace, "web-app")
		require.NoError(t, err)
		assert.Equal(t, componentTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "web-app", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetComponentType(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentTypeNotFound)
	})
}

func TestDeleteComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		ct := testutil.NewComponentType(testNamespace, "web-app")
		svc := newService(t, ct)

		err := svc.DeleteComponentType(ctx, testNamespace, "web-app")
		require.NoError(t, err)

		_, err = svc.GetComponentType(ctx, testNamespace, "web-app")
		require.ErrorIs(t, err, ErrComponentTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteComponentType(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentTypeNotFound)
	})
}

func TestGetComponentTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		ct := testutil.NewComponentType(testNamespace, "no-params")
		svc := newService(t, ct)

		result, err := svc.GetComponentTypeSchema(ctx, testNamespace, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OCSchema", func(t *testing.T) {
		ct := testutil.NewComponentType(testNamespace, "with-schema")
		ct.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OCSchema: &runtime.RawExtension{Raw: []byte(`{"replicas":"integer"}`)},
		}
		svc := newService(t, ct)

		result, err := svc.GetComponentTypeSchema(ctx, testNamespace, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "replicas")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetComponentTypeSchema(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentTypeNotFound)
	})

	t.Run("invalid schema", func(t *testing.T) {
		ct := testutil.NewComponentType(testNamespace, "bad-schema")
		ct.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OCSchema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, ct)

		_, err := svc.GetComponentTypeSchema(ctx, testNamespace, "bad-schema")
		require.Error(t, err)
	})
}
