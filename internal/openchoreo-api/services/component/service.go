// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

const defaultPipelineName = "default"

// componentService handles component-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentService struct {
	k8sClient      client.Client
	projectService projectsvc.Service
	logger         *slog.Logger
}

var _ Service = (*componentService)(nil)

// NewService creates a new component service without authorization.
// It internally creates an unwrapped project service for project validation,
// avoiding double authz when used within the authz-wrapped component service.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &componentService{
		k8sClient:      k8sClient,
		projectService: projectsvc.NewService(k8sClient, logger.With("component", "project-service-internal")),
		logger:         logger,
	}
}

func (s *componentService) CreateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	s.logger.Debug("Creating component", "namespace", namespaceName, "component", component.Name)

	// Validate that the referenced project exists
	if _, err := s.projectService.GetProject(ctx, namespaceName, component.Spec.Owner.ProjectName); err != nil {
		return nil, err
	}

	exists, err := s.componentExists(ctx, namespaceName, component.Name)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component already exists", "namespace", namespaceName, "component", component.Name)
		return nil, ErrComponentAlreadyExists
	}

	// Set defaults
	component.TypeMeta = metav1.TypeMeta{
		Kind:       "Component",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	component.Namespace = namespaceName
	if component.Labels == nil {
		component.Labels = make(map[string]string)
	}
	component.Labels[labels.LabelKeyNamespaceName] = namespaceName
	component.Labels[labels.LabelKeyName] = component.Name
	component.Labels[labels.LabelKeyProjectName] = component.Spec.Owner.ProjectName

	if err := s.k8sClient.Create(ctx, component); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Component already exists", "namespace", namespaceName, "component", component.Name)
			return nil, ErrComponentAlreadyExists
		}
		s.logger.Error("Failed to create component CR", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "namespace", namespaceName, "component", component.Name)
	return component, nil
}

func (s *componentService) UpdateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	s.logger.Debug("Updating component", "namespace", namespaceName, "component", component.Name)

	existing := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: component.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", component.Name)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Prevent project reassignment: if the incoming component specifies a project,
	// it must match the existing component's project
	if component.Spec.Owner.ProjectName != "" && component.Spec.Owner.ProjectName != existing.Spec.Owner.ProjectName {
		return nil, fmt.Errorf("cannot reassign component to a different project")
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	component.ResourceVersion = existing.ResourceVersion
	component.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, component); err != nil {
		s.logger.Error("Failed to update component CR", "error", err)
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	s.logger.Debug("Component updated successfully", "namespace", namespaceName, "component", component.Name)
	return component, nil
}

func (s *componentService) ListComponents(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
	s.logger.Debug("Listing components", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	// Validate that the referenced project exists when filtering by project
	if projectName != "" {
		if _, err := s.projectService.GetProject(ctx, namespaceName, projectName); err != nil {
			return nil, err
		}
	}

	listResource := s.listComponentsResource(namespaceName)

	// Apply project filter if specified. PreFilteredList handles over-fetching
	// and cursor tracking so pagination remains correct.
	var filters []services.ItemFilter[openchoreov1alpha1.Component]
	if projectName != "" {
		filters = append(filters, func(c openchoreov1alpha1.Component) bool {
			return c.Spec.Owner.ProjectName == projectName
		})
	}

	return services.PreFilteredList(listResource, filters...)(ctx, opts)
}

// listComponentsResource returns a ListResource that fetches components from K8s for the given namespace.
func (s *componentService) listComponentsResource(namespaceName string) services.ListResource[openchoreov1alpha1.Component] {
	return func(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if opts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
		}
		if opts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(opts.Cursor))
		}

		var componentList openchoreov1alpha1.ComponentList
		if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
			s.logger.Error("Failed to list components", "error", err)
			return nil, fmt.Errorf("failed to list components: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.Component]{
			Items:      componentList.Items,
			NextCursor: componentList.Continue,
		}
		if componentList.RemainingItemCount != nil {
			remaining := *componentList.RemainingItemCount
			result.RemainingCount = &remaining
		}

		return result, nil
	}
}

func (s *componentService) GetComponent(ctx context.Context, namespaceName, componentName string) (*openchoreov1alpha1.Component, error) {
	s.logger.Debug("Getting component", "namespace", namespaceName, "component", componentName)

	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	return component, nil
}

func (s *componentService) DeleteComponent(ctx context.Context, namespaceName, componentName string) error {
	s.logger.Debug("Deleting component", "namespace", namespaceName, "component", componentName)

	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", componentName)
			return ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return fmt.Errorf("failed to get component: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, component); err != nil {
		s.logger.Error("Failed to delete component CR", "error", err)
		return fmt.Errorf("failed to delete component: %w", err)
	}

	s.logger.Debug("Component deleted successfully", "namespace", namespaceName, "component", componentName)
	return nil
}

func (s *componentService) DeployRelease(ctx context.Context, namespaceName, componentName string, req *DeployReleaseRequest) (*openchoreov1alpha1.ReleaseBinding, error) {
	if strings.TrimSpace(req.ReleaseName) == "" {
		return nil, fmt.Errorf("releaseName is required")
	}
	req.ReleaseName = strings.TrimSpace(req.ReleaseName)

	s.logger.Debug("Deploying release", "namespace", namespaceName, "component", componentName, "release", req.ReleaseName)

	// Get the component to derive the project
	component, err := s.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	projectName := component.Spec.Owner.ProjectName

	// Get the project's deployment pipeline
	project, err := s.projectService.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, err
	}

	pipelineName := project.Spec.DeploymentPipelineRef
	if pipelineName == "" {
		return nil, ErrPipelineNotConfigured
	}

	var pipeline openchoreov1alpha1.DeploymentPipeline
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: pipelineName}, &pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrPipelineNotFound
		}
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Find the lowest environment (not a target in any promotion path)
	lowestEnv := findLowestEnvironment(pipeline.Spec.PromotionPaths)
	if lowestEnv == "" {
		return nil, ErrNoLowestEnvironment
	}

	// Verify the release exists and belongs to this component
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: req.ReleaseName}, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrComponentReleaseNotFound
		}
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}
	if release.Spec.Owner.ComponentName != componentName {
		return nil, ErrComponentReleaseNotFound
	}

	// Create or update the release binding for the lowest environment
	bindingName := fmt.Sprintf("%s-%s", componentName, lowestEnv)
	var binding openchoreov1alpha1.ReleaseBinding
	err = s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: bindingName}, &binding)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("failed to get release binding: %w", err)
		}
		// Create new binding
		binding = openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: namespaceName,
				Labels: map[string]string{
					labels.LabelKeyProjectName:   projectName,
					labels.LabelKeyComponentName: componentName,
				},
			},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName:   projectName,
					ComponentName: componentName,
				},
				Environment: lowestEnv,
				ReleaseName: req.ReleaseName,
			},
		}
		if err := s.k8sClient.Create(ctx, &binding); err != nil {
			return nil, fmt.Errorf("failed to create release binding: %w", err)
		}
	} else {
		// Update existing binding
		binding.Spec.ReleaseName = req.ReleaseName
		if err := s.k8sClient.Update(ctx, &binding); err != nil {
			return nil, fmt.Errorf("failed to update release binding: %w", err)
		}
	}

	s.logger.Debug("Release deployed successfully", "namespace", namespaceName, "component", componentName, "release", req.ReleaseName, "environment", lowestEnv)
	return &binding, nil
}

func (s *componentService) PromoteComponent(ctx context.Context, namespaceName, componentName string, req *PromoteComponentRequest) (*openchoreov1alpha1.ReleaseBinding, error) {
	req.SourceEnvironment = strings.TrimSpace(req.SourceEnvironment)
	req.TargetEnvironment = strings.TrimSpace(req.TargetEnvironment)
	if req.SourceEnvironment == "" || req.TargetEnvironment == "" {
		return nil, fmt.Errorf("sourceEnv and targetEnv are required")
	}

	s.logger.Debug("Promoting component", "namespace", namespaceName, "component", componentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment)

	// Get the component to derive the project
	component, err := s.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	projectName := component.Spec.Owner.ProjectName

	// Validate the promotion path
	if err := s.validatePromotionPath(ctx, namespaceName, projectName, req.SourceEnvironment, req.TargetEnvironment); err != nil {
		return nil, err
	}

	// Get the source release binding
	sourceBinding, err := s.getReleaseBinding(ctx, namespaceName, projectName, componentName, req.SourceEnvironment)
	if err != nil {
		return nil, fmt.Errorf("failed to get source release binding: %w", err)
	}

	// Create or update the target release binding
	targetBindingName := fmt.Sprintf("%s-%s", componentName, req.TargetEnvironment)
	var targetBinding openchoreov1alpha1.ReleaseBinding
	err = s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: targetBindingName}, &targetBinding)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("failed to get target release binding: %w", err)
		}
		// Create new binding
		targetBinding = openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetBindingName,
				Namespace: namespaceName,
				Labels: map[string]string{
					labels.LabelKeyProjectName:   projectName,
					labels.LabelKeyComponentName: componentName,
				},
			},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName:   projectName,
					ComponentName: componentName,
				},
				Environment: req.TargetEnvironment,
				ReleaseName: sourceBinding.Spec.ReleaseName,
			},
		}
		if err := s.k8sClient.Create(ctx, &targetBinding); err != nil {
			return nil, fmt.Errorf("failed to create target release binding: %w", err)
		}
	} else {
		// Update existing binding
		targetBinding.Spec.ReleaseName = sourceBinding.Spec.ReleaseName
		if err := s.k8sClient.Update(ctx, &targetBinding); err != nil {
			return nil, fmt.Errorf("failed to update target release binding: %w", err)
		}
	}

	s.logger.Debug("Component promoted successfully", "namespace", namespaceName, "component", componentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment)
	return &targetBinding, nil
}

func (s *componentService) validatePromotionPath(ctx context.Context, namespaceName, projectName, sourceEnv, targetEnv string) error {
	project, err := s.projectService.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return err
	}

	pipelineName := project.Spec.DeploymentPipelineRef
	if pipelineName == "" {
		pipelineName = defaultPipelineName
	}

	var pipeline openchoreov1alpha1.DeploymentPipeline
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: pipelineName}, &pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrPipelineNotFound
		}
		return fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef == sourceEnv {
			for _, target := range path.TargetEnvironmentRefs {
				if target.Name == targetEnv {
					return nil
				}
			}
		}
	}

	return ErrInvalidPromotionPath
}

func (s *componentService) getReleaseBinding(ctx context.Context, namespaceName, projectName, componentName, environment string) (*openchoreov1alpha1.ReleaseBinding, error) {
	bindingList := &openchoreov1alpha1.ReleaseBindingList{}
	if err := s.k8sClient.List(ctx, bindingList, client.InNamespace(namespaceName)); err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	for i := range bindingList.Items {
		b := &bindingList.Items[i]
		if b.Spec.Owner.ProjectName == projectName &&
			b.Spec.Owner.ComponentName == componentName &&
			b.Spec.Environment == environment {
			return b, nil
		}
	}

	return nil, ErrReleaseBindingNotFound
}

// findLowestEnvironment finds the environment that is not a target in any promotion path.
func findLowestEnvironment(promotionPaths []openchoreov1alpha1.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}

	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}

	// Fallback: return the first source
	return promotionPaths[0].SourceEnvironmentRef
}

func (s *componentService) componentExists(ctx context.Context, namespaceName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, component)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of component %s/%s: %w", namespaceName, componentName, err)
	}
	return true, nil
}
