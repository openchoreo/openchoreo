// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import "errors"

var (
	ErrComponentNotFound        = errors.New("component not found")
	ErrComponentAlreadyExists   = errors.New("component already exists")
	ErrComponentReleaseNotFound = errors.New("component release not found")
	ErrWorkloadNotFound         = errors.New("workload not found")
	ErrTraitNotFound            = errors.New("trait not found")
	ErrValidation               = errors.New("validation error")
	ErrComponentTypeNotFound    = errors.New("component type not found")
)
