// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import "errors"

var (
	ErrClusterComponentTypeNotFound      = errors.New("cluster component type not found")
	ErrClusterComponentTypeAlreadyExists = errors.New("cluster component type already exists")
)
