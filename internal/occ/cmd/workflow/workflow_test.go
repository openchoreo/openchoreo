// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestIsComponentWorkflow(t *testing.T) {
	tests := []struct {
		name string
		wf   gen.Workflow
		want bool
	}{
		{
			name: "has component-scope label",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{
					Name:   "wf-1",
					Labels: &map[string]string{labels.LabelKeyWorkflowType: labels.LabelValueWorkflowTypeComponent},
				},
			},
			want: true,
		},
		{
			name: "wrong value",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{
					Name:   "wf-2",
					Labels: &map[string]string{labels.LabelKeyWorkflowType: "other"},
				},
			},
			want: false,
		},
		{
			name: "no labels",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{Name: "wf-3"},
			},
			want: false,
		},
		{
			name: "different key",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{
					Name:   "wf-4",
					Labels: &map[string]string{"unrelated": "value"},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isComponentWorkflow(tt.wf))
		})
	}
}

func TestApplySetOverrides(t *testing.T) {
	baseRun := func(name, workflowName string) gen.WorkflowRun {
		ns := "test-ns"
		return gen.WorkflowRun{
			Metadata: gen.ObjectMeta{
				Name:      name,
				Namespace: &ns,
			},
			Spec: &gen.WorkflowRunSpec{
				Workflow: gen.WorkflowRunConfig{
					Name: workflowName,
				},
			},
		}
	}

	t.Run("empty set values returns unchanged", func(t *testing.T) {
		req := baseRun("noop-run", "build-wf")
		got, err := applySetOverrides(req, "build-wf", nil)
		require.NoError(t, err)
		assert.Equal(t, "noop-run", got.Metadata.Name)
		assert.Equal(t, "build-wf", got.Spec.Workflow.Name)
	})

	t.Run("override metadata name", func(t *testing.T) {
		req := baseRun("original-run", "deploy-wf")
		got, err := applySetOverrides(req, "deploy-wf", []string{"metadata.name=renamed-run"})
		require.NoError(t, err)
		assert.Equal(t, "renamed-run", got.Metadata.Name)
	})

	t.Run("workflow name override is enforced back", func(t *testing.T) {
		req := baseRun("enforce-run", "protected-wf")
		got, err := applySetOverrides(req, "protected-wf", []string{"spec.workflow.name=hijacked"})
		require.NoError(t, err)
		assert.Equal(t, "protected-wf", got.Spec.Workflow.Name, "workflow name should be enforced")
	})

	t.Run("invalid set value returns error", func(t *testing.T) {
		req := baseRun("bad-input-run", "test-wf")
		_, err := applySetOverrides(req, "test-wf", []string{"no-equals-sign"})
		require.Error(t, err)
	})

	t.Run("multiple overrides applied", func(t *testing.T) {
		req := baseRun("multi-override-run", "ci-wf")
		got, err := applySetOverrides(req, "ci-wf", []string{
			"metadata.name=custom-run",
		})
		require.NoError(t, err)
		assert.Equal(t, "custom-run", got.Metadata.Name)
		assert.Equal(t, "ci-wf", got.Spec.Workflow.Name, "workflow name should be enforced")
	})
}
