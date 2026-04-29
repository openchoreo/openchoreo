// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// SecretInfo represents a secret resource resolved through the secret API.
// Values are never returned, only key names.
type SecretInfo struct {
	Name        string
	Namespace   string
	SecretType  corev1.SecretType
	TargetPlane openchoreov1alpha1.TargetPlaneRef
	Keys        []string
}

// CreateSecretParams holds the parameters for creating a secret.
type CreateSecretParams struct {
	SecretName  string
	SecretType  corev1.SecretType
	TargetPlane openchoreov1alpha1.TargetPlaneRef
	Data        map[string]string
}

// UpdateSecretParams holds the parameters for updating a secret.
type UpdateSecretParams struct {
	Data map[string]string
}

// Service defines the secret operations exposed by the secret creation API.
type Service interface {
	// CreateSecret provisions a new secret across the control plane and the target plane.
	CreateSecret(ctx context.Context, namespaceName string, req *CreateSecretParams) (*SecretInfo, error)
	// UpdateSecret rotates the data for an existing secret.
	UpdateSecret(ctx context.Context, namespaceName, secretName string, req *UpdateSecretParams) (*SecretInfo, error)
	// DeleteSecret removes a secret from the control plane and the target plane.
	DeleteSecret(ctx context.Context, namespaceName, secretName string) error
}
