// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateObservabilityAlertsNotificationChannel = "observabilityalertsnotificationchannel:create"
	actionUpdateObservabilityAlertsNotificationChannel = "observabilityalertsnotificationchannel:update"
	actionViewObservabilityAlertsNotificationChannel   = "observabilityalertsnotificationchannel:view"
	actionDeleteObservabilityAlertsNotificationChannel = "observabilityalertsnotificationchannel:delete"

	resourceTypeObservabilityAlertsNotificationChannel = "observabilityAlertsNotificationChannel"
)

// observabilityAlertsNotificationChannelServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type observabilityAlertsNotificationChannelServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*observabilityAlertsNotificationChannelServiceWithAuthz)(nil)

// NewServiceWithAuthz creates an observability alerts notification channel service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &observabilityAlertsNotificationChannelServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *observabilityAlertsNotificationChannelServiceWithAuthz) CreateObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName string, nc *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateObservabilityAlertsNotificationChannel,
		ResourceType: resourceTypeObservabilityAlertsNotificationChannel,
		ResourceID:   nc.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateObservabilityAlertsNotificationChannel(ctx, namespaceName, nc)
}

func (s *observabilityAlertsNotificationChannelServiceWithAuthz) UpdateObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName string, nc *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateObservabilityAlertsNotificationChannel,
		ResourceType: resourceTypeObservabilityAlertsNotificationChannel,
		ResourceID:   nc.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateObservabilityAlertsNotificationChannel(ctx, namespaceName, nc)
}

func (s *observabilityAlertsNotificationChannelServiceWithAuthz) ListObservabilityAlertsNotificationChannels(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel], error) {
			return s.internal.ListObservabilityAlertsNotificationChannels(ctx, namespaceName, pageOpts)
		},
		func(nc openchoreov1alpha1.ObservabilityAlertsNotificationChannel) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewObservabilityAlertsNotificationChannel,
				ResourceType: resourceTypeObservabilityAlertsNotificationChannel,
				ResourceID:   nc.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *observabilityAlertsNotificationChannelServiceWithAuthz) GetObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewObservabilityAlertsNotificationChannel,
		ResourceType: resourceTypeObservabilityAlertsNotificationChannel,
		ResourceID:   channelName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetObservabilityAlertsNotificationChannel(ctx, namespaceName, channelName)
}

func (s *observabilityAlertsNotificationChannelServiceWithAuthz) DeleteObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteObservabilityAlertsNotificationChannel,
		ResourceType: resourceTypeObservabilityAlertsNotificationChannel,
		ResourceID:   channelName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteObservabilityAlertsNotificationChannel(ctx, namespaceName, channelName)
}
