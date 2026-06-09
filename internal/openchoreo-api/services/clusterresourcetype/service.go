// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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

// clusterResourceTypeService handles cluster resource type business logic without authorization checks.
type clusterResourceTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterResourceTypeService)(nil)

var clusterResourceTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterResourceType",
}

// NewService creates a new cluster resource type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterResourceTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterResourceTypeService) CreateClusterResourceType(ctx context.Context, crt *openchoreov1alpha1.ClusterResourceType) (*openchoreov1alpha1.ClusterResourceType, error) {
	if crt == nil {
		return nil, fmt.Errorf("cluster resource type cannot be nil")
	}

	s.logger.Debug("Creating cluster resource type", "clusterResourceType", crt.Name)

	crt.Status = openchoreov1alpha1.ClusterResourceTypeStatus{}

	if err := s.k8sClient.Create(ctx, crt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster resource type already exists", "clusterResourceType", crt.Name)
			return nil, ErrClusterResourceTypeAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create cluster resource type CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster resource type: %w", err)
	}

	s.logger.Debug("Cluster resource type created successfully", "clusterResourceType", crt.Name)
	crt.TypeMeta = clusterResourceTypeTypeMeta
	return crt, nil
}

func (s *clusterResourceTypeService) UpdateClusterResourceType(ctx context.Context, crt *openchoreov1alpha1.ClusterResourceType) (*openchoreov1alpha1.ClusterResourceType, error) {
	if crt == nil {
		return nil, fmt.Errorf("cluster resource type cannot be nil")
	}

	s.logger.Debug("Updating cluster resource type", "clusterResourceType", crt.Name)

	existing := &openchoreov1alpha1.ClusterResourceType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: crt.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster resource type not found", "clusterResourceType", crt.Name)
			return nil, ErrClusterResourceTypeNotFound
		}
		s.logger.Error("Failed to get cluster resource type", "error", err)
		return nil, fmt.Errorf("failed to get cluster resource type: %w", err)
	}

	crt.Status = openchoreov1alpha1.ClusterResourceTypeStatus{}
	existing.Spec = crt.Spec
	existing.Labels = crt.Labels
	existing.Annotations = crt.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update cluster resource type CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster resource type: %w", err)
	}

	s.logger.Debug("Cluster resource type updated successfully", "clusterResourceType", crt.Name)
	existing.TypeMeta = clusterResourceTypeTypeMeta
	return existing, nil
}

func (s *clusterResourceTypeService) ListClusterResourceTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterResourceType], error) {
	s.logger.Debug("Listing cluster resource types", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}

	var crtList openchoreov1alpha1.ClusterResourceTypeList
	if err := s.k8sClient.List(ctx, &crtList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster resource types", "error", err)
		return nil, fmt.Errorf("failed to list cluster resource types: %w", err)
	}

	for i := range crtList.Items {
		crtList.Items[i].TypeMeta = clusterResourceTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterResourceType]{
		Items:      crtList.Items,
		NextCursor: crtList.Continue,
	}
	if crtList.RemainingItemCount != nil {
		remaining := *crtList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster resource types", "count", len(crtList.Items))
	return result, nil
}

func (s *clusterResourceTypeService) GetClusterResourceType(ctx context.Context, crtName string) (*openchoreov1alpha1.ClusterResourceType, error) {
	s.logger.Debug("Getting cluster resource type", "clusterResourceType", crtName)

	crt := &openchoreov1alpha1.ClusterResourceType{}
	key := client.ObjectKey{
		Name: crtName,
	}

	if err := s.k8sClient.Get(ctx, key, crt); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster resource type not found", "clusterResourceType", crtName)
			return nil, ErrClusterResourceTypeNotFound
		}
		s.logger.Error("Failed to get cluster resource type", "error", err)
		return nil, fmt.Errorf("failed to get cluster resource type: %w", err)
	}

	crt.TypeMeta = clusterResourceTypeTypeMeta
	return crt, nil
}

func (s *clusterResourceTypeService) DeleteClusterResourceType(ctx context.Context, crtName string) error {
	s.logger.Debug("Deleting cluster resource type", "clusterResourceType", crtName)

	crt := &openchoreov1alpha1.ClusterResourceType{}
	crt.Name = crtName

	if err := s.k8sClient.Delete(ctx, crt); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterResourceTypeNotFound
		}
		s.logger.Error("Failed to delete cluster resource type CR", "error", err)
		return fmt.Errorf("failed to delete cluster resource type: %w", err)
	}

	s.logger.Debug("Cluster resource type deleted successfully", "clusterResourceType", crtName)
	return nil
}

func (s *clusterResourceTypeService) GetClusterResourceTypeSchema(ctx context.Context, crtName string) (map[string]any, error) {
	s.logger.Debug("Getting cluster resource type schema", "clusterResourceType", crtName)

	crt, err := s.GetClusterResourceType(ctx, crtName)
	if err != nil {
		return nil, err
	}

	rawSchema, err := schema.SectionToRawJSONSchema(crt.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster resource type schema successfully", "clusterResourceType", crtName)
	return rawSchema, nil
}
