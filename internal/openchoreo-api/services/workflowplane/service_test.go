// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

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
	testWPName    = "test-wp"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), nil, testutil.TestLogger())
}

func TestCreateWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		wp := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: testWPName},
			Spec:       testutil.NewWorkflowPlane(testNamespace, testWPName).Spec,
		}

		result, err := svc.CreateWorkflowPlane(ctx, testNamespace, wp)
		require.NoError(t, err)
		assert.Equal(t, workflowPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.WorkflowPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateWorkflowPlane(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrWorkflowPlaneNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewWorkflowPlane(testNamespace, testWPName)
		svc := newService(t, existing)
		wp := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: testWPName},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateWorkflowPlane(ctx, testNamespace, wp)
		require.ErrorIs(t, err, ErrWorkflowPlaneAlreadyExists)
	})
}

func TestUpdateWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewWorkflowPlane(testNamespace, testWPName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testWPName,
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateWorkflowPlane(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, workflowPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, openchoreov1alpha1.WorkflowPlaneStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateWorkflowPlane(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrWorkflowPlaneNil)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		wp := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateWorkflowPlane(ctx, testNamespace, wp)
		require.ErrorIs(t, err, ErrWorkflowPlaneNotFound)
	})
}

func TestListWorkflowPlanes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		wp1 := testutil.NewWorkflowPlane(testNamespace, "wp-1")
		wp2 := testutil.NewWorkflowPlane(testNamespace, "wp-2")
		svc := newService(t, wp1, wp2)

		result, err := svc.ListWorkflowPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, workflowPlaneTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListWorkflowPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		wpInNs := testutil.NewWorkflowPlane(testNamespace, "wp-in")
		wpOtherNs := testutil.NewWorkflowPlane("other-ns", "wp-out")
		svc := newService(t, wpInNs, wpOtherNs)

		result, err := svc.ListWorkflowPlanes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "wp-in", result.Items[0].Name)
	})
}

func TestGetWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		wp := testutil.NewWorkflowPlane(testNamespace, testWPName)
		svc := newService(t, wp)

		result, err := svc.GetWorkflowPlane(ctx, testNamespace, testWPName)
		require.NoError(t, err)
		assert.Equal(t, workflowPlaneTypeMeta, result.TypeMeta)
		assert.Equal(t, testWPName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetWorkflowPlane(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowPlaneNotFound)
	})
}

func TestDeleteWorkflowPlane(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		wp := testutil.NewWorkflowPlane(testNamespace, testWPName)
		svc := newService(t, wp)

		err := svc.DeleteWorkflowPlane(ctx, testNamespace, testWPName)
		require.NoError(t, err)

		_, err = svc.GetWorkflowPlane(ctx, testNamespace, testWPName)
		require.ErrorIs(t, err, ErrWorkflowPlaneNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteWorkflowPlane(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowPlaneNotFound)
	})
}
