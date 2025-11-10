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

// ComponentTypeService handles ComponentType-related business logic
type ComponentTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewComponentTypeService creates a new ComponentType service
func NewComponentTypeService(k8sClient client.Client, logger *slog.Logger) *ComponentTypeService {
	return &ComponentTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListComponentTypes lists all ComponentTypes in the given organization
func (s *ComponentTypeService) ListComponentTypes(ctx context.Context, orgName string) ([]*models.ComponentTypeResponse, error) {
	s.logger.Debug("Listing ComponentTypes", "org", orgName)

	var ctdList openchoreov1alpha1.ComponentTypeDefinitionList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &ctdList, listOpts...); err != nil {
		s.logger.Error("Failed to list ComponentTypes", "error", err)
		return nil, fmt.Errorf("failed to list ComponentTypes: %w", err)
	}

	ctds := make([]*models.ComponentTypeResponse, 0, len(ctdList.Items))
	for i := range ctdList.Items {
		ctds = append(ctds, s.toComponentTypeResponse(&ctdList.Items[i]))
	}

	s.logger.Debug("Listed ComponentTypes", "org", orgName, "count", len(ctds))
	return ctds, nil
}

// GetComponentType retrieves a specific ComponentType
func (s *ComponentTypeService) GetComponentType(ctx context.Context, orgName, ctdName string) (*models.ComponentTypeResponse, error) {
	s.logger.Debug("Getting ComponentType", "org", orgName, "name", ctdName)

	ctd := &openchoreov1alpha1.ComponentTypeDefinition{}
	key := client.ObjectKey{
		Name:      ctdName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, ctd); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "org", orgName, "name", ctdName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
	}

	return s.toComponentTypeResponse(ctd), nil
}

// GetComponentTypeSchema retrieves the JSON schema for a ComponentType
func (s *ComponentTypeService) GetComponentTypeSchema(ctx context.Context, orgName, ctdName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ComponentType schema", "org", orgName, "name", ctdName)

	// First get the CTD
	ctd := &openchoreov1alpha1.ComponentTypeDefinition{}
	key := client.ObjectKey{
		Name:      ctdName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, ctd); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "org", orgName, "name", ctdName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
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

	s.logger.Debug("Retrieved ComponentType schema successfully", "org", orgName, "name", ctdName)
	return jsonSchema, nil
}

// toComponentTypeResponse converts a ComponentType CR to a ComponentTypeResponse
func (s *ComponentTypeService) toComponentTypeResponse(ctd *openchoreov1alpha1.ComponentTypeDefinition) *models.ComponentTypeResponse {
	// Extract display name and description from annotations
	displayName := ctd.Annotations[controller.AnnotationKeyDisplayName]
	description := ctd.Annotations[controller.AnnotationKeyDescription]

	return &models.ComponentTypeResponse{
		Name:         ctd.Name,
		DisplayName:  displayName,
		Description:  description,
		WorkloadType: ctd.Spec.WorkloadType,
		CreatedAt:    ctd.CreationTimestamp.Time,
	}
}
