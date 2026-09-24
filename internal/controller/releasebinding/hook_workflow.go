// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding/gate"
	"github.com/openchoreo/openchoreo/internal/controller/workflowrun"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Workflow executor for deployment hooks: one WorkflowRun per binding attempt,
// owned by the ReleaseBinding and observed through the WorkflowRun conditions
// the workflowrun controller maintains.

var (
	// errHookWorkflowNotFound: the hook's workflowRef does not resolve.
	errHookWorkflowNotFound = errors.New("hook workflow not found")
	// errHookPlaneUnavailable: the workflow's plane does not resolve. The gate
	// fails closed on this for Sync pre-deploy bindings.
	errHookPlaneUnavailable = errors.New("workflow plane unavailable")
)

const (
	hookRunNameMaxLength = 63
	hookKeyPrefixLength  = 8
)

// hookRunName builds the WorkflowRun name for one attempt:
// <binding>-<pre|post>-<hook>-<key[:8]>[-a<attempt>], shortened with a hash
// suffix when it would exceed the DNS label limit.
func hookRunName(binding string, phase gate.Phase, hookName, key string, attempt int32) string {
	short := "pre"
	if phase == gate.PhasePostDeploy {
		short = "post"
	}
	keyPart := key
	if len(keyPart) > hookKeyPrefixLength {
		keyPart = keyPart[:hookKeyPrefixLength]
	}
	if attempt > 1 {
		keyPart = fmt.Sprintf("%s-a%d", keyPart, attempt)
	}
	name := strings.ToLower(fmt.Sprintf("%s-%s-%s-%s", binding, short, hookName, keyPart))
	if len(name) <= hookRunNameMaxLength {
		return name
	}
	return dpkubernetes.GenerateK8sNameWithLengthLimit(hookRunNameMaxLength, binding, short, hookName, keyPart)
}

// ensureHookRun returns the WorkflowRun for the attempt, creating it when
// absent. Creation resolves the hook's workflow and its plane first so that a
// missing plane is reported before anything is written.
func (r *Reconciler) ensureHookRun(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding,
	h gate.ResolvedHook, key string, attempt int32, params map[string]string) (*openchoreov1alpha1.WorkflowRun, bool, error) {
	ref := h.Hook.WorkflowRef
	if ref == nil {
		return nil, false, errHookWorkflowNotFound
	}
	name := hookRunName(h.Binding.Name, h.Phase, ref.Name, key, attempt)

	existing := &openchoreov1alpha1.WorkflowRun{}
	err := r.Get(ctx, client.ObjectKey{Namespace: rb.Namespace, Name: name}, existing)
	if err == nil {
		return existing, false, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, false, fmt.Errorf("getting WorkflowRun %q: %w", name, err)
	}

	wf, err := controller.ResolveWorkflow(ctx, r.Client, rb.Namespace, ref.Kind, ref.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, fmt.Errorf("%w: %w", errHookWorkflowNotFound, err)
		}
		return nil, false, err
	}
	if _, err := controller.GetWorkflowPlaneFromRef(ctx, r.Client, rb.Namespace, wf.GetWorkflowSpec().WorkflowPlaneRef); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, fmt.Errorf("%w: %w", errHookPlaneUnavailable, err)
		}
		return nil, false, err
	}

	raw, err := json.Marshal(params)
	if err != nil {
		return nil, false, fmt.Errorf("encoding hook parameters: %w", err)
	}
	run := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rb.Namespace,
			Labels: map[string]string{
				labels.LabelKeyNamespaceName:   rb.Namespace,
				labels.LabelKeyWorkflowPurpose: labels.LabelValueWorkflowPurposeDeploymentHook,
				labels.LabelKeyHook:            h.Binding.Name,
				labels.LabelKeyHookPhase:       string(h.Phase),
				labels.LabelKeyReleaseBinding:  rb.Name,
				labels.LabelKeyEnvironmentName: rb.Spec.Environment,
			},
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Kind:       ref.Kind,
				Name:       ref.Name,
				Parameters: &runtime.RawExtension{Raw: raw},
			},
		},
	}
	if err := controllerutil.SetControllerReference(rb, run, r.Scheme); err != nil {
		return nil, false, fmt.Errorf("setting owner reference on WorkflowRun %q: %w", name, err)
	}
	if err := r.Create(ctx, run); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if getErr := r.Get(ctx, client.ObjectKey{Namespace: rb.Namespace, Name: name}, existing); getErr == nil {
				return existing, false, nil
			}
		}
		return nil, false, fmt.Errorf("creating WorkflowRun %q: %w", name, err)
	}
	return run, true, nil
}

// observeHookRun maps a WorkflowRun's conditions to a hook phase. The message
// carries the failing task's name and message when the run reports tasks.
func observeHookRun(run *openchoreov1alpha1.WorkflowRun) (phase openchoreov1alpha1.HookPhase, reason, message string) {
	conds := run.Status.Conditions
	if meta.IsStatusConditionTrue(conds, string(workflowrun.ConditionWorkflowSucceeded)) {
		return openchoreov1alpha1.HookPhaseSucceeded, string(workflowrun.ReasonWorkflowSucceeded), "Workflow completed successfully"
	}
	if meta.IsStatusConditionTrue(conds, string(workflowrun.ConditionWorkflowFailed)) {
		msg := "Workflow execution failed"
		if c := meta.FindStatusCondition(conds, string(workflowrun.ConditionWorkflowFailed)); c != nil && c.Message != "" {
			msg = c.Message
		}
		if task := failingTask(run.Status.Tasks); task != nil {
			msg = fmt.Sprintf("task %q failed", task.Name)
			if task.Message != "" {
				msg += ": " + task.Message
			}
		}
		return openchoreov1alpha1.HookPhaseFailed, string(workflowrun.ReasonWorkflowFailed), msg
	}
	if c := meta.FindStatusCondition(conds, string(workflowrun.ConditionWorkflowCompleted)); c != nil {
		switch {
		case c.Status == metav1.ConditionTrue:
			// Completed without WorkflowSucceeded: a terminal failure such as
			// WorkflowNotFound or ComponentValidationFailed.
			return openchoreov1alpha1.HookPhaseFailed, c.Reason, c.Message
		case c.Reason == string(workflowrun.ReasonWorkflowPlaneNotFound),
			c.Reason == string(workflowrun.ReasonWorkflowPlaneResolutionFailed):
			return openchoreov1alpha1.HookPhasePlaneUnavailable, c.Reason, c.Message
		}
	}
	msg := "Workflow is running"
	if task := runningTask(run.Status.Tasks); task != nil {
		msg = fmt.Sprintf("task %q is running", task.Name)
	}
	return openchoreov1alpha1.HookPhaseRunning, string(workflowrun.ReasonWorkflowRunning), msg
}

func failingTask(tasks []openchoreov1alpha1.WorkflowTask) *openchoreov1alpha1.WorkflowTask {
	for i := range tasks {
		if tasks[i].Phase == "Failed" || tasks[i].Phase == "Error" {
			return &tasks[i]
		}
	}
	return nil
}

func runningTask(tasks []openchoreov1alpha1.WorkflowTask) *openchoreov1alpha1.WorkflowTask {
	for i := range tasks {
		if tasks[i].Phase == "Running" {
			return &tasks[i]
		}
	}
	return nil
}
