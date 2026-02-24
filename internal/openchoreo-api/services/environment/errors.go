// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import "errors"

var (
	ErrEnvironmentNil           = errors.New("environment is nil")
	ErrEnvironmentNotFound      = errors.New("environment not found")
	ErrEnvironmentAlreadyExists = errors.New("environment already exists")
	ErrDataPlaneNotFound        = errors.New("dataplane not found")
)

// ValidationError carries a K8s validation rejection message.
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }
