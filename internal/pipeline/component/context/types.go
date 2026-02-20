// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// MetadataContext provides structured metadata for resource generation.
// This is computed by the controller and passed to the renderer.
type MetadataContext struct {
	// ComponentName is the name of the component.
	// Example: "my-service"
	ComponentName string `json:"componentName" validate:"required"`

	// ComponentUID is the unique identifier of the component.
	// Example: "a1b2c3d4-5678-90ab-cdef-1234567890ab"
	ComponentUID string `json:"componentUID" validate:"required"`

	// ProjectName is the name of the project.
	// Example: "my-project"
	ProjectName string `json:"projectName" validate:"required"`

	// ProjectUID is the unique identifier of the project.
	// Example: "b2c3d4e5-6789-01bc-def0-234567890abc"
	ProjectUID string `json:"projectUID" validate:"required"`

	// DataPlaneName is the name of the data plane.
	// Example: "my-dataplane"
	DataPlaneName string `json:"dataPlaneName" validate:"required"`

	// DataPlaneUID is the unique identifier of the data plane.
	// Example: "c3d4e5f6-7890-12cd-ef01-34567890abcd"
	DataPlaneUID string `json:"dataPlaneUID" validate:"required"`

	// EnvironmentName is the name of the environment.
	// Example: "production"
	EnvironmentName string `json:"environmentName" validate:"required"`

	// EnvironmentUID is the unique identifier of the environment.
	// Example: "d4e5f6g7-8901-23de-f012-4567890abcde"
	EnvironmentUID string `json:"environmentUID" validate:"required"`

	// Name is the base name to use for generated resources.
	// Example: "my-service-dev-a1b2c3d4"
	Name string `json:"name" validate:"required"`

	// Namespace is the target namespace for the resources.
	// Example: "dp-acme-corp-payment-dev-x1y2z3w4"
	Namespace string `json:"namespace" validate:"required"`

	// ComponentNamespace is the namespace on which the component is created.
	// Example: "cp-acme-corp"
	ComponentNamespace string `json:"componentNamespace" validate:"required"`

	// Labels are common labels to add to all resources.
	// Example: {"openchoreo.dev/component": "my-service", ...}
	Labels map[string]string `json:"labels" validate:"required"`

	// Annotations are common annotations to add to all resources.
	Annotations map[string]string `json:"annotations" validate:"required"`

	// PodSelectors are platform-injected selectors for pod identity.
	// Used in Deployment selectors, Service selectors, etc.
	// Example: {
	//   "openchoreo.dev/namespace": "dp-acme-corp-payment-dev-x1y2z3w4",
	//   "openchoreo.dev/project": "acme-corp",
	//   "openchoreo.dev/component": "payment",
	//   "openchoreo.dev/environment": "dev",
	//   "openchoreo.dev/component-uid": "abc123",
	//   "openchoreo.dev/environment-uid": "dev",
	//   "openchoreo.dev/project-uid": "xyz789",
	// }
	PodSelectors map[string]string `json:"podSelectors" validate:"required,min=1"`
}

// ComponentContextInput contains all inputs needed to build a component rendering context.
type ComponentContextInput struct {
	// Component is the component definition.
	Component *v1alpha1.Component `validate:"required"`

	// ComponentType is the type definition for the component.
	ComponentType *v1alpha1.ComponentType `validate:"required"`

	// ReleaseBinding contains release reference and environment-specific overrides.
	ReleaseBinding *v1alpha1.ReleaseBinding

	// DataPlane contains the data plane configuration.
	// Required - controller must provide this.
	DataPlane *v1alpha1.DataPlane `validate:"required"`

	// Environment contains the environment configuration.
	// Required - controller must provide this.
	Environment *v1alpha1.Environment `validate:"required"`

	// WorkloadData is the pre-computed workload data (containers, endpoints).
	// Should be computed once by the caller using ExtractWorkloadData and shared.
	WorkloadData WorkloadData

	// Configurations is the pre-computed configurations from workload.
	// Should be computed once by the caller using ExtractConfigurationsFromWorkload
	// and shared across ComponentContext and all TraitContexts.
	Configurations ContainerConfigurations

	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext `validate:"required"`

	// DefaultNotificationChannel is the default notification channel name for the environment.
	// Optional - if not provided, the defaultNotificationChannel field in EnvironmentData will be empty.
	DefaultNotificationChannel string
}

// TraitContextInput contains all inputs needed to build a trait rendering context.
type TraitContextInput struct {
	// Trait is the trait definition.
	Trait *v1alpha1.Trait `validate:"required"`

	// Instance contains the specific instance configuration.
	Instance v1alpha1.ComponentTrait `validate:"required"`

	// Component is the component this trait is being applied to.
	Component *v1alpha1.Component `validate:"required"`

	// ReleaseBinding contains release reference and environment-specific overrides.
	// Can be nil if no overrides are needed.
	ReleaseBinding *v1alpha1.ReleaseBinding

	// WorkloadData is the pre-computed workload data (containers, endpoints).
	// Should be computed once by the caller using ExtractWorkloadData and shared.
	WorkloadData WorkloadData

	// Configurations is the pre-computed configurations from workload.
	// Should be computed once by the caller using ExtractConfigurationsFromWorkload
	// and shared across ComponentContext and all TraitContexts.
	Configurations ContainerConfigurations

	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext `validate:"required"`

	// SchemaCache is an optional cache for schema bundles, keyed by trait name with suffix.
	// Used to avoid rebuilding schemas for the same trait used multiple times.
	// BuildTraitContext will check this cache before building and populate it after.
	// Cache keys use format "{traitName}:parameters" and "{traitName}:envOverrides".
	SchemaCache map[string]*SchemaBundle

	// DataPlane contains the data plane configuration.
	// Required - controller must provide this.
	DataPlane *v1alpha1.DataPlane `validate:"required"`

	// Environment contains the environment configuration.
	// Required - controller must provide this.
	Environment *v1alpha1.Environment `validate:"required"`

	// DefaultNotificationChannel is the default notification channel name for the environment.
	// Optional - if not provided, the defaultNotificationChannel field in EnvironmentData will be empty.
	DefaultNotificationChannel string
}

// SchemaInput contains schema information for building structural and JSON schemas.
type SchemaInput struct {
	// Types defines reusable type definitions.
	Types *runtime.RawExtension

	// ParametersSchema is the parameters schema definition.
	ParametersSchema *runtime.RawExtension

	// EnvOverridesSchema is the envOverrides schema definition.
	EnvOverridesSchema *runtime.RawExtension
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

	// Environment provides environment-specific gateway configuration.
	// Accessed via ${environment.publicVirtualHost}, ${environment.publicHTTPPort}, etc.
	// Falls back to DataPlane gateway values if Environment.Gateway is not configured.
	Environment EnvironmentData `json:"environment"`

	// Parameters from Component.Spec.Parameters, pruned to ComponentType.Schema.Parameters.
	// Accessed via ${parameters.*}
	Parameters map[string]any `json:"parameters"`

	// EnvOverrides from ReleaseBinding.Spec.ComponentTypeEnvOverrides, pruned to ComponentType.Schema.EnvOverrides.
	// Accessed via ${envOverrides.*}
	EnvOverrides map[string]any `json:"envOverrides"`

	// Workload contains workload specification (container, endpoints, connections).
	// Accessed via ${workload.container}, ${workload.endpoints}, etc.
	Workload WorkloadData `json:"workload"`

	// Configurations are extracted configuration items from workload.
	// Contains configs and secrets for the single container.
	// Accessed via ${configurations.configs.envs}, ${configurations.secrets.files}, etc.
	Configurations ContainerConfigurations `json:"configurations"`
}

// DataPlaneData provides data plane configuration in templates.
type DataPlaneData struct {
	SecretStore             string                     `json:"secretStore,omitempty"`
	PublicVirtualHost       string                     `json:"publicVirtualHost,omitempty"`
	OrganizationVirtualHost string                     `json:"organizationVirtualHost,omitempty"`
	ObservabilityPlaneRef   *ObservabilityPlaneRefData `json:"observabilityPlaneRef,omitempty"`
}

// ObservabilityPlaneRefData provides observability plane reference in templates.
type ObservabilityPlaneRefData struct {
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
}

// EnvironmentData provides environment-specific gateway configuration in templates.
// If the environment does not have gateway configuration, values fallback to DataPlane gateway.
type EnvironmentData struct {
	PublicVirtualHost          string `json:"publicVirtualHost,omitempty"`
	OrganizationVirtualHost    string `json:"organizationVirtualHost,omitempty"`
	DefaultNotificationChannel string `json:"defaultNotificationChannel,omitempty"`
}

// WorkloadData contains workload information for templates.
type WorkloadData struct {
	Container ContainerData           `json:"container"`
	Endpoints map[string]EndpointData `json:"endpoints"`
}

// ContainerData contains container information.
type ContainerData struct {
	Image   string   `json:"image,omitempty"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// EndpointData contains endpoint information.
type EndpointData struct {
	DisplayName string      `json:"displayName,omitempty"`
	Port        int32       `json:"port"`
	TargetPort  int32       `json:"targetPort"`
	Type        string      `json:"type"`
	BasePath    string      `json:"basePath,omitempty"`
	Schema      *SchemaData `json:"schema,omitempty"`
	Visibility  []string    `json:"visibility"`
}

// SchemaData contains API schema information for an endpoint.
type SchemaData struct {
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
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

// TraitContext represents the evaluated context for rendering trait resources.
// This is the output of BuildTraitContext and is used by the template engine.
type TraitContext struct {
	// Metadata provides structured naming and labeling information.
	// Accessed via ${metadata.name}, ${metadata.namespace}, ${metadata.componentName}, etc.
	Metadata MetadataContext `json:"metadata"`

	// Trait contains trait-specific metadata.
	// Accessed via ${trait.name}, ${trait.instanceName}
	Trait TraitMetadata `json:"trait"`

	// Parameters from TraitInstance.Parameters, pruned to Trait.Schema.Parameters.
	// Accessed via ${parameters.*}
	Parameters map[string]any `json:"parameters"`

	// EnvOverrides from ReleaseBinding.Spec.TraitOverrides[instanceName], pruned to Trait.Schema.EnvOverrides.
	// Accessed via ${envOverrides.*}
	EnvOverrides map[string]any `json:"envOverrides"`

	// DataPlane provides data plane configuration.
	// Accessed via ${dataplane.secretStore}, ${dataplane.publicVirtualHost}
	DataPlane DataPlaneData `json:"dataplane"`

	// Environment provides environment-specific gateway configuration.
	// Accessed via ${environment.publicVirtualHost}, ${environment.publicHTTPPort}, etc.
	// Falls back to DataPlane gateway values if Environment.Gateway is not configured.
	Environment EnvironmentData `json:"environment"`

	// Workload contains workload specification (container, endpoints).
	// Accessed via ${workload.container}, ${workload.endpoints}
	Workload WorkloadData `json:"workload"`

	// Configurations are extracted configuration items from workload.
	// Contains configs and secrets for the single container.
	// Accessed via ${configurations.configs.envs}, ${configurations.secrets.files}, etc.
	Configurations ContainerConfigurations `json:"configurations"`
}

// TraitMetadata contains trait-specific metadata.
type TraitMetadata struct {
	// Name is the name of the trait.
	// Example: "storage"
	Name string `json:"name"`

	// InstanceName is the unique instance name within the component.
	// Example: "my-storage"
	InstanceName string `json:"instanceName"`
}
