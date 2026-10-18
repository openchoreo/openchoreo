// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import "errors"

var (
	ErrEndpointNotFound      = errors.New("endpoint not found")
	ErrEndpointAlreadyExists = errors.New("endpoint already exists")
)
