// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

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

// resourceReleaseService handles resource release business logic without authorization checks.
type resourceReleaseService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var resourceReleaseTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ResourceRelease",
}

var _ Service = (*resourceReleaseService)(nil)

// NewService creates a new resource release service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &resourceReleaseService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *resourceReleaseService) ListResourceReleases(ctx context.Context, namespaceName, resourceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceRelease], error) {
	s.logger.Debug("Listing resource releases", "namespace", namespaceName, "resource", resourceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceRelease], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var rrList openchoreov1alpha1.ResourceReleaseList
		if err := s.k8sClient.List(ctx, &rrList, listOpts...); err != nil {
			s.logger.Error("Failed to list resource releases", "error", err)
			return nil, fmt.Errorf("failed to list resource releases: %w", err)
		}

		for i := range rrList.Items {
			rrList.Items[i].TypeMeta = resourceReleaseTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.ResourceRelease]{
			Items:      rrList.Items,
			NextCursor: rrList.Continue,
		}
		if rrList.RemainingItemCount != nil {
			remaining := *rrList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	if resourceName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(rr openchoreov1alpha1.ResourceRelease) bool {
				return rr.Spec.Owner.ResourceName == resourceName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *resourceReleaseService) GetResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) (*openchoreov1alpha1.ResourceRelease, error) {
	s.logger.Debug("Getting resource release", "namespace", namespaceName, "resourceRelease", resourceReleaseName)

	rr := &openchoreov1alpha1.ResourceRelease{}
	key := client.ObjectKey{
		Name:      resourceReleaseName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource release not found", "namespace", namespaceName, "resourceRelease", resourceReleaseName)
			return nil, ErrResourceReleaseNotFound
		}
		s.logger.Error("Failed to get resource release", "error", err)
		return nil, fmt.Errorf("failed to get resource release: %w", err)
	}

	rr.TypeMeta = resourceReleaseTypeMeta
	return rr, nil
}

func (s *resourceReleaseService) CreateResourceRelease(ctx context.Context, namespaceName string, rr *openchoreov1alpha1.ResourceRelease) (*openchoreov1alpha1.ResourceRelease, error) {
	if rr == nil {
		return nil, ErrResourceReleaseNil
	}

	s.logger.Debug("Creating resource release", "namespace", namespaceName, "resourceRelease", rr.Name)

	existing := &openchoreov1alpha1.ResourceRelease{}
	key := client.ObjectKey{
		Name:      rr.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, key, existing); err == nil {
		return nil, ErrResourceReleaseAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to check resource release existence: %w", err)
	}

	rr.Namespace = namespaceName
	rr.Status = openchoreov1alpha1.ResourceReleaseStatus{}

	if err := s.k8sClient.Create(ctx, rr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Resource release already exists", "namespace", namespaceName, "resourceRelease", rr.Name)
			return nil, ErrResourceReleaseAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create resource release CR", "error", err)
		return nil, fmt.Errorf("failed to create resource release: %w", err)
	}

	rr.TypeMeta = resourceReleaseTypeMeta
	s.logger.Debug("Resource release created successfully", "namespace", namespaceName, "resourceRelease", rr.Name)
	return rr, nil
}

func (s *resourceReleaseService) DeleteResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) error {
	s.logger.Debug("Deleting resource release", "namespace", namespaceName, "resourceRelease", resourceReleaseName)

	rr := &openchoreov1alpha1.ResourceRelease{}
	rr.Name = resourceReleaseName
	rr.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrResourceReleaseNotFound
		}
		s.logger.Error("Failed to delete resource release CR", "error", err)
		return fmt.Errorf("failed to delete resource release: %w", err)
	}

	s.logger.Debug("Resource release deleted successfully", "namespace", namespaceName, "resourceRelease", resourceReleaseName)
	return nil
}
