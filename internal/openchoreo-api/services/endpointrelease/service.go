// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

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

// endpointReleaseService handles endpoint release business logic without authorization checks.
type endpointReleaseService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var endpointReleaseTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "EndpointRelease",
}

var _ Service = (*endpointReleaseService)(nil)

// NewService creates a new endpoint release service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &endpointReleaseService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *endpointReleaseService) ListEndpointReleases(ctx context.Context, namespaceName, endpointName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointRelease], error) {
	s.logger.Debug("Listing endpoint releases", "namespace", namespaceName, "endpoint", endpointName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointRelease], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var rrList openchoreov1alpha1.EndpointReleaseList
		if err := s.k8sClient.List(ctx, &rrList, listOpts...); err != nil {
			s.logger.Error("Failed to list endpoint releases", "error", err)
			return nil, fmt.Errorf("failed to list endpoint releases: %w", err)
		}

		for i := range rrList.Items {
			rrList.Items[i].TypeMeta = endpointReleaseTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.EndpointRelease]{
			Items:      rrList.Items,
			NextCursor: rrList.Continue,
		}
		if rrList.RemainingItemCount != nil {
			remaining := *rrList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	if endpointName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(rr openchoreov1alpha1.EndpointRelease) bool {
				return rr.Spec.Owner.EndpointName == endpointName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *endpointReleaseService) GetEndpointRelease(ctx context.Context, namespaceName, endpointReleaseName string) (*openchoreov1alpha1.EndpointRelease, error) {
	s.logger.Debug("Getting endpoint release", "namespace", namespaceName, "endpointRelease", endpointReleaseName)

	rr := &openchoreov1alpha1.EndpointRelease{}
	key := client.ObjectKey{
		Name:      endpointReleaseName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint release not found", "namespace", namespaceName, "endpointRelease", endpointReleaseName)
			return nil, ErrEndpointReleaseNotFound
		}
		s.logger.Error("Failed to get endpoint release", "error", err)
		return nil, fmt.Errorf("failed to get endpoint release: %w", err)
	}

	rr.TypeMeta = endpointReleaseTypeMeta
	return rr, nil
}

func (s *endpointReleaseService) DeleteEndpointRelease(ctx context.Context, namespaceName, endpointReleaseName string) error {
	s.logger.Debug("Deleting endpoint release", "namespace", namespaceName, "endpointRelease", endpointReleaseName)

	rr := &openchoreov1alpha1.EndpointRelease{}
	rr.Name = endpointReleaseName
	rr.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrEndpointReleaseNotFound
		}
		s.logger.Error("Failed to delete endpoint release CR", "error", err)
		return fmt.Errorf("failed to delete endpoint release: %w", err)
	}

	s.logger.Debug("Endpoint release deleted successfully", "namespace", namespaceName, "endpointRelease", endpointReleaseName)
	return nil
}
