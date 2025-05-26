// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package dataplane

// RBAC annotations for the dataplane controller are defined in this file.

// +kubebuilder:rbac:groups=core.choreo.dev,resources=dataplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=dataplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=dataplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
