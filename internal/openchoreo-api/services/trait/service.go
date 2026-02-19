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
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// traitService handles trait business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type traitService struct {
	k8sClient client.Client
	logger    *slog.Logger
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
	t.TypeMeta = metav1.TypeMeta{
		Kind:       "Trait",
		APIVersion: "core.choreo.dev/v1alpha1",
	}
	t.Namespace = namespaceName
	if t.Labels == nil {
		t.Labels = make(map[string]string)
	}
	t.Labels[labels.LabelKeyNamespaceName] = namespaceName
	t.Labels[labels.LabelKeyName] = t.Name

	if err := s.k8sClient.Create(ctx, t); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Trait already exists", "namespace", namespaceName, "trait", t.Name)
			return nil, ErrTraitAlreadyExists
		}
		s.logger.Error("Failed to create trait CR", "error", err)
		return nil, fmt.Errorf("failed to create trait: %w", err)
	}

	s.logger.Debug("Trait created successfully", "namespace", namespaceName, "trait", t.Name)
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

	// Apply incoming spec directly from the request body, preserving server-managed fields
	t.ResourceVersion = existing.ResourceVersion
	t.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, t); err != nil {
		s.logger.Error("Failed to update trait CR", "error", err)
		return nil, fmt.Errorf("failed to update trait: %w", err)
	}

	s.logger.Debug("Trait updated successfully", "namespace", namespaceName, "trait", t.Name)
	return t, nil
}

func (s *traitService) ListTraits(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Trait], error) {
	s.logger.Debug("Listing traits", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var tList openchoreov1alpha1.TraitList
	if err := s.k8sClient.List(ctx, &tList, listOpts...); err != nil {
		s.logger.Error("Failed to list traits", "error", err)
		return nil, fmt.Errorf("failed to list traits: %w", err)
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

	return t, nil
}

func (s *traitService) DeleteTrait(ctx context.Context, namespaceName, traitName string) error {
	s.logger.Debug("Deleting trait", "namespace", namespaceName, "trait", traitName)

	t := &openchoreov1alpha1.Trait{}
	key := client.ObjectKey{
		Name:      traitName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, t); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Trait not found", "namespace", namespaceName, "trait", traitName)
			return ErrTraitNotFound
		}
		s.logger.Error("Failed to get trait", "error", err)
		return fmt.Errorf("failed to get trait: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, t); err != nil {
		s.logger.Error("Failed to delete trait CR", "error", err)
		return fmt.Errorf("failed to delete trait: %w", err)
	}

	s.logger.Debug("Trait deleted successfully", "namespace", namespaceName, "trait", traitName)
	return nil
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
