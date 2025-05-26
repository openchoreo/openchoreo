// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

// RBAC annotations for the project controller are defined in this file.

// +kubebuilder:rbac:groups=core.choreo.dev,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
