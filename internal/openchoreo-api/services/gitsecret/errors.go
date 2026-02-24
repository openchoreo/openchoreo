// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import "errors"

var (
	ErrGitSecretAlreadyExists   = errors.New("git secret already exists")
	ErrGitSecretNotFound        = errors.New("git secret not found")
	ErrSecretStoreNotConfigured = errors.New("secret store not configured")
	ErrInvalidSecretType        = errors.New("secret type must be 'basic-auth' or 'ssh-auth'")
	ErrBuildPlaneNotFound       = errors.New("build plane not found")
)
