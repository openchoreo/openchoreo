// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

// RBAC annotations for the component controller are defined in this file.

// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
