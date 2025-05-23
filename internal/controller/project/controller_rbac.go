/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package project

// RBAC annotations for the project controller are defined in this file.

// +kubebuilder:rbac:groups=core.choreo.dev,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
