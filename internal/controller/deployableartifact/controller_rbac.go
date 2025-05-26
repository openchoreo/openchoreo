// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployableartifact

// RBAC annotations for the deployableartifact controller are defined in this file.

// +kubebuilder:rbac:groups=core.choreo.dev,resources=deployableartifacts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=deployableartifacts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=deployableartifacts/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
