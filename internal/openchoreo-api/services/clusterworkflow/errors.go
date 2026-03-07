// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import "errors"

var (
	ErrClusterWorkflowNotFound      = errors.New("cluster workflow not found")
	ErrClusterWorkflowAlreadyExists = errors.New("cluster workflow already exists")
	ErrClusterWorkflowNil           = errors.New("cluster workflow cannot be nil")
)
