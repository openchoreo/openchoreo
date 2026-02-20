// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import "errors"

var (
	ErrBuildPlaneNotFound = errors.New("build plane not found")

	errNotImplemented = errors.New("not implemented on the authz-wrapped service as it is not exposed externally")
)
