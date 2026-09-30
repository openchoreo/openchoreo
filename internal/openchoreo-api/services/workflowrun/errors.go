// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import "errors"

var (
	ErrWorkflowRunNotFound          = errors.New("workflow run not found")
	ErrWorkflowRunAlreadyExists     = errors.New("workflow run already exists")
	ErrWorkflowNotFound             = errors.New("workflow not found")
	ErrWorkflowRunReferenceNotFound = errors.New("workflow run reference not found")
	ErrInvalidCommitSHA             = errors.New("invalid commit SHA format")
	// ErrWorkflowRunNotSuspended is returned by Resume when the run has nothing to resume.
	ErrWorkflowRunNotSuspended = errors.New("workflow run is not suspended")
	// ErrWorkflowRunCompleted is returned by Stop when the run has already reached a terminal phase.
	ErrWorkflowRunCompleted = errors.New("workflow run has already completed")
	// ErrWorkflowRunNotStarted is returned when the run has no live Argo Workflow on the workflow plane yet.
	ErrWorkflowRunNotStarted = errors.New("workflow run has not started on the workflow plane")
)
