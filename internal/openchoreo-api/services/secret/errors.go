// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import "errors"

var (
	ErrSecretAlreadyExists      = errors.New("secret already exists")
	ErrSecretNotFound           = errors.New("secret not found")
	ErrPlaneNotFound            = errors.New("target plane not found")
	ErrSecretStoreNotConfigured = errors.New("secret store not configured")
)
