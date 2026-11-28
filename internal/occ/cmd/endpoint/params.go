// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

// ListParams defines parameters for listing endpoints
type ListParams struct {
	Namespace string
	Project   string
}

// GetParams defines parameters for getting a single endpoint
type GetParams struct {
	Namespace    string
	EndpointName string
}

// DeleteParams defines parameters for deleting a single endpoint
type DeleteParams struct {
	Namespace    string
	EndpointName string
}

// PromoteParams defines parameters for promoting an endpoint to a target environment
type PromoteParams struct {
	Namespace    string
	EndpointName string
	Environment  string
}

// TrafficGetParams defines parameters for showing an endpoint's traffic
type TrafficGetParams struct {
	Namespace    string
	EndpointName string
}

// TrafficSetParams defines parameters for saving a split. Without an
// environment it is the endpoint's default split.
type TrafficSetParams struct {
	Namespace    string
	EndpointName string
	Environment  string
	Split        map[string]int32
	DryRun       bool
}

// TrafficResetParams defines parameters for dropping an environment's own split
type TrafficResetParams struct {
	Namespace    string
	EndpointName string
	Environment  string
	DryRun       bool
}
