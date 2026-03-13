// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

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

func TestCreateClusterWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cwp := testutil.NewClusterWorkflowPlane("test-cwp")

		result, err := svc.CreateClusterWorkflowPlane(ctx, cwp)
		require.NoError(t, err)
		assert.Equal(t, clusterWorkflowPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cwp", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterWorkflowPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterWorkflowPlane(ctx, nil)
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterWorkflowPlane("dup-cwp")
		svc := newService(t, existing)
		dup := testutil.NewClusterWorkflowPlane("dup-cwp")

		_, err := svc.CreateClusterWorkflowPlane(ctx, dup)
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneAlreadyExists)
	})
}

func TestUpdateClusterWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterWorkflowPlane("test-cwp")
		svc := newService(t, existing)

		update := testutil.NewClusterWorkflowPlane("test-cwp")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterWorkflowPlane(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterWorkflowPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterWorkflowPlane(ctx, nil)
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneNil)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		cwp := testutil.NewClusterWorkflowPlane("nonexistent")

		_, err := svc.UpdateClusterWorkflowPlane(ctx, cwp)
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneNotFound)
	})
}

func TestListClusterWorkflowPlanes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		cwp1 := testutil.NewClusterWorkflowPlane("cwp-1")
		cwp2 := testutil.NewClusterWorkflowPlane("cwp-2")
		svc := newService(t, cwp1, cwp2)

		result, err := svc.ListClusterWorkflowPlanes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterWorkflowPlaneTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterWorkflowPlanes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})
}

func TestGetClusterWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cwp := testutil.NewClusterWorkflowPlane("test-cwp")
		svc := newService(t, cwp)

		result, err := svc.GetClusterWorkflowPlane(ctx, "test-cwp")
		require.NoError(t, err)
		assert.Equal(t, clusterWorkflowPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cwp", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterWorkflowPlane(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneNotFound)
	})
}

func TestDeleteClusterWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cwp := testutil.NewClusterWorkflowPlane("test-cwp")
		svc := newService(t, cwp)

		err := svc.DeleteClusterWorkflowPlane(ctx, "test-cwp")
		require.NoError(t, err)

		_, err = svc.GetClusterWorkflowPlane(ctx, "test-cwp")
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterWorkflowPlane(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterWorkflowPlaneNotFound)
	})
}
