// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import "errors"

var (
	ErrComponentNotFound        = errors.New("component not found")
	ErrReleaseLinesNotEnabled   = errors.New("component does not have release lines enabled")
	ErrReleaseLineNotFound      = errors.New("release line not found")
	ErrReleaseLineAlreadyExists = errors.New("release line already exists")
)
