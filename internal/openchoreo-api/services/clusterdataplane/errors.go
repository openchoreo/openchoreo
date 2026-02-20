// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import "errors"

var (
	ErrClusterDataPlaneNil           = errors.New("cluster data plane is nil")
	ErrClusterDataPlaneNotFound      = errors.New("cluster data plane not found")
	ErrClusterDataPlaneAlreadyExists = errors.New("cluster data plane already exists")
)
