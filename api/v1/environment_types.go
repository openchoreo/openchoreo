/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EnvironmentSpec defines the desired state of Environment.
type EnvironmentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Environment. Edit environment_types.go to remove/update
	DataPlaneRef string        `json:"dataPlaneRef,omitempty"`
	IsProduction bool          `json:"isProduction,omitempty"`
	Gateway      GatewayConfig `json:"gateway,omitempty"`
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

type GatewayConfig struct {
	Security  SecurityConfig `json:"security,omitempty"`
	DNSPrefix string         `json:"dnsPrefix,omitempty"`
}
type SecurityConfig struct {
	// +optional
	RemoteJWKS `json:"remoteJwks"`
}

type RemoteJWKS struct {
	URI string `json:"uri"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
