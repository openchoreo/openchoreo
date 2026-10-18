// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

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

// clusterEndpointTypeService handles cluster endpoint type business logic without authorization checks.
type clusterEndpointTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterEndpointTypeService)(nil)

var clusterEndpointTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterEndpointType",
}

// NewService creates a new cluster endpoint type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterEndpointTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterEndpointTypeService) CreateClusterEndpointType(ctx context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error) {
	if crt == nil {
		return nil, fmt.Errorf("cluster endpoint type cannot be nil")
	}

	s.logger.Debug("Creating cluster endpoint type", "clusterEndpointType", crt.Name)

	crt.Status = openchoreov1alpha1.ClusterEndpointTypeStatus{}

	if err := s.k8sClient.Create(ctx, crt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster endpoint type already exists", "clusterEndpointType", crt.Name)
			return nil, ErrClusterEndpointTypeAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create cluster endpoint type CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster endpoint type: %w", err)
	}

	s.logger.Debug("Cluster endpoint type created successfully", "clusterEndpointType", crt.Name)
	crt.TypeMeta = clusterEndpointTypeTypeMeta
	return crt, nil
}

func (s *clusterEndpointTypeService) UpdateClusterEndpointType(ctx context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error) {
	if crt == nil {
		return nil, fmt.Errorf("cluster endpoint type cannot be nil")
	}

	s.logger.Debug("Updating cluster endpoint type", "clusterEndpointType", crt.Name)

	existing := &openchoreov1alpha1.ClusterEndpointType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: crt.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster endpoint type not found", "clusterEndpointType", crt.Name)
			return nil, ErrClusterEndpointTypeNotFound
		}
		s.logger.Error("Failed to get cluster endpoint type", "error", err)
		return nil, fmt.Errorf("failed to get cluster endpoint type: %w", err)
	}

	crt.Status = openchoreov1alpha1.ClusterEndpointTypeStatus{}
	existing.Spec = crt.Spec
	existing.Labels = crt.Labels
	existing.Annotations = crt.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update cluster endpoint type CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster endpoint type: %w", err)
	}

	s.logger.Debug("Cluster endpoint type updated successfully", "clusterEndpointType", crt.Name)
	existing.TypeMeta = clusterEndpointTypeTypeMeta
	return existing, nil
}

func (s *clusterEndpointTypeService) ListClusterEndpointTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterEndpointType], error) {
	s.logger.Debug("Listing cluster endpoint types", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}

	var crtList openchoreov1alpha1.ClusterEndpointTypeList
	if err := s.k8sClient.List(ctx, &crtList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster endpoint types", "error", err)
		return nil, fmt.Errorf("failed to list cluster endpoint types: %w", err)
	}

	for i := range crtList.Items {
		crtList.Items[i].TypeMeta = clusterEndpointTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterEndpointType]{
		Items:      crtList.Items,
		NextCursor: crtList.Continue,
	}
	if crtList.RemainingItemCount != nil {
		remaining := *crtList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster endpoint types", "count", len(crtList.Items))
	return result, nil
}

func (s *clusterEndpointTypeService) GetClusterEndpointType(ctx context.Context, cetName string) (*openchoreov1alpha1.ClusterEndpointType, error) {
	s.logger.Debug("Getting cluster endpoint type", "clusterEndpointType", cetName)

	crt := &openchoreov1alpha1.ClusterEndpointType{}
	key := client.ObjectKey{
		Name: cetName,
	}

	if err := s.k8sClient.Get(ctx, key, crt); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster endpoint type not found", "clusterEndpointType", cetName)
			return nil, ErrClusterEndpointTypeNotFound
		}
		s.logger.Error("Failed to get cluster endpoint type", "error", err)
		return nil, fmt.Errorf("failed to get cluster endpoint type: %w", err)
	}

	crt.TypeMeta = clusterEndpointTypeTypeMeta
	return crt, nil
}

func (s *clusterEndpointTypeService) DeleteClusterEndpointType(ctx context.Context, cetName string) error {
	s.logger.Debug("Deleting cluster endpoint type", "clusterEndpointType", cetName)

	crt := &openchoreov1alpha1.ClusterEndpointType{}
	crt.Name = cetName

	if err := s.k8sClient.Delete(ctx, crt); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterEndpointTypeNotFound
		}
		s.logger.Error("Failed to delete cluster endpoint type CR", "error", err)
		return fmt.Errorf("failed to delete cluster endpoint type: %w", err)
	}

	s.logger.Debug("Cluster endpoint type deleted successfully", "clusterEndpointType", cetName)
	return nil
}

func (s *clusterEndpointTypeService) GetClusterEndpointTypeSchema(ctx context.Context, cetName string) (map[string]any, error) {
	s.logger.Debug("Getting cluster endpoint type schema", "clusterEndpointType", cetName)

	crt, err := s.GetClusterEndpointType(ctx, cetName)
	if err != nil {
		return nil, err
	}

	rawSchema, err := schema.SectionToRawJSONSchema(crt.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster endpoint type schema successfully", "clusterEndpointType", cetName)
	return rawSchema, nil
}
