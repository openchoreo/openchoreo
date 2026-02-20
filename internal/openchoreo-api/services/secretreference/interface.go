// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the secret reference service interface.
type Service interface {
	CreateSecretReference(ctx context.Context, namespaceName string, sr *openchoreov1alpha1.SecretReference) (*openchoreov1alpha1.SecretReference, error)
	UpdateSecretReference(ctx context.Context, namespaceName string, sr *openchoreov1alpha1.SecretReference) (*openchoreov1alpha1.SecretReference, error)
	ListSecretReferences(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.SecretReference], error)
	GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (*openchoreov1alpha1.SecretReference, error)
	DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) error
}
