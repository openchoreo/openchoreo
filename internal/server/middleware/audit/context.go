// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
)

// NewAuditContext returns a copy of ctx carrying a fresh audit data
// container pre-populated with resource, plus the container itself so the
// caller can read back whatever SetResource later wrote into it.
func NewAuditContext(ctx context.Context, resource *Resource) (context.Context, *AuditData) {
	data := &AuditData{Resource: resource}
	return context.WithValue(ctx, auditDataKey, data), data
}

// getAuditData retrieves or creates the audit data container from context
func getAuditData(ctx context.Context) *AuditData {
	if data, ok := ctx.Value(auditDataKey).(*AuditData); ok {
		return data
	}
	return nil
}

// SetResource stores resource information for audit logging. Handlers should
// call this once they know the resource's real identity (typically from the
// object a create/update returned).
func SetResource(ctx context.Context, resource *Resource) {
	if data := getAuditData(ctx); data != nil {
		data.Resource = resource
	}
}
