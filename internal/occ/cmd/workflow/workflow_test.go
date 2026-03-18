// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"testing"

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
			if got := isComponentWorkflow(tt.wf); got != tt.want {
				t.Errorf("isComponentWorkflow() = %v, want %v", got, tt.want)
			}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Metadata.Name != "noop-run" {
			t.Errorf("name = %q, want %q", got.Metadata.Name, "noop-run")
		}
		if got.Spec.Workflow.Name != "build-wf" {
			t.Errorf("workflow name = %q, want %q", got.Spec.Workflow.Name, "build-wf")
		}
	})

	t.Run("override metadata name", func(t *testing.T) {
		req := baseRun("original-run", "deploy-wf")
		got, err := applySetOverrides(req, "deploy-wf", []string{"metadata.name=renamed-run"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Metadata.Name != "renamed-run" {
			t.Errorf("name = %q, want %q", got.Metadata.Name, "renamed-run")
		}
	})

	t.Run("workflow name override is enforced back", func(t *testing.T) {
		req := baseRun("enforce-run", "protected-wf")
		got, err := applySetOverrides(req, "protected-wf", []string{"spec.workflow.name=hijacked"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Spec.Workflow.Name != "protected-wf" {
			t.Errorf("workflow name = %q, want %q (should be enforced)", got.Spec.Workflow.Name, "protected-wf")
		}
	})

	t.Run("invalid set value returns error", func(t *testing.T) {
		req := baseRun("bad-input-run", "test-wf")
		_, err := applySetOverrides(req, "test-wf", []string{"no-equals-sign"})
		if err == nil {
			t.Fatal("expected error for invalid set value")
		}
	})

	t.Run("multiple overrides applied", func(t *testing.T) {
		req := baseRun("multi-override-run", "ci-wf")
		got, err := applySetOverrides(req, "ci-wf", []string{
			"metadata.name=custom-run",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Metadata.Name != "custom-run" {
			t.Errorf("name = %q, want %q", got.Metadata.Name, "custom-run")
		}
		// Workflow name should still be enforced
		if got.Spec.Workflow.Name != "ci-wf" {
			t.Errorf("workflow name = %q, want %q", got.Spec.Workflow.Name, "ci-wf")
		}
	})
}
