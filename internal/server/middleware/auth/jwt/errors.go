// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import "errors"

// JWT-specific authentication errors
var (
	ErrMissingToken        = errors.New("missing or invalid authentication token")
	ErrInvalidToken        = errors.New("invalid or expired token")
	ErrInvalidClaims       = errors.New("token claims validation failed")
	ErrAuthorizationFailed = errors.New("authorization failed")
	ErrJWKSUnavailable     = errors.New("JWKS endpoint unavailable")
	ErrKeyNotFound         = errors.New("token signing key not available")
)

// JWT-specific error codes
const (
	CodeMissingToken        = "MISSING_TOKEN"
	CodeInvalidToken        = "INVALID_TOKEN"
	CodeInvalidClaims       = "INVALID_CLAIMS"
	CodeAuthorizationFailed = "AUTHORIZATION_FAILED"
	CodeJWKSUnavailable     = "JWKS_UNAVAILABLE"
	CodeKeyNotFound         = "KEY_NOT_FOUND"
)
