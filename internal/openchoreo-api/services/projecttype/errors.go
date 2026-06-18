// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import "errors"

var (
	ErrProjectTypeNotFound      = errors.New("project type not found")
	ErrProjectTypeAlreadyExists = errors.New("project type already exists")
)
