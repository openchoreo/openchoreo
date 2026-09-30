// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the hook service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
type Service interface {
	CreateHook(ctx context.Context, namespaceName string, h *openchoreov1alpha1.Hook) (*openchoreov1alpha1.Hook, error)
	UpdateHook(ctx context.Context, namespaceName string, h *openchoreov1alpha1.Hook) (*openchoreov1alpha1.Hook, error)
	ListHooks(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Hook], error)
	GetHook(ctx context.Context, namespaceName, hookName string) (*openchoreov1alpha1.Hook, error)
	DeleteHook(ctx context.Context, namespaceName, hookName string) error
}
