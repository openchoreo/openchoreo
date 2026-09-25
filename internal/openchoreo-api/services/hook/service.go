// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// hookService handles hook business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type hookService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var hookTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Hook",
}

var _ Service = (*hookService)(nil)

// NewService creates a new hook service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &hookService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *hookService) CreateHook(ctx context.Context, namespaceName string, h *openchoreov1alpha1.Hook) (*openchoreov1alpha1.Hook, error) {
	if h == nil {
		return nil, fmt.Errorf("hook cannot be nil")
	}

	s.logger.Debug("Creating hook", "namespace", namespaceName, "hook", h.Name)

	// Set defaults
	h.Namespace = namespaceName
	h.Status = openchoreov1alpha1.HookStatus{}

	if err := s.k8sClient.Create(ctx, h); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Hook already exists", "namespace", namespaceName, "hook", h.Name)
			return nil, ErrHookAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create hook CR", "error", err)
		return nil, fmt.Errorf("failed to create hook: %w", err)
	}

	s.logger.Debug("Hook created successfully", "namespace", namespaceName, "hook", h.Name)
	h.TypeMeta = hookTypeMeta
	return h, nil
}

func (s *hookService) UpdateHook(ctx context.Context, namespaceName string, h *openchoreov1alpha1.Hook) (*openchoreov1alpha1.Hook, error) {
	if h == nil {
		return nil, fmt.Errorf("hook cannot be nil")
	}

	s.logger.Debug("Updating hook", "namespace", namespaceName, "hook", h.Name)

	existing := &openchoreov1alpha1.Hook{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: h.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Hook not found", "namespace", namespaceName, "hook", h.Name)
			return nil, ErrHookNotFound
		}
		s.logger.Error("Failed to get hook", "error", err)
		return nil, fmt.Errorf("failed to get hook: %w", err)
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = h.Spec
	existing.Labels = h.Labels
	existing.Annotations = h.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update hook CR", "error", err)
		return nil, fmt.Errorf("failed to update hook: %w", err)
	}

	s.logger.Debug("Hook updated successfully", "namespace", namespaceName, "hook", h.Name)
	existing.TypeMeta = hookTypeMeta
	return existing, nil
}

func (s *hookService) ListHooks(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Hook], error) {
	s.logger.Debug("Listing hooks", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var hookList openchoreov1alpha1.HookList
	if err := s.k8sClient.List(ctx, &hookList, listOpts...); err != nil {
		s.logger.Error("Failed to list hooks", "error", err)
		return nil, fmt.Errorf("failed to list hooks: %w", err)
	}

	for i := range hookList.Items {
		hookList.Items[i].TypeMeta = hookTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.Hook]{
		Items:      hookList.Items,
		NextCursor: hookList.Continue,
	}
	if hookList.RemainingItemCount != nil {
		remaining := *hookList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed hooks", "namespace", namespaceName, "count", len(hookList.Items))
	return result, nil
}

func (s *hookService) GetHook(ctx context.Context, namespaceName, hookName string) (*openchoreov1alpha1.Hook, error) {
	s.logger.Debug("Getting hook", "namespace", namespaceName, "hook", hookName)

	h := &openchoreov1alpha1.Hook{}
	key := client.ObjectKey{
		Name:      hookName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, h); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Hook not found", "namespace", namespaceName, "hook", hookName)
			return nil, ErrHookNotFound
		}
		s.logger.Error("Failed to get hook", "error", err)
		return nil, fmt.Errorf("failed to get hook: %w", err)
	}

	h.TypeMeta = hookTypeMeta
	return h, nil
}

func (s *hookService) DeleteHook(ctx context.Context, namespaceName, hookName string) error {
	s.logger.Debug("Deleting hook", "namespace", namespaceName, "hook", hookName)

	h := &openchoreov1alpha1.Hook{}
	h.Name = hookName
	h.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, h); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrHookNotFound
		}
		s.logger.Error("Failed to delete hook CR", "error", err)
		return fmt.Errorf("failed to delete hook: %w", err)
	}

	s.logger.Debug("Hook deleted successfully", "namespace", namespaceName, "hook", hookName)
	return nil
}
