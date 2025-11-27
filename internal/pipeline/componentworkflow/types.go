// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowpipeline

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline orchestrates component workflow rendering by combining ComponentWorkflowRun,
// ComponentWorkflow to generate fully resolved resources (e.g., Argo Workflow).
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput contains all required inputs for component workflow rendering.
type RenderInput struct {
	// ComponentWorkflowRun is the workflow execution instance with system and developer parameters (required).
	ComponentWorkflowRun *v1alpha1.ComponentWorkflowRun

	// ComponentWorkflow contains the schema and resource template (required).
	ComponentWorkflow *v1alpha1.ComponentWorkflow

	// Context provides workflow execution context metadata (required).
	Context ComponentWorkflowContext
}

// RenderOutput contains the rendered workflow resource and associated metadata.
type RenderOutput struct {
	// Resource is the fully rendered workflow resource as a map that can be converted
	// to unstructured.Unstructured for Kubernetes API operations.
	Resource map[string]any

	// Metadata contains rendering process information such as warnings.
	Metadata *RenderMetadata
}

// RenderMetadata contains non-fatal information about the rendering process.
type RenderMetadata struct {
	// Warnings lists non-fatal issues encountered during rendering.
	Warnings []string
}

// ComponentWorkflowContext provides contextual metadata for component workflow rendering.
// These values are injected into CEL expressions as ${ctx.*} variables.
type ComponentWorkflowContext struct {
	// OrgName is the organization name (typically the namespace).
	OrgName string

	// ProjectName is the project name from the component workflow owner.
	ProjectName string

	// ComponentName is the component name from the component workflow owner.
	ComponentName string

	// ComponentWorkflowRunName is the name of the component workflow run CR.
	ComponentWorkflowRunName string
}