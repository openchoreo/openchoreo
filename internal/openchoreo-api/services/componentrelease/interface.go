// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the component release service interface.
type Service interface {
	ListComponentReleases(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentRelease], error)
	GetComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) (*openchoreov1alpha1.ComponentRelease, error)
}
