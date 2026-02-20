// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the release service interface.
type Service interface {
	ListReleases(ctx context.Context, namespaceName, componentName, environmentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Release], error)
	GetRelease(ctx context.Context, namespaceName, releaseName string) (*openchoreov1alpha1.Release, error)
}
