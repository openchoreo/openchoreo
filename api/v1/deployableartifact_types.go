// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeployableArtifactSpec defines the desired state of DeployableArtifact.
type DeployableArtifactSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// DeployableArtifactSpec defines the spec section of DeployableArtifact.
	TargetArtifact TargetArtifact `json:"targetArtifact"`

	// Configuration parameters for this deployable artifact.
	// +optional
	Configuration *Configuration `json:"configuration,omitempty"`
}

// Configuration is the top-level configuration block of DeployableArtifactSpec.
type Configuration struct {
	// A list of endpoints exposed by the component.
	// +optional
	EndpointTemplates []EndpointTemplate `json:"endpointTemplates,omitempty"`

	// Dependencies required by this component.
	// +optional
	Dependencies *Dependencies `json:"dependencies,omitempty"`

	// Application runtime parameters/configurations.
	// +optional
	Application *Application `json:"application,omitempty"`
}

// TargetArtifact references the source artifact to be deployed.
type TargetArtifact struct {
	// Mutually exclusive references to a build or an image.
	// +optional
	FromBuildRef *FromBuildRef `json:"fromBuildRef,omitempty"`

	// Mutually exclusive references to a specific image tag.
	// +optional
	FromImageRef *FromImageRef `json:"fromImageRef,omitempty"`
}

// FromBuildRef points to an existing Build resource and optionally
// a specific git revision.
type FromBuildRef struct {
	// Name of the referenced Build resource.
	// +optional
	Name string `json:"name,omitempty"`

	// GitRevision to select the latest Build that matches it.
	// +optional
	GitRevision string `json:"gitRevision,omitempty"`
}

// FromImageRef points to an image tag to deploy.
type FromImageRef struct {
	// Name of the image tag (e.g., “1.2.0”, “latest”, etc.).
	// +optional
	Tag string `json:"tag,omitempty"`

	// Whether to skip version validation (for semantic version compliance).
	// +optional
	SkipVersionValidation bool `json:"skipVersionValidation,omitempty"`
}

// EndpointTemplate represents an endpoint derived from a component descriptor.
type EndpointTemplate struct {
	// Specification of the endpoint
	// +required
	metav1.ObjectMeta `json:"metadata"`
	// +required
	Spec EndpointSpec `json:"spec"`
}

// Dependencies captures references to connections and other dependencies.
type Dependencies struct {
	// TODO: Add Dependencies fields here
}

// Application captures runtime-specific configurations.
type Application struct {
	// Command line arguments to pass.
	// +optional
	Args []string `json:"args,omitempty"`

	// Explicit environment variables.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// Bulk import environment variables from references.
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty"`

	// Single-file mounts.
	// +optional
	FileMounts []FileMount `json:"fileMounts,omitempty"`

	// Bulk import file mounts from references.
	// +optional
	FileMountsFrom []FileMountsFromSource `json:"fileMountsFrom,omitempty"`

	// Resource limits for CPU/memory, etc.
	// +optional
	ResourceLimits *ResourceLimits `json:"resourceLimits,omitempty"`

	// Probes (readiness/liveness) to monitor the container.
	// +optional
	Probes *Probes `json:"probes,omitempty"`

	// Scaling configuration (only for non-task components).
	// +optional
	Scaling *ScalingConfig `json:"scaling,omitempty"`

	// Task configuration (mutually exclusive with scaling).
	// +optional
	Task *TaskConfig `json:"task,omitempty"`
}

// EnvVar represents an environment variable present in the container.
type EnvVar struct {
	// The environment variable key.
	// +required
	Key string `json:"key"`

	// The literal value of the environment variable.
	// Mutually exclusive with valueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// Extract the environment variable value from another resource.
	// Mutually exclusive with value.
	// +optional
	ValueFrom *EnvVarValueFrom `json:"valueFrom,omitempty"`
}

// EnvVarValueFrom holds references to external sources for environment variables.
type EnvVarValueFrom struct {
	// Reference to a configuration group.
	// +optional
	ConfigurationGroupRef *ConfigurationGroupKeyRef `json:"configurationGroupRef,omitempty"`

	// Reference to a secret resource.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// ConfigurationGroupKeyRef references a specific key in a configuration group.
type ConfigurationGroupKeyRef struct {
	// +required
	Name string `json:"name"`
	// +required
	Key string `json:"key"`
}

// SecretKeyRef references a specific key in a K8s secret.
type SecretKeyRef struct {
	// +required
	Name string `json:"name"`
	// +required
	Key string `json:"key"`
}

// EnvFromSource allows importing all environment variables from a source.
type EnvFromSource struct {
	// Reference to a configuration group (entire group).
	// +optional
	ConfigurationGroupRef *ConfigurationGroupRef `json:"configurationGroupRef,omitempty"`

	// Reference to a secret resource (entire secret).
	// +optional
	SecretRef *SecretRefBasic `json:"secretRef,omitempty"`
}

// ConfigurationGroupRef references a configuration group as a whole.
type ConfigurationGroupRef struct {
	// +required
	Name string `json:"name"`
}

// SecretRefBasic references a secret resource as a whole.
type SecretRefBasic struct {
	// +required
	Name string `json:"name"`
}

// FileMount represents one file mounted from data/inline content.
type FileMount struct {
	// +required
	MountPath string `json:"mountPath"`

	// Inline file content.
	// Mutually exclusive with valueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// References to an external data source for the file content.
	// +optional
	ValueFrom *FileMountValueFrom `json:"valueFrom,omitempty"`
}

// FileMountValueFrom references an external data source for file content.
type FileMountValueFrom struct {
	// +optional
	ConfigurationGroupRef *ConfigurationGroupKeyRef `json:"configurationGroupRef,omitempty"`
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// FileMountsFromSource allows importing multiple files from a source.
type FileMountsFromSource struct {
	// +optional
	ConfigurationGroupRef *ConfigurationGroupMountRef `json:"configurationGroupRef,omitempty"`

	// +optional
	SecretRef *SecretMountRef `json:"secretRef,omitempty"`
}

// ConfigurationGroupMountRef references a config group as files in a directory.
type ConfigurationGroupMountRef struct {
	// +required
	Name string `json:"name"`

	// Absolute directory path to mount the config group contents.
	// +required
	MountPath string `json:"mountPath"`
}

// SecretMountRef references a secret resource as files in a directory.
type SecretMountRef struct {
	// +required
	Name string `json:"name"`

	// Absolute directory path to mount the secret contents.
	// +required
	MountPath string `json:"mountPath"`
}

// ResourceLimits define the CPU/memory constraints for the container.
type ResourceLimits struct {
	// +optional
	CPU string `json:"cpu,omitempty"`
	// +optional
	Memory string `json:"memory,omitempty"`
}

// Probes define readiness/liveness checks.
type Probes struct {
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
}

// ScalingConfig holds either HPA or S2Z config.
type ScalingConfig struct {
	// +optional
	HPA *HPAConfig `json:"hpa,omitempty"`

	// +optional
	S2Z *S2ZConfig `json:"s2z,omitempty"`
}

// HPAConfig configures Horizontal Pod Autoscaling.
type HPAConfig struct {
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
	// +optional
	CPUThreshold *int32 `json:"cpuThreshold,omitempty"`
	// +optional
	MemoryThreshold *int32 `json:"memoryThreshold,omitempty"`
}

// S2ZConfig configures scale-to-zero.
type S2ZConfig struct {
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
	// +optional
	QueueLength *int32 `json:"queueLength,omitempty"`
}

// TaskConfig captures scheduling/manual execution details for a task.
type TaskConfig struct {
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// Only applicable for scheduled tasks.
	// +optional
	Schedule *TaskSchedule `json:"schedule,omitempty"`
}

// TaskSchedule defines the cron schedule and timezone.
type TaskSchedule struct {
	// +required
	Cron string `json:"cron"`
	// +optional
	Timezone string `json:"timezone,omitempty"`
}

// DeployableArtifactStatus defines the observed state of DeployableArtifact.
type DeployableArtifactStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DeployableArtifact is the Schema for the deployableartifacts API.
type DeployableArtifact struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeployableArtifactSpec   `json:"spec,omitempty"`
	Status DeployableArtifactStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeployableArtifactList contains a list of DeployableArtifact.
type DeployableArtifactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeployableArtifact `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeployableArtifact{}, &DeployableArtifactList{})
}

func (p *DeployableArtifact) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *DeployableArtifact) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}
