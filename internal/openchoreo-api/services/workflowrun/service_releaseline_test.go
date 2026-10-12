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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestTriggerWorkflow_ReleaseLine(t *testing.T) {
	ctx := context.Background()

	rawJSON := func(v any) *runtime.RawExtension {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return &runtime.RawExtension{Raw: b}
	}

	newComponent := func(name string, enabled bool) *openchoreov1alpha1.Component {
		comp := testutil.NewComponent(testNamespace, "proj", name)
		comp.Spec.ReleaseLinesEnabled = enabled
		comp.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowConfig{
			Kind: openchoreov1alpha1.WorkflowRefKindWorkflow,
			Name: testWorkflowName,
			Parameters: rawJSON(map[string]any{
				"repository": map[string]any{
					"url":      "https://github.com/example/repo",
					"revision": map[string]any{"branch": "main"},
				},
			}),
		}
		return comp
	}
	newLine := func(name, component string, branch string) *openchoreov1alpha1.ReleaseLine {
		line := &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: "proj", ComponentName: component},
			},
		}
		if branch != "" {
			line.Spec.Workflow = &openchoreov1alpha1.ReleaseLineWorkflow{
				Parameters: rawJSON(map[string]any{"repository": map[string]any{"revision": map[string]any{"branch": branch}}}),
			}
		}
		return line
	}

	objects := []client.Object{
		testutil.NewWorkflow(testNamespace, testWorkflowName),
		newComponent("lines", true),
		newComponent("plain", false),
		newLine("lines-next", "lines", "next"),
		newLine("lines-main", "lines", ""),
		newLine("other-next", "other", "next"),
	}
	k8sClient := testutil.NewFakeClient(objects...)
	svc := NewService(k8sClient, nil, nil, testutil.TestLogger())

	fetchRun := func(t *testing.T, name string) *openchoreov1alpha1.WorkflowRun {
		t.Helper()
		run := &openchoreov1alpha1.WorkflowRun{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, run))
		return run
	}

	t.Run("applies the line's workflow parameters and labels the run", func(t *testing.T) {
		result, err := svc.TriggerWorkflow(ctx, testNamespace, "", "lines", "", "lines-next")
		require.NoError(t, err)

		run := fetchRun(t, result.Name)
		assert.Equal(t, "lines-next", run.Labels[ocLabels.LabelKeyReleaseLine])
		var params map[string]any
		require.NoError(t, json.Unmarshal(run.Spec.Workflow.Parameters.Raw, &params))
		assert.Equal(t, map[string]any{
			"url":      "https://github.com/example/repo",
			"revision": map[string]any{"branch": "next"},
		}, params["repository"])
	})

	t.Run("a line without overrides builds the component's parameters", func(t *testing.T) {
		result, err := svc.TriggerWorkflow(ctx, testNamespace, "", "lines", "", "lines-main")
		require.NoError(t, err)

		run := fetchRun(t, result.Name)
		assert.Equal(t, "lines-main", run.Labels[ocLabels.LabelKeyReleaseLine])
		assert.JSONEq(t, `{"repository":{"url":"https://github.com/example/repo","revision":{"branch":"main"}}}`,
			string(run.Spec.Workflow.Parameters.Raw))
	})

	t.Run("a component without release lines is unchanged", func(t *testing.T) {
		result, err := svc.TriggerWorkflow(ctx, testNamespace, "", "plain", "", "")
		require.NoError(t, err)
		assert.NotContains(t, fetchRun(t, result.Name).Labels, ocLabels.LabelKeyReleaseLine)
	})

	t.Run("rejects invalid combinations", func(t *testing.T) {
		_, err := svc.TriggerWorkflow(ctx, testNamespace, "", "lines", "", "")
		require.ErrorIs(t, err, ErrReleaseLineRequired)

		_, err = svc.TriggerWorkflow(ctx, testNamespace, "", "plain", "", "lines-next")
		require.ErrorIs(t, err, ErrReleaseLinesNotEnabled)

		_, err = svc.TriggerWorkflow(ctx, testNamespace, "", "lines", "", "absent")
		require.ErrorIs(t, err, ErrReleaseLineNotFound)

		_, err = svc.TriggerWorkflow(ctx, testNamespace, "", "lines", "", "other-next")
		require.ErrorIs(t, err, ErrReleaseLineNotFound, "a line of another component is not this component's line")
	})
}
