// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import "errors"

var (
	ErrComponentTypeNotFound      = errors.New("component type not found")
	ErrComponentTypeAlreadyExists = errors.New("component type already exists")
)
