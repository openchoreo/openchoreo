// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import "errors"

var (
	ErrEndpointReleaseBindingNotFound      = errors.New("endpoint release binding not found")
	ErrEndpointReleaseBindingAlreadyExists = errors.New("endpoint release binding already exists")
	ErrEndpointNotFound                    = errors.New("referenced endpoint not found")
)
