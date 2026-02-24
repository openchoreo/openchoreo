// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import "errors"

var ErrForbidden = errors.New("insufficient permissions to perform this action")

// ValidationError represents a request validation failure.
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }
