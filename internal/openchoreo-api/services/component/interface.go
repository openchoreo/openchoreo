// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// DeployReleaseRequest contains the parameters for deploying a release.
type DeployReleaseRequest struct {
	ReleaseName string
}

// PromoteComponentRequest contains the parameters for promoting a component.
type PromoteComponentRequest struct {
	SourceEnvironment string
	TargetEnvironment string
}

// GenerateReleaseRequest contains the parameters for generating a component release.
type GenerateReleaseRequest struct {
	ReleaseName string
}

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
	DeployRelease(ctx context.Context, namespaceName, componentName string, req *DeployReleaseRequest) (*openchoreov1alpha1.ReleaseBinding, error)
	PromoteComponent(ctx context.Context, namespaceName, componentName string, req *PromoteComponentRequest) (*openchoreov1alpha1.ReleaseBinding, error)
	GenerateRelease(ctx context.Context, namespaceName, componentName string, req *GenerateReleaseRequest) (*openchoreov1alpha1.ComponentRelease, error)
	GetComponentSchema(ctx context.Context, namespaceName, componentName string) (*extv1.JSONSchemaProps, error)
}
