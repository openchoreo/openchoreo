// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import "errors"

var (
	ErrProjectReleaseNotFound      = errors.New("project release not found")
	ErrProjectReleaseAlreadyExists = errors.New("project release already exists")
	ErrProjectReleaseNil           = errors.New("project release cannot be nil")
)
