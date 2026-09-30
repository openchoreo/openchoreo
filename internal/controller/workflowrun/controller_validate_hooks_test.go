// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func hookRun(lbls map[string]string, owners ...metav1.OwnerReference) *openchoreodevv1alpha1.WorkflowRun {
	return &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "scan-pre-trivy-abc", Namespace: "ns", Labels: lbls, OwnerReferences: owners},
		Spec:       openchoreodevv1alpha1.WorkflowRunSpec{Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: "ClusterWorkflow", Name: "trivy"}},
	}
}

// Deployment-hook runs reference a hook's workflow, which is never in a
// component's allowedWorkflows; keying the exemption on the purpose label (and
// only that label) is what keeps build runs under today's rules while letting
// hook runs through. A run that merely claims to be a hook must prove it with
// the owner reference and the hook labels, or anyone could bypass build rules.
func TestValidateDeploymentHookWorkflowRun(t *testing.T) {
	owner := metav1.OwnerReference{APIVersion: "openchoreo.dev/v1alpha1", Kind: "ReleaseBinding", Name: "web-bff-prod", UID: "u"}
	full := map[string]string{
		labels.LabelKeyWorkflowPurpose: labels.LabelValueWorkflowPurposeDeploymentHook,
		labels.LabelKeyHook:            "scan",
		labels.LabelKeyHookPhase:       "preDeploy",
		labels.LabelKeyReleaseBinding:  "web-bff-prod",
		// A hook run may also carry component labels; they must not pull it into build validation.
		labels.LabelKeyProjectName:   "shop",
		labels.LabelKeyComponentName: "web-bff",
	}
	// r has no client: reaching the component lookup would panic, which is the point.
	r := &Reconciler{}

	t.Run("valid hook run skips build validation", func(t *testing.T) {
		res := r.validateComponentWorkflowRun(context.Background(), hookRun(full, owner))
		if res.shouldReturn {
			t.Fatalf("expected pass, got %+v", res)
		}
	})
	t.Run("missing hook labels fails", func(t *testing.T) {
		lbls := map[string]string{labels.LabelKeyWorkflowPurpose: labels.LabelValueWorkflowPurposeDeploymentHook, labels.LabelKeyReleaseBinding: "web-bff-prod"}
		run := hookRun(lbls, owner)
		res := r.validateComponentWorkflowRun(context.Background(), run)
		c := meta.FindStatusCondition(run.Status.Conditions, string(ConditionWorkflowCompleted))
		if !res.shouldReturn || c == nil || c.Reason != string(ReasonComponentValidationFailed) || !strings.Contains(c.Message, labels.LabelKeyHook) {
			t.Fatalf("expected ComponentValidationFailed naming the missing label, got %+v / %+v", res, c)
		}
	})
	t.Run("not owned by the named ReleaseBinding fails", func(t *testing.T) {
		other := owner
		other.Name = "someone-else"
		run := hookRun(full, other)
		res := r.validateComponentWorkflowRun(context.Background(), run)
		c := meta.FindStatusCondition(run.Status.Conditions, string(ConditionWorkflowCompleted))
		if !res.shouldReturn || c == nil || !strings.Contains(c.Message, "must be owned by ReleaseBinding") {
			t.Fatalf("expected ownership failure, got %+v / %+v", res, c)
		}
	})
	t.Run("build purpose keeps existing rules", func(t *testing.T) {
		// Only one of the two component labels: today's rule rejects it before any lookup.
		run := hookRun(map[string]string{labels.LabelKeyWorkflowPurpose: labels.LabelValueWorkflowPurposeBuild, labels.LabelKeyProjectName: "shop"})
		res := r.validateComponentWorkflowRun(context.Background(), run)
		if !res.shouldReturn {
			t.Fatal("build run with a lone project label must still be rejected")
		}
	})
}
