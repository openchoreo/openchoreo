// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
)

// getAuditData retrieves or creates the audit data container from context
func getAuditData(ctx context.Context) *AuditData {
	if data, ok := ctx.Value(auditDataKey).(*AuditData); ok {
		return data
	}
	return nil
}

// SetResource stores resource information for audit logging
// Handlers should call this to specify which resource is being acted upon
func SetResource(ctx context.Context, resource *Resource) {
	if data := getAuditData(ctx); data != nil {
		data.Resource = resource
	}
}

// GetResource retrieves the resource information from the context
// Returns nil if no resource has been set
func GetResource(ctx context.Context) *Resource {
	if data := getAuditData(ctx); data != nil {
		return data.Resource
	}
	return nil
}

// SetMetadata stores additional audit metadata
// Handlers can call this to add custom fields to the audit event
func SetMetadata(ctx context.Context, metadata map[string]any) {
	if data := getAuditData(ctx); data != nil {
		data.Metadata = metadata
	}
}

// AddMetadata adds a single key-value pair to the audit metadata
// If metadata doesn't exist yet, it creates a new map
func AddMetadata(ctx context.Context, key string, value any) {
	if data := getAuditData(ctx); data != nil {
		if data.Metadata == nil {
			data.Metadata = make(map[string]any)
		}
		data.Metadata[key] = value
	}
}

// GetMetadata retrieves the audit metadata from the context
// Returns nil if no metadata has been set
func GetMetadata(ctx context.Context) map[string]any {
	if data := getAuditData(ctx); data != nil {
		return data.Metadata
	}
	return nil
}
