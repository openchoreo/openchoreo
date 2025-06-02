// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

// RBAC annotations for the endpoint controller are defined in this file.

// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.envoyproxy.io,resources=securitypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=endpoints/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
