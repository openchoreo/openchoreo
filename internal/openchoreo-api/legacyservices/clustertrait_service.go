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

// ClusterTraitService handles ClusterTrait-related business logic
type ClusterTraitService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewClusterTraitService creates a new ClusterTrait service
func NewClusterTraitService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ClusterTraitService {
	return &ClusterTraitService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListClusterTraits lists all ClusterTraits
func (s *ClusterTraitService) ListClusterTraits(ctx context.Context) ([]*models.TraitResponse, error) {
	s.logger.Debug("Listing ClusterTraits")

	var traitList openchoreov1alpha1.ClusterTraitList
	if err := s.k8sClient.List(ctx, &traitList); err != nil {
		s.logger.Error("Failed to list ClusterTraits", "error", err)
		return nil, fmt.Errorf("failed to list ClusterTraits: %w", err)
	}

	traits := make([]*models.TraitResponse, 0, len(traitList.Items))
	for i := range traitList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterTrait, ResourceTypeClusterTrait, traitList.Items[i].Name,
			authz.ResourceHierarchy{}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized cluster trait", "clusterTrait", traitList.Items[i].Name)
				continue
			}
			return nil, err
		}
		traits = append(traits, s.toTraitResponse(&traitList.Items[i]))
	}

	s.logger.Debug("Listed ClusterTraits", "count", len(traits))
	return traits, nil
}

// GetClusterTrait retrieves a specific ClusterTrait
func (s *ClusterTraitService) GetClusterTrait(ctx context.Context, name string) (*models.TraitResponse, error) {
	s.logger.Debug("Getting ClusterTrait", "name", name)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterTrait, ResourceTypeClusterTrait, name,
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	trait := &openchoreov1alpha1.ClusterTrait{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ClusterTrait not found", "name", name)
			return nil, ErrClusterTraitNotFound
		}
		s.logger.Error("Failed to get ClusterTrait", "error", err)
		return nil, fmt.Errorf("failed to get ClusterTrait: %w", err)
	}

	return s.toTraitResponse(trait), nil
}

// GetClusterTraitSchema retrieves the JSON schema for a ClusterTrait
func (s *ClusterTraitService) GetClusterTraitSchema(ctx context.Context, name string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ClusterTrait schema", "name", name)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterTrait, ResourceTypeClusterTrait, name,
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	trait := &openchoreov1alpha1.ClusterTrait{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ClusterTrait not found", "name", name)
			return nil, ErrClusterTraitNotFound
		}
		s.logger.Error("Failed to get ClusterTrait", "error", err)
		return nil, fmt.Errorf("failed to get ClusterTrait: %w", err)
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

	s.logger.Debug("Retrieved ClusterTrait schema successfully", "name", name)
	return jsonSchema, nil
}

// toTraitResponse converts a ClusterTrait CR to a TraitResponse
func (s *ClusterTraitService) toTraitResponse(trait *openchoreov1alpha1.ClusterTrait) *models.TraitResponse {
	displayName := trait.Annotations[controller.AnnotationKeyDisplayName]
	description := trait.Annotations[controller.AnnotationKeyDescription]

	return &models.TraitResponse{
		Name:        trait.Name,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   trait.CreationTimestamp.Time,
	}
}
