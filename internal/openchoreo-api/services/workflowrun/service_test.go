// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func TestDeleteWorkflowRun(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)

		err := svc.DeleteWorkflowRun(ctx, testNamespace, testRunName)
		require.NoError(t, err)

		// Verify the run is actually gone
		_, err = svc.GetWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteWorkflowRun(ctx, testNamespace, "nonexistent")
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

func TestComputeWorkflowRunStatus(t *testing.T) {
	t.Run("no conditions returns pending", func(t *testing.T) {
		assert.Equal(t, workflowRunStatusPending, computeWorkflowRunStatus(nil))
	})

	t.Run("empty conditions returns pending", func(t *testing.T) {
		assert.Equal(t, workflowRunStatusPending, computeWorkflowRunStatus([]metav1.Condition{}))
	})

	t.Run("workflow failed", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowFailed", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusFailed, computeWorkflowRunStatus(conditions))
	})

	t.Run("workflow succeeded", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowSucceeded", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusSucceeded, computeWorkflowRunStatus(conditions))
	})

	t.Run("workflow running", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowRunning", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusRunning, computeWorkflowRunStatus(conditions))
	})

	t.Run("failed takes precedence over succeeded", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowSucceeded", Status: metav1.ConditionTrue},
			{Type: "WorkflowFailed", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusFailed, computeWorkflowRunStatus(conditions))
	})

	t.Run("succeeded takes precedence over running", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowRunning", Status: metav1.ConditionTrue},
			{Type: "WorkflowSucceeded", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusSucceeded, computeWorkflowRunStatus(conditions))
	})

	t.Run("condition false returns pending", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowRunning", Status: metav1.ConditionFalse},
		}
		assert.Equal(t, workflowRunStatusPending, computeWorkflowRunStatus(conditions))
	})
}

func TestGenerateWorkflowRunName(t *testing.T) {
	t.Run("generates valid name", func(t *testing.T) {
		name, err := generateWorkflowRunName("my-component")
		require.NoError(t, err)
		assert.Contains(t, name, "my-component-run-")
		assert.Len(t, name, len("my-component-run-")+8) // 8 hex chars
	})

	t.Run("generates unique names", func(t *testing.T) {
		name1, err := generateWorkflowRunName("comp")
		require.NoError(t, err)
		name2, err := generateWorkflowRunName("comp")
		require.NoError(t, err)
		assert.NotEqual(t, name1, name2)
	})

	t.Run("truncates long base name to fit 63 char limit", func(t *testing.T) {
		longName := "this-is-a-very-long-component-name-that-exceeds-kubernetes-limits"
		name, err := generateWorkflowRunName(longName)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(name), 63)
		assert.Contains(t, name, "-run-")
	})
}

func TestGetNestedStringInParams(t *testing.T) {
	makeRaw := func(data map[string]any) *runtime.RawExtension {
		b, _ := json.Marshal(data)
		return &runtime.RawExtension{Raw: b}
	}

	t.Run("simple key", func(t *testing.T) {
		raw := makeRaw(map[string]any{"url": "https://github.com/example"})
		val, err := getNestedStringInParams(raw, "url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", val)
	})

	t.Run("nested key", func(t *testing.T) {
		raw := makeRaw(map[string]any{
			"repo": map[string]any{
				"url": "https://github.com/example",
			},
		})
		val, err := getNestedStringInParams(raw, "repo.url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", val)
	})

	t.Run("strips parameters prefix", func(t *testing.T) {
		raw := makeRaw(map[string]any{"url": "https://github.com/example"})
		val, err := getNestedStringInParams(raw, "parameters.url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", val)
	})

	t.Run("nil raw extension", func(t *testing.T) {
		_, err := getNestedStringInParams(nil, "url")
		require.Error(t, err)
	})

	t.Run("key not found", func(t *testing.T) {
		raw := makeRaw(map[string]any{"url": "https://github.com/example"})
		_, err := getNestedStringInParams(raw, "missing")
		require.Error(t, err)
	})

	t.Run("value is not a string", func(t *testing.T) {
		raw := makeRaw(map[string]any{"count": 42})
		_, err := getNestedStringInParams(raw, "count")
		require.Error(t, err)
	})

	t.Run("intermediate is not an object", func(t *testing.T) {
		raw := makeRaw(map[string]any{"repo": "not-an-object"})
		_, err := getNestedStringInParams(raw, "repo.url")
		require.Error(t, err)
	})
}

func TestSetNestedStringInParams(t *testing.T) {
	makeRaw := func(data map[string]any) *runtime.RawExtension {
		b, _ := json.Marshal(data)
		return &runtime.RawExtension{Raw: b}
	}

	t.Run("simple key", func(t *testing.T) {
		raw := makeRaw(map[string]any{"commit": "old"})
		result, err := setNestedStringInParams(raw, "commit", "abc1234")
		require.NoError(t, err)

		val, err := getNestedStringInParams(result, "commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("nested key", func(t *testing.T) {
		raw := makeRaw(map[string]any{
			"repo": map[string]any{
				"commit": "old",
			},
		})
		result, err := setNestedStringInParams(raw, "repo.commit", "abc1234")
		require.NoError(t, err)

		val, err := getNestedStringInParams(result, "repo.commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("strips parameters prefix", func(t *testing.T) {
		raw := makeRaw(map[string]any{"commit": "old"})
		result, err := setNestedStringInParams(raw, "parameters.commit", "abc1234")
		require.NoError(t, err)

		val, err := getNestedStringInParams(result, "commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("creates intermediate objects", func(t *testing.T) {
		raw := makeRaw(map[string]any{})
		result, err := setNestedStringInParams(raw, "repo.commit", "abc1234")
		require.NoError(t, err)

		val, err := getNestedStringInParams(result, "repo.commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("nil raw extension", func(t *testing.T) {
		_, err := setNestedStringInParams(nil, "commit", "abc1234")
		require.Error(t, err)
	})

	t.Run("intermediate is not an object", func(t *testing.T) {
		raw := makeRaw(map[string]any{"repo": "not-an-object"})
		_, err := setNestedStringInParams(raw, "repo.commit", "abc1234")
		require.Error(t, err)
	})
}

func TestTriggerWorkflow(t *testing.T) {
	ctx := context.Background()

	// buildWorkflowSchema returns an OpenAPIV3-style schema with x-openchoreo-component-parameter-repository extensions.
	buildWorkflowSchema := func() *runtime.RawExtension {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repoUrl": map[string]any{
					"type": "string",
					"x-openchoreo-component-parameter-repository-url": true,
				},
				"commit": map[string]any{
					"type": "string",
					"x-openchoreo-component-parameter-repository-commit": true,
				},
			},
		}
		b, _ := json.Marshal(schema)
		return &runtime.RawExtension{Raw: b}
	}

	buildComponentWithWorkflow := func(namespace, project, name, workflowName string, kind openchoreov1alpha1.WorkflowRefKind) *openchoreov1alpha1.Component {
		params, _ := json.Marshal(map[string]any{
			"repoUrl": "https://github.com/example/repo",
			"commit":  "",
		})
		comp := testutil.NewComponent(namespace, project, name)
		comp.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowConfig{
			Kind: kind,
			Name: workflowName,
			Parameters: &runtime.RawExtension{
				Raw: params,
			},
		}
		return comp
	}

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "nonexistent", "abc1234")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get component")
	})

	t.Run("component without workflow config", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, "proj", "my-comp")
		svc := newService(t, comp)
		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "my-comp", "abc1234")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have a workflow configured")
	})

	t.Run("invalid commit SHA", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "my-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)
		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "my-comp", "not-a-sha!")
		require.ErrorIs(t, err, ErrInvalidCommitSHA)
	})

	t.Run("project name mismatch", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		comp := buildComponentWithWorkflow(testNamespace, "real-project", "my-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)
		_, err := svc.TriggerWorkflow(ctx, testNamespace, "wrong-project", "my-comp", "abc1234")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match component owner project")
	})

	t.Run("success with workflow ref", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "my-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		result, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "my-comp", "abc1234f")
		require.NoError(t, err)
		assert.Equal(t, "my-comp", result.ComponentName)
		assert.Equal(t, "proj", result.ProjectName)
		assert.Equal(t, testNamespace, result.NamespaceName)
		assert.Equal(t, "abc1234f", result.Commit)
		assert.Equal(t, workflowRunStatusPending, result.Status)
		assert.Contains(t, result.Name, "my-comp-run-")
	})

	t.Run("success with cluster workflow ref", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow(testWorkflowName)
		cwf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "my-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindClusterWorkflow)
		svc := newService(t, cwf, comp)

		result, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "my-comp", "abc1234f")
		require.NoError(t, err)
		assert.Equal(t, "my-comp", result.ComponentName)
		assert.Contains(t, result.Name, "my-comp-run-")
	})

	t.Run("success with empty commit", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "my-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		result, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "my-comp", "")
		require.NoError(t, err)
		assert.Empty(t, result.Commit)
	})

	t.Run("success with empty project name derives from component owner", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "owned-proj", "my-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		result, err := svc.TriggerWorkflow(ctx, testNamespace, "", "my-comp", "abc1234f")
		require.NoError(t, err)
		assert.Equal(t, "owned-proj", result.ProjectName)
	})
}

func TestGetWorkflowRunStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("pending status with no conditions", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, testRunName)
		require.NoError(t, err)
		assert.Equal(t, workflowRunStatusPending, result.Status)
		assert.Empty(t, result.Steps)
		assert.False(t, result.HasLiveObservability)
	})

	t.Run("running status with conditions", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-running")
		fakeClient := testutil.NewFakeClient(run)
		run.Status.Conditions = []metav1.Condition{
			{Type: "WorkflowRunning", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Now()},
		}
		require.NoError(t, fakeClient.Status().Update(ctx, run))
		svc := NewService(fakeClient, nil, nil, testutil.TestLogger())

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "run-running")
		require.NoError(t, err)
		assert.Equal(t, workflowRunStatusRunning, result.Status)
	})

	t.Run("failed status with conditions", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-failed")
		fakeClient := testutil.NewFakeClient(run)
		run.Status.Conditions = []metav1.Condition{
			{Type: "WorkflowRunning", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Now()},
			{Type: "WorkflowFailed", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Now()},
		}
		require.NoError(t, fakeClient.Status().Update(ctx, run))
		svc := NewService(fakeClient, nil, nil, testutil.TestLogger())

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "run-failed")
		require.NoError(t, err)
		assert.Equal(t, workflowRunStatusFailed, result.Status)
	})

	t.Run("succeeded status with conditions", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-succeeded")
		fakeClient := testutil.NewFakeClient(run)
		run.Status.Conditions = []metav1.Condition{
			{Type: "WorkflowSucceeded", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Now()},
		}
		require.NoError(t, fakeClient.Status().Update(ctx, run))
		svc := NewService(fakeClient, nil, nil, testutil.TestLogger())

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "run-succeeded")
		require.NoError(t, err)
		assert.Equal(t, workflowRunStatusSucceeded, result.Status)
	})

	t.Run("maps tasks to steps with timestamps", func(t *testing.T) {
		startTime := metav1.NewTime(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
		endTime := metav1.NewTime(time.Date(2026, 1, 15, 10, 5, 0, 0, time.UTC))

		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-with-tasks")
		fakeClient := testutil.NewFakeClient(run)
		run.Status.Tasks = []openchoreov1alpha1.WorkflowTask{
			{
				Name:        "checkout-source",
				Phase:       "Succeeded",
				StartedAt:   &startTime,
				CompletedAt: &endTime,
			},
			{
				Name:      "build",
				Phase:     "Running",
				StartedAt: &startTime,
			},
		}
		require.NoError(t, fakeClient.Status().Update(ctx, run))
		svc := NewService(fakeClient, nil, nil, testutil.TestLogger())

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "run-with-tasks")
		require.NoError(t, err)
		require.Len(t, result.Steps, 2)

		assert.Equal(t, "checkout-source", result.Steps[0].Name)
		assert.Equal(t, "Succeeded", result.Steps[0].Phase)
		require.NotNil(t, result.Steps[0].StartedAt)
		assert.True(t, startTime.Time.Equal(*result.Steps[0].StartedAt))
		require.NotNil(t, result.Steps[0].FinishedAt)
		assert.True(t, endTime.Time.Equal(*result.Steps[0].FinishedAt))

		assert.Equal(t, "build", result.Steps[1].Name)
		assert.Equal(t, "Running", result.Steps[1].Phase)
		require.NotNil(t, result.Steps[1].StartedAt)
		assert.Nil(t, result.Steps[1].FinishedAt)
	})

	t.Run("tasks with no timestamps have nil step timestamps", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-no-ts")
		fakeClient := testutil.NewFakeClient(run)
		run.Status.Tasks = []openchoreov1alpha1.WorkflowTask{
			{Name: "pending-step", Phase: "Pending"},
		}
		require.NoError(t, fakeClient.Status().Update(ctx, run))
		svc := NewService(fakeClient, nil, nil, testutil.TestLogger())

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "run-no-ts")
		require.NoError(t, err)
		require.Len(t, result.Steps, 1)
		assert.Equal(t, "pending-step", result.Steps[0].Name)
		assert.Equal(t, "Pending", result.Steps[0].Phase)
		assert.Nil(t, result.Steps[0].StartedAt)
		assert.Nil(t, result.Steps[0].FinishedAt)
	})
}

func TestGetWorkflowRunLogs(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, "nonexistent", "", nil)
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("missing run reference", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, testRunName, "", nil)
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})

	t.Run("partial run reference with empty name", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-partial-ref")
		run.Status.RunReference = &openchoreov1alpha1.ResourceReference{
			Name:      "",
			Namespace: "some-ns",
		}
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, "run-partial-ref", "", nil)
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})

	t.Run("partial run reference with empty namespace", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-partial-ns")
		run.Status.RunReference = &openchoreov1alpha1.ResourceReference{
			Name:      "some-name",
			Namespace: "",
		}
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, "run-partial-ns", "", nil)
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})
}

func TestGetWorkflowRunEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetWorkflowRunEvents(ctx, testNamespace, "nonexistent", "")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("missing run reference", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunEvents(ctx, testNamespace, testRunName, "")
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})

	t.Run("partial run reference with empty name", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-event-partial")
		run.Status.RunReference = &openchoreov1alpha1.ResourceReference{
			Name:      "",
			Namespace: "some-ns",
		}
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunEvents(ctx, testNamespace, "run-event-partial", "")
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})
}

func TestListWorkflowRunsCombinedFilters(t *testing.T) {
	ctx := context.Background()

	t.Run("filter by project and component together", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-match")
		r1.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   "proj-a",
			ocLabels.LabelKeyComponentName: "comp-x",
		}
		r2 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-proj-only")
		r2.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   "proj-a",
			ocLabels.LabelKeyComponentName: "comp-y",
		}
		r3 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-comp-only")
		r3.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   "proj-b",
			ocLabels.LabelKeyComponentName: "comp-x",
		}
		svc := newService(t, r1, r2, r3)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "proj-a", "comp-x", "", services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "run-match", result.Items[0].Name)
	})

	t.Run("filter by project component and workflow name", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, "wf-alpha", "run-all-match")
		r1.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   "proj-a",
			ocLabels.LabelKeyComponentName: "comp-x",
		}
		r2 := testutil.NewWorkflowRun(testNamespace, "wf-beta", "run-wrong-wf")
		r2.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   "proj-a",
			ocLabels.LabelKeyComponentName: "comp-x",
		}
		svc := newService(t, r1, r2)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "proj-a", "comp-x", "wf-alpha", services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "run-all-match", result.Items[0].Name)
	})

	t.Run("combined filters with no matches returns empty", func(t *testing.T) {
		r1 := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "run-nomatch")
		r1.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   "proj-a",
			ocLabels.LabelKeyComponentName: "comp-x",
		}
		svc := newService(t, r1)

		result, err := svc.ListWorkflowRuns(ctx, testNamespace, "proj-a", "comp-z", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})
}

func TestMatchesTaskNameEdgeCases(t *testing.T) {
	t.Run("empty node name", func(t *testing.T) {
		assert.False(t, matchesTaskName("", "checkout-source"))
	})

	t.Run("empty task name", func(t *testing.T) {
		assert.False(t, matchesTaskName("checkout-source", ""))
	})

	t.Run("both empty", func(t *testing.T) {
		assert.True(t, matchesTaskName("", ""))
	})

	t.Run("node name with multiple dots matches last segment", func(t *testing.T) {
		assert.True(t, matchesTaskName("workflow[0].group.checkout-source", "checkout-source"))
	})

	t.Run("node name with dot does not match partial segment", func(t *testing.T) {
		assert.False(t, matchesTaskName("workflow[0].checkout-source-extended", "checkout-source"))
	})

	t.Run("task name is substring but not after dot", func(t *testing.T) {
		assert.False(t, matchesTaskName("my-checkout-source", "checkout-source"))
	})
}

func TestComputeWorkflowRunStatusAdditional(t *testing.T) {
	t.Run("unknown condition type returns pending", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "SomeOtherCondition", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusPending, computeWorkflowRunStatus(conditions))
	})

	t.Run("all three conditions true returns failed (highest precedence)", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowRunning", Status: metav1.ConditionTrue},
			{Type: "WorkflowSucceeded", Status: metav1.ConditionTrue},
			{Type: "WorkflowFailed", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusFailed, computeWorkflowRunStatus(conditions))
	})

	t.Run("failed false and running true returns running", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowFailed", Status: metav1.ConditionFalse},
			{Type: "WorkflowRunning", Status: metav1.ConditionTrue},
		}
		assert.Equal(t, workflowRunStatusRunning, computeWorkflowRunStatus(conditions))
	})

	t.Run("all conditions false returns pending", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: "WorkflowFailed", Status: metav1.ConditionFalse},
			{Type: "WorkflowSucceeded", Status: metav1.ConditionFalse},
			{Type: "WorkflowRunning", Status: metav1.ConditionFalse},
		}
		assert.Equal(t, workflowRunStatusPending, computeWorkflowRunStatus(conditions))
	})
}

func TestGenerateWorkflowRunNameAdditional(t *testing.T) {
	t.Run("empty base name", func(t *testing.T) {
		name, err := generateWorkflowRunName("")
		require.NoError(t, err)
		assert.Contains(t, name, "-run-")
		// Should be "-run-" + 8 hex chars
		assert.Len(t, name, len("-run-")+8)
	})

	t.Run("base name at max length produces valid name", func(t *testing.T) {
		// 63 - len("-run-") - 8 = 50 chars max for base
		baseName := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuv1234" // exactly 50 chars
		name, err := generateWorkflowRunName(baseName)
		require.NoError(t, err)
		assert.Len(t, name, 63) // exactly at the limit
		assert.Contains(t, name, "-run-")
		assert.True(t, len(name) <= 63)
	})

	t.Run("name format is base-run-hex", func(t *testing.T) {
		name, err := generateWorkflowRunName("mycomp")
		require.NoError(t, err)
		// Verify the format: should start with "mycomp-run-"
		assert.True(t, len(name) > len("mycomp-run-"))
		prefix := name[:len("mycomp-run-")]
		assert.Equal(t, "mycomp-run-", prefix)
		// Hex suffix should be 8 characters of hex
		hexSuffix := name[len("mycomp-run-"):]
		assert.Len(t, hexSuffix, 8)
		for _, c := range hexSuffix {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"expected hex character, got %c", c)
		}
	})
}

func TestGetNestedStringInParamsAdditional(t *testing.T) {
	makeRaw := func(data map[string]any) *runtime.RawExtension {
		b, _ := json.Marshal(data)
		return &runtime.RawExtension{Raw: b}
	}

	t.Run("deeply nested three levels", func(t *testing.T) {
		raw := makeRaw(map[string]any{
			"a": map[string]any{
				"b": map[string]any{
					"c": "deep-value",
				},
			},
		})
		val, err := getNestedStringInParams(raw, "a.b.c")
		require.NoError(t, err)
		assert.Equal(t, "deep-value", val)
	})

	t.Run("invalid JSON in raw extension", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: []byte("{invalid json")}
		_, err := getNestedStringInParams(raw, "key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})

	t.Run("nil raw bytes", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: nil}
		_, err := getNestedStringInParams(raw, "key")
		require.Error(t, err)
	})

	t.Run("empty path after stripping parameters prefix", func(t *testing.T) {
		raw := makeRaw(map[string]any{"": "value"})
		// "parameters." stripped leaves empty string, which becomes [""]
		val, err := getNestedStringInParams(raw, "parameters.")
		require.NoError(t, err)
		assert.Equal(t, "value", val)
	})

	t.Run("value is boolean not string", func(t *testing.T) {
		raw := makeRaw(map[string]any{"enabled": true})
		_, err := getNestedStringInParams(raw, "enabled")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a string")
	})

	t.Run("value is array not string", func(t *testing.T) {
		raw := makeRaw(map[string]any{"items": []string{"a", "b"}})
		_, err := getNestedStringInParams(raw, "items")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a string")
	})
}

func TestSetNestedStringInParamsAdditional(t *testing.T) {
	makeRaw := func(data map[string]any) *runtime.RawExtension {
		b, _ := json.Marshal(data)
		return &runtime.RawExtension{Raw: b}
	}

	t.Run("invalid JSON in raw extension", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: []byte("{invalid json")}
		_, err := setNestedStringInParams(raw, "key", "value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})

	t.Run("nil raw bytes", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: nil}
		_, err := setNestedStringInParams(raw, "key", "value")
		require.Error(t, err)
	})

	t.Run("deeply nested three levels creates all intermediates", func(t *testing.T) {
		raw := makeRaw(map[string]any{})
		result, err := setNestedStringInParams(raw, "a.b.c", "deep-value")
		require.NoError(t, err)

		val, err := getNestedStringInParams(result, "a.b.c")
		require.NoError(t, err)
		assert.Equal(t, "deep-value", val)
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		raw := makeRaw(map[string]any{"key": "old"})
		result, err := setNestedStringInParams(raw, "key", "new")
		require.NoError(t, err)

		val, err := getNestedStringInParams(result, "key")
		require.NoError(t, err)
		assert.Equal(t, "new", val)
	})

	t.Run("preserves sibling keys", func(t *testing.T) {
		raw := makeRaw(map[string]any{
			"repo": map[string]any{
				"url":    "https://github.com/example",
				"commit": "old-sha",
			},
		})
		result, err := setNestedStringInParams(raw, "repo.commit", "new-sha")
		require.NoError(t, err)

		// Verify updated key
		val, err := getNestedStringInParams(result, "repo.commit")
		require.NoError(t, err)
		assert.Equal(t, "new-sha", val)

		// Verify sibling key is preserved
		url, err := getNestedStringInParams(result, "repo.url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", url)
	})
}

func TestTriggerWorkflowAdditional(t *testing.T) {
	ctx := context.Background()

	buildWorkflowSchema := func() *runtime.RawExtension {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repoUrl": map[string]any{
					"type": "string",
					"x-openchoreo-component-parameter-repository-url": true,
				},
				"commit": map[string]any{
					"type": "string",
					"x-openchoreo-component-parameter-repository-commit": true,
				},
			},
		}
		b, _ := json.Marshal(schema)
		return &runtime.RawExtension{Raw: b}
	}

	buildComponentWithWorkflow := func(namespace, project, name, workflowName string, kind openchoreov1alpha1.WorkflowRefKind) *openchoreov1alpha1.Component {
		params, _ := json.Marshal(map[string]any{
			"repoUrl": "https://github.com/example/repo",
			"commit":  "",
		})
		comp := testutil.NewComponent(namespace, project, name)
		comp.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowConfig{
			Kind: kind,
			Name: workflowName,
			Parameters: &runtime.RawExtension{
				Raw: params,
			},
		}
		return comp
	}

	t.Run("empty repo URL in component parameters", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		emptyRepoParams, _ := json.Marshal(map[string]any{
			"repoUrl": "",
			"commit":  "",
		})
		comp := testutil.NewComponent(testNamespace, "proj", "no-repo-comp")
		comp.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowConfig{
			Kind: openchoreov1alpha1.WorkflowRefKindWorkflow,
			Name: testWorkflowName,
			Parameters: &runtime.RawExtension{
				Raw: emptyRepoParams,
			},
		}
		svc := newService(t, wf, comp)

		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "no-repo-comp", "abc1234f")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty repository URL")
	})

	t.Run("commit SHA too short is valid if at least 7 chars", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "short-sha-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		result, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "short-sha-comp", "abc1234")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", result.Commit)
	})

	t.Run("commit SHA exactly 40 chars is valid", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "full-sha-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		fullSHA := "abcdef1234567890abcdef1234567890abcdef12"
		result, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "full-sha-comp", fullSHA)
		require.NoError(t, err)
		assert.Equal(t, fullSHA, result.Commit)
	})

	t.Run("commit SHA too short rejected", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "too-short-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "too-short-comp", "abc12")
		require.ErrorIs(t, err, ErrInvalidCommitSHA)
	})

	t.Run("commit SHA too long rejected", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "too-long-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		tooLong := "abcdef1234567890abcdef1234567890abcdef1234" // 41 chars
		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "too-long-comp", tooLong)
		require.ErrorIs(t, err, ErrInvalidCommitSHA)
	})

	t.Run("commit SHA with non-hex chars rejected", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "nonhex-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "nonhex-comp", "ghijklm")
		require.ErrorIs(t, err, ErrInvalidCommitSHA)
	})

	t.Run("workflow not found for component", func(t *testing.T) {
		// Component references a workflow that doesn't exist
		comp := buildComponentWithWorkflow(testNamespace, "proj", "missing-wf-comp", "nonexistent-workflow", openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, comp)

		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "missing-wf-comp", "abc1234f")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workflow")
	})

	t.Run("cluster workflow not found for component", func(t *testing.T) {
		comp := buildComponentWithWorkflow(testNamespace, "proj", "missing-cwf-comp", "nonexistent-cwf", openchoreov1alpha1.WorkflowRefKindClusterWorkflow)
		svc := newService(t, comp)

		_, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "missing-cwf-comp", "abc1234f")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get ClusterWorkflow")
	})

	t.Run("response contains correct workflow run labels", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: buildWorkflowSchema(),
		}
		comp := buildComponentWithWorkflow(testNamespace, "proj", "label-comp", testWorkflowName, openchoreov1alpha1.WorkflowRefKindWorkflow)
		svc := newService(t, wf, comp)

		result, err := svc.TriggerWorkflow(ctx, testNamespace, "proj", "label-comp", "abc1234f")
		require.NoError(t, err)

		assert.Equal(t, "label-comp", result.ComponentName)
		assert.Equal(t, "proj", result.ProjectName)
		assert.Equal(t, testNamespace, result.NamespaceName)
		assert.Equal(t, "abc1234f", result.Commit)
		assert.Equal(t, workflowRunStatusPending, result.Status)
		assert.NotEmpty(t, result.Name)
	})
}

func TestCreateWorkflowRunPreservesNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("overrides namespace from input to the provided namespace", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		svc := newService(t, wf)
		run := testutil.NewWorkflowRun("wrong-namespace", testWorkflowName, "ns-override-run")

		result, err := svc.CreateWorkflowRun(ctx, testNamespace, run)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("clears status on create", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, testWorkflowName)
		svc := newService(t, wf)
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "status-cleared-run")
		run.Status = openchoreov1alpha1.WorkflowRunStatus{
			Conditions: []metav1.Condition{
				{Type: "WorkflowRunning", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Now()},
			},
		}

		result, err := svc.CreateWorkflowRun(ctx, testNamespace, run)
		require.NoError(t, err)
		assert.Equal(t, openchoreov1alpha1.WorkflowRunStatus{}, result.Status)
	})
}

func TestUpdateWorkflowRunPreservesServerFields(t *testing.T) {
	ctx := context.Background()

	t.Run("preserves resource version from server", func(t *testing.T) {
		existing := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "rv-test")
		svc := newService(t, existing)

		update := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "rv-test")
		update.Labels = map[string]string{"new": "label"}

		result, err := svc.UpdateWorkflowRun(ctx, testNamespace, update)
		require.NoError(t, err)
		// ResourceVersion should be set by the server (fake client increments it)
		assert.NotEmpty(t, result.ResourceVersion)
		assert.Equal(t, "label", result.Labels["new"])
	})
}

func TestDeleteWorkflowRunVerifiesRemoval(t *testing.T) {
	ctx := context.Background()

	t.Run("double delete returns not found", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, "double-del")
		svc := newService(t, run)

		err := svc.DeleteWorkflowRun(ctx, testNamespace, "double-del")
		require.NoError(t, err)

		err = svc.DeleteWorkflowRun(ctx, testNamespace, "double-del")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})
}
