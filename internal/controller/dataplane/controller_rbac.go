// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

// RBAC annotations for the dataplane controller are defined in this file.

// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
