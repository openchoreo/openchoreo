// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
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

	// Dependencies contains pre-computed dependency environment variables.
	// Optional - if not provided, dependencies context will be empty.
	Dependencies ConnectionsData

	// DefaultNotificationChannel is the default notification channel name for the environment.
	// Optional - if not provided, the defaultNotificationChannel field in EnvironmentData will be empty.
	DefaultNotificationChannel string
}

// TraitContextBase contains the inputs common to building both regular and embedded trait
// contexts (metadata, dataplane, environment, workload data, configurations, dependencies,
// notification channel). Pipeline callers typically build this once at the start of a render
// and embed it in each TraitContextInput / EmbeddedTraitContextInput inside the trait loops.
type TraitContextBase struct {
	// Metadata provides structured naming and labeling information.
	// Required - controller must provide this.
	Metadata MetadataContext `validate:"required"`

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

	// Dependencies contains pre-computed dependency environment variables.
	// Optional - if not provided, dependencies context will be empty.
	Dependencies ConnectionsData

	// DefaultNotificationChannel is the default notification channel name for the environment.
	// Optional - if not provided, the defaultNotificationChannel field in EnvironmentData will be empty.
	DefaultNotificationChannel string
}

// TraitContextInput contains all inputs needed to build a trait rendering context.
//
// Both component-level and embedded traits use this input. The only difference is how
// callers obtain the resolved parameter and environmentConfigs maps:
//   - Component-level traits: use ExtractTraitInstanceBindings to deserialize the
//     ComponentTrait instance and ReleaseBinding override.
//   - Embedded traits: use ResolveEmbeddedTraitBindings to evaluate CEL expressions
//     against the component context.
//
// By the time the input reaches BuildTraitContext both flows look identical.
type TraitContextInput struct {
	// TraitContextBase contains the fields shared across every trait built in a render
	// (metadata, dataplane, environment, workload data, configurations, dependencies,
	// notification channel). Pipeline callers typically build it once and embed it here.
	TraitContextBase

	// Trait is the trait definition.
	Trait *v1alpha1.Trait `validate:"required"`

	// InstanceName is the unique instance name for this trait within the component.
	InstanceName string `validate:"required"`

	// ResolvedParameters contains the parameter map after resolution. For component-level
	// traits this is JSON-deserialized from the ComponentTrait instance. For embedded
	// traits this is the result of CEL evaluation against the component context.
	ResolvedParameters map[string]any

	// ResolvedEnvironmentConfigs contains the environmentConfigs map after resolution.
	// For component-level traits it comes from ReleaseBinding.Spec.TraitEnvironmentConfigs;
	// for embedded traits it comes from CEL-evaluated bindings.
	ResolvedEnvironmentConfigs map[string]any

	// SchemaCache is an optional cache for schema bundles, keyed by trait kind+name with suffix.
	// Used to avoid rebuilding schemas for the same trait used multiple times.
	// BuildTraitContext will check this cache before building and populate it after.
	// Cache keys use format "{kind}:{traitName}:parameters" and "{kind}:{traitName}:environmentConfigs".
	SchemaCache map[string]*SchemaBundle
}

// SchemaInput contains schema information for building structural and JSON schemas.
type SchemaInput struct {
	// ParametersSchema is the parameters schema section.
	ParametersSchema *v1alpha1.SchemaSection

	// EnvironmentConfigsSchema is the environmentConfigs schema section.
	EnvironmentConfigsSchema *v1alpha1.SchemaSection
}

// ComponentContext represents the evaluated context for rendering component resources.
// This is the output of BuildComponentContext and is used by the template engine.
type ComponentContext struct {
	// Metadata provides structured naming and labeling information.
	// Accessed via ${metadata.name}, ${metadata.namespace}, ${metadata.componentName}, etc.
	Metadata MetadataContext `json:"metadata"`

	// DataPlane provides data plane configuration.
	// Accessed via ${dataplane.secretStore}, ${dataplane.gateway.ingress.external.https.host}, etc.
	DataPlane DataPlaneData `json:"dataplane"`

	// Environment provides environment-specific gateway configuration.
	// Accessed via ${environment.gateway.ingress.external.https.host}, etc.
	// Falls back to DataPlane gateway values if Environment.Gateway is not configured.
	Environment EnvironmentData `json:"environment"`

	// Gateway provides the effective gateway configuration for this deployment.
	// Uses environment gateway if configured, falls back to DataPlane gateway.
	// Accessed via ${gateway.ingress.external.https.host}, etc.
	Gateway *GatewayData `json:"gateway,omitempty"`

	// Parameters from Component.Spec.Parameters, pruned to ComponentType.Spec.Parameters schema.
	// Accessed via ${parameters.*}
	Parameters map[string]any `json:"parameters"`

	// EnvironmentConfigs from ReleaseBinding.Spec.ComponentTypeEnvironmentConfigs, pruned to ComponentType.Spec.EnvironmentConfigs schema.
	// Accessed via ${environmentConfigs.*}
	EnvironmentConfigs map[string]any `json:"environmentConfigs"`

	// Workload contains workload specification (container, endpoints, connections).
	// Accessed via ${workload.container}, ${workload.endpoints}, etc.
	Workload WorkloadData `json:"workload"`

	// Configurations are extracted configuration items from workload.
	// Contains configs and secrets for the single container.
	// Accessed via ${configurations.configs.envs}, ${configurations.secrets.files}, etc.
	Configurations ContainerConfigurations `json:"configurations"`

	// Dependencies contains dependency metadata and merged env vars for templates.
	// Accessed via ${dependencies.items} and ${dependencies.envVars}.
	Dependencies ConnectionsContextData `json:"dependencies"`
}

// DataPlaneData provides data plane configuration in templates.
type DataPlaneData struct {
	SecretStore           string                     `json:"secretStore,omitempty"`
	Gateway               *GatewayData               `json:"gateway,omitempty"`
	ObservabilityPlaneRef *ObservabilityPlaneRefData `json:"observabilityPlaneRef,omitempty"`
}

// GatewayData provides gateway configuration in templates.
type GatewayData struct {
	Ingress *GatewayNetworkData `json:"ingress,omitempty"`
	Egress  *GatewayNetworkData `json:"egress,omitempty"`
}

// GatewayNetworkData provides traffic gateway data for ingress/egress in templates.
type GatewayNetworkData struct {
	External *GatewayEndpointData `json:"external,omitempty"`
	Internal *GatewayEndpointData `json:"internal,omitempty"`
}

// GatewayEndpointData provides endpoint data for a gateway in templates.
type GatewayEndpointData struct {
	Name      string               `json:"name,omitempty"`
	Namespace string               `json:"namespace,omitempty"`
	HTTP      *GatewayListenerData `json:"http,omitempty"`
	HTTPS     *GatewayListenerData `json:"https,omitempty"`
	TLS       *GatewayListenerData `json:"tls,omitempty"`
}

// GatewayListenerData provides listener data for a gateway in templates.
type GatewayListenerData struct {
	ListenerName string `json:"listenerName,omitempty"`
	Port         int32  `json:"port,omitempty"`
	Host         string `json:"host,omitempty"`
}

// ObservabilityPlaneRefData provides observability plane reference in templates.
type ObservabilityPlaneRefData struct {
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
}

// EnvironmentData provides environment-specific gateway configuration in templates.
// If the environment does not have gateway configuration, values fallback to DataPlane gateway.
type EnvironmentData struct {
	Gateway                    *GatewayData `json:"gateway,omitempty"`
	DefaultNotificationChannel string       `json:"defaultNotificationChannel,omitempty"`
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
	DisplayName string   `json:"displayName,omitempty"`
	Port        int32    `json:"port"`
	TargetPort  int32    `json:"targetPort"`
	Type        string   `json:"type"`
	BasePath    string   `json:"basePath,omitempty"`
	Visibility  []string `json:"visibility"`
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

// ConnectionItem represents a single connection with its target metadata and resolved env vars.
type ConnectionItem struct {
	Namespace  string        `json:"namespace"`
	Project    string        `json:"project"`
	Component  string        `json:"component"`
	Endpoint   string        `json:"endpoint"`
	Visibility string        `json:"visibility"`
	EnvVars    []EnvVarEntry `json:"envVars"`
}

// ConnectionsData contains the per-item dependency views (endpoint connections and resource
// dependencies) that the controller resolves before invoking the pipeline. This is the input
// type used in RenderInput and context inputs.
type ConnectionsData struct {
	Items     []ConnectionItem         `json:"items"`
	Resources []ResourceDependencyItem `json:"resources"`
}

// ConnectionsContextData is the template context representation of dependencies. It exposes
// per-item views (Items, Resources) plus merged flat lists (EnvVars, VolumeMounts, Volumes)
// for templates that want a single combined surface.
//
//	${dependencies.items}        — endpoint per-item view
//	${dependencies.resources}    — resource per-item view
//	${dependencies.envVars}      — merged across endpoints + resources
//	${dependencies.volumeMounts} — merged across resources
//	${dependencies.volumes}      — merged across resources
type ConnectionsContextData struct {
	Items        []ConnectionItem         `json:"items"`
	Resources    []ResourceDependencyItem `json:"resources"`
	EnvVars      []EnvVarEntry            `json:"envVars"`
	VolumeMounts []VolumeMountEntry       `json:"volumeMounts"`
	Volumes      []VolumeEntry            `json:"volumes"`
}

// newDependenciesContextData creates a ConnectionsContextData from ConnectionsData,
// merging all per-item env vars, volume mounts, and volumes into the top-level merged fields.
// Ensures no nil slices so CEL templates always see empty lists instead of null.
func newDependenciesContextData(data ConnectionsData) ConnectionsContextData {
	items := make([]ConnectionItem, len(data.Items))
	resources := make([]ResourceDependencyItem, len(data.Resources))
	mergedEnvVars := make([]EnvVarEntry, 0, len(data.Items)+len(data.Resources))
	mergedVolumeMounts := make([]VolumeMountEntry, 0, len(data.Resources))
	mergedVolumes := make([]VolumeEntry, 0, len(data.Resources))

	for i, item := range data.Items {
		if item.EnvVars == nil {
			item.EnvVars = []EnvVarEntry{}
		}
		items[i] = item
		mergedEnvVars = append(mergedEnvVars, item.EnvVars...)
	}

	for i, item := range data.Resources {
		if item.EnvVars == nil {
			item.EnvVars = []EnvVarEntry{}
		}
		if item.VolumeMounts == nil {
			item.VolumeMounts = []VolumeMountEntry{}
		}
		if item.Volumes == nil {
			item.Volumes = []VolumeEntry{}
		}
		resources[i] = item
		mergedEnvVars = append(mergedEnvVars, item.EnvVars...)
		mergedVolumeMounts = append(mergedVolumeMounts, item.VolumeMounts...)
		mergedVolumes = append(mergedVolumes, item.Volumes...)
	}

	return ConnectionsContextData{
		Items:        items,
		Resources:    resources,
		EnvVars:      mergedEnvVars,
		VolumeMounts: mergedVolumeMounts,
		Volumes:      mergedVolumes,
	}
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

	// Parameters from TraitInstance.Parameters, pruned to Trait.Spec.Parameters schema.
	// Accessed via ${parameters.*}
	Parameters map[string]any `json:"parameters"`

	// EnvironmentConfigs from ReleaseBinding.Spec.TraitEnvironmentConfigs[instanceName], pruned to Trait.Spec.EnvironmentConfigs schema.
	// Accessed via ${environmentConfigs.*}
	EnvironmentConfigs map[string]any `json:"environmentConfigs"`

	// DataPlane provides data plane configuration.
	// Accessed via ${dataplane.secretStore}, ${dataplane.gateway.ingress.external.https.host}, etc.
	DataPlane DataPlaneData `json:"dataplane"`

	// Environment provides environment-specific gateway configuration.
	// Accessed via ${environment.gateway.ingress.external.https.host}, etc.
	// Falls back to DataPlane gateway values if Environment.Gateway is not configured.
	Environment EnvironmentData `json:"environment"`

	// Gateway provides the effective gateway configuration for this deployment.
	// Uses environment gateway if configured, falls back to DataPlane gateway.
	// Accessed via ${gateway.ingress.external.https.host}, etc.
	Gateway *GatewayData `json:"gateway,omitempty"`

	// Workload contains workload specification (container, endpoints).
	// Accessed via ${workload.container}, ${workload.endpoints}
	Workload WorkloadData `json:"workload"`

	// Configurations are extracted configuration items from workload.
	// Contains configs and secrets for the single container.
	// Accessed via ${configurations.configs.envs}, ${configurations.secrets.files}, etc.
	Configurations ContainerConfigurations `json:"configurations"`

	// Dependencies contains dependency metadata and merged env vars for templates.
	// Accessed via ${dependencies.items} and ${dependencies.envVars}.
	Dependencies ConnectionsContextData `json:"dependencies"`
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
