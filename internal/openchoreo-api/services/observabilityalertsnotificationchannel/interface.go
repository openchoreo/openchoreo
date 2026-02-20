// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the observability alerts notification channel service interface.
type Service interface {
	CreateObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName string, nc *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error)
	UpdateObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName string, nc *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error)
	ListObservabilityAlertsNotificationChannels(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel], error)
	GetObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) (*openchoreov1alpha1.ObservabilityAlertsNotificationChannel, error)
	DeleteObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) error
}
