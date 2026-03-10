// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import "errors"

var (
	ErrClusterWorkflowPlaneNil           = errors.New("cluster workflow plane is nil")
	ErrClusterWorkflowPlaneNotFound      = errors.New("cluster workflow plane not found")
	ErrClusterWorkflowPlaneAlreadyExists = errors.New("cluster workflow plane already exists")
)
