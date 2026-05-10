// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import "errors"

var (
	ErrResourceTypeNotFound      = errors.New("resource type not found")
	ErrResourceTypeAlreadyExists = errors.New("resource type already exists")
)
