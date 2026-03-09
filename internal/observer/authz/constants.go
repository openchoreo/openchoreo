// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

type Action string

const (
	ActionViewLogs      Action = "logs:view"
	ActionViewTraces    Action = "traces:view"
	ActionViewMetrics   Action = "metrics:view"
	ActionViewAlerts    Action = "alerts:view"
	ActionViewIncidents Action = "incidents:view"
)

type ResourceType string

const (
	ResourceTypeUnknown     ResourceType = "unknown"
	ResourceTypeComponent   ResourceType = "component"
	ResourceTypeProject     ResourceType = "project"
	ResourceTypeNamespace   ResourceType = "namespace"
	ResourceTypeWorkflowRun ResourceType = "workflowRun"
)
