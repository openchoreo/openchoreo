// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import "errors"

var (
	ErrWorkloadNotFound      = errors.New("workload not found")
	ErrWorkloadAlreadyExists = errors.New("workload already exists")
	ErrComponentNotFound     = errors.New("component not found")
)
