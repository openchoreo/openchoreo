// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the release line service interface. Release lines are addressed through their component.
type Service interface {
	ListReleaseLines(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseLine], error)
	GetReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) (*openchoreov1alpha1.ReleaseLine, error)
	CreateReleaseLine(ctx context.Context, namespaceName, componentName string, releaseLine *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error)
	UpdateReleaseLine(ctx context.Context, namespaceName, componentName string, releaseLine *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error)
	DeleteReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) error
}
