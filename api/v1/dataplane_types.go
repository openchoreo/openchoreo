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

// KubernetesClusterSpec defines the configuration for the target Kubernetes cluster
type KubernetesClusterSpec struct {
	// Name of the Kubernetes cluster
	Name string `json:"name"`
	// Reference to the connection configuration for the cluster
	ConnectionConfigRef string `json:"connectionConfigRef"`
	// FeatureFlags specifies enabled/disabled features
	FeatureFlags FeatureFlagsSpec `json:"featureFlags"`
}

// FeatureFlagsSpec defines the feature flags for the data plane
type FeatureFlagsSpec struct {
	// Enable/disable Cilium networking
	Cilium bool `json:"cilium"`
	// Enable/disable scale to zero functionality
	ScaleToZero bool `json:"scaleToZero"`
	// GatewayType specifies the type of gateway to be used (e.g., envoy)
	GatewayType string `json:"gatewayType"`
}

// GatewaySpec defines the gateway configuration for the data plane
type GatewaySpec struct {
	// Public virtual host for the gateway
	PublicVirtualHost string `json:"publicVirtualHost"`
	// Organization-specific virtual host for the gateway
	OrganizationVirtualHost string `json:"organizationVirtualHost"`
}

// DataPlaneSpec defines the desired state of DataPlane.
type DataPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// KubernetesCluster specifies the target cluster configuration
	KubernetesCluster KubernetesClusterSpec `json:"kubernetesCluster"`
	// Gateway specifies the gateway configuration
	Gateway GatewaySpec `json:"gateway"`
}

// DataPlaneStatus defines the observed state of DataPlane.
type DataPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=dp;dps

// DataPlane is the Schema for the dataplanes API.
type DataPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataPlaneSpec   `json:"spec,omitempty"`
	Status DataPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataPlaneList contains a list of DataPlane.
type DataPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPlane{}, &DataPlaneList{})
}

func (d *DataPlane) GetConditions() []metav1.Condition {
	return d.Status.Conditions
}

func (d *DataPlane) SetConditions(conditions []metav1.Condition) {
	d.Status.Conditions = conditions
}
