// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import "errors"

// Common service errors
var (
	ErrWorkflowNotFound             = errors.New("workflow not found")
	ErrWorkflowRunNotFound          = errors.New("workflow run not found")
	ErrWorkflowRunAlreadyExists     = errors.New("workflow run already exists")
	ErrWorkflowRunReferenceNotFound = errors.New("workflow run reference not found")
	ErrInvalidCommitSHA             = errors.New("invalid commit SHA format")
	ErrForbidden                    = errors.New("insufficient permissions to perform this action")
	ErrWorkflowPlaneNotFound        = errors.New("workflow plane not found")
)
