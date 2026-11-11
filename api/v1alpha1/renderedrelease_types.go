// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RenderedReleaseSpec defines the desired state of RenderedRelease.
type RenderedReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner RenderedReleaseOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HPA, PDB, NetworkPolicy, CRDs, etc. that can
	// be applied to the data plane.
	// +kubebuilder:validation:Optional
	Resources []RenderedManifest `json:"resources,omitempty"`

	// Interval watch interval for the release resources when stable.
	// Defaults to the controller's stable interval (1m unless configured) if not
	// specified. Set to 0 to stop periodic checks.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// ProgressingInterval watch interval for the release resources when transitioning.
	// Defaults to the controller's progressing interval (10s unless configured) if not
	// specified. Set to 0 to stop periodic checks.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"
	// +optional
	ProgressingInterval *metav1.Duration `json:"progressingInterval,omitempty"`

	// TargetPlane specifies which plane this release should be deployed to.
	// Defaults to "dataplane" if not specified.
	// +kubebuilder:validation:Enum=dataplane;observabilityplane
	// +kubebuilder:default=dataplane
	TargetPlane string `json:"targetPlane,omitempty"`

	// There is no API server default: the binding controllers write the whole spec
	// without this field, and a defaulted value would read as a change on every
	// reconcile.
	// TODO: default to Manage once the binding controllers set mode, as for targetPlane (#5362).

	// Mode decides whether the manifests in spec.resources are applied. Manage
	// applies them. Observe does not, and removes whatever this release applied
	// earlier. Observed resources are read in both modes. Unset means Manage.
	// +kubebuilder:validation:Enum=Manage;Observe
	// +optional
	Mode RenderedReleaseMode `json:"mode,omitempty"`

	// ObservedResources lists existing objects on the target plane that this
	// release reads and reports in status.observedResources. They are never
	// created, updated, labeled or deleted, in either mode.
	// +listType=map
	// +listMapKey=id
	// +optional
	ObservedResources []ObservedResource `json:"observedResources,omitempty"`
}

// RenderedReleaseMode decides whether a RenderedRelease applies spec.resources.
type RenderedReleaseMode string

const (
	// RenderedReleaseModeManage applies spec.resources to the target plane.
	RenderedReleaseModeManage RenderedReleaseMode = "Manage"
	// RenderedReleaseModeObserve does not apply spec.resources. Anything the release
	// applied earlier is removed.
	RenderedReleaseModeObserve RenderedReleaseMode = "Observe"
)

// ObservedResource names an existing object on the target plane for a
// RenderedRelease to read.
type ObservedResource struct {
	// ID identifies this entry in status.observedResources. It is unique within
	// observedResources.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ID string `json:"id"`

	// APIVersion is the object's API version, such as "apps/v1" or "v1".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^([^/]+/)?[^/]+$`
	APIVersion string `json:"apiVersion"`

	// Kind is the object's kind, such as "Deployment".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[A-Z][A-Za-z0-9]*$`
	Kind string `json:"kind"`

	// Name is the object's name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Namespace is the object's namespace. Empty for cluster-scoped kinds.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RenderedReleaseStatus defines the observed state of RenderedRelease.
type RenderedReleaseStatus struct {
	// Resources contain the list of resources that have been successfully applied to the data plane
	// +optional
	Resources []RenderedManifestStatus `json:"resources,omitempty"`

	// ObservedResources reports the objects listed in spec.observedResources, as
	// read from the target plane.
	// +optional
	ObservedResources []RenderedManifestStatus `json:"observedResources,omitempty"`

	// Conditions represent the latest available observations of the RenderedRelease's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RenderedRelease is the Schema for the renderedreleases API.
type RenderedRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RenderedReleaseSpec   `json:"spec,omitempty"`
	Status RenderedReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RenderedReleaseList contains a list of RenderedRelease.
type RenderedReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RenderedRelease `json:"items"`
}

// RenderedReleaseOwner identifies the owner of a RenderedRelease. ProjectName
// is always required. The optional name fields disambiguate the binding kind:
//   - ComponentName set: produced by a ReleaseBinding (component)
//   - ResourceName set:  produced by a ResourceReleaseBinding
//   - EndpointName set:  produced by an EndpointReleaseBinding
//   - All unset:         produced by a ProjectReleaseBinding (project-level
//     infra including the cell namespace)
//
// At most one of ComponentName, ResourceName or EndpointName may be set.
// +kubebuilder:validation:XValidation:rule="!(has(self.componentName) && has(self.resourceName))",message="componentName and resourceName both cannot be set"
// +kubebuilder:validation:XValidation:rule="!has(self.endpointName) || !(has(self.componentName) || has(self.resourceName))",message="endpointName cannot be set together with componentName or resourceName"
type RenderedReleaseOwner struct {
	// ProjectName is the name of the Project the owner belongs to.
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// ComponentName is set when the RenderedRelease is owned by a Component
	// (via ReleaseBinding). Mutually exclusive with ResourceName.
	// +optional
	ComponentName string `json:"componentName,omitempty"`
	// ResourceName is set when the RenderedRelease is owned by a Resource
	// (via ResourceReleaseBinding). Mutually exclusive with ComponentName.
	// +optional
	ResourceName string `json:"resourceName,omitempty"`
	// EndpointName is set when the RenderedRelease is owned by an Endpoint
	// (via EndpointReleaseBinding). Mutually exclusive with ComponentName and
	// ResourceName.
	// +optional
	EndpointName string `json:"endpointName,omitempty"`
}

// RenderedManifest defines a Kubernetes resource template that can be applied to the data plane.
type RenderedManifest struct {
	// Unique identifier for the resource
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Object contains the complete Kubernetes resource definition
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Object *runtime.RawExtension `json:"object"`
}

// RenderedManifestStatus tracks a resource that was applied to, or observed on, the target plane.
type RenderedManifestStatus struct {
	// ID corresponds to the resource ID in spec.resources, or to the ID in
	// spec.observedResources for entries in status.observedResources.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Group is the API group of the resource (e.g., "apps", "batch")
	// Empty string for core resources
	// +optional
	Group string `json:"group,omitempty"`

	// Version is the API version of the resource (e.g., "v1", "v1beta1")
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Kind is the type of the resource (e.g., "Deployment", "Service")
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the name of the resource in the data plane
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the resource in the data plane
	// Empty for cluster-scoped resources
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Status captures the entire .status field of the resource on the target plane.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *runtime.RawExtension `json:"status,omitempty"`

	// HealthStatus indicates the health of the resource in the data plane.
	// +optional
	HealthStatus HealthStatus `json:"healthStatus,omitempty"`

	// LastObservedTime stores the last time the status was observed
	// +optional
	LastObservedTime *metav1.Time `json:"lastObservedTime,omitempty"`
}

// HealthStatus represents the health of a resource
type HealthStatus string

const (
	// HealthStatusUnknown indicates that the health of the resource is not known.
	HealthStatusUnknown HealthStatus = "Unknown"
	// HealthStatusProgressing indicates that the resource is in a transitioning state to become healthy.
	HealthStatusProgressing HealthStatus = "Progressing"
	// HealthStatusHealthy indicates that the resource is healthy and operating as expected.
	HealthStatusHealthy HealthStatus = "Healthy"
	// HealthStatusSuspended indicates that the resource is intentionally paused such as CronJob, Deployment with paused rollout, etc.
	HealthStatusSuspended HealthStatus = "Suspended"
	// HealthStatusDegraded indicates that the resource is not healthy and not operating as expected.
	HealthStatusDegraded HealthStatus = "Degraded"
)

// GetConditions returns the conditions from the status
func (in *RenderedRelease) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions in the status
func (in *RenderedRelease) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&RenderedRelease{}, &RenderedReleaseList{})
}
