// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

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

// clusterComponentTypeService handles cluster component type business logic without authorization checks.
type clusterComponentTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterComponentTypeService)(nil)

var clusterComponentTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterComponentType",
}

// NewService creates a new cluster component type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterComponentTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterComponentTypeService) CreateClusterComponentType(ctx context.Context, cct *openchoreov1alpha1.ClusterComponentType) (*openchoreov1alpha1.ClusterComponentType, error) {
	if cct == nil {
		return nil, fmt.Errorf("cluster component type cannot be nil")
	}

	s.logger.Debug("Creating cluster component type", "clusterComponentType", cct.Name)

	// Set defaults
	cct.Status = openchoreov1alpha1.ClusterComponentTypeStatus{}

	if err := s.k8sClient.Create(ctx, cct); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster component type already exists", "clusterComponentType", cct.Name)
			return nil, ErrClusterComponentTypeAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create cluster component type CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster component type: %w", err)
	}

	s.logger.Debug("Cluster component type created successfully", "clusterComponentType", cct.Name)
	cct.TypeMeta = clusterComponentTypeTypeMeta
	return cct, nil
}

func (s *clusterComponentTypeService) UpdateClusterComponentType(ctx context.Context, cct *openchoreov1alpha1.ClusterComponentType) (*openchoreov1alpha1.ClusterComponentType, error) {
	if cct == nil {
		return nil, fmt.Errorf("cluster component type cannot be nil")
	}

	s.logger.Debug("Updating cluster component type", "clusterComponentType", cct.Name)

	existing := &openchoreov1alpha1.ClusterComponentType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: cct.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster component type not found", "clusterComponentType", cct.Name)
			return nil, ErrClusterComponentTypeNotFound
		}
		s.logger.Error("Failed to get cluster component type", "error", err)
		return nil, fmt.Errorf("failed to get cluster component type: %w", err)
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	cct.Status = openchoreov1alpha1.ClusterComponentTypeStatus{}
	existing.Spec = cct.Spec
	existing.Labels = cct.Labels
	existing.Annotations = cct.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update cluster component type CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster component type: %w", err)
	}

	s.logger.Debug("Cluster component type updated successfully", "clusterComponentType", cct.Name)
	existing.TypeMeta = clusterComponentTypeTypeMeta
	return existing, nil
}

func (s *clusterComponentTypeService) ListClusterComponentTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterComponentType], error) {
	s.logger.Debug("Listing cluster component types", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}

	var cctList openchoreov1alpha1.ClusterComponentTypeList
	if err := s.k8sClient.List(ctx, &cctList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster component types", "error", err)
		return nil, fmt.Errorf("failed to list cluster component types: %w", err)
	}

	for i := range cctList.Items {
		cctList.Items[i].TypeMeta = clusterComponentTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterComponentType]{
		Items:      cctList.Items,
		NextCursor: cctList.Continue,
	}
	if cctList.RemainingItemCount != nil {
		remaining := *cctList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster component types", "count", len(cctList.Items))
	return result, nil
}

func (s *clusterComponentTypeService) GetClusterComponentType(ctx context.Context, cctName string) (*openchoreov1alpha1.ClusterComponentType, error) {
	s.logger.Debug("Getting cluster component type", "clusterComponentType", cctName)

	cct := &openchoreov1alpha1.ClusterComponentType{}
	key := client.ObjectKey{
		Name: cctName,
	}

	if err := s.k8sClient.Get(ctx, key, cct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster component type not found", "clusterComponentType", cctName)
			return nil, ErrClusterComponentTypeNotFound
		}
		s.logger.Error("Failed to get cluster component type", "error", err)
		return nil, fmt.Errorf("failed to get cluster component type: %w", err)
	}

	cct.TypeMeta = clusterComponentTypeTypeMeta
	return cct, nil
}

// DeleteClusterComponentType removes a cluster-scoped component type by name.
func (s *clusterComponentTypeService) DeleteClusterComponentType(ctx context.Context, cctName string) error {
	s.logger.Debug("Deleting cluster component type", "clusterComponentType", cctName)

	cct := &openchoreov1alpha1.ClusterComponentType{}
	cct.Name = cctName

	if err := s.k8sClient.Delete(ctx, cct); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterComponentTypeNotFound
		}
		s.logger.Error("Failed to delete cluster component type CR", "error", err)
		return fmt.Errorf("failed to delete cluster component type: %w", err)
	}

	s.logger.Debug("Cluster component type deleted successfully", "clusterComponentType", cctName)
	return nil
}

func (s *clusterComponentTypeService) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (map[string]any, error) {
	s.logger.Debug("Getting cluster component type schema", "clusterComponentType", cctName)

	cct, err := s.GetClusterComponentType(ctx, cctName)
	if err != nil {
		return nil, err
	}

	// Convert to raw JSON Schema map, preserving vendor extensions (x-*) for frontend consumers
	rawSchema, err := schema.SectionToRawJSONSchema(cct.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster component type schema successfully", "clusterComponentType", cctName)
	return rawSchema, nil
}
