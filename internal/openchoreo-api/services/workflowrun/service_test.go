// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"testing"

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
		_, err := svc.GetWorkflowRunStatus(ctx, testNamespace, "nonexistent", "")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("pending status with no conditions", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)

		result, err := svc.GetWorkflowRunStatus(ctx, testNamespace, testRunName, "")
		require.NoError(t, err)
		assert.Equal(t, workflowRunStatusPending, result.Status)
		assert.Empty(t, result.Steps)
		assert.False(t, result.HasLiveObservability)
	})
}

func TestGetWorkflowRunLogs(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, "nonexistent", "", "", nil)
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("missing run reference", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, testRunName, "", "", nil)
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})
}

func TestGetWorkflowRunEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetWorkflowRunEvents(ctx, testNamespace, "nonexistent", "", "")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("missing run reference", func(t *testing.T) {
		run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
		svc := newService(t, run)
		_, err := svc.GetWorkflowRunEvents(ctx, testNamespace, testRunName, "", "")
		require.ErrorIs(t, err, ErrWorkflowRunReferenceNotFound)
	})
}
