// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterbuildplane

import "errors"

var (
	ErrClusterBuildPlaneNil      = errors.New("cluster build plane is nil")
	ErrClusterBuildPlaneNotFound = errors.New("cluster build plane not found")
)
