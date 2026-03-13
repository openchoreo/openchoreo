// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

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

func TestCreateClusterDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cdp := testutil.NewClusterDataPlane("test-cdp")

		result, err := svc.CreateClusterDataPlane(ctx, cdp)
		require.NoError(t, err)
		assert.Equal(t, clusterDataPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cdp", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterDataPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterDataPlane(ctx, nil)
		require.ErrorIs(t, err, ErrClusterDataPlaneNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterDataPlane("dup-cdp")
		svc := newService(t, existing)
		dup := testutil.NewClusterDataPlane("dup-cdp")

		_, err := svc.CreateClusterDataPlane(ctx, dup)
		require.ErrorIs(t, err, ErrClusterDataPlaneAlreadyExists)
	})
}

func TestUpdateClusterDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterDataPlane("test-cdp")
		svc := newService(t, existing)

		update := testutil.NewClusterDataPlane("test-cdp")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateClusterDataPlane(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterDataPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterDataPlane(ctx, nil)
		require.ErrorIs(t, err, ErrClusterDataPlaneNil)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		cdp := testutil.NewClusterDataPlane("nonexistent")

		_, err := svc.UpdateClusterDataPlane(ctx, cdp)
		require.ErrorIs(t, err, ErrClusterDataPlaneNotFound)
	})
}

func TestListClusterDataPlanes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		cdp1 := testutil.NewClusterDataPlane("cdp-1")
		cdp2 := testutil.NewClusterDataPlane("cdp-2")
		svc := newService(t, cdp1, cdp2)

		result, err := svc.ListClusterDataPlanes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterDataPlaneTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterDataPlanes(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterDataPlanes(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cdp := testutil.NewClusterDataPlane("test-cdp")
		svc := newService(t, cdp)

		result, err := svc.GetClusterDataPlane(ctx, "test-cdp")
		require.NoError(t, err)
		assert.Equal(t, clusterDataPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cdp", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterDataPlane(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterDataPlaneNotFound)
	})
}

func TestDeleteClusterDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cdp := testutil.NewClusterDataPlane("test-cdp")
		svc := newService(t, cdp)

		err := svc.DeleteClusterDataPlane(ctx, "test-cdp")
		require.NoError(t, err)

		_, err = svc.GetClusterDataPlane(ctx, "test-cdp")
		require.ErrorIs(t, err, ErrClusterDataPlaneNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterDataPlane(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterDataPlaneNotFound)
	})
}
