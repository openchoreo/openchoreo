// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

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

// componentReleaseService handles component release business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentReleaseService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var componentReleaseTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ComponentRelease",
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
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var crList openchoreov1alpha1.ComponentReleaseList
		if err := s.k8sClient.List(ctx, &crList, listOpts...); err != nil {
			s.logger.Error("Failed to list component releases", "error", err)
			return nil, fmt.Errorf("failed to list component releases: %w", err)
		}

		for i := range crList.Items {
			crList.Items[i].TypeMeta = componentReleaseTypeMeta
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

	cr.TypeMeta = componentReleaseTypeMeta
	return cr, nil
}

func (s *componentReleaseService) CreateComponentRelease(ctx context.Context, namespaceName string, cr *openchoreov1alpha1.ComponentRelease) (*openchoreov1alpha1.ComponentRelease, error) {
	if cr == nil {
		return nil, ErrComponentReleaseNil
	}

	s.logger.Debug("Creating component release", "namespace", namespaceName, "componentRelease", cr.Name)

	// Check if component release already exists
	existing := &openchoreov1alpha1.ComponentRelease{}
	key := client.ObjectKey{
		Name:      cr.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, key, existing); err == nil {
		return nil, ErrComponentReleaseAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to check component release existence: %w", err)
	}

	cr.Namespace = namespaceName
	cr.Status = openchoreov1alpha1.ComponentReleaseStatus{}

	if err := s.k8sClient.Create(ctx, cr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Component release already exists", "namespace", namespaceName, "componentRelease", cr.Name)
			return nil, ErrComponentReleaseAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create component release CR", "error", err)
		return nil, fmt.Errorf("failed to create component release: %w", err)
	}

	cr.TypeMeta = componentReleaseTypeMeta
	s.logger.Debug("Component release created successfully", "namespace", namespaceName, "componentRelease", cr.Name)
	return cr, nil
}

func (s *componentReleaseService) DeleteComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) error {
	s.logger.Debug("Deleting component release", "namespace", namespaceName, "componentRelease", componentReleaseName)

	cr := &openchoreov1alpha1.ComponentRelease{}
	cr.Name = componentReleaseName
	cr.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to delete component release CR", "error", err)
		return fmt.Errorf("failed to delete component release: %w", err)
	}

	s.logger.Debug("Component release deleted successfully", "namespace", namespaceName, "componentRelease", componentReleaseName)
	return nil
}
