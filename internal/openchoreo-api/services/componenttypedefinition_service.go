// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/exp/slog"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// ComponentTypeDefinitionService handles ComponentTypeDefinition-related business logic
type ComponentTypeDefinitionService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewComponentTypeDefinitionService creates a new ComponentTypeDefinition service
func NewComponentTypeDefinitionService(k8sClient client.Client, logger *slog.Logger) *ComponentTypeDefinitionService {
	return &ComponentTypeDefinitionService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListComponentTypeDefinitions lists all ComponentTypeDefinitions in the given organization
func (s *ComponentTypeDefinitionService) ListComponentTypeDefinitions(ctx context.Context, orgName string) ([]*models.ComponentTypeDefinitionResponse, error) {
	s.logger.Debug("Listing ComponentTypeDefinitions", "org", orgName)

	var ctdList openchoreov1alpha1.ComponentTypeDefinitionList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &ctdList, listOpts...); err != nil {
		s.logger.Error("Failed to list ComponentTypeDefinitions", "error", err)
		return nil, fmt.Errorf("failed to list ComponentTypeDefinitions: %w", err)
	}

	ctds := make([]*models.ComponentTypeDefinitionResponse, 0, len(ctdList.Items))
	for i := range ctdList.Items {
		ctds = append(ctds, s.toComponentTypeDefinitionResponse(&ctdList.Items[i]))
	}

	s.logger.Debug("Listed ComponentTypeDefinitions", "org", orgName, "count", len(ctds))
	return ctds, nil
}

// GetComponentTypeDefinition retrieves a specific ComponentTypeDefinition
func (s *ComponentTypeDefinitionService) GetComponentTypeDefinition(ctx context.Context, orgName, ctdName string) (*models.ComponentTypeDefinitionResponse, error) {
	s.logger.Debug("Getting ComponentTypeDefinition", "org", orgName, "name", ctdName)

	ctd := &openchoreov1alpha1.ComponentTypeDefinition{}
	key := client.ObjectKey{
		Name:      ctdName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, ctd); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentTypeDefinition not found", "org", orgName, "name", ctdName)
			return nil, ErrComponentTypeDefinitionNotFound
		}
		s.logger.Error("Failed to get ComponentTypeDefinition", "error", err)
		return nil, fmt.Errorf("failed to get ComponentTypeDefinition: %w", err)
	}

	return s.toComponentTypeDefinitionResponse(ctd), nil
}

// GetComponentTypeDefinitionSchema retrieves the JSON schema for a ComponentTypeDefinition
func (s *ComponentTypeDefinitionService) GetComponentTypeDefinitionSchema(ctx context.Context, orgName, ctdName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ComponentTypeDefinition schema", "org", orgName, "name", ctdName)

	// First get the CTD
	ctd := &openchoreov1alpha1.ComponentTypeDefinition{}
	key := client.ObjectKey{
		Name:      ctdName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, ctd); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentTypeDefinition not found", "org", orgName, "name", ctdName)
			return nil, ErrComponentTypeDefinitionNotFound
		}
		s.logger.Error("Failed to get ComponentTypeDefinition", "error", err)
		return nil, fmt.Errorf("failed to get ComponentTypeDefinition: %w", err)
	}

	// Extract types from RawExtension
	var types map[string]any
	if ctd.Spec.Schema.Types != nil && ctd.Spec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(ctd.Spec.Schema.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Build schema definition
	def := schema.Definition{
		Types: types,
	}

	// Extract parameters schema from RawExtension
	if ctd.Spec.Schema.Parameters != nil && ctd.Spec.Schema.Parameters.Raw != nil {
		var params map[string]any
		if err := json.Unmarshal(ctd.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved ComponentTypeDefinition schema successfully", "org", orgName, "name", ctdName)
	return jsonSchema, nil
}

// toComponentTypeDefinitionResponse converts a ComponentTypeDefinition CR to a ComponentTypeDefinitionResponse
func (s *ComponentTypeDefinitionService) toComponentTypeDefinitionResponse(ctd *openchoreov1alpha1.ComponentTypeDefinition) *models.ComponentTypeDefinitionResponse {
	// Extract display name and description from annotations
	displayName := ctd.Annotations[controller.AnnotationKeyDisplayName]
	description := ctd.Annotations[controller.AnnotationKeyDescription]

	return &models.ComponentTypeDefinitionResponse{
		Name:         ctd.Name,
		DisplayName:  displayName,
		Description:  description,
		WorkloadType: ctd.Spec.WorkloadType,
		CreatedAt:    ctd.CreationTimestamp.Time,
	}
}
