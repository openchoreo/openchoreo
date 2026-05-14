// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// CreateSecretParams holds the parameters for creating a secret.
type CreateSecretParams struct {
	SecretName  string
	SecretType  corev1.SecretType
	TargetPlane openchoreov1alpha1.TargetPlaneRef
	Data        map[string]string
}

// UpdateSecretParams holds the parameters for replacing a secret's data.
// The Data map is the final state: keys present in the existing secret but
// absent here are pruned from the K8s Secret, PushSecret, and SecretReference.
type UpdateSecretParams struct {
	Data map[string]string
}

// Service defines the secret operations exposed by the Secret API.
type Service interface {
	CreateSecret(ctx context.Context, namespaceName string, req *CreateSecretParams) (*corev1.Secret, error)
	UpdateSecret(ctx context.Context, namespaceName, secretName string, req *UpdateSecretParams) (*corev1.Secret, error)
	GetSecret(ctx context.Context, namespaceName, secretName string) (*corev1.Secret, error)
	ListSecrets(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[corev1.Secret], error)
	DeleteSecret(ctx context.Context, namespaceName, secretName string) error
}
