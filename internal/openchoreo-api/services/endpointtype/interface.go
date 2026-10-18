// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the endpoint type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateEndpointType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.EndpointType) (*openchoreov1alpha1.EndpointType, error)
	UpdateEndpointType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.EndpointType) (*openchoreov1alpha1.EndpointType, error)
	ListEndpointTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointType], error)
	GetEndpointType(ctx context.Context, namespaceName, etName string) (*openchoreov1alpha1.EndpointType, error)
	DeleteEndpointType(ctx context.Context, namespaceName, etName string) error
	GetEndpointTypeSchema(ctx context.Context, namespaceName, etName string) (map[string]any, error)
}
