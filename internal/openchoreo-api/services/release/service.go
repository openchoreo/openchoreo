// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// releaseService handles release business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type releaseService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*releaseService)(nil)

// NewService creates a new release service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &releaseService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *releaseService) ListReleases(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Release], error) {
	s.logger.Debug("Listing releases", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Release], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if pageOpts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(pageOpts.Limit)))
		}
		if pageOpts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(pageOpts.Cursor))
		}

		var rList openchoreov1alpha1.ReleaseList
		if err := s.k8sClient.List(ctx, &rList, listOpts...); err != nil {
			s.logger.Error("Failed to list releases", "error", err)
			return nil, fmt.Errorf("failed to list releases: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.Release]{
			Items:      rList.Items,
			NextCursor: rList.Continue,
		}
		if rList.RemainingItemCount != nil {
			remaining := *rList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	// Apply component filter if specified
	if componentName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(r openchoreov1alpha1.Release) bool {
				return r.Spec.Owner.ComponentName == componentName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *releaseService) GetRelease(ctx context.Context, namespaceName, releaseName string) (*openchoreov1alpha1.Release, error) {
	s.logger.Debug("Getting release", "namespace", namespaceName, "release", releaseName)

	r := &openchoreov1alpha1.Release{}
	key := client.ObjectKey{
		Name:      releaseName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, r); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Release not found", "namespace", namespaceName, "release", releaseName)
			return nil, ErrReleaseNotFound
		}
		s.logger.Error("Failed to get release", "error", err)
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	return r, nil
}
