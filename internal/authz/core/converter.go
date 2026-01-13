// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// GetAuthzSubjectContext converts auth.SubjectContext to authz SubjectContext
func GetAuthzSubjectContext(authCtx *auth.SubjectContext) *SubjectContext {
	if authCtx == nil {
		return nil
	}

	return &SubjectContext{
		Type:              authCtx.Type,
		EntitlementClaim:  authCtx.EntitlementClaim,
		EntitlementValues: authCtx.EntitlementValues,
	}
}
