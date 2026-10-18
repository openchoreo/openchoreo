// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

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
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

var endpointTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Endpoint",
}

// endpointService handles endpoint-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type endpointService struct {
	k8sClient      client.Client
	projectService projectsvc.Service
	logger         *slog.Logger
}

var _ Service = (*endpointService)(nil)

// NewService creates a new endpoint service without authorization.
// It internally creates an unwrapped project service for project validation,
// avoiding double authz when used within the authz-wrapped endpoint service.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &endpointService{
		k8sClient:      k8sClient,
		projectService: projectsvc.NewService(k8sClient, logger.With("component", "project-service-internal")),
		logger:         logger,
	}
}

func (s *endpointService) CreateEndpoint(ctx context.Context, namespaceName string, endpoint *openchoreov1alpha1.Endpoint) (*openchoreov1alpha1.Endpoint, error) {
	if endpoint == nil {
		return nil, fmt.Errorf("endpoint cannot be nil")
	}

	s.logger.Debug("Creating endpoint", "namespace", namespaceName, "endpoint", endpoint.Name)

	// Validate that the referenced project exists
	if _, err := s.projectService.GetProject(ctx, namespaceName, endpoint.Spec.Owner.ProjectName); err != nil {
		return nil, err
	}

	exists, err := s.endpointExists(ctx, namespaceName, endpoint.Name)
	if err != nil {
		s.logger.Error("Failed to check endpoint existence", "error", err)
		return nil, fmt.Errorf("failed to check endpoint existence: %w", err)
	}
	if exists {
		s.logger.Warn("Endpoint already exists", "namespace", namespaceName, "endpoint", endpoint.Name)
		return nil, ErrEndpointAlreadyExists
	}

	// Set defaults
	endpoint.Status = openchoreov1alpha1.EndpointStatus{}
	endpoint.Namespace = namespaceName
	if endpoint.Labels == nil {
		endpoint.Labels = make(map[string]string)
	}
	endpoint.Labels[labels.LabelKeyProjectName] = endpoint.Spec.Owner.ProjectName

	if err := s.k8sClient.Create(ctx, endpoint); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Endpoint already exists", "namespace", namespaceName, "endpoint", endpoint.Name)
			return nil, ErrEndpointAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create endpoint CR", "error", err)
		return nil, fmt.Errorf("failed to create endpoint: %w", err)
	}

	s.logger.Debug("Endpoint created successfully", "namespace", namespaceName, "endpoint", endpoint.Name)
	endpoint.TypeMeta = endpointTypeMeta
	return endpoint, nil
}

func (s *endpointService) UpdateEndpoint(ctx context.Context, namespaceName string, endpoint *openchoreov1alpha1.Endpoint) (*openchoreov1alpha1.Endpoint, error) {
	if endpoint == nil {
		return nil, fmt.Errorf("endpoint cannot be nil")
	}

	s.logger.Debug("Updating endpoint", "namespace", namespaceName, "endpoint", endpoint.Name)

	existing := &openchoreov1alpha1.Endpoint{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: endpoint.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint not found", "namespace", namespaceName, "endpoint", endpoint.Name)
			return nil, ErrEndpointNotFound
		}
		s.logger.Error("Failed to get endpoint", "error", err)
		return nil, fmt.Errorf("failed to get endpoint: %w", err)
	}

	// Clear status from user input — status is server-managed
	endpoint.Status = openchoreov1alpha1.EndpointStatus{}

	// Prevent project reassignment: spec.owner is immutable per CRD CEL,
	// but check explicitly so we return a friendly validation error instead
	// of a server-side admission rejection.
	if endpoint.Spec.Owner.ProjectName != existing.Spec.Owner.ProjectName {
		return nil, &services.ValidationError{Msg: "spec.owner.projectName is immutable"}
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = endpoint.Spec
	existing.Labels = endpoint.Labels
	existing.Annotations = endpoint.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update endpoint CR", "error", err)
		return nil, fmt.Errorf("failed to update endpoint: %w", err)
	}

	s.logger.Debug("Endpoint updated successfully", "namespace", namespaceName, "endpoint", endpoint.Name)
	existing.TypeMeta = endpointTypeMeta
	return existing, nil
}

func (s *endpointService) ListEndpoints(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Endpoint], error) {
	s.logger.Debug("Listing endpoints", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	// Validate that the referenced project exists when filtering by project
	if projectName != "" {
		if _, err := s.projectService.GetProject(ctx, namespaceName, projectName); err != nil {
			return nil, err
		}
	}

	listEndpoint := s.listEndpointsEndpoint(namespaceName)

	// Apply project filter if specified. PreFilteredList handles over-fetching
	// and cursor tracking so pagination remains correct.
	var filters []services.ItemFilter[openchoreov1alpha1.Endpoint]
	if projectName != "" {
		filters = append(filters, func(r openchoreov1alpha1.Endpoint) bool {
			return r.Spec.Owner.ProjectName == projectName
		})
	}

	return services.PreFilteredList(listEndpoint, filters...)(ctx, opts)
}

// listEndpointsEndpoint returns a ListEndpoint that fetches endpoints from K8s for the given namespace.
func (s *endpointService) listEndpointsEndpoint(namespaceName string) services.ListResource[openchoreov1alpha1.Endpoint] {
	return func(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Endpoint], error) {
		commonOpts, err := services.BuildListOptions(opts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var endpointList openchoreov1alpha1.EndpointList
		if err := s.k8sClient.List(ctx, &endpointList, listOpts...); err != nil {
			s.logger.Error("Failed to list endpoints", "error", err)
			return nil, fmt.Errorf("failed to list endpoints: %w", err)
		}

		for i := range endpointList.Items {
			endpointList.Items[i].TypeMeta = endpointTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.Endpoint]{
			Items:      endpointList.Items,
			NextCursor: endpointList.Continue,
		}
		if endpointList.RemainingItemCount != nil {
			remaining := *endpointList.RemainingItemCount
			result.RemainingCount = &remaining
		}

		return result, nil
	}
}

func (s *endpointService) GetEndpoint(ctx context.Context, namespaceName, endpointName string) (*openchoreov1alpha1.Endpoint, error) {
	s.logger.Debug("Getting endpoint", "namespace", namespaceName, "endpoint", endpointName)

	endpoint := &openchoreov1alpha1.Endpoint{}
	key := client.ObjectKey{
		Name:      endpointName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, endpoint); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint not found", "namespace", namespaceName, "endpoint", endpointName)
			return nil, ErrEndpointNotFound
		}
		s.logger.Error("Failed to get endpoint", "error", err)
		return nil, fmt.Errorf("failed to get endpoint: %w", err)
	}

	endpoint.TypeMeta = endpointTypeMeta
	return endpoint, nil
}

func (s *endpointService) DeleteEndpoint(ctx context.Context, namespaceName, endpointName string) error {
	s.logger.Debug("Deleting endpoint", "namespace", namespaceName, "endpoint", endpointName)

	endpoint := &openchoreov1alpha1.Endpoint{}
	endpoint.Name = endpointName
	endpoint.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, endpoint); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrEndpointNotFound
		}
		s.logger.Error("Failed to delete endpoint CR", "error", err)
		return fmt.Errorf("failed to delete endpoint: %w", err)
	}

	s.logger.Debug("Endpoint deleted successfully", "namespace", namespaceName, "endpoint", endpointName)
	return nil
}

func (s *endpointService) endpointExists(ctx context.Context, namespaceName, endpointName string) (bool, error) {
	endpoint := &openchoreov1alpha1.Endpoint{}
	key := client.ObjectKey{
		Name:      endpointName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, endpoint)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of endpoint %s/%s: %w", namespaceName, endpointName, err)
	}
	return true, nil
}
