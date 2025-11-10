// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline orchestrates the complete rendering workflow for Component resources.
// It combines Component, ComponentTypeDefinition, Addons, Workload and ComponentDeployment
// to generate fully resolved Kubernetes resource manifests.
type Pipeline struct {
	templateEngine *template.Engine
	options        RenderOptions
}

// RenderInput contains all inputs needed to render a component's resources.
type RenderInput struct {
	// ComponentType is the component type containing resource templates.
	// Required.
	ComponentType *v1alpha1.ComponentType

	// Component is the component specification with parameters.
	// Required.
	Component *v1alpha1.Component

	// Addons is the list of addon definitions used by the component.
	// Optional - if nil or empty, no addons are processed.
	Addons []v1alpha1.Addon

	// Workload contains the workload spec with build information.
	// Required.
	Workload *v1alpha1.Workload

	// Environment to which the component is being deployed.
	// Required.
	Environment *v1alpha1.Environment

	// ComponentDeployment contains environment-specific overrides for the component.
	// Optional - if nil, no environment overrides are applied.
	ComponentDeployment *v1alpha1.ComponentDeployment

	// DataPlane contains the data plane configuration.
	// Optional - can be nil if no data plane is configured.
	DataPlane *v1alpha1.DataPlane

	// SecretReferences is a map of SecretReference objects needed for rendering.
	// Keyed by SecretReference name.
	// Optional - can be nil if no secret references need to be resolved.
	SecretReferences map[string]*v1alpha1.SecretReference

	// Metadata provides structured naming information.
	// Required - controller must compute and provide this.
	Metadata pipelinecontext.MetadataContext
}

// RenderOutput contains the results of the rendering process.
type RenderOutput struct {
	// Resources is the list of fully rendered Kubernetes resource manifests.
	Resources []map[string]any

	// Metadata contains information about the rendering process.
	Metadata *RenderMetadata
}

// RenderMetadata contains information about the rendering process.
type RenderMetadata struct {
	// ResourceCount is the total number of resources rendered.
	ResourceCount int

	// BaseResourceCount is the number of resources from the ComponentTypeDefinition.
	BaseResourceCount int

	// AddonCount is the number of addons processed.
	AddonCount int

	// AddonResourceCount is the number of resources created by addons.
	AddonResourceCount int

	// Warnings contains non-fatal issues encountered during rendering.
	Warnings []string
}

// RenderOptions configures the rendering behavior.
type RenderOptions struct {
	// EnableValidation enables resource validation after rendering.
	// When enabled, resources missing required fields (apiVersion, kind, metadata.name) will cause rendering to fail.
	EnableValidation bool

	// ResourceLabels are additional labels to add to all rendered resources.
	ResourceLabels map[string]string

	// ResourceAnnotations are additional annotations to add to all rendered resources.
	ResourceAnnotations map[string]string
}

// DefaultRenderOptions returns the default rendering options.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		EnableValidation:    true,
		ResourceLabels:      map[string]string{},
		ResourceAnnotations: map[string]string{},
	}
}
