// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Common error codes for authentication (can be extended by specific auth mechanisms)
const (
	CodeInternalError = "INTERNAL_ERROR"
)

// AuthError represents an authentication error with HTTP status and error code
type AuthError struct {
	// Code is the machine-readable error code
	Code string
	// Message is a human-readable error message
	Message string
	// HTTPStatus is the HTTP status code to return
	HTTPStatus int
	// Err is the underlying error, if any
	Err error
}

func (e *AuthError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// IsClientError returns true if this is a client-side error (4xx)
func (e *AuthError) IsClientError() bool {
	return e.HTTPStatus >= 400 && e.HTTPStatus < 500
}

// IsServerError returns true if this is a server-side error (5xx)
func (e *AuthError) IsServerError() bool {
	return e.HTTPStatus >= 500
}

// NewAuthError creates a new authentication error
func NewAuthError(code, message string, status int, err error) *AuthError {
	return &AuthError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Err:        err,
	}
}

// DefaultErrorHandler is the default error handler for authentication failures.
// It logs server errors (5xx) but not client errors (4xx) as they are expected user errors.
// This can be used by any authentication mechanism.
func DefaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	logger := slog.Default()

	var authErr *AuthError
	if errors.As(err, &authErr) {
		// Log server errors (5xx) but not client errors (4xx)
		if authErr.IsServerError() {
			logger.Error("Authentication server error",
				"code", authErr.Code,
				"message", authErr.Message,
				"status", authErr.HTTPStatus,
				"error", authErr.Err,
				"path", r.URL.Path,
				"method", r.Method,
			)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(authErr.HTTPStatus)
		fmt.Fprintf(w, `{"error":"%s","message":"%s"}`, authErr.Code, authErr.Message)
		return
	}

	// Fallback for unknown errors (treat as server error)
	logger.Error("Unexpected authentication error",
		"error", err,
		"path", r.URL.Path,
		"method", r.Method,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `{"error":"%s","message":"An unexpected error occurred"}`, CodeInternalError)
}
