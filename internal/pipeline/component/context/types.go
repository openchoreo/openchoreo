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
	ComponentName string `json:"componentName,omitempty"`

	// ComponentUID is the unique identifier of the component.
	// Example: "a1b2c3d4-5678-90ab-cdef-1234567890ab"
	ComponentUID string `json:"componentUID,omitempty"`

	// ProjectName is the name of the project.
	// Example: "my-project"
	ProjectName string `json:"projectName,omitempty"`

	// ProjectUID is the unique identifier of the project.
	// Example: "b2c3d4e5-6789-01bc-def0-234567890abc"
	ProjectUID string `json:"projectUID,omitempty"`

	// DataPlaneName is the name of the data plane.
	// Example: "my-dataplane"
	DataPlaneName string `json:"dataPlaneName,omitempty"`

	// DataPlaneUID is the unique identifier of the data plane.
	// Example: "c3d4e5f6-7890-12cd-ef01-34567890abcd"
	DataPlaneUID string `json:"dataPlaneUID,omitempty"`

	// EnvironmentName is the name of the environment.
	// Example: "production"
	EnvironmentName string `json:"environmentName,omitempty"`

	// EnvironmentUID is the unique identifier of the environment.
	// Example: "d4e5f6g7-8901-23de-f012-4567890abcde"
	EnvironmentUID string `json:"environmentUID,omitempty"`

	// Name is the base name to use for generated resources.
	// Example: "my-service-dev-a1b2c3d4"
	Name string `json:"name,omitempty"`

	// Namespace is the target namespace for the resources.
	// Example: "dp-acme-corp-payment-dev-x1y2z3w4"
	Namespace string `json:"namespace,omitempty"`

	// Labels are common labels to add to all resources.
	// Example: {"openchoreo.dev/component": "my-service", ...}
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are common annotations to add to all resources.
	Annotations map[string]string `json:"annotations,omitempty"`

	// PodSelectors are platform-injected selectors for pod identity.
	// Used in Deployment selectors, Service selectors, etc.
	// Example: {
	//   "openchoreo.dev/component-uid": "abc123",
	//   "openchoreo.dev/environment-uid": "dev",
	//   "openchoreo.dev/project-uid": "xyz789",
	// }
	PodSelectors map[string]string `json:"podSelectors,omitempty"`
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

	// DataPlane contains the data plane configuration.
	// Required - controller must provide this.
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

// ComponentContext represents the evaluated context for rendering component resources.
// This is the output of BuildComponentContext and is used by the template engine.
type ComponentContext struct {
	// Metadata provides structured naming and labeling information.
	// Accessed via ${metadata.name}, ${metadata.namespace}, ${metadata.componentName}, etc.
	Metadata MetadataContext `json:"metadata"`

	// DataPlane provides data plane configuration.
	// Accessed via ${dataplane.secretStore}, ${dataplane.publicVirtualHost}
	DataPlane DataPlaneData `json:"dataplane"`

	// Parameters are merged component parameters with defaults applied.
	// Dynamic - depends on ComponentType schema.
	Parameters map[string]any `json:"parameters"`

	// Workload contains workload specification (containers, endpoints, connections).
	// Accessed via ${workload.name}, ${workload.containers}, etc.
	Workload WorkloadData `json:"workload"`

	// Configurations are extracted configuration items from workload.
	// Keyed by container name, contains configs and secrets.
	// Accessed via ${configurations["containerName"].configs.envs}, etc.
	Configurations map[string]ContainerConfigurations `json:"configurations"`
}

// DataPlaneData provides data plane configuration in templates.
type DataPlaneData struct {
	SecretStore       string `json:"secretStore,omitempty"`
	PublicVirtualHost string `json:"publicVirtualHost,omitempty"`
}

// WorkloadData contains workload information for templates.
type WorkloadData struct {
	Containers map[string]ContainerData `json:"containers"`
}

// ContainerData contains container information.
type ContainerData struct {
	Image   string   `json:"image,omitempty"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// ContainerConfigurations contains configs and secrets for a container.
type ContainerConfigurations struct {
	Configs ConfigurationItems `json:"configs"`
	Secrets ConfigurationItems `json:"secrets"`
}

// ConfigurationItems contains environment and file configurations.
type ConfigurationItems struct {
	Envs  []EnvConfiguration  `json:"envs"`
	Files []FileConfiguration `json:"files"`
}

// EnvConfiguration represents an environment variable configuration.
type EnvConfiguration struct {
	Name      string         `json:"name"`
	Value     string         `json:"value,omitempty"`
	RemoteRef *RemoteRefData `json:"remoteRef,omitempty"`
}

// FileConfiguration represents a file configuration.
type FileConfiguration struct {
	Name      string         `json:"name"`
	MountPath string         `json:"mountPath"`
	Value     string         `json:"value,omitempty"`
	RemoteRef *RemoteRefData `json:"remoteRef,omitempty"`
}

// RemoteRefData contains remote reference data for secrets.
type RemoteRefData struct {
	Key      string `json:"key"`
	Property string `json:"property,omitempty"`
	Version  string `json:"version,omitempty"`
}
