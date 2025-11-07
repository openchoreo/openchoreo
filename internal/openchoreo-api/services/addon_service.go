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

// AddonService handles Addon-related business logic
type AddonService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewAddonService creates a new Addon service
func NewAddonService(k8sClient client.Client, logger *slog.Logger) *AddonService {
	return &AddonService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListAddons lists all Addons in the given organization
func (s *AddonService) ListAddons(ctx context.Context, orgName string) ([]*models.AddonResponse, error) {
	s.logger.Debug("Listing Addons", "org", orgName)

	var addonList openchoreov1alpha1.AddonList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &addonList, listOpts...); err != nil {
		s.logger.Error("Failed to list Addons", "error", err)
		return nil, fmt.Errorf("failed to list Addons: %w", err)
	}

	addons := make([]*models.AddonResponse, 0, len(addonList.Items))
	for i := range addonList.Items {
		addons = append(addons, s.toAddonResponse(&addonList.Items[i]))
	}

	s.logger.Debug("Listed Addons", "org", orgName, "count", len(addons))
	return addons, nil
}

// GetAddon retrieves a specific Addon
func (s *AddonService) GetAddon(ctx context.Context, orgName, addonName string) (*models.AddonResponse, error) {
	s.logger.Debug("Getting Addon", "org", orgName, "name", addonName)

	addon := &openchoreov1alpha1.Addon{}
	key := client.ObjectKey{
		Name:      addonName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, addon); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Addon not found", "org", orgName, "name", addonName)
			return nil, ErrAddonNotFound
		}
		s.logger.Error("Failed to get Addon", "error", err)
		return nil, fmt.Errorf("failed to get Addon: %w", err)
	}

	return s.toAddonResponse(addon), nil
}

// GetAddonSchema retrieves the JSON schema for an Addon
func (s *AddonService) GetAddonSchema(ctx context.Context, orgName, addonName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting Addon schema", "org", orgName, "name", addonName)

	// First get the Addon
	addon := &openchoreov1alpha1.Addon{}
	key := client.ObjectKey{
		Name:      addonName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, addon); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Addon not found", "org", orgName, "name", addonName)
			return nil, ErrAddonNotFound
		}
		s.logger.Error("Failed to get Addon", "error", err)
		return nil, fmt.Errorf("failed to get Addon: %w", err)
	}

	// Extract types from RawExtension
	var types map[string]any
	if addon.Spec.Schema.Types != nil && addon.Spec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(addon.Spec.Schema.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Build schema definition
	def := schema.Definition{
		Types: types,
	}

	// Extract parameters schema from RawExtension
	if addon.Spec.Schema.Parameters != nil && addon.Spec.Schema.Parameters.Raw != nil {
		var params map[string]any
		if err := json.Unmarshal(addon.Spec.Schema.Parameters.Raw, &params); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
		def.Schemas = []map[string]any{params}
	}

	// Convert to JSON Schema
	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved Addon schema successfully", "org", orgName, "name", addonName)
	return jsonSchema, nil
}

// toAddonResponse converts an Addon CR to an AddonResponse
func (s *AddonService) toAddonResponse(addon *openchoreov1alpha1.Addon) *models.AddonResponse {
	// Extract display name and description from annotations
	displayName := addon.Annotations[controller.AnnotationKeyDisplayName]
	description := addon.Annotations[controller.AnnotationKeyDescription]

	return &models.AddonResponse{
		Name:        addon.Name,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   addon.CreationTimestamp.Time,
	}
}
