// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// releaseLineService handles release line business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type releaseLineService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var releaseLineTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ReleaseLine",
}

var _ Service = (*releaseLineService)(nil)

// NewService creates a new release line service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &releaseLineService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// getComponent returns the component that owns the release lines.
func getComponent(ctx context.Context, c client.Reader, namespaceName, componentName string) (*openchoreov1alpha1.Component, error) {
	comp := &openchoreov1alpha1.Component{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: componentName}, comp); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrComponentNotFound
		}
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	return comp, nil
}

// ListReleaseLines returns a page of the release lines of a component.
func (s *releaseLineService) ListReleaseLines(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseLine], error) {
	s.logger.Debug("Listing release lines", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	comp, err := getComponent(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		return nil, err
	}

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseLine], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var list openchoreov1alpha1.ReleaseLineList
		if err := s.k8sClient.List(ctx, &list, listOpts...); err != nil {
			s.logger.Error("Failed to list release lines", "error", err)
			return nil, fmt.Errorf("failed to list release lines: %w", err)
		}

		for i := range list.Items {
			list.Items[i].TypeMeta = releaseLineTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.ReleaseLine]{
			Items:      list.Items,
			NextCursor: list.Continue,
		}
		if list.RemainingItemCount != nil {
			remaining := *list.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	return services.PreFilteredList(listFn, func(rl openchoreov1alpha1.ReleaseLine) bool {
		return rl.Spec.Owner.ProjectName == comp.Spec.Owner.ProjectName && rl.Spec.Owner.ComponentName == componentName
	})(ctx, opts)
}

// componentReleaseLine returns the named release line of comp, or ErrReleaseLineNotFound when the
// line does not exist or belongs to another component.
func (s *releaseLineService) componentReleaseLine(ctx context.Context, comp *openchoreov1alpha1.Component, name string) (*openchoreov1alpha1.ReleaseLine, error) {
	rl := &openchoreov1alpha1.ReleaseLine{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: comp.Namespace, Name: name}, rl); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseLineNotFound
		}
		return nil, fmt.Errorf("failed to get release line: %w", err)
	}
	if rl.Spec.Owner.ProjectName != comp.Spec.Owner.ProjectName || rl.Spec.Owner.ComponentName != comp.Name {
		return nil, ErrReleaseLineNotFound
	}
	rl.TypeMeta = releaseLineTypeMeta
	return rl, nil
}

// setReleaseLineOwner fills the owner from the component, rejecting an owner that names another component.
func setReleaseLineOwner(comp *openchoreov1alpha1.Component, rl *openchoreov1alpha1.ReleaseLine) error {
	owner := &rl.Spec.Owner
	if owner.ComponentName != "" && owner.ComponentName != comp.Name {
		return &services.ValidationError{Msg: fmt.Sprintf("spec.owner.componentName must be %q", comp.Name)}
	}
	if owner.ProjectName != "" && owner.ProjectName != comp.Spec.Owner.ProjectName {
		return &services.ValidationError{Msg: fmt.Sprintf("spec.owner.projectName must be %q", comp.Spec.Owner.ProjectName)}
	}
	owner.ComponentName = comp.Name
	owner.ProjectName = comp.Spec.Owner.ProjectName
	return nil
}

// GetReleaseLine returns a release line of a component.
func (s *releaseLineService) GetReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) (*openchoreov1alpha1.ReleaseLine, error) {
	comp, err := getComponent(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	return s.componentReleaseLine(ctx, comp, releaseLineName)
}

// CreateReleaseLine creates a release line for a component with release lines enabled.
func (s *releaseLineService) CreateReleaseLine(ctx context.Context, namespaceName, componentName string, rl *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
	if rl == nil {
		return nil, fmt.Errorf("release line cannot be nil")
	}
	s.logger.Debug("Creating release line", "namespace", namespaceName, "component", componentName, "releaseLine", rl.Name)

	comp, err := getComponent(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	if !comp.Spec.ReleaseLinesEnabled {
		return nil, ErrReleaseLinesNotEnabled
	}
	if err := setReleaseLineOwner(comp, rl); err != nil {
		return nil, err
	}

	rl.Namespace = namespaceName
	rl.Status = openchoreov1alpha1.ReleaseLineStatus{}
	if err := s.k8sClient.Create(ctx, rl); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrReleaseLineAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create release line", "error", err)
		return nil, fmt.Errorf("failed to create release line: %w", err)
	}

	rl.TypeMeta = releaseLineTypeMeta
	return rl, nil
}

// UpdateReleaseLine replaces the spec, labels and annotations of a component's release line.
func (s *releaseLineService) UpdateReleaseLine(ctx context.Context, namespaceName, componentName string, rl *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
	if rl == nil {
		return nil, fmt.Errorf("release line cannot be nil")
	}
	s.logger.Debug("Updating release line", "namespace", namespaceName, "component", componentName, "releaseLine", rl.Name)

	comp, err := getComponent(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	existing, err := s.componentReleaseLine(ctx, comp, rl.Name)
	if err != nil {
		return nil, err
	}
	if err := setReleaseLineOwner(comp, rl); err != nil {
		return nil, err
	}

	existing.Spec = rl.Spec
	existing.Labels = rl.Labels
	existing.Annotations = rl.Annotations
	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseLineNotFound
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update release line", "error", err)
		return nil, fmt.Errorf("failed to update release line: %w", err)
	}

	existing.TypeMeta = releaseLineTypeMeta
	return existing, nil
}

// DeleteReleaseLine deletes a component's release line. Its finalizer removes the line's bindings,
// releases and Workload.
func (s *releaseLineService) DeleteReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) error {
	s.logger.Debug("Deleting release line", "namespace", namespaceName, "component", componentName, "releaseLine", releaseLineName)

	comp, err := getComponent(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		return err
	}
	rl, err := s.componentReleaseLine(ctx, comp, releaseLineName)
	if err != nil {
		return err
	}
	if err := s.k8sClient.Delete(ctx, rl); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrReleaseLineNotFound
		}
		s.logger.Error("Failed to delete release line", "error", err)
		return fmt.Errorf("failed to delete release line: %w", err)
	}
	return nil
}
