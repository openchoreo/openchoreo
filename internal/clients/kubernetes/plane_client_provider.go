// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// DataPlaneClientProvider provides Kubernetes clients for data plane clusters.
type DataPlaneClientProvider interface {
	DataPlaneClient(dp *openchoreov1alpha1.DataPlane) (client.Client, error)
	ClusterDataPlaneClient(cdp *openchoreov1alpha1.ClusterDataPlane) (client.Client, error)
}

// ObservabilityPlaneClientProvider provides Kubernetes clients for observability plane clusters.
type ObservabilityPlaneClientProvider interface {
	ObservabilityPlaneClient(op *openchoreov1alpha1.ObservabilityPlane) (client.Client, error)
	ClusterObservabilityPlaneClient(cop *openchoreov1alpha1.ClusterObservabilityPlane) (client.Client, error)
}

// WorkflowPlaneClientProvider provides Kubernetes clients for workflow plane clusters.
type WorkflowPlaneClientProvider interface {
	WorkflowPlaneClient(wp *openchoreov1alpha1.WorkflowPlane) (client.Client, error)
	ClusterWorkflowPlaneClient(cwp *openchoreov1alpha1.ClusterWorkflowPlane) (client.Client, error)
}

// PlaneClientProvider provides Kubernetes clients for all plane types.
// Controllers that need clients for multiple plane types accept this composite interface.
type PlaneClientProvider interface {
	DataPlaneClientProvider
	ObservabilityPlaneClientProvider
	WorkflowPlaneClientProvider
}

// planeClientProvider is the production implementation backed by KubeMultiClientManager.
type planeClientProvider struct {
	clientMgr  *KubeMultiClientManager
	gatewayURL string
}

// NewPlaneClientProvider creates a PlaneClientProvider backed by a KubeMultiClientManager.
func NewPlaneClientProvider(clientMgr *KubeMultiClientManager, gatewayURL string) PlaneClientProvider {
	return &planeClientProvider{
		clientMgr:  clientMgr,
		gatewayURL: gatewayURL,
	}
}

func (p *planeClientProvider) DataPlaneClient(dp *openchoreov1alpha1.DataPlane) (client.Client, error) {
	return GetK8sClientFromDataPlane(p.clientMgr, dp, p.gatewayURL)
}

func (p *planeClientProvider) ClusterDataPlaneClient(cdp *openchoreov1alpha1.ClusterDataPlane) (client.Client, error) {
	return GetK8sClientFromClusterDataPlane(p.clientMgr, cdp, p.gatewayURL)
}

func (p *planeClientProvider) ObservabilityPlaneClient(op *openchoreov1alpha1.ObservabilityPlane) (client.Client, error) {
	return GetK8sClientFromObservabilityPlane(p.clientMgr, op, p.gatewayURL)
}

func (p *planeClientProvider) ClusterObservabilityPlaneClient(cop *openchoreov1alpha1.ClusterObservabilityPlane) (client.Client, error) {
	return GetK8sClientFromClusterObservabilityPlane(p.clientMgr, cop, p.gatewayURL)
}

func (p *planeClientProvider) WorkflowPlaneClient(wp *openchoreov1alpha1.WorkflowPlane) (client.Client, error) {
	return GetK8sClientFromWorkflowPlane(p.clientMgr, wp, p.gatewayURL)
}

func (p *planeClientProvider) ClusterWorkflowPlaneClient(cwp *openchoreov1alpha1.ClusterWorkflowPlane) (client.Client, error) {
	return GetK8sClientFromClusterWorkflowPlane(p.clientMgr, cwp, p.gatewayURL)
}
