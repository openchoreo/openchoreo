// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import "errors"

var (
	ErrComponentNotFound        = errors.New("component not found")
	ErrComponentAlreadyExists   = errors.New("component already exists")
	ErrComponentReleaseNotFound = errors.New("component release not found")
	ErrReleaseBindingNotFound   = errors.New("release binding not found")
	ErrPipelineNotFound         = errors.New("deployment pipeline not found")
	ErrPipelineNotConfigured    = errors.New("project has no deployment pipeline configured")
	ErrNoLowestEnvironment      = errors.New("no lowest environment found in deployment pipeline")
	ErrInvalidPromotionPath     = errors.New("invalid promotion path")
	ErrWorkloadNotFound         = errors.New("workload not found")
	ErrTraitNameCollision       = errors.New("trait name collision across kinds")
)
