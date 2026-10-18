// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

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

// endpointTypeService handles endpoint type business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type endpointTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var endpointTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "EndpointType",
}

var _ Service = (*endpointTypeService)(nil)

// NewService creates a new endpoint type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &endpointTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *endpointTypeService) CreateEndpointType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.EndpointType) (*openchoreov1alpha1.EndpointType, error) {
	if rt == nil {
		return nil, fmt.Errorf("endpoint type cannot be nil")
	}

	s.logger.Debug("Creating endpoint type", "namespace", namespaceName, "endpointType", rt.Name)

	exists, err := s.endpointTypeExists(ctx, namespaceName, rt.Name)
	if err != nil {
		s.logger.Error("Failed to check endpoint type existence", "error", err)
		return nil, fmt.Errorf("failed to check endpoint type existence: %w", err)
	}
	if exists {
		s.logger.Warn("Endpoint type already exists", "namespace", namespaceName, "endpointType", rt.Name)
		return nil, ErrEndpointTypeAlreadyExists
	}

	rt.Namespace = namespaceName
	rt.Status = openchoreov1alpha1.EndpointTypeStatus{}
	if err := s.k8sClient.Create(ctx, rt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Endpoint type already exists", "namespace", namespaceName, "endpointType", rt.Name)
			return nil, ErrEndpointTypeAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create endpoint type CR", "error", err)
		return nil, fmt.Errorf("failed to create endpoint type: %w", err)
	}

	s.logger.Debug("Endpoint type created successfully", "namespace", namespaceName, "endpointType", rt.Name)
	rt.TypeMeta = endpointTypeTypeMeta
	return rt, nil
}

func (s *endpointTypeService) UpdateEndpointType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.EndpointType) (*openchoreov1alpha1.EndpointType, error) {
	if rt == nil {
		return nil, fmt.Errorf("endpoint type cannot be nil")
	}

	s.logger.Debug("Updating endpoint type", "namespace", namespaceName, "endpointType", rt.Name)

	existing := &openchoreov1alpha1.EndpointType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rt.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint type not found", "namespace", namespaceName, "endpointType", rt.Name)
			return nil, ErrEndpointTypeNotFound
		}
		s.logger.Error("Failed to get endpoint type", "error", err)
		return nil, fmt.Errorf("failed to get endpoint type: %w", err)
	}

	// Clear status from user input — status is server-managed
	rt.Status = openchoreov1alpha1.EndpointTypeStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = rt.Spec
	existing.Labels = rt.Labels
	existing.Annotations = rt.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update endpoint type CR", "error", err)
		return nil, fmt.Errorf("failed to update endpoint type: %w", err)
	}

	s.logger.Debug("Endpoint type updated successfully", "namespace", namespaceName, "endpointType", rt.Name)
	existing.TypeMeta = endpointTypeTypeMeta
	return existing, nil
}

func (s *endpointTypeService) ListEndpointTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointType], error) {
	s.logger.Debug("Listing endpoint types", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var rtList openchoreov1alpha1.EndpointTypeList
	if err := s.k8sClient.List(ctx, &rtList, listOpts...); err != nil {
		s.logger.Error("Failed to list endpoint types", "error", err)
		return nil, fmt.Errorf("failed to list endpoint types: %w", err)
	}

	for i := range rtList.Items {
		rtList.Items[i].TypeMeta = endpointTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.EndpointType]{
		Items:      rtList.Items,
		NextCursor: rtList.Continue,
	}
	if rtList.RemainingItemCount != nil {
		remaining := *rtList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed endpoint types", "namespace", namespaceName, "count", len(rtList.Items))
	return result, nil
}

func (s *endpointTypeService) GetEndpointType(ctx context.Context, namespaceName, etName string) (*openchoreov1alpha1.EndpointType, error) {
	s.logger.Debug("Getting endpoint type", "namespace", namespaceName, "endpointType", etName)

	rt := &openchoreov1alpha1.EndpointType{}
	key := client.ObjectKey{
		Name:      etName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rt); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint type not found", "namespace", namespaceName, "endpointType", etName)
			return nil, ErrEndpointTypeNotFound
		}
		s.logger.Error("Failed to get endpoint type", "error", err)
		return nil, fmt.Errorf("failed to get endpoint type: %w", err)
	}

	rt.TypeMeta = endpointTypeTypeMeta
	return rt, nil
}

func (s *endpointTypeService) DeleteEndpointType(ctx context.Context, namespaceName, etName string) error {
	s.logger.Debug("Deleting endpoint type", "namespace", namespaceName, "endpointType", etName)

	rt := &openchoreov1alpha1.EndpointType{}
	rt.Name = etName
	rt.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rt); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrEndpointTypeNotFound
		}
		s.logger.Error("Failed to delete endpoint type CR", "error", err)
		return fmt.Errorf("failed to delete endpoint type: %w", err)
	}

	s.logger.Debug("Endpoint type deleted successfully", "namespace", namespaceName, "endpointType", etName)
	return nil
}

func (s *endpointTypeService) GetEndpointTypeSchema(ctx context.Context, namespaceName, etName string) (map[string]any, error) {
	s.logger.Debug("Getting endpoint type schema", "namespace", namespaceName, "endpointType", etName)

	rt, err := s.GetEndpointType(ctx, namespaceName, etName)
	if err != nil {
		return nil, err
	}

	rawSchema, err := schema.SectionToRawJSONSchema(rt.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved endpoint type schema successfully", "namespace", namespaceName, "endpointType", etName)
	return rawSchema, nil
}

func (s *endpointTypeService) endpointTypeExists(ctx context.Context, namespaceName, etName string) (bool, error) {
	rt := &openchoreov1alpha1.EndpointType{}
	key := client.ObjectKey{
		Name:      etName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, rt)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of endpoint type %s/%s: %w", namespaceName, etName, err)
	}
	return true, nil
}
