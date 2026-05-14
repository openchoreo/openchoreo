// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

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

// traitService handles trait business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type traitService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var traitTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Trait",
}

var _ Service = (*traitService)(nil)

// NewService creates a new trait service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &traitService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *traitService) CreateTrait(ctx context.Context, namespaceName string, t *openchoreov1alpha1.Trait) (*openchoreov1alpha1.Trait, error) {
	if t == nil {
		return nil, fmt.Errorf("trait cannot be nil")
	}

	s.logger.Debug("Creating trait", "namespace", namespaceName, "trait", t.Name)

	exists, err := s.traitExists(ctx, namespaceName, t.Name)
	if err != nil {
		s.logger.Error("Failed to check trait existence", "error", err)
		return nil, fmt.Errorf("failed to check trait existence: %w", err)
	}
	if exists {
		s.logger.Warn("Trait already exists", "namespace", namespaceName, "trait", t.Name)
		return nil, ErrTraitAlreadyExists
	}

	// Set defaults
	t.Namespace = namespaceName
	t.Status = openchoreov1alpha1.TraitStatus{}
	if err := s.k8sClient.Create(ctx, t); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Trait already exists", "namespace", namespaceName, "trait", t.Name)
			return nil, ErrTraitAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create trait CR", "error", err)
		return nil, fmt.Errorf("failed to create trait: %w", err)
	}

	s.logger.Debug("Trait created successfully", "namespace", namespaceName, "trait", t.Name)
	t.TypeMeta = traitTypeMeta
	return t, nil
}

func (s *traitService) UpdateTrait(ctx context.Context, namespaceName string, t *openchoreov1alpha1.Trait) (*openchoreov1alpha1.Trait, error) {
	if t == nil {
		return nil, fmt.Errorf("trait cannot be nil")
	}

	s.logger.Debug("Updating trait", "namespace", namespaceName, "trait", t.Name)

	existing := &openchoreov1alpha1.Trait{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: t.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "namespace", namespaceName, "trait", t.Name)
			return nil, ErrTraitNotFound
		}
		s.logger.Error("Failed to get trait", "error", err)
		return nil, fmt.Errorf("failed to get trait: %w", err)
	}

	// Clear status from user input — status is server-managed
	t.Status = openchoreov1alpha1.TraitStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = t.Spec
	existing.Labels = t.Labels
	existing.Annotations = t.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update trait CR", "error", err)
		return nil, fmt.Errorf("failed to update trait: %w", err)
	}

	s.logger.Debug("Trait updated successfully", "namespace", namespaceName, "trait", t.Name)
	existing.TypeMeta = traitTypeMeta
	return existing, nil
}

func (s *traitService) ListTraits(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Trait], error) {
	s.logger.Debug("Listing traits", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var tList openchoreov1alpha1.TraitList
	if err := s.k8sClient.List(ctx, &tList, listOpts...); err != nil {
		s.logger.Error("Failed to list traits", "error", err)
		return nil, fmt.Errorf("failed to list traits: %w", err)
	}

	for i := range tList.Items {
		tList.Items[i].TypeMeta = traitTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.Trait]{
		Items:      tList.Items,
		NextCursor: tList.Continue,
	}
	if tList.RemainingItemCount != nil {
		remaining := *tList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed traits", "namespace", namespaceName, "count", len(tList.Items))
	return result, nil
}

func (s *traitService) GetTrait(ctx context.Context, namespaceName, traitName string) (*openchoreov1alpha1.Trait, error) {
	s.logger.Debug("Getting trait", "namespace", namespaceName, "trait", traitName)

	t := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, t); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "namespace", namespaceName, "trait", traitName)
			return nil, ErrTraitNotFound
		}
		s.logger.Error("Failed to get trait", "error", err)
		return nil, fmt.Errorf("failed to get trait: %w", err)
	}

	t.TypeMeta = traitTypeMeta
	return t, nil
}

func (s *traitService) DeleteTrait(ctx context.Context, namespaceName, traitName string) error {
	s.logger.Debug("Deleting trait", "namespace", namespaceName, "trait", traitName)

	t := &openchoreov1alpha1.Trait{}
	t.Name = traitName
	t.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, t); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrTraitNotFound
		}
		s.logger.Error("Failed to delete trait CR", "error", err)
		return fmt.Errorf("failed to delete trait: %w", err)
	}

	s.logger.Debug("Trait deleted successfully", "namespace", namespaceName, "trait", traitName)
	return nil
}

func (s *traitService) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (map[string]any, error) {
	s.logger.Debug("Getting trait schema", "namespace", namespaceName, "trait", traitName)

	t, err := s.GetTrait(ctx, namespaceName, traitName)
	if err != nil {
		return nil, err
	}

	// Convert to raw JSON Schema map, preserving vendor extensions (x-*) for frontend consumers
	rawSchema, err := schema.SectionToRawJSONSchema(t.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved trait schema successfully", "namespace", namespaceName, "trait", traitName)
	return rawSchema, nil
}

func (s *traitService) traitExists(ctx context.Context, namespaceName, traitName string) (bool, error) {
	t := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, t)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of trait %s/%s: %w", namespaceName, traitName, err)
	}
	return true, nil
}
