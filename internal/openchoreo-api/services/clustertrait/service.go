// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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

// clusterTraitService handles cluster trait business logic without authorization checks.
type clusterTraitService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterTraitService)(nil)

// NewService creates a new cluster trait service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterTraitService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterTraitService) CreateClusterTrait(ctx context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error) {
	if ct == nil {
		return nil, fmt.Errorf("cluster trait cannot be nil")
	}

	s.logger.Debug("Creating cluster trait", "clusterTrait", ct.Name)

	// Set defaults
	ct.TypeMeta = metav1.TypeMeta{
		Kind:       "ClusterTrait",
		APIVersion: "openchoreo.dev/v1alpha1",
	}

	if err := s.k8sClient.Create(ctx, ct); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster trait already exists", "clusterTrait", ct.Name)
			return nil, ErrClusterTraitAlreadyExists
		}
		s.logger.Error("Failed to create cluster trait CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster trait: %w", err)
	}

	s.logger.Debug("Cluster trait created successfully", "clusterTrait", ct.Name)
	return ct, nil
}

func (s *clusterTraitService) UpdateClusterTrait(ctx context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error) {
	if ct == nil {
		return nil, fmt.Errorf("cluster trait cannot be nil")
	}

	s.logger.Debug("Updating cluster trait", "clusterTrait", ct.Name)

	existing := &openchoreov1alpha1.ClusterTrait{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: ct.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster trait not found", "clusterTrait", ct.Name)
			return nil, ErrClusterTraitNotFound
		}
		s.logger.Error("Failed to get cluster trait", "error", err)
		return nil, fmt.Errorf("failed to get cluster trait: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	ct.ResourceVersion = existing.ResourceVersion

	if err := s.k8sClient.Update(ctx, ct); err != nil {
		s.logger.Error("Failed to update cluster trait CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster trait: %w", err)
	}

	s.logger.Debug("Cluster trait updated successfully", "clusterTrait", ct.Name)
	return ct, nil
}

func (s *clusterTraitService) ListClusterTraits(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterTrait], error) {
	s.logger.Debug("Listing cluster traits", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var traitList openchoreov1alpha1.ClusterTraitList
	if err := s.k8sClient.List(ctx, &traitList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster traits", "error", err)
		return nil, fmt.Errorf("failed to list cluster traits: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterTrait]{
		Items:      traitList.Items,
		NextCursor: traitList.Continue,
	}
	if traitList.RemainingItemCount != nil {
		remaining := *traitList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster traits", "count", len(traitList.Items))
	return result, nil
}

func (s *clusterTraitService) GetClusterTrait(ctx context.Context, clusterTraitName string) (*openchoreov1alpha1.ClusterTrait, error) {
	s.logger.Debug("Getting cluster trait", "clusterTrait", clusterTraitName)

	trait := &openchoreov1alpha1.ClusterTrait{}
	key := client.ObjectKey{
		Name: clusterTraitName,
	}

	if err := s.k8sClient.Get(ctx, key, trait); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster trait not found", "clusterTrait", clusterTraitName)
			return nil, ErrClusterTraitNotFound
		}
		s.logger.Error("Failed to get cluster trait", "error", err)
		return nil, fmt.Errorf("failed to get cluster trait: %w", err)
	}

	return trait, nil
}

// DeleteClusterTrait removes a cluster-scoped trait by name.
func (s *clusterTraitService) DeleteClusterTrait(ctx context.Context, clusterTraitName string) error {
	s.logger.Debug("Deleting cluster trait", "clusterTrait", clusterTraitName)

	trait := &openchoreov1alpha1.ClusterTrait{}
	trait.Name = clusterTraitName

	if err := s.k8sClient.Delete(ctx, trait); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterTraitNotFound
		}
		s.logger.Error("Failed to delete cluster trait CR", "error", err)
		return fmt.Errorf("failed to delete cluster trait: %w", err)
	}

	s.logger.Debug("Cluster trait deleted successfully", "clusterTrait", clusterTraitName)
	return nil
}

func (s *clusterTraitService) GetClusterTraitSchema(ctx context.Context, clusterTraitName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting cluster trait schema", "clusterTrait", clusterTraitName)

	trait, err := s.GetClusterTrait(ctx, clusterTraitName)
	if err != nil {
		return nil, err
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

	s.logger.Debug("Retrieved cluster trait schema successfully", "clusterTrait", clusterTraitName)
	return jsonSchema, nil
}
