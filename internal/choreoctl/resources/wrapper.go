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
