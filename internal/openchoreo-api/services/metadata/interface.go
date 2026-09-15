// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package metadata

import "context"

// Service provides platform metadata.
type Service interface {
	GetMetadata(ctx context.Context) (*Metadata, error)
}

// Metadata describes how the platform is configured.
type Metadata struct {
	AuditLogs AuditLogs
}

// AuditLogs describes where clients query audit logs.
type AuditLogs struct {
	Enabled     bool
	ObserverURL string
}
