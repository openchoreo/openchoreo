// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

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
	testOPName    = "test-op"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: testOPName},
			Spec:       testutil.NewObservabilityPlane(testNamespace, testOPName).Spec,
		}

		result, err := svc.CreateObservabilityPlane(ctx, testNamespace, op)
		require.NoError(t, err)
		assert.Equal(t, observabilityPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ObservabilityPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateObservabilityPlane(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewObservabilityPlane(testNamespace, testOPName)
		svc := newService(t, existing)
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: testOPName},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateObservabilityPlane(ctx, testNamespace, op)
		require.ErrorIs(t, err, ErrObservabilityPlaneAlreadyExists)
	})
}

func TestUpdateObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewObservabilityPlane(testNamespace, testOPName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testOPName,
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateObservabilityPlane(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, observabilityPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, openchoreov1alpha1.ObservabilityPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateObservabilityPlane(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateObservabilityPlane(ctx, testNamespace, op)
		require.ErrorIs(t, err, ErrObservabilityPlaneNotFound)
	})
}

func TestListObservabilityPlanes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		op1 := testutil.NewObservabilityPlane(testNamespace, "op-1")
		op2 := testutil.NewObservabilityPlane(testNamespace, "op-2")
		svc := newService(t, op1, op2)

		result, err := svc.ListObservabilityPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, observabilityPlaneTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListObservabilityPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		opInNs := testutil.NewObservabilityPlane(testNamespace, "op-in")
		opOtherNs := testutil.NewObservabilityPlane("other-ns", "op-out")
		svc := newService(t, opInNs, opOtherNs)

		result, err := svc.ListObservabilityPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "op-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListObservabilityPlanes(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		op := testutil.NewObservabilityPlane(testNamespace, testOPName)
		svc := newService(t, op)

		result, err := svc.GetObservabilityPlane(ctx, testNamespace, testOPName)
		require.NoError(t, err)
		assert.Equal(t, observabilityPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, testOPName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetObservabilityPlane(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrObservabilityPlaneNotFound)
	})
}

func TestDeleteObservabilityPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		op := testutil.NewObservabilityPlane(testNamespace, testOPName)
		svc := newService(t, op)

		err := svc.DeleteObservabilityPlane(ctx, testNamespace, testOPName)
		require.NoError(t, err)

		_, err = svc.GetObservabilityPlane(ctx, testNamespace, testOPName)
		require.ErrorIs(t, err, ErrObservabilityPlaneNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteObservabilityPlane(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrObservabilityPlaneNotFound)
	})
}
