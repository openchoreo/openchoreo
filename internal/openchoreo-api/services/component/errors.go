// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import "errors"

var (
	ErrComponentNotFound      = errors.New("component not found")
	ErrComponentAlreadyExists = errors.New("component already exists")
)
