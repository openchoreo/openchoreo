// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

// RBAC annotations for the dataplane controller are defined in this file.

// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=buildplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
