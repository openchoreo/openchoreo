// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import "errors"

var (
	ErrTraitNotFound      = errors.New("trait not found")
	ErrTraitAlreadyExists = errors.New("trait already exists")
)
