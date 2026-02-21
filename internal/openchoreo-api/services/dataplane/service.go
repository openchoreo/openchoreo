// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

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

// dataPlaneService handles data plane-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type dataPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*dataPlaneService)(nil)

// NewService creates a new data plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &dataPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *dataPlaneService) ListDataPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DataPlane], error) {
	s.logger.Debug("Listing data planes", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var dataPlaneList openchoreov1alpha1.DataPlaneList
	if err := s.k8sClient.List(ctx, &dataPlaneList, listOpts...); err != nil {
		s.logger.Error("Failed to list data planes", "error", err)
		return nil, fmt.Errorf("failed to list data planes: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.DataPlane]{
		Items:      dataPlaneList.Items,
		NextCursor: dataPlaneList.Continue,
	}
	if dataPlaneList.RemainingItemCount != nil {
		remaining := *dataPlaneList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed data planes", "namespace", namespaceName, "count", len(dataPlaneList.Items))
	return result, nil
}

func (s *dataPlaneService) GetDataPlane(ctx context.Context, namespaceName, dpName string) (*openchoreov1alpha1.DataPlane, error) {
	s.logger.Debug("Getting data plane", "namespace", namespaceName, "dataPlane", dpName)

	dataPlane := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, dataPlane); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Data plane not found", "namespace", namespaceName, "dataPlane", dpName)
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get data plane", "error", err)
		return nil, fmt.Errorf("failed to get data plane: %w", err)
	}

	return dataPlane, nil
}

func (s *dataPlaneService) CreateDataPlane(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.DataPlane, error) {
	if dp == nil {
		return nil, fmt.Errorf("data plane cannot be nil")
	}

	s.logger.Debug("Creating data plane", "namespace", namespaceName, "dataPlane", dp.Name)

	exists, err := s.dataPlaneExists(ctx, namespaceName, dp.Name)
	if err != nil {
		s.logger.Error("Failed to check data plane existence", "error", err)
		return nil, fmt.Errorf("failed to check data plane existence: %w", err)
	}
	if exists {
		s.logger.Warn("Data plane already exists", "namespace", namespaceName, "dataPlane", dp.Name)
		return nil, ErrDataPlaneAlreadyExists
	}

	// Set defaults
	dp.TypeMeta = metav1.TypeMeta{
		Kind:       "DataPlane",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	dp.Namespace = namespaceName
	if dp.Labels == nil {
		dp.Labels = make(map[string]string)
	}
	dp.Labels[labels.LabelKeyNamespaceName] = namespaceName
	dp.Labels[labels.LabelKeyName] = dp.Name

	if err := s.k8sClient.Create(ctx, dp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Data plane already exists", "namespace", namespaceName, "dataPlane", dp.Name)
			return nil, ErrDataPlaneAlreadyExists
		}
		s.logger.Error("Failed to create data plane CR", "error", err)
		return nil, fmt.Errorf("failed to create data plane: %w", err)
	}

	s.logger.Debug("Data plane created successfully", "namespace", namespaceName, "dataPlane", dp.Name)
	return dp, nil
}

func (s *dataPlaneService) UpdateDataPlane(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.DataPlane, error) {
	if dp == nil {
		return nil, ErrDataPlaneNil
	}

	s.logger.Debug("Updating data plane", "namespace", namespaceName, "dataPlane", dp.Name)

	existing := &openchoreov1alpha1.DataPlane{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: dp.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get data plane", "error", err)
		return nil, fmt.Errorf("failed to get data plane: %w", err)
	}

	// Preserve server-managed fields
	dp.ResourceVersion = existing.ResourceVersion
	dp.Namespace = namespaceName
	// Ensure required labels are set
	if dp.Labels == nil {
		dp.Labels = make(map[string]string)
	}
	dp.Labels[labels.LabelKeyNamespaceName] = namespaceName
	dp.Labels[labels.LabelKeyName] = dp.Name

	if err := s.k8sClient.Update(ctx, dp); err != nil {
		s.logger.Error("Failed to update data plane CR", "error", err)
		return nil, fmt.Errorf("failed to update data plane: %w", err)
	}

	s.logger.Debug("Data plane updated successfully", "namespace", namespaceName, "dataPlane", dp.Name)
	return dp, nil
}

func (s *dataPlaneService) DeleteDataPlane(ctx context.Context, namespaceName, dpName string) error {
	s.logger.Debug("Deleting data plane", "namespace", namespaceName, "dataPlane", dpName)

	dp := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Data plane not found", "namespace", namespaceName, "dataPlane", dpName)
			return ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get data plane", "error", err)
		return fmt.Errorf("failed to get data plane: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, dp); err != nil {
		s.logger.Error("Failed to delete data plane CR", "error", err)
		return fmt.Errorf("failed to delete data plane: %w", err)
	}

	s.logger.Debug("Data plane deleted successfully", "namespace", namespaceName, "dataPlane", dpName)
	return nil
}

func (s *dataPlaneService) dataPlaneExists(ctx context.Context, namespaceName, dpName string) (bool, error) {
	dataPlane := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, dataPlane)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of data plane %s/%s: %w", namespaceName, dpName, err)
	}
	return true, nil
}
