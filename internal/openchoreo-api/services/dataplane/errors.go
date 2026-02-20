// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import "errors"

var (
	ErrDataPlaneNotFound      = errors.New("data plane not found")
	ErrDataPlaneAlreadyExists = errors.New("data plane already exists")
)
