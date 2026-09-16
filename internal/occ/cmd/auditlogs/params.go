// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package auditlogs implements the occ auditlogs command.
package auditlogs

// QueryParams holds the flags of an audit log query.
type QueryParams struct {
	Since string
	Start string
	End   string

	Limit     int
	SortOrder string
	Output    string

	ActorIDs      []string
	ActorTypes    []string
	Issuers       []string
	SessionIDs    []string
	Entitlements  []string
	Actions       []string
	Categories    []string
	Results       []string
	Surfaces      []string
	Producers     []string
	OperationIDs  []string
	RequestIDs    []string
	EventIDs      []string
	SourceIPs     []string
	UserAgents    []string
	ResourceTypes []string
	ResourceNames []string
	Namespaces    []string
	Projects      []string
	Components    []string
	Resources     []string
	Environments  []string
	Search        string
}
