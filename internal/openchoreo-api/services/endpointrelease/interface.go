// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the endpoint release service interface.
type Service interface {
	ListEndpointReleases(ctx context.Context, namespaceName, endpointName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointRelease], error)
	GetEndpointRelease(ctx context.Context, namespaceName, endpointReleaseName string) (*openchoreov1alpha1.EndpointRelease, error)
	DeleteEndpointRelease(ctx context.Context, namespaceName, endpointReleaseName string) error
}
