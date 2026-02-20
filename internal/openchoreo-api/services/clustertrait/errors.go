// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import "errors"

var (
	ErrClusterTraitNotFound      = errors.New("cluster trait not found")
	ErrClusterTraitAlreadyExists = errors.New("cluster trait already exists")
)
