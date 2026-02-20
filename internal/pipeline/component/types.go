// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline orchestrates the complete rendering workflow for Component resources.
// It combines Component, ComponentType, Traits, Workload and ReleaseBinding
// to generate fully resolved Kubernetes resource manifests.
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput contains all inputs needed to render a component's resources.
type RenderInput struct {
	// ComponentType is the component type containing resource templates.
	// Required.
	ComponentType *v1alpha1.ComponentType `validate:"required"`

	// Component is the component specification with parameters.
	// Required.
	Component *v1alpha1.Component `validate:"required"`

	// Traits is the list of trait definitions used by the component.
	// Optional - if nil or empty, no traits are processed.
	Traits []v1alpha1.Trait

	// Workload contains the workload spec with build information.
	// Required.
	Workload *v1alpha1.Workload `validate:"required"`

	// Environment to which the component is being deployed.
	// Required.
	Environment *v1alpha1.Environment `validate:"required"`

	// ReleaseBinding contains release reference and environment-specific overrides for the component.
	ReleaseBinding *v1alpha1.ReleaseBinding

	// DataPlane contains the data plane configuration.
	// Required
	DataPlane *v1alpha1.DataPlane `validate:"required"`

	// SecretReferences is a map of SecretReference objects needed for rendering.
	// Keyed by SecretReference name.
	// Optional - can be nil if no secret references need to be resolved.
	SecretReferences map[string]*v1alpha1.SecretReference

	// Metadata provides structured naming information.
	// Required - controller must compute and provide this.
	Metadata pipelinecontext.MetadataContext `validate:"required"`

	// DefaultNotificationChannel is the default notification channel name for the environment.
	// Optional - if not provided, traits won't have access to a default notification channel.
	DefaultNotificationChannel string
}

// ApplyTargetPlaneDefaults normalizes empty targetPlane fields to "dataplane".
// This handles backward compatibility with resources created before the targetPlane field existed.
//
// Deprecated: This method exists for backward compatibility during development
// and should be removed when reaching 1.0.
func (input *RenderInput) ApplyTargetPlaneDefaults() {
	// Normalize ComponentType resources
	for i := range input.ComponentType.Spec.Resources {
		if input.ComponentType.Spec.Resources[i].TargetPlane == "" {
			input.ComponentType.Spec.Resources[i].TargetPlane = v1alpha1.TargetPlaneDataPlane
		}
	}

	// Normalize Traits
	for i := range input.Traits {
		for j := range input.Traits[i].Spec.Creates {
			if input.Traits[i].Spec.Creates[j].TargetPlane == "" {
				input.Traits[i].Spec.Creates[j].TargetPlane = v1alpha1.TargetPlaneDataPlane
			}
		}
		for j := range input.Traits[i].Spec.Patches {
			if input.Traits[i].Spec.Patches[j].TargetPlane == "" {
				input.Traits[i].Spec.Patches[j].TargetPlane = v1alpha1.TargetPlaneDataPlane
			}
		}
	}
}

// RenderOutput contains the results of the rendering process.
type RenderOutput struct {
	// Resources is the list of fully rendered Kubernetes resource manifests with their target planes.
	Resources []renderer.RenderedResource

	// Metadata contains information about the rendering process.
	Metadata *RenderMetadata
}

// RenderMetadata contains information about the rendering process.
type RenderMetadata struct {
	// ResourceCount is the total number of resources rendered.
	ResourceCount int

	// BaseResourceCount is the number of resources from the ComponentType.
	BaseResourceCount int

	// TraitCount is the number of traits processed.
	TraitCount int

	// TraitResourceCount is the number of resources created by traits.
	TraitResourceCount int

	// Warnings contains non-fatal issues encountered during rendering.
	Warnings []string
}
