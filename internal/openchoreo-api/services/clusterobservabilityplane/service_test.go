// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateClusterObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cop := testutil.NewClusterObservabilityPlane("test-cop")

		result, err := svc.CreateClusterObservabilityPlane(ctx, cop)
		require.NoError(t, err)
		assert.Equal(t, clusterObservabilityPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cop", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterObservabilityPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterObservabilityPlane(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterObservabilityPlane("dup-cop")
		svc := newService(t, existing)
		dup := testutil.NewClusterObservabilityPlane("dup-cop")

		_, err := svc.CreateClusterObservabilityPlane(ctx, dup)
		require.ErrorIs(t, err, ErrClusterObservabilityPlaneAlreadyExists)
	})
}

func TestUpdateClusterObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterObservabilityPlane("test-cop")
		svc := newService(t, existing)

		update := testutil.NewClusterObservabilityPlane("test-cop")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterObservabilityPlane(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterObservabilityPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, openchoreov1alpha1.ClusterObservabilityPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterObservabilityPlane(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		cop := testutil.NewClusterObservabilityPlane("nonexistent")

		_, err := svc.UpdateClusterObservabilityPlane(ctx, cop)
		require.ErrorIs(t, err, ErrClusterObservabilityPlaneNotFound)
	})
}

func TestListClusterObservabilityPlanes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		cop1 := testutil.NewClusterObservabilityPlane("cop-1")
		cop2 := testutil.NewClusterObservabilityPlane("cop-2")
		svc := newService(t, cop1, cop2)

		result, err := svc.ListClusterObservabilityPlanes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterObservabilityPlaneTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterObservabilityPlanes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterObservabilityPlanes(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cop := testutil.NewClusterObservabilityPlane("test-cop")
		svc := newService(t, cop)

		result, err := svc.GetClusterObservabilityPlane(ctx, "test-cop")
		require.NoError(t, err)
		assert.Equal(t, clusterObservabilityPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cop", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterObservabilityPlane(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterObservabilityPlaneNotFound)
	})
}

func TestDeleteClusterObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cop := testutil.NewClusterObservabilityPlane("test-cop")
		svc := newService(t, cop)

		err := svc.DeleteClusterObservabilityPlane(ctx, "test-cop")
		require.NoError(t, err)

		_, err = svc.GetClusterObservabilityPlane(ctx, "test-cop")
		require.ErrorIs(t, err, ErrClusterObservabilityPlaneNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterObservabilityPlane(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterObservabilityPlaneNotFound)
	})
}
