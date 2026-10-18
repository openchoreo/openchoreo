// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import "errors"

var (
	ErrClusterEndpointTypeNotFound      = errors.New("cluster endpoint type not found")
	ErrClusterEndpointTypeAlreadyExists = errors.New("cluster endpoint type already exists")
)
