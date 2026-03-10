// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// clusterTraitService handles cluster trait business logic without authorization checks.
type clusterTraitService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterTraitService)(nil)

var clusterTraitTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterTrait",
}

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
	ct.Status = openchoreov1alpha1.ClusterTraitStatus{}

	if err := s.k8sClient.Create(ctx, ct); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster trait already exists", "clusterTrait", ct.Name)
			return nil, ErrClusterTraitAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create cluster trait CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster trait: %w", err)
	}

	s.logger.Debug("Cluster trait created successfully", "clusterTrait", ct.Name)
	ct.TypeMeta = clusterTraitTypeMeta
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

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	ct.Status = openchoreov1alpha1.ClusterTraitStatus{}
	existing.Spec = ct.Spec
	existing.Labels = ct.Labels
	existing.Annotations = ct.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update cluster trait CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster trait: %w", err)
	}

	s.logger.Debug("Cluster trait updated successfully", "clusterTrait", ct.Name)
	existing.TypeMeta = clusterTraitTypeMeta
	return existing, nil
}

func (s *clusterTraitService) ListClusterTraits(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterTrait], error) {
	s.logger.Debug("Listing cluster traits", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}

	var traitList openchoreov1alpha1.ClusterTraitList
	if err := s.k8sClient.List(ctx, &traitList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster traits", "error", err)
		return nil, fmt.Errorf("failed to list cluster traits: %w", err)
	}

	for i := range traitList.Items {
		traitList.Items[i].TypeMeta = clusterTraitTypeMeta
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

	trait.TypeMeta = clusterTraitTypeMeta
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

func (s *clusterTraitService) GetClusterTraitSchema(ctx context.Context, clusterTraitName string) (map[string]any, error) {
	s.logger.Debug("Getting cluster trait schema", "clusterTrait", clusterTraitName)

	trait, err := s.GetClusterTrait(ctx, clusterTraitName)
	if err != nil {
		return nil, err
	}

	// Convert to raw JSON Schema map, preserving vendor extensions (x-*) for frontend consumers
	rawSchema, err := schema.SectionToRawJSONSchema(trait.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster trait schema successfully", "clusterTrait", clusterTraitName)
	return rawSchema, nil
}
