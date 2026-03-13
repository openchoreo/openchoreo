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

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// ComponentTypeService handles ComponentType-related business logic
type ComponentTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewComponentTypeService creates a new ComponentType service
func NewComponentTypeService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ComponentTypeService {
	return &ComponentTypeService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// AuthorizeCreate checks if the current user is authorized to create a ComponentType
func (s *ComponentTypeService) AuthorizeCreate(ctx context.Context, namespaceName, ctName string) error {
	return checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateComponentType,
		ResourceTypeComponentType, ctName, authz.ResourceHierarchy{Namespace: namespaceName})
}

// ListComponentTypes lists all ComponentTypes in the given namespace
func (s *ComponentTypeService) ListComponentTypes(ctx context.Context, namespaceName string) ([]*models.ComponentTypeResponse, error) {
	s.logger.Debug("Listing ComponentTypes", "namespace", namespaceName)

	var ctList openchoreov1alpha1.ComponentTypeList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &ctList, listOpts...); err != nil {
		s.logger.Error("Failed to list ComponentTypes", "error", err)
		return nil, fmt.Errorf("failed to list ComponentTypes: %w", err)
	}

	cts := make([]*models.ComponentTypeResponse, 0, len(ctList.Items))
	for i := range ctList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentType, ResourceTypeComponentType, ctList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized component type", "namespace", namespaceName, "componentType", ctList.Items[i].Name)
				continue
			}
			return nil, err
		}
		cts = append(cts, s.toComponentTypeResponse(&ctList.Items[i]))
	}

	s.logger.Debug("Listed ComponentTypes", "namespace", namespaceName, "count", len(cts))
	return cts, nil
}

// GetComponentType retrieves a specific ComponentType
func (s *ComponentTypeService) GetComponentType(ctx context.Context, namespaceName, ctName string) (*models.ComponentTypeResponse, error) {
	s.logger.Debug("Getting ComponentType", "namespace", namespaceName, "name", ctName)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentType, ResourceTypeComponentType, ctName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "namespace", namespaceName, "name", ctName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
	}

	return s.toComponentTypeResponse(ct), nil
}

// GetComponentTypeSchema retrieves the JSON schema for a ComponentType
func (s *ComponentTypeService) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ComponentType schema", "namespace", namespaceName, "name", ctName)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentType, ResourceTypeComponentType, ctName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	// First get the CT
	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "namespace", namespaceName, "name", ctName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.SectionToJSONSchema(ct.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved ComponentType schema successfully", "namespace", namespaceName, "name", ctName)
	return jsonSchema, nil
}

// toComponentTypeResponse converts a ComponentType CR to a ComponentTypeResponse
func (s *ComponentTypeService) toComponentTypeResponse(ct *openchoreov1alpha1.ComponentType) *models.ComponentTypeResponse {
	// Extract display name and description from annotations
	displayName := ct.Annotations[controller.AnnotationKeyDisplayName]
	description := ct.Annotations[controller.AnnotationKeyDescription]

	// Convert allowed workflows to response format
	allowedWorkflows := make([]models.AllowedWorkflowResponse, 0, len(ct.Spec.AllowedWorkflows))
	for _, ref := range ct.Spec.AllowedWorkflows {
		allowedWorkflows = append(allowedWorkflows, models.AllowedWorkflowResponse{
			Kind: string(ref.Kind),
			Name: ref.Name,
		})
	}

	// Convert allowed traits to response format
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
