// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

// Action represents an authorization action for RCA agent resources.
type Action string

const (
	ActionViewRCAReport   Action = "rcareport:view"
	ActionUpdateRCAReport Action = "rcareport:update"
)

// ResourceType represents a resource type in the authorization model.
type ResourceType string

const (
	ResourceTypeUnknown   ResourceType = "unknown"
	ResourceTypeComponent ResourceType = "component"
	ResourceTypeProject   ResourceType = "project"
	ResourceTypeNamespace ResourceType = "namespace"
)
