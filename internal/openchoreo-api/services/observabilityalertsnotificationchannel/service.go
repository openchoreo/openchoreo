// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

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

// observabilityAlertsNotificationChannelService handles business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type observabilityAlertsNotificationChannelService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*observabilityAlertsNotificationChannelService)(nil)

// NewService creates a new observability alerts notification channel service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &observabilityAlertsNotificationChannelService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *observabilityAlertsNotificationChannelService) CreateObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName string, nc *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error) {
	if nc == nil {
		return nil, fmt.Errorf("observability alerts notification channel cannot be nil")
	}

	s.logger.Debug("Creating observability alerts notification channel", "namespace", namespaceName, "channel", nc.Name)

	exists, err := s.channelExists(ctx, namespaceName, nc.Name)
	if err != nil {
		s.logger.Error("Failed to check observability alerts notification channel existence", "error", err)
		return nil, fmt.Errorf("failed to check observability alerts notification channel existence: %w", err)
	}
	if exists {
		s.logger.Warn("Observability alerts notification channel already exists", "namespace", namespaceName, "channel", nc.Name)
		return nil, ErrObservabilityAlertsNotificationChannelAlreadyExists
	}

	// Set defaults
	nc.TypeMeta = metav1.TypeMeta{
		Kind:       "ObservabilityAlertsNotificationChannel",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	nc.Namespace = namespaceName
	if nc.Labels == nil {
		nc.Labels = make(map[string]string)
	}
	nc.Labels[labels.LabelKeyNamespaceName] = namespaceName
	nc.Labels[labels.LabelKeyName] = nc.Name

	if err := s.k8sClient.Create(ctx, nc); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Observability alerts notification channel already exists", "namespace", namespaceName, "channel", nc.Name)
			return nil, ErrObservabilityAlertsNotificationChannelAlreadyExists
		}
		s.logger.Error("Failed to create observability alerts notification channel CR", "error", err)
		return nil, fmt.Errorf("failed to create observability alerts notification channel: %w", err)
	}

	s.logger.Debug("Observability alerts notification channel created successfully", "namespace", namespaceName, "channel", nc.Name)
	return nc, nil
}

func (s *observabilityAlertsNotificationChannelService) UpdateObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName string, nc *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error) {
	if nc == nil {
		return nil, fmt.Errorf("observability alerts notification channel cannot be nil")
	}

	s.logger.Debug("Updating observability alerts notification channel", "namespace", namespaceName, "channel", nc.Name)

	existing := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: nc.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Observability alerts notification channel not found", "namespace", namespaceName, "channel", nc.Name)
			return nil, ErrObservabilityAlertsNotificationChannelNotFound
		}
		s.logger.Error("Failed to get observability alerts notification channel", "error", err)
		return nil, fmt.Errorf("failed to get observability alerts notification channel: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	nc.ResourceVersion = existing.ResourceVersion
	nc.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, nc); err != nil {
		s.logger.Error("Failed to update observability alerts notification channel CR", "error", err)
		return nil, fmt.Errorf("failed to update observability alerts notification channel: %w", err)
	}

	s.logger.Debug("Observability alerts notification channel updated successfully", "namespace", namespaceName, "channel", nc.Name)
	return nc, nil
}

func (s *observabilityAlertsNotificationChannelService) ListObservabilityAlertsNotificationChannels(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel], error) {
	s.logger.Debug("Listing observability alerts notification channels", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var ncList openchoreov1alpha1.ObservabilityAlertsNotificationChannelList
	if err := s.k8sClient.List(ctx, &ncList, listOpts...); err != nil {
		s.logger.Error("Failed to list observability alerts notification channels", "error", err)
		return nil, fmt.Errorf("failed to list observability alerts notification channels: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel]{
		Items:      ncList.Items,
		NextCursor: ncList.Continue,
	}
	if ncList.RemainingItemCount != nil {
		remaining := *ncList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed observability alerts notification channels", "namespace", namespaceName, "count", len(ncList.Items))
	return result, nil
}

func (s *observabilityAlertsNotificationChannelService) GetObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error) {
	s.logger.Debug("Getting observability alerts notification channel", "namespace", namespaceName, "channel", channelName)

	nc := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}
	key := client.ObjectKey{
		Name:      channelName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, nc); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Observability alerts notification channel not found", "namespace", namespaceName, "channel", channelName)
			return nil, ErrObservabilityAlertsNotificationChannelNotFound
		}
		s.logger.Error("Failed to get observability alerts notification channel", "error", err)
		return nil, fmt.Errorf("failed to get observability alerts notification channel: %w", err)
	}

	return nc, nil
}

func (s *observabilityAlertsNotificationChannelService) DeleteObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) error {
	s.logger.Debug("Deleting observability alerts notification channel", "namespace", namespaceName, "channel", channelName)

	nc := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}
	key := client.ObjectKey{
		Name:      channelName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, nc); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Observability alerts notification channel not found", "namespace", namespaceName, "channel", channelName)
			return ErrObservabilityAlertsNotificationChannelNotFound
		}
		s.logger.Error("Failed to get observability alerts notification channel", "error", err)
		return fmt.Errorf("failed to get observability alerts notification channel: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, nc); err != nil {
		s.logger.Error("Failed to delete observability alerts notification channel CR", "error", err)
		return fmt.Errorf("failed to delete observability alerts notification channel: %w", err)
	}

	s.logger.Debug("Observability alerts notification channel deleted successfully", "namespace", namespaceName, "channel", channelName)
	return nil
}

func (s *observabilityAlertsNotificationChannelService) channelExists(ctx context.Context, namespaceName, channelName string) (bool, error) {
	nc := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}
	key := client.ObjectKey{
		Name:      channelName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, nc)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of observability alerts notification channel %s/%s: %w", namespaceName, channelName, err)
	}
	return true, nil
}
