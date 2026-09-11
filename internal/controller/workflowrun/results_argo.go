// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

// argoResultExtractor reads outputs off an Argo Workflow's node statuses.
//
// Argo already carries what results need: every Pod node's outputs.parameters holds the
// values that step declared, which is why the shipped builders can surface an image
// reference without a new template - the value was always there, nothing read it.
type argoResultExtractor struct {
	run *argoproj.Workflow
}

// newArgoResultExtractor binds an extractor to one Argo run.
func newArgoResultExtractor(run *argoproj.Workflow) ResultExtractor {
	return &argoResultExtractor{run: run}
}

var _ ResultExtractor = (*argoResultExtractor)(nil)

// Outputs collects each task's output parameters, one entry per task.
func (e *argoResultExtractor) Outputs() RunOutputs {
	outputs := make(RunOutputs)
	for name, node := range e.winningNodes() {
		if node.Outputs == nil {
			continue
		}
		taskOutputs := make(TaskOutputs, len(node.Outputs.Parameters))
		for _, param := range node.Outputs.Parameters {
			if param.Name == "" || param.Value == nil {
				continue
			}
			taskOutputs[param.Name] = string(*param.Value)
		}
		if len(taskOutputs) == 0 {
			continue
		}
		outputs[name] = taskOutputs
	}

	if len(outputs) == 0 {
		return nil
	}
	return outputs
}

// Phases reports each task's phase under the same names Outputs uses, so an expression can
// branch on whether a step ran. Both read the same winning node, so a task's phase always
// describes the attempt its outputs came from.
func (e *argoResultExtractor) Phases() map[string]string {
	phases := make(map[string]string)
	for name, node := range e.winningNodes() {
		phases[name] = string(node.Phase)
	}

	if len(phases) == 0 {
		return nil
	}
	return phases
}

// winningNodes picks the one Pod node that represents each task.
//
// Only Pod nodes are considered, matching the task list in status: a step's outputs are
// duplicated onto its enclosing StepGroup and DAG nodes, and including those would key the
// same values under a second set of names that never appear in status.tasks.
//
// Two Pod nodes can still resolve to one task name - a retried step, or the same template
// used twice - and Status.Nodes is a map, so simply taking the first or last one seen would
// make the answer depend on Go's map iteration order. Superseded retry attempts are dropped
// first, then any remaining tie is broken on finish time and node name, so a run always
// yields the same results.
func (e *argoResultExtractor) winningNodes() map[string]argoproj.NodeStatus {
	if e.run == nil || e.run.Status.Nodes == nil {
		return nil
	}

	superseded := supersededAttempts(e.run.Status.Nodes)
	retryNames := retryAttemptNames(e.run.Status.Nodes)

	winners := make(map[string]argoproj.NodeStatus)
	for _, node := range e.run.Status.Nodes {
		if node.Type != argoproj.NodeTypePod || superseded[node.ID] {
			continue
		}
		name := argoTaskNameFor(node, retryNames)
		if name == "" {
			continue
		}
		if current, ok := winners[name]; ok && !supersedes(node, current) {
			continue
		}
		winners[name] = node
	}
	return winners
}

// supersedes reports whether candidate should replace current as a task's representative:
// the later finish wins, and an equal finish falls back to node name so the choice is
// stable rather than dependent on iteration order.
func supersedes(candidate, current argoproj.NodeStatus) bool {
	if !candidate.FinishedAt.Equal(&current.FinishedAt) {
		return current.FinishedAt.Before(&candidate.FinishedAt)
	}
	return candidate.Name > current.Name
}

// retryAttemptNames maps a retry attempt's node ID to the display name of the Retry node
// that owns it.
//
// Argo suffixes each attempt with its index: a step "publish-image" under a retryStrategy
// produces Pod nodes whose own displayName is "publish-image(0)", "publish-image(1)", and so
// on, while the Retry parent keeps the bare "publish-image". Naming the task after the Pod
// would put an entry in status that no results[].valueFrom.taskResult.task can reference -
// the declaration says "publish-image" and would never match.
func retryAttemptNames(nodes argoproj.Nodes) map[string]string {
	names := make(map[string]string)
	for _, node := range nodes {
		if node.Type != argoproj.NodeTypeRetry {
			continue
		}
		parent := argoTaskName(node)
		if parent == "" {
			continue
		}
		for _, id := range node.Children {
			names[id] = parent
		}
	}
	return names
}

// supersededAttempts returns the node IDs of retry attempts that a later attempt replaced.
//
// Argo keeps every attempt of a retried step as a Pod child of a NodeTypeRetry node and
// treats the last child as the retry's result. Reporting an earlier attempt's outputs -
// which is what the failed attempt of a step that later succeeded would give - would be
// wrong, so drop them before anything else looks at the nodes.
func supersededAttempts(nodes argoproj.Nodes) map[string]bool {
	superseded := make(map[string]bool)
	for _, node := range nodes {
		if node.Type != argoproj.NodeTypeRetry || len(node.Children) < 2 {
			continue
		}
		for _, id := range node.Children[:len(node.Children)-1] {
			superseded[id] = true
		}
	}
	return superseded
}
