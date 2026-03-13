// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

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

func TestCreateTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		tr := testutil.NewTrait(testNamespace, "ingress")

		result, err := svc.CreateTrait(ctx, testNamespace, tr)
		require.NoError(t, err)
		assert.Equal(t, traitTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.TraitStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateTrait(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewTrait(testNamespace, "ingress")
		svc := newService(t, existing)
		tr := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress"},
		}

		_, err := svc.CreateTrait(ctx, testNamespace, tr)
		require.ErrorIs(t, err, ErrTraitAlreadyExists)
	})

	t.Run("same name in other namespace succeeds", func(t *testing.T) {
		existing := testutil.NewTrait("other-ns", "ingress")
		svc := newService(t, existing)
		tr := testutil.NewTrait(testNamespace, "ingress")

		result, err := svc.CreateTrait(ctx, testNamespace, tr)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewTrait(testNamespace, "ingress")
		svc := newService(t, existing)

		update := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ingress",
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateTrait(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, traitTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateTrait(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		tr := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateTrait(ctx, testNamespace, tr)
		require.ErrorIs(t, err, ErrTraitNotFound)
	})
}

func TestListTraits(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		tr1 := testutil.NewTrait(testNamespace, "trait-1")
		tr2 := testutil.NewTrait(testNamespace, "trait-2")
		svc := newService(t, tr1, tr2)

		result, err := svc.ListTraits(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, traitTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListTraits(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListTraits(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		trInNs := testutil.NewTrait(testNamespace, "trait-in")
		trOtherNs := testutil.NewTrait("other-ns", "trait-out")
		svc := newService(t, trInNs, trOtherNs)

		result, err := svc.ListTraits(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "trait-in", result.Items[0].Name)
	})
}

func TestGetTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		tr := testutil.NewTrait(testNamespace, "ingress")
		svc := newService(t, tr)

		result, err := svc.GetTrait(ctx, testNamespace, "ingress")
		require.NoError(t, err)
		assert.Equal(t, traitTypeMeta, result.TypeMeta)
		assert.Equal(t, "ingress", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetTrait(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrTraitNotFound)
	})
}

func TestDeleteTrait(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		tr := testutil.NewTrait(testNamespace, "ingress")
		svc := newService(t, tr)

		err := svc.DeleteTrait(ctx, testNamespace, "ingress")
		require.NoError(t, err)

		_, err = svc.GetTrait(ctx, testNamespace, "ingress")
		require.ErrorIs(t, err, ErrTraitNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteTrait(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrTraitNotFound)
	})
}

func TestGetTraitSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		tr := testutil.NewTrait(testNamespace, "no-params")
		svc := newService(t, tr)

		result, err := svc.GetTraitSchema(ctx, testNamespace, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OCSchema", func(t *testing.T) {
		tr := testutil.NewTrait(testNamespace, "with-schema")
		tr.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OCSchema: &runtime.RawExtension{Raw: []byte(`{"replicas":"integer"}`)},
		}
		svc := newService(t, tr)

		result, err := svc.GetTraitSchema(ctx, testNamespace, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "replicas")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetTraitSchema(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrTraitNotFound)
	})

	t.Run("invalid schema", func(t *testing.T) {
		tr := testutil.NewTrait(testNamespace, "bad-schema")
		tr.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OCSchema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, tr)

		_, err := svc.GetTraitSchema(ctx, testNamespace, "bad-schema")
		require.Error(t, err)
	})
}
