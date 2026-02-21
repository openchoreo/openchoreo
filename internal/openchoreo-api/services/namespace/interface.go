// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the namespace service interface.
type Service interface {
	CreateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error)
	UpdateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error)
	ListNamespaces(ctx context.Context, opts services.ListOptions) (*services.ListResult[corev1.Namespace], error)
	GetNamespace(ctx context.Context, namespaceName string) (*corev1.Namespace, error)
	DeleteNamespace(ctx context.Context, namespaceName string) error
}
