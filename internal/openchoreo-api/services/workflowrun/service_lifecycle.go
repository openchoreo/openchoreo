// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

// AnnotationKeyStopReason records, on the WorkflowRun, why an operator stopped it.
const AnnotationKeyStopReason = "openchoreo.dev/workflow-stop-reason"

// ResumeWorkflowRun mirrors `argo resume`: it clears spec.suspend on the Argo
// Workflow and marks every active Suspend node as Succeeded so the workflow
// controller continues past it. Both changes go through one Update of the
// Workflow object, because the Argo Workflow CRD does not enable the status
// subresource.
//
// Limitation: when Argo has offloaded or compressed the node status
// (status.offloadNodeStatusVersion / status.compressedNodes), the nodes map
// is empty on the object and only spec.suspend can be cleared; a run paused at
// a suspend template then stays paused and must be resumed with the Argo CLI.
func (s *workflowRunService) ResumeWorkflowRun(ctx context.Context, namespaceName, runName string) (*openchoreov1alpha1.WorkflowRun, error) {
	s.logger.Debug("Resuming workflow run", "namespace", namespaceName, "name", runName)

	wfRun, err := s.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	wpClient, wf, err := s.getArgoWorkflow(ctx, namespaceName, wfRun)
	if err != nil {
		return nil, err
	}

	changed := false
	if wf.Spec.Suspend != nil && *wf.Spec.Suspend {
		wf.Spec.Suspend = nil
		changed = true
	}
	now := metav1.Now()
	for id, node := range wf.Status.Nodes {
		if node.Type != argoproj.NodeTypeSuspend || node.Phase != argoproj.NodeRunning {
			continue
		}
		node.Phase = argoproj.NodeSucceeded
		node.FinishedAt = now
		wf.Status.Nodes[id] = node
		changed = true
	}
	if !changed {
		return nil, ErrWorkflowRunNotSuspended
	}

	if err := wpClient.Update(ctx, wf); err != nil {
		s.logger.Error("Failed to resume argo workflow", "error", err, "workflow", wf.Name)
		return nil, fmt.Errorf("failed to resume workflow on the workflow plane: %w", err)
	}

	s.logger.Info("Workflow run resumed", "namespace", namespaceName, "name", runName, "workflow", wf.Name)
	return wfRun, nil
}

// StopWorkflowRun mirrors `argo stop`: it sets spec.shutdown to Stop on the
// Argo Workflow so running steps finish and exit handlers run, then records
// reason on the WorkflowRun as an annotation.
func (s *workflowRunService) StopWorkflowRun(ctx context.Context, namespaceName, runName, reason string) (*openchoreov1alpha1.WorkflowRun, error) {
	s.logger.Debug("Stopping workflow run", "namespace", namespaceName, "name", runName)

	wfRun, err := s.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	wpClient, wf, err := s.getArgoWorkflow(ctx, namespaceName, wfRun)
	if err != nil {
		return nil, err
	}

	switch wf.Status.Phase {
	case argoproj.WorkflowSucceeded, argoproj.WorkflowFailed, argoproj.WorkflowError:
		return nil, ErrWorkflowRunCompleted
	}

	if !wf.Spec.Shutdown.Enabled() {
		wf.Spec.Shutdown = argoproj.ShutdownStrategyStop
		if err := wpClient.Update(ctx, wf); err != nil {
			s.logger.Error("Failed to stop argo workflow", "error", err, "workflow", wf.Name)
			return nil, fmt.Errorf("failed to stop workflow on the workflow plane: %w", err)
		}
	}

	if reason != "" {
		patched := wfRun.DeepCopy()
		if patched.Annotations == nil {
			patched.Annotations = map[string]string{}
		}
		patched.Annotations[AnnotationKeyStopReason] = reason
		if err := s.k8sClient.Patch(ctx, patched, client.MergeFrom(wfRun)); err != nil {
			s.logger.Error("Failed to record stop reason", "error", err)
			return nil, fmt.Errorf("failed to record stop reason on workflow run: %w", err)
		}
		patched.TypeMeta = workflowRunTypeMeta
		wfRun = patched
	}

	s.logger.Info("Workflow run stopped", "namespace", namespaceName, "name", runName, "workflow", wf.Name)
	return wfRun, nil
}

// getArgoWorkflow resolves the workflow plane client for the run and fetches the
// live Argo Workflow named by status.runReference.
func (s *workflowRunService) getArgoWorkflow(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (client.Client, *argoproj.Workflow, error) {
	ref := wfRun.Status.RunReference
	if ref == nil || ref.Name == "" || ref.Namespace == "" {
		return nil, nil, ErrWorkflowRunNotStarted
	}

	workflowPlaneRef, err := s.resolveWorkflowPlaneRef(ctx, namespaceName, wfRun.Spec.Workflow)
	if err != nil {
		return nil, nil, err
	}
	wpClient, err := s.getWorkflowPlaneClient(ctx, namespaceName, workflowPlaneRef)
	if err != nil {
		return nil, nil, err
	}

	wf := &argoproj.Workflow{}
	if err := wpClient.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, wf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil, ErrWorkflowRunNotStarted
		}
		return nil, nil, fmt.Errorf("failed to get workflow %s/%s on the workflow plane: %w", ref.Namespace, ref.Name, err)
	}
	return wpClient, wf, nil
}
