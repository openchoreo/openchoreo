// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"context"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// clusterComponentTypeService handles cluster component type business logic without authorization checks.
type clusterComponentTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterComponentTypeService)(nil)

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
	cct.TypeMeta = metav1.TypeMeta{
		Kind:       "ClusterComponentType",
		APIVersion: "openchoreo.dev/v1alpha1",
	}

	if err := s.k8sClient.Create(ctx, cct); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster component type already exists", "clusterComponentType", cct.Name)
			return nil, ErrClusterComponentTypeAlreadyExists
		}
		s.logger.Error("Failed to create cluster component type CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster component type: %w", err)
	}

	s.logger.Debug("Cluster component type created successfully", "clusterComponentType", cct.Name)
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

	// Apply incoming spec directly from the request body, preserving server-managed fields
	cct.ResourceVersion = existing.ResourceVersion

	if err := s.k8sClient.Update(ctx, cct); err != nil {
		s.logger.Error("Failed to update cluster component type CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster component type: %w", err)
	}

	s.logger.Debug("Cluster component type updated successfully", "clusterComponentType", cct.Name)
	return cct, nil
}

func (s *clusterComponentTypeService) ListClusterComponentTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterComponentType], error) {
	s.logger.Debug("Listing cluster component types", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var cctList openchoreov1alpha1.ClusterComponentTypeList
	if err := s.k8sClient.List(ctx, &cctList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster component types", "error", err)
		return nil, fmt.Errorf("failed to list cluster component types: %w", err)
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

func (s *clusterComponentTypeService) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting cluster component type schema", "clusterComponentType", cctName)

	cct, err := s.GetClusterComponentType(ctx, cctName)
	if err != nil {
		return nil, err
	}

	// Extract types from RawExtension
	var types map[string]any
	if cct.Spec.Schema.Types != nil && cct.Spec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(cct.Spec.Schema.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Build schema definition
	def := schema.Definition{
		Types: types,
		Options: extractor.Options{
			SkipDefaultValidation: true,
		},
	}

	// Extract parameters schema from RawExtension
	if cct.Spec.Schema.Parameters != nil && cct.Spec.Schema.Parameters.Raw != nil {
		var params map[string]any
		if err := yaml.Unmarshal(cct.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster component type schema successfully", "clusterComponentType", cctName)
	return jsonSchema, nil
}
