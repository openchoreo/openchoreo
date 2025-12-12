// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package subject

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Resolver is responsible for resolving user types from authentication tokens
// Each authentication mechanism (JWT, OAuth2, API Key, etc.) should implement this interface
type Resolver interface {
	// ResolveUserType analyzes an authentication token and returns the SubjectContext
	ResolveUserType(authToken string) (*auth.SubjectContext, error)
}
