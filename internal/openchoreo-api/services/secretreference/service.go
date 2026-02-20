// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

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

// secretReferenceService handles secret reference business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type secretReferenceService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*secretReferenceService)(nil)

// NewService creates a new secret reference service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &secretReferenceService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *secretReferenceService) CreateSecretReference(ctx context.Context, namespaceName string, sr *openchoreov1alpha1.SecretReference) (*openchoreov1alpha1.SecretReference, error) {
	if sr == nil {
		return nil, fmt.Errorf("secret reference cannot be nil")
	}

	s.logger.Debug("Creating secret reference", "namespace", namespaceName, "secretReference", sr.Name)

	exists, err := s.secretReferenceExists(ctx, namespaceName, sr.Name)
	if err != nil {
		s.logger.Error("Failed to check secret reference existence", "error", err)
		return nil, fmt.Errorf("failed to check secret reference existence: %w", err)
	}
	if exists {
		s.logger.Warn("Secret reference already exists", "namespace", namespaceName, "secretReference", sr.Name)
		return nil, ErrSecretReferenceAlreadyExists
	}

	// Set defaults
	sr.TypeMeta = metav1.TypeMeta{
		Kind:       "SecretReference",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	sr.Namespace = namespaceName
	if sr.Labels == nil {
		sr.Labels = make(map[string]string)
	}
	sr.Labels[labels.LabelKeyNamespaceName] = namespaceName
	sr.Labels[labels.LabelKeyName] = sr.Name

	if err := s.k8sClient.Create(ctx, sr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Secret reference already exists", "namespace", namespaceName, "secretReference", sr.Name)
			return nil, ErrSecretReferenceAlreadyExists
		}
		s.logger.Error("Failed to create secret reference CR", "error", err)
		return nil, fmt.Errorf("failed to create secret reference: %w", err)
	}

	s.logger.Debug("Secret reference created successfully", "namespace", namespaceName, "secretReference", sr.Name)
	return sr, nil
}

func (s *secretReferenceService) UpdateSecretReference(ctx context.Context, namespaceName string, sr *openchoreov1alpha1.SecretReference) (*openchoreov1alpha1.SecretReference, error) {
	if sr == nil {
		return nil, fmt.Errorf("secret reference cannot be nil")
	}

	s.logger.Debug("Updating secret reference", "namespace", namespaceName, "secretReference", sr.Name)

	existing := &openchoreov1alpha1.SecretReference{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: sr.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Secret reference not found", "namespace", namespaceName, "secretReference", sr.Name)
			return nil, ErrSecretReferenceNotFound
		}
		s.logger.Error("Failed to get secret reference", "error", err)
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	sr.ResourceVersion = existing.ResourceVersion
	sr.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, sr); err != nil {
		s.logger.Error("Failed to update secret reference CR", "error", err)
		return nil, fmt.Errorf("failed to update secret reference: %w", err)
	}

	s.logger.Debug("Secret reference updated successfully", "namespace", namespaceName, "secretReference", sr.Name)
	return sr, nil
}

func (s *secretReferenceService) ListSecretReferences(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.SecretReference], error) {
	s.logger.Debug("Listing secret references", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var srList openchoreov1alpha1.SecretReferenceList
	if err := s.k8sClient.List(ctx, &srList, listOpts...); err != nil {
		s.logger.Error("Failed to list secret references", "error", err)
		return nil, fmt.Errorf("failed to list secret references: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.SecretReference]{
		Items:      srList.Items,
		NextCursor: srList.Continue,
	}
	if srList.RemainingItemCount != nil {
		remaining := *srList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed secret references", "namespace", namespaceName, "count", len(srList.Items))
	return result, nil
}

func (s *secretReferenceService) GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (*openchoreov1alpha1.SecretReference, error) {
	s.logger.Debug("Getting secret reference", "namespace", namespaceName, "secretReference", secretReferenceName)

	sr := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{
		Name:      secretReferenceName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, sr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Secret reference not found", "namespace", namespaceName, "secretReference", secretReferenceName)
			return nil, ErrSecretReferenceNotFound
		}
		s.logger.Error("Failed to get secret reference", "error", err)
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
	}

	return sr, nil
}

func (s *secretReferenceService) DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) error {
	s.logger.Debug("Deleting secret reference", "namespace", namespaceName, "secretReference", secretReferenceName)

	sr := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{
		Name:      secretReferenceName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, sr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Secret reference not found", "namespace", namespaceName, "secretReference", secretReferenceName)
			return ErrSecretReferenceNotFound
		}
		s.logger.Error("Failed to get secret reference", "error", err)
		return fmt.Errorf("failed to get secret reference: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, sr); err != nil {
		s.logger.Error("Failed to delete secret reference CR", "error", err)
		return fmt.Errorf("failed to delete secret reference: %w", err)
	}

	s.logger.Debug("Secret reference deleted successfully", "namespace", namespaceName, "secretReference", secretReferenceName)
	return nil
}

func (s *secretReferenceService) secretReferenceExists(ctx context.Context, namespaceName, secretReferenceName string) (bool, error) {
	sr := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{
		Name:      secretReferenceName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, sr)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of secret reference %s/%s: %w", namespaceName, secretReferenceName, err)
	}
	return true, nil
}
