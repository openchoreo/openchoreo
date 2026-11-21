// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
)

// Service defines the release binding service interface.
type Service interface {
	CreateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error)
	UpdateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error)
	ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error)
	GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*openchoreov1alpha1.ReleaseBinding, error)
	DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error

	ListReleaseBindingRevisions(ctx context.Context, namespaceName, componentName, environment, binding string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBindingRevision], error)
	GetReleaseBindingRevision(ctx context.Context, namespaceName, revisionName string) (*openchoreov1alpha1.ReleaseBindingRevision, error)
	RollbackReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string, req bindingrevision.RollbackRequest) (*bindingrevision.RollbackResult, error)
	DeleteReleaseBindingRevision(ctx context.Context, namespaceName, revisionName string, req bindingrevision.DeleteRequest) (*bindingrevision.DeleteResult, error)
}
