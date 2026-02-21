// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import "errors"

var (
	ErrClusterObservabilityPlaneNil           = errors.New("cluster observability plane is nil")
	ErrClusterObservabilityPlaneNotFound      = errors.New("cluster observability plane not found")
	ErrClusterObservabilityPlaneAlreadyExists = errors.New("cluster observability plane already exists")
)
