// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

import "errors"

var (
	ErrEndpointTypeNotFound      = errors.New("endpoint type not found")
	ErrEndpointTypeAlreadyExists = errors.New("endpoint type already exists")
)
