// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the endpoint service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateEndpoint(ctx context.Context, namespaceName string, endpoint *openchoreov1alpha1.Endpoint) (*openchoreov1alpha1.Endpoint, error)
	UpdateEndpoint(ctx context.Context, namespaceName string, endpoint *openchoreov1alpha1.Endpoint) (*openchoreov1alpha1.Endpoint, error)
	ListEndpoints(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Endpoint], error)
	GetEndpoint(ctx context.Context, namespaceName, endpointName string) (*openchoreov1alpha1.Endpoint, error)
	DeleteEndpoint(ctx context.Context, namespaceName, endpointName string) error
}
