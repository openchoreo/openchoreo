// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import "errors"

var (
	ErrClusterProjectTypeNotFound      = errors.New("cluster project type not found")
	ErrClusterProjectTypeAlreadyExists = errors.New("cluster project type already exists")
)
