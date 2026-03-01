// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import "errors"

var (
	// ErrInvalidSignature is returned when the webhook signature does not match the payload.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrSecretNotConfigured is returned when the webhook secret cannot be retrieved from Kubernetes.
	ErrSecretNotConfigured = errors.New("webhook secret not configured")
)
