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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DockerConfiguration specifies the Docker build configuration
type DockerConfiguration struct {
	// Context specifies the build context path
	Context string `json:"context"`
	// DockerfilePath specifies the path to the Dockerfile
	DockerfilePath string `json:"dockerfilePath"`
}

// BuildConfiguration specifies the build configuration details
type BuildConfiguration struct {
	// Docker specifies the Docker-specific build configuration
	Docker DockerConfiguration `json:"docker"`
}

// BuildTemplateSpec defines the build template configuration
type BuildTemplateSpec struct {
	// Branch specifies the Git branch to use
	Branch string `json:"branch"`
	// Path specifies the repository path to use
	Path string `json:"path"`
	// BuildConfiguration specifies the build settings
	BuildConfiguration BuildConfiguration `json:"buildConfiguration"`
}

// DeploymentTrackSpec defines the desired state of DeploymentTrack.
type DeploymentTrackSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// BuildTemplateSpec defines the build template configuration
	BuildTemplateSpec BuildTemplateSpec `json:"buildTemplateSpec,omitempty"`
}

// DeploymentTrackStatus defines the observed state of DeploymentTrack.
type DeploymentTrackStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DeploymentTrack is the Schema for the deploymenttracks API.
type DeploymentTrack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentTrackSpec   `json:"spec,omitempty"`
	Status DeploymentTrackStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeploymentTrackList contains a list of DeploymentTrack.
type DeploymentTrackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentTrack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentTrack{}, &DeploymentTrackList{})
}
