// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

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

// workloadService handles workload business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type workloadService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*workloadService)(nil)

// NewService creates a new workload service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &workloadService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *workloadService) CreateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error) {
	if w == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	s.logger.Debug("Creating workload", "namespace", namespaceName, "workload", w.Name)

	// Validate that the referenced component exists
	if err := s.validateComponentExists(ctx, namespaceName, w.Spec.Owner.ComponentName); err != nil {
		return nil, err
	}

	exists, err := s.workloadExists(ctx, namespaceName, w.Name)
	if err != nil {
		s.logger.Error("Failed to check workload existence", "error", err)
		return nil, fmt.Errorf("failed to check workload existence: %w", err)
	}
	if exists {
		s.logger.Warn("Workload already exists", "namespace", namespaceName, "workload", w.Name)
		return nil, ErrWorkloadAlreadyExists
	}

	// Set defaults
	w.TypeMeta = metav1.TypeMeta{
		Kind:       "Workload",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	w.Namespace = namespaceName
	if w.Labels == nil {
		w.Labels = make(map[string]string)
	}
	w.Labels[labels.LabelKeyNamespaceName] = namespaceName
	w.Labels[labels.LabelKeyName] = w.Name
	w.Labels[labels.LabelKeyProjectName] = w.Spec.Owner.ProjectName
	w.Labels[labels.LabelKeyComponentName] = w.Spec.Owner.ComponentName

	if err := s.k8sClient.Create(ctx, w); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Workload already exists", "namespace", namespaceName, "workload", w.Name)
			return nil, ErrWorkloadAlreadyExists
		}
		s.logger.Error("Failed to create workload CR", "error", err)
		return nil, fmt.Errorf("failed to create workload: %w", err)
	}

	s.logger.Debug("Workload created successfully", "namespace", namespaceName, "workload", w.Name)
	return w, nil
}

func (s *workloadService) UpdateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error) {
	if w == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	s.logger.Debug("Updating workload", "namespace", namespaceName, "workload", w.Name)

	existing := &openchoreov1alpha1.Workload{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: w.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found", "namespace", namespaceName, "workload", w.Name)
			return nil, ErrWorkloadNotFound
		}
		s.logger.Error("Failed to get workload", "error", err)
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	w.ResourceVersion = existing.ResourceVersion
	w.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, w); err != nil {
		s.logger.Error("Failed to update workload CR", "error", err)
		return nil, fmt.Errorf("failed to update workload: %w", err)
	}

	s.logger.Debug("Workload updated successfully", "namespace", namespaceName, "workload", w.Name)
	return w, nil
}

func (s *workloadService) ListWorkloads(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error) {
	s.logger.Debug("Listing workloads", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if pageOpts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(pageOpts.Limit)))
		}
		if pageOpts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(pageOpts.Cursor))
		}

		var wList openchoreov1alpha1.WorkloadList
		if err := s.k8sClient.List(ctx, &wList, listOpts...); err != nil {
			s.logger.Error("Failed to list workloads", "error", err)
			return nil, fmt.Errorf("failed to list workloads: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.Workload]{
			Items:      wList.Items,
			NextCursor: wList.Continue,
		}
		if wList.RemainingItemCount != nil {
			remaining := *wList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	// Apply component filter if specified
	if componentName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(w openchoreov1alpha1.Workload) bool {
				return w.Spec.Owner.ComponentName == componentName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *workloadService) GetWorkload(ctx context.Context, namespaceName, workloadName string) (*openchoreov1alpha1.Workload, error) {
	s.logger.Debug("Getting workload", "namespace", namespaceName, "workload", workloadName)

	w := &openchoreov1alpha1.Workload{}
	key := client.ObjectKey{
		Name:      workloadName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, w); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found", "namespace", namespaceName, "workload", workloadName)
			return nil, ErrWorkloadNotFound
		}
		s.logger.Error("Failed to get workload", "error", err)
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	return w, nil
}

func (s *workloadService) DeleteWorkload(ctx context.Context, namespaceName, workloadName string) error {
	s.logger.Debug("Deleting workload", "namespace", namespaceName, "workload", workloadName)

	w := &openchoreov1alpha1.Workload{}
	key := client.ObjectKey{
		Name:      workloadName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, w); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found", "namespace", namespaceName, "workload", workloadName)
			return ErrWorkloadNotFound
		}
		s.logger.Error("Failed to get workload", "error", err)
		return fmt.Errorf("failed to get workload: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, w); err != nil {
		s.logger.Error("Failed to delete workload CR", "error", err)
		return fmt.Errorf("failed to delete workload: %w", err)
	}

	s.logger.Debug("Workload deleted successfully", "namespace", namespaceName, "workload", workloadName)
	return nil
}

func (s *workloadService) workloadExists(ctx context.Context, namespaceName, workloadName string) (bool, error) {
	w := &openchoreov1alpha1.Workload{}
	key := client.ObjectKey{
		Name:      workloadName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, w)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of workload %s/%s: %w", namespaceName, workloadName, err)
	}
	return true, nil
}

func (s *workloadService) validateComponentExists(ctx context.Context, namespaceName, componentName string) error {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrComponentNotFound
		}
		return fmt.Errorf("failed to validate component: %w", err)
	}
	return nil
}
