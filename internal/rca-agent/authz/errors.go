// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import "errors"

var (
	ErrAuthzServiceUnavailable = errors.New("authorization service unavailable")
	ErrAuthzInvalidResponse    = errors.New("invalid authorization response")
	ErrAuthzForbidden          = errors.New("insufficient permissions to perform this action")
	ErrAuthzUnauthorized       = errors.New("unauthorized request")
)
