// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

// RBAC annotations for the clusterdataplane controller are defined in this file.

// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
