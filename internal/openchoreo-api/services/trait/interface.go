// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"context"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the trait service interface.
type Service interface {
	CreateTrait(ctx context.Context, namespaceName string, t *openchoreov1alpha1.Trait) (*openchoreov1alpha1.Trait, error)
	UpdateTrait(ctx context.Context, namespaceName string, t *openchoreov1alpha1.Trait) (*openchoreov1alpha1.Trait, error)
	ListTraits(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Trait], error)
	GetTrait(ctx context.Context, namespaceName, traitName string) (*openchoreov1alpha1.Trait, error)
	DeleteTrait(ctx context.Context, namespaceName, traitName string) error
	GetTraitSchema(ctx context.Context, namespaceName, traitName string) (*extv1.JSONSchemaProps, error)
}
