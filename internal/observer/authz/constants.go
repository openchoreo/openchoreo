// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

type Action string

const (
	ActionViewLogs    Action = "logs:view"
	ActionViewTraces  Action = "traces:view"
	ActionViewMetrics Action = "metrics:view"
)

type ResourceType string

const (
	ResourceTypeComponent ResourceType = "component"
	ResourceTypeProject   ResourceType = "project"
	ResourceTypeOrg       ResourceType = "organization"
)
