// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// MetadataContext provides structured metadata for resource generation.
// This is computed by the controller and passed to the renderer.
type MetadataContext struct {
	// Name is the base name to use for generated resources.
	// Example: "my-service-dev-a1b2c3d4"
	Name string

	// Namespace is the target namespace for the resources.
	// Example: "dp-acme-corp-payment-dev-x1y2z3w4"
	Namespace string

	// Labels are common labels to add to all resources.
	// Example: {"openchoreo.org/component": "my-service", ...}
	Labels map[string]string

	// Annotations are common annotations to add to all resources.
	Annotations map[string]string

	// PodSelectors are platform-injected selectors for pod identity.
	// Used in Deployment selectors, Service selectors, etc.
	// Example: {
	//   "openchoreo.org/component-id": "abc123",
	//   "openchoreo.org/environment": "dev",
	//   "openchoreo.org/project-id": "xyz789",
	// }
	PodSelectors map[string]string
}

// ComponentContextInput contains all inputs needed to build a component rendering context.
type ComponentContextInput struct {
	// Component is the component definition.
	Component *v1alpha1.Component

	// ComponentTypeDefinition is the type definition for the component.
	ComponentTypeDefinition *v1alpha1.ComponentTypeDefinition

	// ComponentDeployment contains environment-specific overrides.
	// Can be nil if no overrides are needed.
	ComponentDeployment *v1alpha1.ComponentDeployment

	// Workload contains the workload specification with the built image.
	Workload *v1alpha1.Workload

	// Environment is the name of the environment being rendered for.
	Environment string

	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext
}

// AddonContextInput contains all inputs needed to build an addon rendering context.
type AddonContextInput struct {
	// Addon is the addon definition.
	Addon *v1alpha1.Addon

	// Instance contains the specific instance configuration.
	Instance v1alpha1.ComponentAddon

	// Component is the component this addon is being applied to.
	Component *v1alpha1.Component

	// ComponentDeployment contains environment-specific addon overrides.
	// Can be nil if no overrides are needed.
	ComponentDeployment *v1alpha1.ComponentDeployment

	// Environment is the name of the environment being rendered for.
	Environment string

	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext

	// SchemaCache is an optional cache for structural schemas, keyed by addon name.
	// Used to avoid rebuilding schemas for the same addon used multiple times.
	// BuildAddonContext will check this cache before building and populate it after.
	SchemaCache map[string]*apiextschema.Structural
}

// SchemaInput contains schema information for applying defaults.
type SchemaInput struct {
	// Types defines reusable type definitions.
	Types *runtime.RawExtension

	// ParametersSchema is the parameters schema definition.
	ParametersSchema *runtime.RawExtension

	// EnvOverridesSchema is the envOverrides schema definition.
	EnvOverridesSchema *runtime.RawExtension

	// Structural is the compiled structural schema (cached).
	Structural *apiextschema.Structural
}
