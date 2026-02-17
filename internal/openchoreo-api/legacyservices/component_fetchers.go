// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ComponentSpecFetcher interface for fetching component-specific specifications
type ComponentSpecFetcher interface {
	FetchSpec(ctx context.Context, k8sClient client.Client, namespace, componentName string) (interface{}, error)
	GetTypeName() string
}

// ComponentSpecFetcherRegistry manages all component spec fetchers
type ComponentSpecFetcherRegistry struct {
	fetchers map[string]ComponentSpecFetcher
}

// NewComponentSpecFetcherRegistry creates a new registry with all fetchers
func NewComponentSpecFetcherRegistry() *ComponentSpecFetcherRegistry {
	registry := &ComponentSpecFetcherRegistry{
		fetchers: make(map[string]ComponentSpecFetcher),
	}

	// Register all fetchers
	registry.Register(&WorkloadSpecFetcher{})

	return registry
}

// Register adds a fetcher to the registry
func (r *ComponentSpecFetcherRegistry) Register(fetcher ComponentSpecFetcher) {
	r.fetchers[fetcher.GetTypeName()] = fetcher
}

// GetFetcher retrieves a fetcher by type name
func (r *ComponentSpecFetcherRegistry) GetFetcher(typeName string) (ComponentSpecFetcher, bool) {
	fetcher, exists := r.fetchers[typeName]
	return fetcher, exists
}

type WorkloadSpecFetcher struct{}

func (f *WorkloadSpecFetcher) GetTypeName() string {
	return "Workload"
}

func (f *WorkloadSpecFetcher) FetchSpec(ctx context.Context, k8sClient client.Client, namespace, componentName string) (interface{}, error) {
	// List all Workloads in the namespace and filter by component owner
	workloadList := &openchoreov1alpha1.WorkloadList{}
	if err := k8sClient.List(ctx, workloadList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	// Find the workload that belongs to this component
	for i := range workloadList.Items {
		workload := &workloadList.Items[i]
		if workload.Spec.Owner.ComponentName == componentName {
			return &workload.Spec, nil
		}
	}

	return nil, fmt.Errorf("workload not found for component: %s", componentName)
}
