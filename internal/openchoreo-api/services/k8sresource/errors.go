// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresource

import "errors"

var (
	ErrPlaneNotFound      = errors.New("plane not found")
	ErrInvalidK8sPath     = errors.New("invalid K8s resource path")
	ErrMissingRequired    = errors.New("missing required field")
	ErrGatewayUnavailable = errors.New("gateway client is not available")
	ErrGatewayError       = errors.New("failed to proxy request to plane")
)
