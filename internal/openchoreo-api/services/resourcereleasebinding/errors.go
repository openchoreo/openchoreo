// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import "errors"

var (
	ErrResourceReleaseBindingNotFound      = errors.New("resource release binding not found")
	ErrResourceReleaseBindingAlreadyExists = errors.New("resource release binding already exists")
	ErrResourceNotFound                    = errors.New("referenced resource not found")
)
