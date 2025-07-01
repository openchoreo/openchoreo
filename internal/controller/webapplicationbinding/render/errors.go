// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import "errors"

var (
	ErrInvalidPort      = errors.New("invalid port configuration")
	ErrInvalidResource  = errors.New("invalid resource configuration")
	ErrMissingContainer = errors.New("missing container configuration")
	ErrMissingImage     = errors.New("missing image configuration")
)