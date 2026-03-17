// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace    = "test-ns"
	testWorkflowName = "test-workflow"
	testRunName      = "test-run"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), nil, nil, testutil.TestLogger())
}

func TestCreateWorkflowRun(t *testing.T) {
	ctx := context.Background()

	t.Run("success with workflow ref", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		svc := newService(t, wf)
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)

		result, err := svc.CreateWorkflowRun(ctx, testNamespace, run)
		require.NoError(t, err)
		assert.Equal(t, workflowRunTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.WorkflowRunStatus{}, result.Status)
	})

	t.Run("success with cluster workflow ref", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow(testWorkflowName)
		svc := newService(t, cwf)
		run := testutil.NewWorkflowRun("", testWorkflowName, testRunName)
		run.Spec.Workflow.Kind = openchoreov1alpha1.WorkflowRefKindClusterWorkflow

		result, err := svc.CreateWorkflowRun(ctx, testNamespace, run)
		require.NoError(t, err)
		assert.Equal(t, workflowRunTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateWorkflowRun(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("workflow not found", func(t *testing.T) {
		svc := newService(t)
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)

		_, err := svc.CreateWorkflowRun(ctx, testNamespace, run)
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})

	t.Run("cluster workflow not found", func(t *testing.T) {
		svc := newService(t)
		run := testutil.NewWorkflowRun("", testWorkflowName, testRunName)
		run.Spec.Workflow.Kind = openchoreov1alpha1.WorkflowRefKindClusterWorkflow

		_, err := svc.CreateWorkflowRun(ctx, testNamespace, run)
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		existing := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, wf, existing)
		dup := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)

		_, err := svc.CreateWorkflowRun(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrWorkflowRunAlreadyExists)
	})
}

func TestUpdateWorkflowRun(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, existing)

		update := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		update.Labels = map[string]string{"env": "prod"}
		update.Annotations = map[string]string{"note": "updated"}

		result, err := svc.UpdateWorkflowRun(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, workflowRunTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, "updated", result.Annotations["note"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateWorkflowRun(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		run := &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateWorkflowRun(ctx, testNamespace, run)
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("applies spec changes", func(t *testing.T) {
		existing := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, existing)

		update := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		update.Spec.Workflow.Name = "other-workflow"

		result, err := svc.UpdateWorkflowRun(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, "other-workflow", result.Spec.Workflow.Name)
	})
}

func TestListWorkflowRuns(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-1")
		r2 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-2")
		svc := newService(t, r1, r2)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, workflowRunTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		rIn := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-in")
		rOut := testutil.NewWorkflowRun("other-ns", testWorkflowName, "run-out")
		svc := newService(t, rIn, rOut)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "run-in", result.Items[0].Name)
	})

	t.Run("filter by project name", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-proj1")
		r1.Labels = map[string]string{ocLabels.LabelKeyProjectName: "proj-a"}
		r2 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-proj2")
		r2.Labels = map[string]string{ocLabels.LabelKeyProjectName: "proj-b"}
		svc := newService(t, r1, r2)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "proj-a", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "run-proj1", result.Items[0].Name)
	})

	t.Run("filter by component name", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-comp1")
		r1.Labels = map[string]string{ocLabels.LabelKeyComponentName: "comp-a"}
		r2 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-comp2")
		r2.Labels = map[string]string{ocLabels.LabelKeyComponentName: "comp-b"}
		svc := newService(t, r1, r2)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "", "comp-a", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "run-comp1", result.Items[0].Name)
	})

	t.Run("filter by workflow name", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, "wf-alpha", "run-wf1")
		r2 := testutil.NewWorkflowRun(testNamespace, "wf-beta", "run-wf2")
		svc := newService(t, r1, r2)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "", "", "wf-alpha", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "run-wf1", result.Items[0].Name)
	})
}

func TestGetWorkflowRun(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)

		result, err := svc.GetWorkflowRun(ctx, testNamespace, testRunName)
		require.NoError(t, err)
		assert.Equal(t, workflowRunTypeMeta, result.TypeMeta)
		assert.Equal(t, testRunName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetWorkflowRun(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})
}

func TestMatchesTaskName(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		assert.True(t, matchesTaskName("checkout-source", "checkout-source"))
	})

	t.Run("argo node name format", func(t *testing.T) {
		assert.True(t, matchesTaskName("greeting-service-build-01[0].checkout-source", "checkout-source"))
	})

	t.Run("no match", func(t *testing.T) {
		assert.False(t, matchesTaskName("other-task", "checkout-source"))
	})
}
