// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EnvironmentSpec defines the desired state of Environment.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.dataPlaneRef) || oldSelf.dataPlaneRef == self.dataPlaneRef",message="dataPlaneRef is immutable once set"
type EnvironmentSpec struct {
	// DataPlaneRef references the DataPlane or ClusterDataPlane for this environment.
	// If not specified, defaults to a DataPlane named "default" in the same namespace.
	// Immutable once set.
	DataPlaneRef *DataPlaneRef `json:"dataPlaneRef,omitempty"`
	IsProduction bool          `json:"isProduction,omitempty"`
	Gateway      GatewaySpec   `json:"gateway,omitempty"`

	// Observe controls which data-plane namespaces bindings in observe mode can
	// read in this environment.
	// +optional
	Observe *EnvironmentObserve `json:"observe,omitempty"`
}

// EnvironmentObserve controls observe mode in an environment.
type EnvironmentObserve struct {
	// AllowedNamespaces lists the data-plane namespaces that a
	// ProjectReleaseBinding in observe mode can map to. If empty or omitted, no
	// namespace can be observed.
	// +optional
	AllowedNamespaces []AllowedNamespace `json:"allowedNamespaces,omitempty"`
}

// AllowedNamespace names a data-plane namespace that can be observed.
type AllowedNamespace struct {
	// Name is the namespace's name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
}

// IsNamespaceObservable reports whether the environment allows observing the
// named data-plane namespace.
func (e *Environment) IsNamespaceObservable(namespace string) bool {
	if e.Spec.Observe == nil {
		return false
	}
	for _, allowed := range e.Spec.Observe.AllowedNamespaces {
		if allowed.Name == namespace {
			return true
		}
	}
	return false
}

// EnvironmentStatus defines the observed state of Environment.
type EnvironmentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=env;envs

// Environment is the Schema for the environments API.
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

func (e *Environment) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

func (e *Environment) SetConditions(conditions []metav1.Condition) {
	e.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment.
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
