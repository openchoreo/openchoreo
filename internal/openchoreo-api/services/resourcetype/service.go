// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

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

// resourceTypeService handles resource type business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type resourceTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var resourceTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ResourceType",
}

var _ Service = (*resourceTypeService)(nil)

// NewService creates a new resource type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &resourceTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *resourceTypeService) CreateResourceType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.ResourceType) (*openchoreov1alpha1.ResourceType, error) {
	if rt == nil {
		return nil, fmt.Errorf("resource type cannot be nil")
	}

	s.logger.Debug("Creating resource type", "namespace", namespaceName, "resourceType", rt.Name)

	exists, err := s.resourceTypeExists(ctx, namespaceName, rt.Name)
	if err != nil {
		s.logger.Error("Failed to check resource type existence", "error", err)
		return nil, fmt.Errorf("failed to check resource type existence: %w", err)
	}
	if exists {
		s.logger.Warn("Resource type already exists", "namespace", namespaceName, "resourceType", rt.Name)
		return nil, ErrResourceTypeAlreadyExists
	}

	rt.Namespace = namespaceName
	rt.Status = openchoreov1alpha1.ResourceTypeStatus{}
	if err := s.k8sClient.Create(ctx, rt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Resource type already exists", "namespace", namespaceName, "resourceType", rt.Name)
			return nil, ErrResourceTypeAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create resource type CR", "error", err)
		return nil, fmt.Errorf("failed to create resource type: %w", err)
	}

	s.logger.Debug("Resource type created successfully", "namespace", namespaceName, "resourceType", rt.Name)
	rt.TypeMeta = resourceTypeTypeMeta
	return rt, nil
}

func (s *resourceTypeService) UpdateResourceType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.ResourceType) (*openchoreov1alpha1.ResourceType, error) {
	if rt == nil {
		return nil, fmt.Errorf("resource type cannot be nil")
	}

	s.logger.Debug("Updating resource type", "namespace", namespaceName, "resourceType", rt.Name)

	existing := &openchoreov1alpha1.ResourceType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rt.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource type not found", "namespace", namespaceName, "resourceType", rt.Name)
			return nil, ErrResourceTypeNotFound
		}
		s.logger.Error("Failed to get resource type", "error", err)
		return nil, fmt.Errorf("failed to get resource type: %w", err)
	}

	// Clear status from user input — status is server-managed
	rt.Status = openchoreov1alpha1.ResourceTypeStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = rt.Spec
	existing.Labels = rt.Labels
	existing.Annotations = rt.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update resource type CR", "error", err)
		return nil, fmt.Errorf("failed to update resource type: %w", err)
	}

	s.logger.Debug("Resource type updated successfully", "namespace", namespaceName, "resourceType", rt.Name)
	existing.TypeMeta = resourceTypeTypeMeta
	return existing, nil
}

func (s *resourceTypeService) ListResourceTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceType], error) {
	s.logger.Debug("Listing resource types", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var rtList openchoreov1alpha1.ResourceTypeList
	if err := s.k8sClient.List(ctx, &rtList, listOpts...); err != nil {
		s.logger.Error("Failed to list resource types", "error", err)
		return nil, fmt.Errorf("failed to list resource types: %w", err)
	}

	for i := range rtList.Items {
		rtList.Items[i].TypeMeta = resourceTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ResourceType]{
		Items:      rtList.Items,
		NextCursor: rtList.Continue,
	}
	if rtList.RemainingItemCount != nil {
		remaining := *rtList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed resource types", "namespace", namespaceName, "count", len(rtList.Items))
	return result, nil
}

func (s *resourceTypeService) GetResourceType(ctx context.Context, namespaceName, rtName string) (*openchoreov1alpha1.ResourceType, error) {
	s.logger.Debug("Getting resource type", "namespace", namespaceName, "resourceType", rtName)

	rt := &openchoreov1alpha1.ResourceType{}
	key := client.ObjectKey{
		Name:      rtName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rt); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource type not found", "namespace", namespaceName, "resourceType", rtName)
			return nil, ErrResourceTypeNotFound
		}
		s.logger.Error("Failed to get resource type", "error", err)
		return nil, fmt.Errorf("failed to get resource type: %w", err)
	}

	rt.TypeMeta = resourceTypeTypeMeta
	return rt, nil
}

func (s *resourceTypeService) DeleteResourceType(ctx context.Context, namespaceName, rtName string) error {
	s.logger.Debug("Deleting resource type", "namespace", namespaceName, "resourceType", rtName)

	rt := &openchoreov1alpha1.ResourceType{}
	rt.Name = rtName
	rt.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rt); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrResourceTypeNotFound
		}
		s.logger.Error("Failed to delete resource type CR", "error", err)
		return fmt.Errorf("failed to delete resource type: %w", err)
	}

	s.logger.Debug("Resource type deleted successfully", "namespace", namespaceName, "resourceType", rtName)
	return nil
}

func (s *resourceTypeService) GetResourceTypeSchema(ctx context.Context, namespaceName, rtName string) (map[string]any, error) {
	s.logger.Debug("Getting resource type schema", "namespace", namespaceName, "resourceType", rtName)

	rt, err := s.GetResourceType(ctx, namespaceName, rtName)
	if err != nil {
		return nil, err
	}

	rawSchema, err := schema.SectionToRawJSONSchema(rt.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved resource type schema successfully", "namespace", namespaceName, "resourceType", rtName)
	return rawSchema, nil
}

func (s *resourceTypeService) resourceTypeExists(ctx context.Context, namespaceName, rtName string) (bool, error) {
	rt := &openchoreov1alpha1.ResourceType{}
	key := client.ObjectKey{
		Name:      rtName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, rt)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of resource type %s/%s: %w", namespaceName, rtName, err)
	}
	return true, nil
}
