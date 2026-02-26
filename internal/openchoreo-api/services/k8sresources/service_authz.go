// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewReleaseBinding   = "releasebinding:view"
	resourceTypeReleaseBinding = "releasebinding"
)

// k8sResourcesServiceWithAuthz wraps a Service and adds authorization checks.
type k8sResourcesServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*k8sResourcesServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a k8s resources service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, gatewayClient *gateway.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &k8sResourcesServiceWithAuthz{
		internal:  NewService(k8sClient, gatewayClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *k8sResourcesServiceWithAuthz) GetResourceTree(ctx context.Context, namespaceName, releaseBindingName string) (*K8sResourceTreeResult, error) {
	if err := s.checkReleaseBindingAuthz(ctx, namespaceName, releaseBindingName); err != nil {
		return nil, err
	}
	return s.internal.GetResourceTree(ctx, namespaceName, releaseBindingName)
}

func (s *k8sResourcesServiceWithAuthz) GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string) (*models.ResourceEventsResponse, error) {
	if err := s.checkReleaseBindingAuthz(ctx, namespaceName, releaseBindingName); err != nil {
		return nil, err
	}
	return s.internal.GetResourceEvents(ctx, namespaceName, releaseBindingName, group, version, kind, name)
}

func (s *k8sResourcesServiceWithAuthz) GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName, podName string, sinceSeconds *int64) (*models.ResourcePodLogsResponse, error) {
	if err := s.checkReleaseBindingAuthz(ctx, namespaceName, releaseBindingName); err != nil {
		return nil, err
	}
	return s.internal.GetResourceLogs(ctx, namespaceName, releaseBindingName, podName, sinceSeconds)
}

// checkReleaseBindingAuthz fetches the release binding and checks authorization.
func (s *k8sResourcesServiceWithAuthz) checkReleaseBindingAuthz(ctx context.Context, namespaceName, releaseBindingName string) error {
	var rb openchoreov1alpha1.ReleaseBinding
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: releaseBindingName}, &rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrReleaseBindingNotFound
		}
		return fmt.Errorf("failed to get release binding: %w", err)
	}

	return s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewReleaseBinding,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   releaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
	})
}
