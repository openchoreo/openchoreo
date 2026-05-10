// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import "errors"

var (
	ErrResourceNotFound      = errors.New("resource not found")
	ErrResourceAlreadyExists = errors.New("resource already exists")
)
