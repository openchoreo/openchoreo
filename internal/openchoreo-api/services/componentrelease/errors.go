// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import "errors"

var (
	ErrComponentReleaseNil           = errors.New("component release is nil")
	ErrComponentReleaseNotFound      = errors.New("component release not found")
	ErrComponentReleaseAlreadyExists = errors.New("component release already exists")
)
