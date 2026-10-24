// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import "errors"

var (
	ErrComponentNotFound        = errors.New("component not found")
	ErrDefaultLine              = errors.New("default is the component's implicit line; it has no ReleaseLine to create, change or delete")
	ErrReleaseLineNotFound      = errors.New("release line not found")
	ErrReleaseLineAlreadyExists = errors.New("release line already exists")
)
