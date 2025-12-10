// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// FromAuthSubjectContext converts auth.SubjectContext to authz SubjectContext
func FromAuthSubjectContext(authCtx *auth.SubjectContext) *SubjectContext {
	if authCtx == nil {
		return nil
	}

	return &SubjectContext{
		Type:              SubjectType(authCtx.Type),
		EntitlementClaim:  authCtx.EntitlementClaim,
		EntitlementValues: authCtx.EntitlementValues,
	}
}
