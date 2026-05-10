// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import "errors"

var (
	ErrResourceReleaseNotFound      = errors.New("resource release not found")
	ErrResourceReleaseAlreadyExists = errors.New("resource release already exists")
	ErrResourceReleaseNil           = errors.New("resource release cannot be nil")
)
