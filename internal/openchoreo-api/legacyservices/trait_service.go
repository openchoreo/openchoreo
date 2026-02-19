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

// TraitService handles Trait-related business logic
type TraitService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewTraitService creates a new Trait service
func NewTraitService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *TraitService {
	return &TraitService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// AuthorizeCreate checks if the current user is authorized to create a Trait
func (s *TraitService) AuthorizeCreate(ctx context.Context, namespaceName, traitName string) error {
	return checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateTrait,
		ResourceTypeTrait, traitName, authz.ResourceHierarchy{Namespace: namespaceName})
}

// ListTraits lists all Traits in the given namespace
func (s *TraitService) ListTraits(ctx context.Context, namespaceName string) ([]*models.TraitResponse, error) {
	s.logger.Debug("Listing Traits", "namespace", namespaceName)

	var traitList openchoreov1alpha1.TraitList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &traitList, listOpts...); err != nil {
		s.logger.Error("Failed to list Traits", "error", err)
		return nil, fmt.Errorf("failed to list Traits: %w", err)
	}

	traits := make([]*models.TraitResponse, 0, len(traitList.Items))
	for i := range traitList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewTrait, ResourceTypeTrait, traitList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized trait", "namespace", namespaceName, "trait", traitList.Items[i].Name)
				continue
			}
			return nil, err
		}
		traits = append(traits, s.toTraitResponse(&traitList.Items[i]))
	}

	s.logger.Debug("Listed Traits", "namespace", namespaceName, "count", len(traits))
	return traits, nil
}

// GetTrait retrieves a specific Trait
func (s *TraitService) GetTrait(ctx context.Context, namespaceName, traitName string) (*models.TraitResponse, error) {
	s.logger.Debug("Getting Trait", "namespace", namespaceName, "name", traitName)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewTrait, ResourceTypeTrait, traitName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	trait := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "namespace", namespaceName, "name", traitName)
			return nil, ErrTraitNotFound
		}
		s.logger.Error("Failed to get Trait", "error", err)
		return nil, fmt.Errorf("failed to get Trait: %w", err)
	}

	return s.toTraitResponse(trait), nil
}

// GetTraitSchema retrieves the JSON schema for an Trait
func (s *TraitService) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting Trait schema", "namespace", namespaceName, "name", traitName)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewTrait, ResourceTypeTrait, traitName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	// First get the Trait
	trait := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "namespace", namespaceName, "name", traitName)
			return nil, ErrTraitNotFound
		}
		s.logger.Error("Failed to get Trait", "error", err)
		return nil, fmt.Errorf("failed to get Trait: %w", err)
	}

	// Extract types from RawExtension
	var types map[string]any
	if trait.Spec.Schema.Types != nil && trait.Spec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(trait.Spec.Schema.Types.Raw, &types); err != nil {
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
	if trait.Spec.Schema.Parameters != nil && trait.Spec.Schema.Parameters.Raw != nil {
		var params map[string]any
		if err := yaml.Unmarshal(trait.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved Trait schema successfully", "namespace", namespaceName, "name", traitName)
	return jsonSchema, nil
}

// toTraitResponse converts an Trait CR to an TraitResponse
func (s *TraitService) toTraitResponse(trait *openchoreov1alpha1.Trait) *models.TraitResponse {
	// Extract display name and description from annotations
	displayName := trait.Annotations[controller.AnnotationKeyDisplayName]
	description := trait.Annotations[controller.AnnotationKeyDescription]

	return &models.TraitResponse{
		Name:        trait.Name,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   trait.CreationTimestamp.Time,
	}
}
