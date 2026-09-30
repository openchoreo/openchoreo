// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=chk;chks
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Workflow",type="string",JSONPath=".spec.workflowRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterHook is the Schema for the clusterhooks API.
// ClusterHook is a cluster-scoped version of Hook that can be bound by
// Environments in every namespace. It may only reference a ClusterWorkflow.
type ClusterHook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HookSpec   `json:"spec,omitempty"`
	Status HookStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterHookList contains a list of ClusterHook.
type ClusterHookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterHook `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterHook{}, &ClusterHookList{})
}
