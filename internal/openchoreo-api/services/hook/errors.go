// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import "errors"

var (
	ErrHookNotFound      = errors.New("hook not found")
	ErrHookAlreadyExists = errors.New("hook already exists")
)
