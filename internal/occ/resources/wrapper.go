// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceWrapper wraps a Kubernetes resource object and provides additional
// information about the resource, such as the logical name and the Kubernetes name.
type ResourceWrapper[T client.Object] struct {
	// Resource is the actual Kubernetes resource object
	Resource T

	// LogicalName is the name of the resource from Choreo's perspective (from labels)
	LogicalName string

	// KubernetesName is the actual Kubernetes object name (metadata.name)
	KubernetesName string
}

// GetName returns the logical name of the resource
func (w *ResourceWrapper[T]) GetName() string {
	return w.LogicalName
}

// GetKubernetesName returns the Kubernetes name of the resource
func (w *ResourceWrapper[T]) GetKubernetesName() string {
	return w.KubernetesName
}

// GetResource returns the underlying Kubernetes resource
func (w *ResourceWrapper[T]) GetResource() T {
	return w.Resource
}
