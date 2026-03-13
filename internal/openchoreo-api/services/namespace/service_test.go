// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func controlPlaneNamespace(name string) *corev1.Namespace {
	ns := testutil.NewNamespace(name)
	ns.Labels = map[string]string{
		labels.LabelKeyControlPlaneNamespace: labels.LabelValueTrue,
	}
	return ns
}

func TestCreateNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		ns := testutil.NewNamespace("test-ns")

		result, err := svc.CreateNamespace(ctx, ns)
		require.NoError(t, err)
		assert.Equal(t, labels.LabelValueTrue, result.Labels[labels.LabelKeyControlPlaneNamespace])
		assert.NotNil(t, result.Annotations)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateNamespace(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := controlPlaneNamespace("test-ns")
		svc := newService(t, existing)
		ns := testutil.NewNamespace("test-ns")

		_, err := svc.CreateNamespace(ctx, ns)
		require.ErrorIs(t, err, ErrNamespaceAlreadyExists)
	})
}

func TestUpdateNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := controlPlaneNamespace("test-ns")
		existing.Annotations = map[string]string{
			"existing-key": "existing-value",
		}
		svc := newService(t, existing)

		update := testutil.NewNamespace("test-ns")
		update.Annotations = map[string]string{
			controller.AnnotationKeyDisplayName: "My Namespace",
			controller.AnnotationKeyDescription: "A test namespace",
		}

		result, err := svc.UpdateNamespace(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, labels.LabelValueTrue, result.Labels[labels.LabelKeyControlPlaneNamespace])
		assert.Equal(t, "My Namespace", result.Annotations[controller.AnnotationKeyDisplayName])
		assert.Equal(t, "A test namespace", result.Annotations[controller.AnnotationKeyDescription])
		assert.Equal(t, "existing-value", result.Annotations["existing-key"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateNamespace(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		ns := testutil.NewNamespace("nonexistent")

		_, err := svc.UpdateNamespace(ctx, ns)
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})

	t.Run("not a control plane namespace", func(t *testing.T) {
		bare := testutil.NewNamespace("bare-ns")
		svc := newService(t, bare)
		update := testutil.NewNamespace("bare-ns")

		_, err := svc.UpdateNamespace(ctx, update)
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})
}

func TestListNamespaces(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cp1 := controlPlaneNamespace("cp-ns-1")
		cp2 := controlPlaneNamespace("cp-ns-2")
		svc := newService(t, cp1, cp2)

		result, err := svc.ListNamespaces(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})

	t.Run("empty", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListNamespaces(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("filters only control plane namespaces", func(t *testing.T) {
		cpNs := controlPlaneNamespace("cp-ns")
		regularNs := testutil.NewNamespace("regular-ns")
		svc := newService(t, cpNs, regularNs)

		result, err := svc.ListNamespaces(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "cp-ns", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListNamespaces(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cpNs := controlPlaneNamespace("test-ns")
		svc := newService(t, cpNs)

		result, err := svc.GetNamespace(ctx, "test-ns")
		require.NoError(t, err)
		assert.Equal(t, "test-ns", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetNamespace(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})

	t.Run("not a control plane namespace", func(t *testing.T) {
		bare := testutil.NewNamespace("bare-ns")
		svc := newService(t, bare)

		_, err := svc.GetNamespace(ctx, "bare-ns")
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})
}

func TestDeleteNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cpNs := controlPlaneNamespace("test-ns")
		svc := newService(t, cpNs)

		err := svc.DeleteNamespace(ctx, "test-ns")
		require.NoError(t, err)

		_, err = svc.GetNamespace(ctx, "test-ns")
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteNamespace(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})

	t.Run("not a control plane namespace", func(t *testing.T) {
		bare := testutil.NewNamespace("bare-ns")
		svc := newService(t, bare)

		err := svc.DeleteNamespace(ctx, "bare-ns")
		require.ErrorIs(t, err, ErrNamespaceNotFound)
	})
}
