// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// CostQueryRequest is the request for per-component cost records.
// The observer resolves Environment, Project, and Component names to UIDs before
// forwarding the query to the FinOps adapter.
type CostQueryRequest struct {
	// Namespace name
	Namespace string
	// Environment name. Resolved to an environment UID.
	Environment string
	// Project name. Optional; when set, narrows the response to this project.
	Project string
	// Component name. Optional; when set, narrows the response to this component.
	// Requires Project to also be set.
	Component string
	// StartTime is the inclusive lower bound of the cost window (RFC 3339).
	StartTime string
	// EndTime is the exclusive upper bound of the cost window (RFC 3339).
	EndTime string
	// Granularity, when set, splits each component into one record per time
	// bucket. Uses <count><unit> notation (e.g. 1h, 2d, 3w).
	Granularity string
}

// RecommendationQueryRequest is the request for right-sizing
// recommendations. Like CostQueryRequest, names are resolved to UIDs before
// forwarding, but the FinOps adapter also needs the names forwarded alongside
// the UIDs (it calls back into the observer's name-based metrics API).
type RecommendationQueryRequest struct {
	Namespace   string
	Environment string
	Project     string
	Component   string
	StartTime   string
	EndTime     string
}
