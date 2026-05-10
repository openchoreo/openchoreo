// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

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

var resourceTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Resource",
}

// resourceService handles resource-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type resourceService struct {
	k8sClient      client.Client
	projectService projectsvc.Service
	logger         *slog.Logger
}

var _ Service = (*resourceService)(nil)

// NewService creates a new resource service without authorization.
// It internally creates an unwrapped project service for project validation,
// avoiding double authz when used within the authz-wrapped resource service.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &resourceService{
		k8sClient:      k8sClient,
		projectService: projectsvc.NewService(k8sClient, logger.With("component", "project-service-internal")),
		logger:         logger,
	}
}

func (s *resourceService) CreateResource(ctx context.Context, namespaceName string, resource *openchoreov1alpha1.Resource) (*openchoreov1alpha1.Resource, error) {
	if resource == nil {
		return nil, fmt.Errorf("resource cannot be nil")
	}

	s.logger.Debug("Creating resource", "namespace", namespaceName, "resource", resource.Name)

	// Validate that the referenced project exists
	if _, err := s.projectService.GetProject(ctx, namespaceName, resource.Spec.Owner.ProjectName); err != nil {
		return nil, err
	}

	exists, err := s.resourceExists(ctx, namespaceName, resource.Name)
	if err != nil {
		s.logger.Error("Failed to check resource existence", "error", err)
		return nil, fmt.Errorf("failed to check resource existence: %w", err)
	}
	if exists {
		s.logger.Warn("Resource already exists", "namespace", namespaceName, "resource", resource.Name)
		return nil, ErrResourceAlreadyExists
	}

	// Set defaults
	resource.Status = openchoreov1alpha1.ResourceStatus{}
	resource.Namespace = namespaceName
	if resource.Labels == nil {
		resource.Labels = make(map[string]string)
	}
	resource.Labels[labels.LabelKeyProjectName] = resource.Spec.Owner.ProjectName

	if err := s.k8sClient.Create(ctx, resource); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Resource already exists", "namespace", namespaceName, "resource", resource.Name)
			return nil, ErrResourceAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create resource CR", "error", err)
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	s.logger.Debug("Resource created successfully", "namespace", namespaceName, "resource", resource.Name)
	resource.TypeMeta = resourceTypeMeta
	return resource, nil
}

func (s *resourceService) UpdateResource(ctx context.Context, namespaceName string, resource *openchoreov1alpha1.Resource) (*openchoreov1alpha1.Resource, error) {
	if resource == nil {
		return nil, fmt.Errorf("resource cannot be nil")
	}

	s.logger.Debug("Updating resource", "namespace", namespaceName, "resource", resource.Name)

	existing := &openchoreov1alpha1.Resource{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: resource.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource not found", "namespace", namespaceName, "resource", resource.Name)
			return nil, ErrResourceNotFound
		}
		s.logger.Error("Failed to get resource", "error", err)
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	// Clear status from user input — status is server-managed
	resource.Status = openchoreov1alpha1.ResourceStatus{}

	// Prevent project reassignment: spec.owner is immutable per CRD CEL,
	// but check explicitly so we return a friendly validation error instead
	// of a server-side admission rejection.
	if resource.Spec.Owner.ProjectName != existing.Spec.Owner.ProjectName {
		return nil, &services.ValidationError{Msg: "spec.owner.projectName is immutable"}
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = resource.Spec
	existing.Labels = resource.Labels
	existing.Annotations = resource.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update resource CR", "error", err)
		return nil, fmt.Errorf("failed to update resource: %w", err)
	}

	s.logger.Debug("Resource updated successfully", "namespace", namespaceName, "resource", resource.Name)
	existing.TypeMeta = resourceTypeMeta
	return existing, nil
}

func (s *resourceService) ListResources(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Resource], error) {
	s.logger.Debug("Listing resources", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	// Validate that the referenced project exists when filtering by project
	if projectName != "" {
		if _, err := s.projectService.GetProject(ctx, namespaceName, projectName); err != nil {
			return nil, err
		}
	}

	listResource := s.listResourcesResource(namespaceName)

	// Apply project filter if specified. PreFilteredList handles over-fetching
	// and cursor tracking so pagination remains correct.
	var filters []services.ItemFilter[openchoreov1alpha1.Resource]
	if projectName != "" {
		filters = append(filters, func(r openchoreov1alpha1.Resource) bool {
			return r.Spec.Owner.ProjectName == projectName
		})
	}

	return services.PreFilteredList(listResource, filters...)(ctx, opts)
}

// listResourcesResource returns a ListResource that fetches resources from K8s for the given namespace.
func (s *resourceService) listResourcesResource(namespaceName string) services.ListResource[openchoreov1alpha1.Resource] {
	return func(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Resource], error) {
		commonOpts, err := services.BuildListOptions(opts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var resourceList openchoreov1alpha1.ResourceList
		if err := s.k8sClient.List(ctx, &resourceList, listOpts...); err != nil {
			s.logger.Error("Failed to list resources", "error", err)
			return nil, fmt.Errorf("failed to list resources: %w", err)
		}

		for i := range resourceList.Items {
			resourceList.Items[i].TypeMeta = resourceTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.Resource]{
			Items:      resourceList.Items,
			NextCursor: resourceList.Continue,
		}
		if resourceList.RemainingItemCount != nil {
			remaining := *resourceList.RemainingItemCount
			result.RemainingCount = &remaining
		}

		return result, nil
	}
}

func (s *resourceService) GetResource(ctx context.Context, namespaceName, resourceName string) (*openchoreov1alpha1.Resource, error) {
	s.logger.Debug("Getting resource", "namespace", namespaceName, "resource", resourceName)

	resource := &openchoreov1alpha1.Resource{}
	key := client.ObjectKey{
		Name:      resourceName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, resource); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource not found", "namespace", namespaceName, "resource", resourceName)
			return nil, ErrResourceNotFound
		}
		s.logger.Error("Failed to get resource", "error", err)
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	resource.TypeMeta = resourceTypeMeta
	return resource, nil
}

func (s *resourceService) DeleteResource(ctx context.Context, namespaceName, resourceName string) error {
	s.logger.Debug("Deleting resource", "namespace", namespaceName, "resource", resourceName)

	resource := &openchoreov1alpha1.Resource{}
	resource.Name = resourceName
	resource.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrResourceNotFound
		}
		s.logger.Error("Failed to delete resource CR", "error", err)
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	s.logger.Debug("Resource deleted successfully", "namespace", namespaceName, "resource", resourceName)
	return nil
}

func (s *resourceService) resourceExists(ctx context.Context, namespaceName, resourceName string) (bool, error) {
	resource := &openchoreov1alpha1.Resource{}
	key := client.ObjectKey{
		Name:      resourceName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, resource)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of resource %s/%s: %w", namespaceName, resourceName, err)
	}
	return true, nil
}
