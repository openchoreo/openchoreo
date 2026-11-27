// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import "errors"

var ErrForbidden = errors.New("insufficient permissions to perform this action")

// ErrConflict is returned by an update when the resource changed since the
// resourceVersion the caller sent was read. The caller should read the resource
// again, reapply its change and retry.
var ErrConflict = errors.New("the resource was modified since it was read")

// ValidationError represents a request validation failure.
// StatusCode carries the originating HTTP status when the error came from kube-apiserver
// (typically 422 for a webhook or CRD CEL denial). It is unset for openchoreo-api's own
// pre-kube input checks, which the handler maps to HTTP 400.
type ValidationError struct {
	Msg        string
	StatusCode int
}

func (e *ValidationError) Error() string { return e.Msg }
