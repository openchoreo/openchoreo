// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowpipeline

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline orchestrates workflow rendering by combining WorkflowRun,
// Workflow to generate fully resolved resources (e.g., Argo Workflow).
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput contains all required inputs for workflow rendering.
type RenderInput struct {
	// WorkflowRun is the workflow execution instance with developer parameters (required).
	WorkflowRun *v1alpha1.WorkflowRun

	// Workflow contains the schema and resource template (required).
	Workflow *v1alpha1.Workflow

	// Context provides workflow execution context metadata (required).
	Context WorkflowContext
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
	// ID is the unique identifier for this resource from the Workflow spec.
	ID string

	// Resource is the fully rendered Kubernetes resource as a map.
	Resource map[string]any
}

// RenderMetadata contains non-fatal information about the rendering process.
type RenderMetadata struct {
	// Warnings lists non-fatal issues encountered during rendering.
	Warnings []string
}

// WorkflowContext provides contextual metadata for workflow rendering.
// These values are injected into CEL expressions as ${metadata.*} variables.
type WorkflowContext struct {
	// NamespaceName is the namespace name.
	NamespaceName string

	// WorkflowRunName is the name of the workflow run CR.
	WorkflowRunName string

	// SecretRef contains resolved SecretReference data for template rendering.
	// This is optional and populated only when workflow annotations map a secretRef parameter.
	SecretRef *SecretRefInfo
}

// SecretRefInfo contains resolved SecretReference details exposed to CEL as secretRef.*.
type SecretRefInfo struct {
	// Name is the SecretReference resource name.
	Name string

	// Type is the Kubernetes secret type from SecretReference.spec.template.type.
	Type string

	// Data contains key-to-remote reference mappings from SecretReference.spec.data.
	Data []SecretDataInfo
}

// SecretDataInfo represents a single SecretReference data item.
type SecretDataInfo struct {
	// SecretKey is the key name in the resulting Kubernetes Secret.
	SecretKey string

	// RemoteRef points to the backing external secret.
	RemoteRef RemoteRefInfo
}

// RemoteRefInfo represents a remote secret reference.
type RemoteRefInfo struct {
	// Key is the remote secret key/path.
	Key string

	// Property is the optional field in the remote secret value.
	Property string
}
