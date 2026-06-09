// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

import "errors"

var (
	ErrClusterResourceTypeNotFound      = errors.New("cluster resource type not found")
	ErrClusterResourceTypeAlreadyExists = errors.New("cluster resource type already exists")
)
