// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// ClusterComponentTypeService handles ClusterComponentType-related business logic
type ClusterComponentTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewClusterComponentTypeService creates a new ClusterComponentType service
func NewClusterComponentTypeService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ClusterComponentTypeService {
	return &ClusterComponentTypeService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListClusterComponentTypes lists all ClusterComponentTypes
func (s *ClusterComponentTypeService) ListClusterComponentTypes(ctx context.Context) ([]*models.ComponentTypeResponse, error) {
	s.logger.Debug("Listing ClusterComponentTypes")

	var ctList openchoreov1alpha1.ClusterComponentTypeList
	if err := s.k8sClient.List(ctx, &ctList); err != nil {
		s.logger.Error("Failed to list ClusterComponentTypes", "error", err)
		return nil, fmt.Errorf("failed to list ClusterComponentTypes: %w", err)
	}

	cts := make([]*models.ComponentTypeResponse, 0, len(ctList.Items))
	for i := range ctList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterComponentType, ResourceTypeClusterComponentType, ctList.Items[i].Name,
			authz.ResourceHierarchy{}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized cluster component type", "clusterComponentType", ctList.Items[i].Name)
				continue
			}
			return nil, err
		}
		cts = append(cts, s.toComponentTypeResponse(&ctList.Items[i]))
	}

	s.logger.Debug("Listed ClusterComponentTypes", "count", len(cts))
	return cts, nil
}

// GetClusterComponentType retrieves a specific ClusterComponentType
func (s *ClusterComponentTypeService) GetClusterComponentType(ctx context.Context, name string) (*models.ComponentTypeResponse, error) {
	s.logger.Debug("Getting ClusterComponentType", "name", name)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterComponentType, ResourceTypeClusterComponentType, name,
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	ct := &openchoreov1alpha1.ClusterComponentType{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ClusterComponentType not found", "name", name)
			return nil, ErrClusterComponentTypeNotFound
		}
		s.logger.Error("Failed to get ClusterComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ClusterComponentType: %w", err)
	}

	return s.toComponentTypeResponse(ct), nil
}

// GetClusterComponentTypeSchema retrieves the JSON schema for a ClusterComponentType
func (s *ClusterComponentTypeService) GetClusterComponentTypeSchema(ctx context.Context, name string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ClusterComponentType schema", "name", name)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterComponentType, ResourceTypeClusterComponentType, name,
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	ct := &openchoreov1alpha1.ClusterComponentType{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ClusterComponentType not found", "name", name)
			return nil, ErrClusterComponentTypeNotFound
		}
		s.logger.Error("Failed to get ClusterComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ClusterComponentType: %w", err)
	}

	// Extract types from RawExtension
	var types map[string]any
	if ct.Spec.Schema.Types != nil && ct.Spec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(ct.Spec.Schema.Types.Raw, &types); err != nil {
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
	if ct.Spec.Schema.Parameters != nil && ct.Spec.Schema.Parameters.Raw != nil {
		var params map[string]any
		if err := yaml.Unmarshal(ct.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved ClusterComponentType schema successfully", "name", name)
	return jsonSchema, nil
}

// toComponentTypeResponse converts a ClusterComponentType CR to a ComponentTypeResponse
func (s *ClusterComponentTypeService) toComponentTypeResponse(ct *openchoreov1alpha1.ClusterComponentType) *models.ComponentTypeResponse {
	displayName := ct.Annotations[controller.AnnotationKeyDisplayName]
	description := ct.Annotations[controller.AnnotationKeyDescription]

	allowedWorkflows := make([]string, 0, len(ct.Spec.AllowedWorkflows))
	allowedWorkflows = append(allowedWorkflows, ct.Spec.AllowedWorkflows...)

	allowedTraits := make([]models.AllowedTraitResponse, 0, len(ct.Spec.AllowedTraits))
	for _, ref := range ct.Spec.AllowedTraits {
		allowedTraits = append(allowedTraits, models.AllowedTraitResponse{
			Kind: string(ref.Kind),
			Name: ref.Name,
		})
	}

	return &models.ComponentTypeResponse{
		Name:             ct.Name,
		DisplayName:      displayName,
		Description:      description,
		WorkloadType:     ct.Spec.WorkloadType,
		AllowedWorkflows: allowedWorkflows,
		AllowedTraits:    allowedTraits,
		CreatedAt:        ct.CreationTimestamp.Time,
	}
}
