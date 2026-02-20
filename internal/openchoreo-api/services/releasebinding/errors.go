// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import "errors"

var (
	ErrReleaseBindingNotFound      = errors.New("release binding not found")
	ErrReleaseBindingAlreadyExists = errors.New("release binding already exists")
	ErrComponentNotFound           = errors.New("component not found")
)
