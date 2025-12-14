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

// TraitService handles Trait-related business logic
type TraitService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewTraitService creates a new Trait service
func NewTraitService(k8sClient client.Client, logger *slog.Logger) *TraitService {
	return &TraitService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListTraits lists all Traits in the given organization
func (s *TraitService) ListTraits(ctx context.Context, orgName string, opts *models.ListOptions) (*models.ListResponse[*models.TraitResponse], error) {
	if opts == nil {
		opts = &models.ListOptions{Limit: models.DefaultPageLimit}
	}
	s.logger.Debug("Listing Traits", "org", orgName, "limit", opts.Limit, "continue", opts.Continue)

	var traitList openchoreov1alpha1.TraitList

	listOpts := &client.ListOptions{
		Namespace: orgName,
		Limit:     int64(opts.Limit),
		Continue:  opts.Continue,
	}

	if err := s.k8sClient.List(ctx, &traitList, listOpts); err != nil {
		return nil, HandleListError(err, s.logger, opts.Continue, "traits")
	}

	traits := make([]*models.TraitResponse, 0, len(traitList.Items))
	for i := range traitList.Items {
		traits = append(traits, s.toTraitResponse(&traitList.Items[i]))
	}

	s.logger.Debug("Listed Traits", "org", orgName, "count", len(traits), "hasMore", traitList.Continue != "")
	return &models.ListResponse[*models.TraitResponse]{
		Items: traits,
		Metadata: models.ResponseMetadata{
			ResourceVersion: traitList.ResourceVersion,
			Continue:        traitList.Continue,
			HasMore:         traitList.Continue != "",
		},
	}, nil
}

// GetTrait retrieves a specific Trait
func (s *TraitService) GetTrait(ctx context.Context, orgName, traitName string) (*models.TraitResponse, error) {
	s.logger.Debug("Getting Trait", "org", orgName, "name", traitName)

	trait := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "org", orgName, "name", traitName)
			return nil, ErrTraitNotFound
		}
		s.logger.Error("Failed to get Trait", "error", err)
		return nil, fmt.Errorf("failed to get Trait: %w", err)
	}

	return s.toTraitResponse(trait), nil
}

// GetTraitSchema retrieves the JSON schema for an Trait
func (s *TraitService) GetTraitSchema(ctx context.Context, orgName, traitName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting Trait schema", "org", orgName, "name", traitName)

	// First get the Trait
	trait := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "org", orgName, "name", traitName)
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
		if err := json.Unmarshal(trait.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved Trait schema successfully", "org", orgName, "name", traitName)
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
