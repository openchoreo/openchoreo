// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the component service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design (discussion #1716).
type Service interface {
	CreateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error)
	UpdateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error)
	ListComponents(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error)
	GetComponent(ctx context.Context, namespaceName, componentName string) (*openchoreov1alpha1.Component, error)
	DeleteComponent(ctx context.Context, namespaceName, componentName string) error
}
