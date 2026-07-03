// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

import "errors"

var (
	ErrComponentReleaseBindingNotFound      = errors.New("release binding not found")
	ErrComponentReleaseBindingAlreadyExists = errors.New("release binding already exists")
	ErrComponentNotFound                    = errors.New("component not found")
)
