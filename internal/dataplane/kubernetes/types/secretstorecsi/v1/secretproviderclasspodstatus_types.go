/*
Copyright 2020 The Kubernetes Authors.

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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// InternalNodeLabel used for setting the node name spc pod status belongs to
	InternalNodeLabel = "internal.secrets-store.csi.k8s.io/node-name"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretProviderClassPodStatusStatus defines the observed state of SecretProviderClassPodStatus
type SecretProviderClassPodStatusStatus struct {
	PodName                 string                      `json:"podName,omitempty"`
	SecretProviderClassName string                      `json:"secretProviderClassName,omitempty"`
	Mounted                 bool                        `json:"mounted,omitempty"`
	TargetPath              string                      `json:"targetPath,omitempty"`
	Objects                 []SecretProviderClassObject `json:"objects,omitempty"`
}

// SecretProviderClassObject defines the object fetched from external secrets store
type SecretProviderClassObject struct {
	ID      string `json:"id,omitempty"`
	Version string `json:"version,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SecretProviderClassPodStatus is the Schema for the secretproviderclassespodstatus API
type SecretProviderClassPodStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status SecretProviderClassPodStatusStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SecretProviderClassPodStatusList contains a list of SecretProviderClassPodStatus
type SecretProviderClassPodStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretProviderClassPodStatus `json:"items"`
}
