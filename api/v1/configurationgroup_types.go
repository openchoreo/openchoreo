/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigurationGroupSpec defines the desired state of ConfigurationGroup
type ConfigurationGroupSpec struct {
	// Scope of the configuration group.
	// TODO need to define hierarchy when scope is specific
	//
	// +optional
	// +kubebuilder:default={}
	Scope map[string]string `json:"scope,omitempty"`

	// Environment groups that the configuration group is applicable.
	// This will be used when there are multiple similar environments to avoid repetition.
	//
	// +optional
	EnvironmentGroups []EnvironmentGroup `json:"environmentGroups,omitempty"`

	// Configuration parameters of the configuration group.
	//
	// +required
	Configurations []ConfigurationGroupConfiguration `json:"configurations"`
}

// EnvironmentGroup defines a group of environments
type EnvironmentGroup struct {
	// Name of the environment group.
	//
	// +required
	Name string `json:"name"`

	// List of environments that are part of the environment group.
	//
	// +required
	Environments []string `json:"environments"`
}

// ConfigurationGroupConfiguration defines a configuration parameter
type ConfigurationGroupConfiguration struct {
	// Key of the configuration parameter.
	//
	// +required
	// +immutable
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// List of values for the configuration parameter.
	// These values can be applicable either to a specific environment or an environment group.
	// The value for each specified key may be either a config or a secret. These can be mixed across environments.
	// e.g. use a config value for dev and a secret for prod.
	//
	// +required
	Values []ConfigurationValue `json:"values"`
}

// ConfigurationValue defines the value of a configuration parameter
type ConfigurationValue struct {
	// Reference to the environment group to which this configuration parameter is applicable.
	//
	// This field is mutually exclusive with environment field.
	//
	// +optional
	EnvironmentGroupRef string `json:"environmentGroupRef,omitempty"`

	// Reference to the environment to which this configuration parameter is applicable.
	//
	// This field is mutually exclusive with environmentGroupRef field.
	//
	// +optional
	Environment string `json:"environment,omitempty"`

	// Value of the configuration parameter.
	//
	// This field is mutually exclusive with vaultKey.
	//
	// +optional
	Value string `json:"value,omitempty"`

	// Reference to the secret vault key that contains the value for this configuration parameter.
	//
	// This field is mutually exclusive with value.
	//
	// +optional
	VaultKey string `json:"vaultKey,omitempty"`
}

// ConfigurationGroupStatus defines the observed state of ConfigurationGroup
type ConfigurationGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of the ConfigurationGroup's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=configgrp,categories={choreo,all}
// +kubebuilder:printcolumn:name="DisplayName",type="string",JSONPath=".metadata.annotations.core\\.choreo\\.dev/display-name"
// +kubebuilder:printcolumn:name="Organization",type="string",JSONPath=".metadata.labels.core\\.choreo\\.dev/organization"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ConfigurationGroup is the Schema for the configurationgroups API
type ConfigurationGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigurationGroupSpec   `json:"spec,omitempty"`
	Status ConfigurationGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigurationGroupList contains a list of ConfigurationGroup
type ConfigurationGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigurationGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigurationGroup{}, &ConfigurationGroupList{})
}
