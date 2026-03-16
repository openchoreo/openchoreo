// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

type systemAction string

const (
	SystemActionCreateWorkflowRun systemAction = "workflowrun:create"
	SystemActionViewWorkflowRun   systemAction = "workflowrun:view"

	SystemActionViewWorkflowPlane systemAction = "workflowplane:view"
)

type ResourceType string

const (
	ResourceTypeWorkflowPlane ResourceType = "workflowPlane"
	ResourceTypeWorkflowRun   ResourceType = "workflowRun"
)

// Workflow run status constants
const (
	WorkflowRunStatusPending   = "Pending"
	WorkflowRunStatusRunning   = "Running"
	WorkflowRunStatusSucceeded = "Succeeded"
	WorkflowRunStatusFailed    = "Failed"
)

// Resource ready status constants
const (
	statusReady    = "Ready"
	statusNotReady = "NotReady"
	statusUnknown  = "Unknown"
)
