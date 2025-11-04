// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowpipeline

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/crd-renderer/template"
)

// Pipeline orchestrates the complete rendering workflow for Workflow resources.
// It combines Workflow, WorkflowDefinition, and ComponentTypeDefinition
// to generate fully resolved workflow resources (e.g., Argo Workflow).
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput contains all inputs needed to render a workflow resource.
type RenderInput struct {
	// Workflow is the workflow instance with developer parameters.
	// Required.
	Workflow *v1alpha1.Workflow

	// WorkflowDefinition contains the schema, fixed parameters, and resource template.
	// Required.
	WorkflowDefinition *v1alpha1.WorkflowDefinition

	// ComponentTypeDefinition provides parameter overrides specific to component type.
	// Optional - if nil, no component-type-specific overrides are applied.
	ComponentTypeDefinition *v1alpha1.ComponentTypeDefinition

	// Context provides workflow execution context (org, project, component metadata).
	// Required - controller must compute and provide this.
	Context WorkflowContext
}

// RenderOutput contains the results of the rendering process.
type RenderOutput struct {
	// Resource is the fully rendered workflow resource (e.g., Argo Workflow).
	// This is a map[string]any representation that can be converted to unstructured.Unstructured.
	Resource map[string]any

	// Metadata contains information about the rendering process.
	Metadata *RenderMetadata
}

// RenderMetadata contains information about the rendering process.
type RenderMetadata struct {
	// Warnings contains non-fatal issues encountered during rendering.
	Warnings []string
}

// WorkflowContext provides contextual information for workflow rendering.
// These values are injected into CEL expressions as ${ctx.*} variables.
type WorkflowContext struct {
	// OrgName is the organization name (typically the namespace)
	OrgName string

	// ProjectName is the project name from the workflow owner
	ProjectName string

	// ComponentName is the component name from the workflow owner
	ComponentName string

	// WorkflowName is the name of the workflow CR
	WorkflowName string

	// Timestamp is the Unix timestamp when rendering started.
	// Auto-generated during rendering.
	Timestamp int64

	// UUID is a short unique identifier (8 chars) for this workflow execution.
	// Auto-generated during rendering.
	UUID string
}
