// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the resource release service interface.
type Service interface {
	ListResourceReleases(ctx context.Context, namespaceName, resourceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceRelease], error)
	GetResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) (*openchoreov1alpha1.ResourceRelease, error)
	CreateResourceRelease(ctx context.Context, namespaceName string, rr *openchoreov1alpha1.ResourceRelease) (*openchoreov1alpha1.ResourceRelease, error)
	DeleteResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) error
}
