// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import "errors"

var (
	ErrProjectReleaseBindingNotFound      = errors.New("project release binding not found")
	ErrProjectReleaseBindingAlreadyExists = errors.New("project release binding already exists")
	ErrProjectNotFound                    = errors.New("referenced project not found")
)
