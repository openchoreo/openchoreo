// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import "errors"

var (
	ErrWorkflowRunNotFound      = errors.New("workflow run not found")
	ErrWorkflowRunAlreadyExists = errors.New("workflow run already exists")
	ErrWorkflowNotFound         = errors.New("workflow not found")
)
