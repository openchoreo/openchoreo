// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import "errors"

var (
	ErrObservabilityPlaneNil           = errors.New("observability plane is nil")
	ErrObservabilityPlaneNotFound      = errors.New("observability plane not found")
	ErrObservabilityPlaneAlreadyExists = errors.New("observability plane already exists")
)
