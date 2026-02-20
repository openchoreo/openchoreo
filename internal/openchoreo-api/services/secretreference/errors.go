// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import "errors"

var (
	ErrSecretReferenceNotFound      = errors.New("secret reference not found")
	ErrSecretReferenceAlreadyExists = errors.New("secret reference already exists")
)
