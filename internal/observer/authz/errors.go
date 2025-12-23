// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import "errors"

var (
	ErrAuthzServiceUnavailable = errors.New("authorization service unavailable")
	ErrAuthzTimeout            = errors.New("authorization request timeout")
	ErrAuthzInvalidResponse    = errors.New("invalid authorization response")
	ErrAuthzForbidden          = errors.New("insufficient permissions to perform this action")
	ErrAuthzUnauthorized       = errors.New("unauthorized request")
)
