// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresource

import "context"

// QueryRequest holds the parameters for a K8s resource query.
type QueryRequest struct {
	PlaneType       string
	PlaneName       string
	PlaneNamespace  string // empty for cluster-scoped planes
	K8sResourcePath string
	RawQuery        string
	Project         string
	Component       string
	Environment     string
}

// QueryResponse holds the result of a K8s resource query.
type QueryResponse struct {
	Data           map[string]interface{}
	K8sStatusCode  int
	PlaneType      string
	PlaneName      string
	PlaneNamespace string
}

// Service defines the interface for querying K8s resources through planes.
type Service interface {
	QueryK8sResources(ctx context.Context, req *QueryRequest) (*QueryResponse, error)
}
