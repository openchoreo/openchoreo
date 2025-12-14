// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
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
func (s *ComponentTypeService) ListComponentTypes(ctx context.Context, orgName string, opts *models.ListOptions) (*models.ListResponse[*models.ComponentTypeResponse], error) {
	if opts == nil {
		opts = &models.ListOptions{Limit: models.DefaultPageLimit}
	}
	s.logger.Debug("Listing ComponentTypes", "org", orgName, "limit", opts.Limit, "continue", opts.Continue)

	var ctList openchoreov1alpha1.ComponentTypeList
	listOpts := &client.ListOptions{
		Namespace: orgName,
		Limit:     int64(opts.Limit),
		Continue:  opts.Continue,
	}

	if err := s.k8sClient.List(ctx, &ctList, listOpts); err != nil {
		return nil, HandleListError(err, s.logger, opts.Continue, "component types")
	}

	// Convert items to response type
	componentTypes := make([]*models.ComponentTypeResponse, 0, len(ctList.Items))
	for _, ct := range ctList.Items {
		componentTypes = append(componentTypes, s.toComponentTypeResponse(&ct))
	}

	s.logger.Debug("Listed component types", "org", orgName, "count", len(componentTypes), "hasMore", ctList.Continue != "")
	return &models.ListResponse[*models.ComponentTypeResponse]{
		Items: componentTypes,
		Metadata: models.ResponseMetadata{
			ResourceVersion: ctList.ResourceVersion,
			Continue:        ctList.Continue,
			HasMore:         ctList.Continue != "",
		},
	}, nil
}

// GetComponentType retrieves a specific ComponentType
func (s *ComponentTypeService) GetComponentType(ctx context.Context, orgName, ctName string) (*models.ComponentTypeResponse, error) {
	s.logger.Debug("Getting ComponentType", "org", orgName, "name", ctName)

	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "org", orgName, "name", ctName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
	}

	return s.toComponentTypeResponse(ct), nil
}

// GetComponentTypeSchema retrieves the JSON schema for a ComponentType
func (s *ComponentTypeService) GetComponentTypeSchema(ctx context.Context, orgName, ctName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ComponentType schema", "org", orgName, "name", ctName)

	// First get the CT
	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "org", orgName, "name", ctName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
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
		if err := json.Unmarshal(ct.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved ComponentType schema successfully", "org", orgName, "name", ctName)
	return jsonSchema, nil
}

// toComponentTypeResponse converts a ComponentType CR to a ComponentTypeResponse
func (s *ComponentTypeService) toComponentTypeResponse(ct *openchoreov1alpha1.ComponentType) *models.ComponentTypeResponse {
	// Extract display name and description from annotations
	displayName := ct.Annotations[controller.AnnotationKeyDisplayName]
	description := ct.Annotations[controller.AnnotationKeyDescription]

	// Convert allowed component-workflows to string list
	allowedWorkflows := make([]string, 0, len(ct.Spec.AllowedWorkflows))
	allowedWorkflows = append(allowedWorkflows, ct.Spec.AllowedWorkflows...)

	return &models.ComponentTypeResponse{
		Name:             ct.Name,
		DisplayName:      displayName,
		Description:      description,
		WorkloadType:     ct.Spec.WorkloadType,
		AllowedWorkflows: allowedWorkflows,
		CreatedAt:        ct.CreationTimestamp.Time,
	}
}
