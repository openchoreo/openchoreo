// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// clusterProjectTypeService handles cluster project type business logic without authorization checks.
type clusterProjectTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterProjectTypeService)(nil)

var clusterProjectTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterProjectType",
}

// NewService creates a new cluster project type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterProjectTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterProjectTypeService) CreateClusterProjectType(ctx context.Context, cpt *openchoreov1alpha1.ClusterProjectType) (*openchoreov1alpha1.ClusterProjectType, error) {
	if cpt == nil {
		return nil, fmt.Errorf("cluster project type cannot be nil")
	}

	s.logger.Debug("Creating cluster project type", "clusterProjectType", cpt.Name)

	cpt.Status = openchoreov1alpha1.ClusterProjectTypeStatus{}

	if err := s.k8sClient.Create(ctx, cpt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster project type already exists", "clusterProjectType", cpt.Name)
			return nil, ErrClusterProjectTypeAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create cluster project type CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster project type: %w", err)
	}

	s.logger.Debug("Cluster project type created successfully", "clusterProjectType", cpt.Name)
	cpt.TypeMeta = clusterProjectTypeTypeMeta
	return cpt, nil
}

func (s *clusterProjectTypeService) UpdateClusterProjectType(ctx context.Context, cpt *openchoreov1alpha1.ClusterProjectType) (*openchoreov1alpha1.ClusterProjectType, error) {
	if cpt == nil {
		return nil, fmt.Errorf("cluster project type cannot be nil")
	}

	s.logger.Debug("Updating cluster project type", "clusterProjectType", cpt.Name)

	existing := &openchoreov1alpha1.ClusterProjectType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: cpt.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster project type not found", "clusterProjectType", cpt.Name)
			return nil, ErrClusterProjectTypeNotFound
		}
		s.logger.Error("Failed to get cluster project type", "error", err)
		return nil, fmt.Errorf("failed to get cluster project type: %w", err)
	}

	cpt.Status = openchoreov1alpha1.ClusterProjectTypeStatus{}
	existing.Spec = cpt.Spec
	existing.Labels = cpt.Labels
	existing.Annotations = cpt.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update cluster project type CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster project type: %w", err)
	}

	s.logger.Debug("Cluster project type updated successfully", "clusterProjectType", cpt.Name)
	existing.TypeMeta = clusterProjectTypeTypeMeta
	return existing, nil
}

func (s *clusterProjectTypeService) ListClusterProjectTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterProjectType], error) {
	s.logger.Debug("Listing cluster project types", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}

	var cptList openchoreov1alpha1.ClusterProjectTypeList
	if err := s.k8sClient.List(ctx, &cptList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster project types", "error", err)
		return nil, fmt.Errorf("failed to list cluster project types: %w", err)
	}

	for i := range cptList.Items {
		cptList.Items[i].TypeMeta = clusterProjectTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterProjectType]{
		Items:      cptList.Items,
		NextCursor: cptList.Continue,
	}
	if cptList.RemainingItemCount != nil {
		remaining := *cptList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster project types", "count", len(cptList.Items))
	return result, nil
}

func (s *clusterProjectTypeService) GetClusterProjectType(ctx context.Context, cptName string) (*openchoreov1alpha1.ClusterProjectType, error) {
	s.logger.Debug("Getting cluster project type", "clusterProjectType", cptName)

	cpt := &openchoreov1alpha1.ClusterProjectType{}
	key := client.ObjectKey{
		Name: cptName,
	}

	if err := s.k8sClient.Get(ctx, key, cpt); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster project type not found", "clusterProjectType", cptName)
			return nil, ErrClusterProjectTypeNotFound
		}
		s.logger.Error("Failed to get cluster project type", "error", err)
		return nil, fmt.Errorf("failed to get cluster project type: %w", err)
	}

	cpt.TypeMeta = clusterProjectTypeTypeMeta
	return cpt, nil
}

func (s *clusterProjectTypeService) DeleteClusterProjectType(ctx context.Context, cptName string) error {
	s.logger.Debug("Deleting cluster project type", "clusterProjectType", cptName)

	cpt := &openchoreov1alpha1.ClusterProjectType{}
	cpt.Name = cptName

	if err := s.k8sClient.Delete(ctx, cpt); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterProjectTypeNotFound
		}
		s.logger.Error("Failed to delete cluster project type CR", "error", err)
		return fmt.Errorf("failed to delete cluster project type: %w", err)
	}

	s.logger.Debug("Cluster project type deleted successfully", "clusterProjectType", cptName)
	return nil
}

func (s *clusterProjectTypeService) GetClusterProjectTypeSchema(ctx context.Context, cptName string) (map[string]any, error) {
	s.logger.Debug("Getting cluster project type schema", "clusterProjectType", cptName)

	cpt, err := s.GetClusterProjectType(ctx, cptName)
	if err != nil {
		return nil, err
	}

	rawSchema, err := schema.SectionToRawJSONSchema(cpt.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster project type schema successfully", "clusterProjectType", cptName)
	return rawSchema, nil
}
