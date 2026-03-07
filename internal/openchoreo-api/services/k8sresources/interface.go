// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ReleaseResourceTree represents the resource tree for a single release.
type ReleaseResourceTree struct {
	Name        string
	TargetPlane string
	Nodes       []models.ResourceNode
	Release     *openchoreov1alpha1.RenderedRelease // Full RenderedRelease CR
}

// K8sResourceTreeResult is the result of GetResourceTree.
type K8sResourceTreeResult struct {
	RenderedReleases []ReleaseResourceTree
}

// Service defines the k8s resources service interface for release bindings.
type Service interface {
	GetResourceTree(ctx context.Context, namespaceName, releaseBindingName string) (*K8sResourceTreeResult, error)
	GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string) (*models.ResourceEventsResponse, error)
	GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName, podName string, sinceSeconds *int64) (*models.ResourcePodLogsResponse, error)
}
