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
	// ComponentName is the name of the component.
	// Example: "my-service"
	ComponentName string

	// ComponentUID is the unique identifier of the component.
	// Example: "a1b2c3d4-5678-90ab-cdef-1234567890ab"
	ComponentUID string

	// ProjectName is the name of the project.
	// Example: "my-project"
	ProjectName string

	// ProjectUID is the unique identifier of the project.
	// Example: "b2c3d4e5-6789-01bc-def0-234567890abc"
	ProjectUID string

	// DataPlaneName is the name of the data plane.
	// Example: "my-dataplane"
	DataPlaneName string

	// DataPlaneUID is the unique identifier of the data plane.
	// Example: "c3d4e5f6-7890-12cd-ef01-34567890abcd"
	DataPlaneUID string

	// EnvironmentName is the name of the environment.
	// Example: "production"
	EnvironmentName string

	// EnvironmentUID is the unique identifier of the environment.
	// Example: "d4e5f6g7-8901-23de-f012-4567890abcde"
	EnvironmentUID string

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

	// ComponentType is the type definition for the component.
	ComponentType *v1alpha1.ComponentType

	// ComponentDeployment contains environment-specific overrides.
	// Can be nil if no overrides are needed.
	// Deprecated: this field will be removed in a future release. Use ReleaseBinding instead.
	ComponentDeployment *v1alpha1.ComponentDeployment

	// ReleaseBinding contains release reference and environment-specific overrides.
	// Can be nil if no overrides are needed.
	ReleaseBinding *v1alpha1.ReleaseBinding

	// Workload contains the workload specification with the built image.
	Workload *v1alpha1.Workload

	// Environment to which the component is being deployed.
	Environment EnvironmentContext

	// DataPlane contains the data plane configuration.
	// Optional - can be nil if no data plane is configured.
	DataPlane *v1alpha1.DataPlane

	// SecretReferences is a map of SecretReference objects needed for rendering.
	// Keyed by SecretReference name.
	// Optional - can be nil if no secret references need to be resolved.
	SecretReferences map[string]*v1alpha1.SecretReference

	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext
}

// TraitContextInput contains all inputs needed to build a trait rendering context.
type TraitContextInput struct {
	// Trait is the trait definition.
	Trait *v1alpha1.Trait

	// Instance contains the specific instance configuration.
	Instance v1alpha1.ComponentTrait

	// Component is the component this trait is being applied to.
	Component *v1alpha1.Component

	// ComponentDeployment contains environment-specific trait overrides.
	// Can be nil if no overrides are needed.
	// Deprecated: this field will be removed in a future release. Use ReleaseBinding instead.
	ComponentDeployment *v1alpha1.ComponentDeployment

	// ReleaseBinding contains release reference and environment-specific overrides.
	// Can be nil if no overrides are needed.
	ReleaseBinding *v1alpha1.ReleaseBinding

	// Environment is the name of the environment being rendered for.
	Environment EnvironmentContext

	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext

	// SchemaCache is an optional cache for structural schemas, keyed by trait name.
	// Used to avoid rebuilding schemas for the same trait used multiple times.
	// BuildTraitContext will check this cache before building and populate it after.
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

type EnvironmentContext struct {
	// Name is the name of the environment.
	Name string

	// VirtualHost is the virtual host that is associated with the environment.
	VirtualHost string
}
