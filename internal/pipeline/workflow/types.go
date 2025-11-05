// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowpipeline

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline orchestrates workflow rendering by combining Workflow, WorkflowDefinition,
// and ComponentTypeDefinition to generate fully resolved resources (e.g., Argo Workflow).
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput contains all required inputs for workflow rendering.
type RenderInput struct {
	// Workflow is the workflow instance with developer parameters (required).
	Workflow *v1alpha1.Workflow

	// WorkflowDefinition contains the schema, fixed parameters, and resource template (required).
	WorkflowDefinition *v1alpha1.WorkflowDefinition

	// ComponentTypeDefinition provides component-type-specific parameter overrides (optional).
	ComponentTypeDefinition *v1alpha1.ComponentTypeDefinition

	// Context provides workflow execution context metadata (required).
	Context WorkflowContext
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

// WorkflowContext provides contextual metadata for workflow rendering.
// These values are injected into CEL expressions as ${ctx.*} variables.
type WorkflowContext struct {
	// OrgName is the organization name (typically the namespace).
	OrgName string

	// ProjectName is the project name from the workflow owner.
	ProjectName string

	// ComponentName is the component name from the workflow owner.
	ComponentName string

	// WorkflowName is the name of the workflow CR.
	WorkflowName string

	// Timestamp is the Unix timestamp when rendering started (auto-generated).
	Timestamp int64

	// UUID is a short unique identifier (8 chars) for this workflow execution (auto-generated).
	UUID string
}
