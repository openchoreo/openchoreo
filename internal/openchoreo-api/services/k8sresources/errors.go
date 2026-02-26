// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import "errors"

var (
	ErrReleaseBindingNotFound = errors.New("release binding not found")
	ErrReleaseNotFound        = errors.New("release not found")
	ErrEnvironmentNotFound    = errors.New("environment not found")
	ErrResourceNotFound       = errors.New("resource not found in release")
)
