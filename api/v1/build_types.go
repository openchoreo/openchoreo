/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type BuildEnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type BuildEnvironmentFrom struct {
	SecretRef string `json:"secretRef"`
}

type BuildEnvironment struct {
	Env     []BuildEnvironmentVariable `json:"env,omitempty"`
	EnvFrom []BuildEnvironmentFrom     `json:"envFrom,omitempty"`
}

type BuildpackName string

const (
	BuildpackReact     BuildpackName = "React"
	BuildpackGo        BuildpackName = "Go"
	BuildpackBallerina BuildpackName = "Ballerina"
	BuildpackNodeJS    BuildpackName = "Node.js"
	BuildpackPython    BuildpackName = "Python"
	BuildpackRuby      BuildpackName = "Ruby"
	BuildpackPHP       BuildpackName = "PHP"
	//BuildpackJava      BuildpackName = "Java"

)

// SupportedVersions maps each buildpack to its supported versions.
// Refer (builder:google-22): https://cloud.google.com/docs/buildpacks/builders
var SupportedVersions = map[BuildpackName][]string{
	BuildpackReact:     {"18.20.6", "19.9.0", "20.18.3", "21.7.3", "22.14.0", "23.7.0"},
	BuildpackGo:        {"1.x"},
	BuildpackBallerina: {"2201.7.5", "2201.8.9", "2201.9.6", "2201.10.4", "2201.11.0"},
	BuildpackNodeJS:    {"12.x.x", "14.x.x", "16.x.x", "18.x.x", "20.x.x", "22.x.x"},
	BuildpackPython:    {"3.10.x", "3.11.x", "3.12.x"},
	BuildpackRuby:      {"3.1.x", "3.2.x", "3.3.x"},
	BuildpackPHP:       {"8.1.x", "8.2.x", "8.3.x"},
	//BuildpackJava:      {"8", "11", "17", "18", "21"},
}

type DockerConfiguration struct {
	// Context specifies the build context path
	Context string `json:"context"`
	// DockerfilePath specifies the path to the Dockerfile
	DockerfilePath string `json:"dockerfilePath"`
}

type BuildpackConfiguration struct {
	Name    BuildpackName `json:"name"`
	Version string        `json:"version,omitempty"`
}

// BuildConfiguration specifies the build configuration details
type BuildConfiguration struct {
	// Docker specifies the Docker-specific build configuration
	Docker *DockerConfiguration `json:"docker,omitempty"`
	// Buildpack specifies the buildpack to use
	Buildpack *BuildpackConfiguration `json:"buildpack,omitempty"`
}

// BuildSpec defines the desired state of Build.
type BuildSpec struct {
	Branch             string             `json:"branch,omitempty"`
	GitRevision        string             `json:"gitRevision,omitempty"`
	Path               string             `json:"path,omitempty"`
	AutoBuild          bool               `json:"autoBuild,omitempty"`
	BuildConfiguration BuildConfiguration `json:"buildConfiguration"`
	BuildEnvironment   BuildEnvironment   `json:"buildEnvironment,omitempty"`
}

func (b *Build) GetConditions() []metav1.Condition {
	return b.Status.Conditions
}

func (b *Build) SetConditions(conditions []metav1.Condition) {
	b.Status.Conditions = conditions
}

type Image struct {
	Image string `json:"image"`
}

// BuildStatus defines the observed state of Build.
type BuildStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of an object's current state.
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
	ImageStatus Image              `json:"imageStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Build is the Schema for the builds API.
type Build struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildSpec   `json:"spec"`
	Status BuildStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BuildList contains a list of Build.
type BuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Build `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Build{}, &BuildList{})
}
