// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace = "test-ns"
	testDPName    = "test-dp"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: testDPName},
			Spec:       testutil.NewDataPlane(testNamespace, testDPName).Spec,
		}

		result, err := svc.CreateDataPlane(ctx, testNamespace, dp)
		require.NoError(t, err)
		assert.Equal(t, dataPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.DataPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateDataPlane(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewDataPlane(testNamespace, testDPName)
		svc := newService(t, existing)
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: testDPName},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateDataPlane(ctx, testNamespace, dp)
		require.ErrorIs(t, err, ErrDataPlaneAlreadyExists)
	})
}

func TestUpdateDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewDataPlane(testNamespace, testDPName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testDPName,
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateDataPlane(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, dataPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateDataPlane(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrDataPlaneNil)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateDataPlane(ctx, testNamespace, dp)
		require.ErrorIs(t, err, ErrDataPlaneNotFound)
	})
}

func TestListDataPlanes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		dp1 := testutil.NewDataPlane(testNamespace, "dp-1")
		dp2 := testutil.NewDataPlane(testNamespace, "dp-2")
		svc := newService(t, dp1, dp2)

		result, err := svc.ListDataPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, dataPlaneTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListDataPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListDataPlanes(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		dpInNs := testutil.NewDataPlane(testNamespace, "dp-in")
		dpOtherNs := testutil.NewDataPlane("other-ns", "dp-out")
		svc := newService(t, dpInNs, dpOtherNs)

		result, err := svc.ListDataPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "dp-in", result.Items[0].Name)
	})
}

func TestGetDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		dp := testutil.NewDataPlane(testNamespace, testDPName)
		svc := newService(t, dp)

		result, err := svc.GetDataPlane(ctx, testNamespace, testDPName)
		require.NoError(t, err)
		assert.Equal(t, dataPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, testDPName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetDataPlane(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrDataPlaneNotFound)
	})
}

func TestDeleteDataPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		dp := testutil.NewDataPlane(testNamespace, testDPName)
		svc := newService(t, dp)

		err := svc.DeleteDataPlane(ctx, testNamespace, testDPName)
		require.NoError(t, err)

		_, err = svc.GetDataPlane(ctx, testNamespace, testDPName)
		require.ErrorIs(t, err, ErrDataPlaneNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteDataPlane(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrDataPlaneNotFound)
	})
}
