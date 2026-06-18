// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

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

// projectReleaseService handles project release business logic without authorization checks.
type projectReleaseService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var projectReleaseTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ProjectRelease",
}

var _ Service = (*projectReleaseService)(nil)

// NewService creates a new project release service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &projectReleaseService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *projectReleaseService) ListProjectReleases(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectRelease], error) {
	s.logger.Debug("Listing project releases", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectRelease], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var prList openchoreov1alpha1.ProjectReleaseList
		if err := s.k8sClient.List(ctx, &prList, listOpts...); err != nil {
			s.logger.Error("Failed to list project releases", "error", err)
			return nil, fmt.Errorf("failed to list project releases: %w", err)
		}

		for i := range prList.Items {
			prList.Items[i].TypeMeta = projectReleaseTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.ProjectRelease]{
			Items:      prList.Items,
			NextCursor: prList.Continue,
		}
		if prList.RemainingItemCount != nil {
			remaining := *prList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	if projectName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(pr openchoreov1alpha1.ProjectRelease) bool {
				return pr.Spec.Owner.ProjectName == projectName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *projectReleaseService) GetProjectRelease(ctx context.Context, namespaceName, projectReleaseName string) (*openchoreov1alpha1.ProjectRelease, error) {
	s.logger.Debug("Getting project release", "namespace", namespaceName, "projectRelease", projectReleaseName)

	pr := &openchoreov1alpha1.ProjectRelease{}
	key := client.ObjectKey{
		Name:      projectReleaseName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, pr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project release not found", "namespace", namespaceName, "projectRelease", projectReleaseName)
			return nil, ErrProjectReleaseNotFound
		}
		s.logger.Error("Failed to get project release", "error", err)
		return nil, fmt.Errorf("failed to get project release: %w", err)
	}

	pr.TypeMeta = projectReleaseTypeMeta
	return pr, nil
}

func (s *projectReleaseService) CreateProjectRelease(ctx context.Context, namespaceName string, pr *openchoreov1alpha1.ProjectRelease) (*openchoreov1alpha1.ProjectRelease, error) {
	if pr == nil {
		return nil, ErrProjectReleaseNil
	}

	s.logger.Debug("Creating project release", "namespace", namespaceName, "projectRelease", pr.Name)

	existing := &openchoreov1alpha1.ProjectRelease{}
	key := client.ObjectKey{
		Name:      pr.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, key, existing); err == nil {
		return nil, ErrProjectReleaseAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to check project release existence: %w", err)
	}

	pr.Namespace = namespaceName
	pr.Status = openchoreov1alpha1.ProjectReleaseStatus{}

	if err := s.k8sClient.Create(ctx, pr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Project release already exists", "namespace", namespaceName, "projectRelease", pr.Name)
			return nil, ErrProjectReleaseAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create project release CR", "error", err)
		return nil, fmt.Errorf("failed to create project release: %w", err)
	}

	pr.TypeMeta = projectReleaseTypeMeta
	s.logger.Debug("Project release created successfully", "namespace", namespaceName, "projectRelease", pr.Name)
	return pr, nil
}

func (s *projectReleaseService) DeleteProjectRelease(ctx context.Context, namespaceName, projectReleaseName string) error {
	s.logger.Debug("Deleting project release", "namespace", namespaceName, "projectRelease", projectReleaseName)

	pr := &openchoreov1alpha1.ProjectRelease{}
	pr.Name = projectReleaseName
	pr.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, pr); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrProjectReleaseNotFound
		}
		s.logger.Error("Failed to delete project release CR", "error", err)
		return fmt.Errorf("failed to delete project release: %w", err)
	}

	s.logger.Debug("Project release deleted successfully", "namespace", namespaceName, "projectRelease", projectReleaseName)
	return nil
}
