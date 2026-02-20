// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// componentReleaseService handles component release business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentReleaseService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*componentReleaseService)(nil)

// NewService creates a new component release service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &componentReleaseService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *componentReleaseService) ListComponentReleases(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentRelease], error) {
	s.logger.Debug("Listing component releases", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentRelease], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if pageOpts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(pageOpts.Limit)))
		}
		if pageOpts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(pageOpts.Cursor))
		}

		var crList openchoreov1alpha1.ComponentReleaseList
		if err := s.k8sClient.List(ctx, &crList, listOpts...); err != nil {
			s.logger.Error("Failed to list component releases", "error", err)
			return nil, fmt.Errorf("failed to list component releases: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.ComponentRelease]{
			Items:      crList.Items,
			NextCursor: crList.Continue,
		}
		if crList.RemainingItemCount != nil {
			remaining := *crList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	// Apply component filter if specified
	if componentName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(cr openchoreov1alpha1.ComponentRelease) bool {
				return cr.Spec.Owner.ComponentName == componentName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *componentReleaseService) GetComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) (*openchoreov1alpha1.ComponentRelease, error) {
	s.logger.Debug("Getting component release", "namespace", namespaceName, "componentRelease", componentReleaseName)

	cr := &openchoreov1alpha1.ComponentRelease{}
	key := client.ObjectKey{
		Name:      componentReleaseName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, cr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "namespace", namespaceName, "componentRelease", componentReleaseName)
			return nil, ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to get component release", "error", err)
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	return cr, nil
}
