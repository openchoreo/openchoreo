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

	// Resources contains additional rendered Kubernetes resources (e.g., secrets, configmaps)
	// to be applied alongside the main workflow resource.
	Resources []RenderedResource

	// Metadata contains rendering process information such as warnings.
	Metadata *RenderMetadata
}

// RenderedResource represents a rendered Kubernetes resource with its identifier.
type RenderedResource struct {
	// ID is the unique identifier for this resource from the ComponentWorkflow spec.
	ID string

	// Resource is the fully rendered Kubernetes resource as a map.
	Resource map[string]any
}

// RenderMetadata contains non-fatal information about the rendering process.
type RenderMetadata struct {
	// Warnings lists non-fatal issues encountered during rendering.
	Warnings []string
}

// ComponentWorkflowContext provides contextual metadata for component workflow rendering.
// These values are injected into CEL expressions as ${metadata.*} variables.
type ComponentWorkflowContext struct {
	// NamespaceName is the namespace name.
	NamespaceName string

	// ProjectName is the project name from the component workflow owner.
	ProjectName string

	// ComponentName is the component name from the component workflow owner.
	ComponentName string

	// WorkflowRunName is the name of the component workflow run CR.
	WorkflowRunName string
}
