// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import "errors"

var (
	ErrNamespaceNotFound      = errors.New("namespace not found")
	ErrNamespaceAlreadyExists = errors.New("namespace already exists")
)
