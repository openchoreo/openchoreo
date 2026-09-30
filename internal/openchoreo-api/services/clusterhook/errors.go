// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import "errors"

var (
	ErrClusterHookNotFound      = errors.New("cluster hook not found")
	ErrClusterHookAlreadyExists = errors.New("cluster hook already exists")
)
