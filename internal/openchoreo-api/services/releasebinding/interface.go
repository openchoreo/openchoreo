// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the release binding service interface.
type Service interface {
	CreateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error)
	UpdateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error)
	ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error)
	GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*openchoreov1alpha1.ReleaseBinding, error)
	DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error
}
