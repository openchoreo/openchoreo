// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import "errors"

var (
	ErrWorkflowPlaneNil           = errors.New("workflow plane is nil")
	ErrWorkflowPlaneNotFound      = errors.New("workflow plane not found")
	ErrWorkflowPlaneAlreadyExists = errors.New("workflow plane already exists")

	errNotImplemented = errors.New("not implemented on the authz-wrapped service as it is not exposed externally")
)
