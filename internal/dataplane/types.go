// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ProjectContext is a struct that holds the all necessary data required for the resource handlers to perform their operations.
type ProjectContext struct {
	DeploymentPipeline *openchoreov1alpha1.DeploymentPipeline
	Project            *openchoreov1alpha1.Project
	EnvironmentNames   []string
	NamespaceNames     []string
}

// EnvironmentContext is a struct that holds the all necessary data required for the resource handlers of the environment
// to perform its operations.
type EnvironmentContext struct {
	Environment *openchoreov1alpha1.Environment
	DataPlane   *openchoreov1alpha1.DataPlane
}
