// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

type systemAction string

const (
	SystemActionCreateProject systemAction = "project:create"
	SystemActionViewProject   systemAction = "project:view"
	SystemActionDeleteProject systemAction = "project:delete"
)

type ResourceType string

const (
	ResourceTypeProject ResourceType = "project"
)
