// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace     = "test-ns"
	testProjectName   = "test-project"
	testComponentName = "test-comp"
	testWorkloadName  = "test-workload"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)

		result, err := svc.CreateWorkload(ctx, testNamespace, w)
		require.NoError(t, err)
		assert.Equal(t, workloadTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.WorkloadStatus{}, result.Status)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateWorkload(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t)
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)

		_, err := svc.CreateWorkload(ctx, testNamespace, w)
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		existing := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		svc := newService(t, comp, existing)
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)

		_, err := svc.CreateWorkload(ctx, testNamespace, w)
		require.ErrorIs(t, err, ErrWorkloadAlreadyExists)
	})

	t.Run("same name in other namespace succeeds", func(t *testing.T) {
		existing := testutil.NewWorkload("other-ns", testProjectName, testComponentName, testWorkloadName)
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, existing, comp)
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)

		result, err := svc.CreateWorkload(ctx, testNamespace, w)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("sets labels when labels are nil", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		w.Labels = nil

		result, err := svc.CreateWorkload(ctx, testNamespace, w)
		require.NoError(t, err)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})
}

func TestUpdateWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		svc := newService(t, existing)

		update := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		update.Labels = map[string]string{"env": "prod"}
		update.Annotations = map[string]string{"note": "updated"}

		result, err := svc.UpdateWorkload(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, workloadTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, "updated", result.Annotations["note"])
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateWorkload(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, "nonexistent")

		_, err := svc.UpdateWorkload(ctx, testNamespace, w)
		require.ErrorIs(t, err, ErrWorkloadNotFound)
	})

	t.Run("clears status from user input", func(t *testing.T) {
		existing := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		svc := newService(t, existing)

		update := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)

		result, err := svc.UpdateWorkload(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, openchoreov1alpha1.WorkloadStatus{}, result.Status)
	})

	t.Run("preserves special labels when user labels are nil", func(t *testing.T) {
		existing := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		svc := newService(t, existing)

		update := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		update.Labels = nil

		result, err := svc.UpdateWorkload(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})
}

func TestListWorkloads(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		w1 := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, "wl-1")
		w2 := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, "wl-2")
		svc := newService(t, w1, w2)

		result, err := svc.ListWorkloads(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, workloadTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListWorkloads(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		wInNs := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, "wl-in")
		wOtherNs := testutil.NewWorkload("other-ns", testProjectName, testComponentName, "wl-out")
		svc := newService(t, wInNs, wOtherNs)

		result, err := svc.ListWorkloads(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "wl-in", result.Items[0].Name)
	})

	t.Run("filter by component name", func(t *testing.T) {
		w1 := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, "wl-comp1")
		w2 := testutil.NewWorkload(testNamespace, testProjectName, "other-comp", "wl-comp2")
		svc := newService(t, w1, w2)

		result, err := svc.ListWorkloads(ctx, testNamespace, testComponentName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "wl-comp1", result.Items[0].Name)
	})

	t.Run("no component filter returns all", func(t *testing.T) {
		w1 := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, "wl-a")
		w2 := testutil.NewWorkload(testNamespace, testProjectName, "other-comp", "wl-b")
		svc := newService(t, w1, w2)

		result, err := svc.ListWorkloads(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})
}

func TestGetWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		svc := newService(t, w)

		result, err := svc.GetWorkload(ctx, testNamespace, testWorkloadName)
		require.NoError(t, err)
		assert.Equal(t, workloadTypeMeta, result.TypeMeta)
		assert.Equal(t, testWorkloadName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetWorkload(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkloadNotFound)
	})
}

func TestDeleteWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		w := testutil.NewWorkload(testNamespace, testProjectName, testComponentName, testWorkloadName)
		svc := newService(t, w)

		err := svc.DeleteWorkload(ctx, testNamespace, testWorkloadName)
		require.NoError(t, err)

		_, err = svc.GetWorkload(ctx, testNamespace, testWorkloadName)
		require.ErrorIs(t, err, ErrWorkloadNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteWorkload(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkloadNotFound)
	})
}

func TestGetWorkloadSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("returns valid schema", func(t *testing.T) {
		svc := newService(t)

		schema, err := svc.GetWorkloadSchema(ctx)
		require.NoError(t, err)
		assert.Equal(t, "object", schema.Type)
		assert.Contains(t, schema.Required, "container")

		container, ok := schema.Properties["container"]
		require.True(t, ok)
		assert.Contains(t, container.Required, "image")
	})
}
