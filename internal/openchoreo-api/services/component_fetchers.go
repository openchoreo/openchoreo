// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	registry.Register(&ServiceSpecFetcher{})
	registry.Register(&WebApplicationSpecFetcher{})
	registry.Register(&WorkloadSpecFetcher{})
	// Future: registry.Register(&ScheduledTaskSpecFetcher{})
	// Future: registry.Register(&APISpecFetcher{})

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

// ServiceSpecFetcher fetches Service specifications
type ServiceSpecFetcher struct{}

func (f *ServiceSpecFetcher) GetTypeName() string {
	return "Service"
}

func (f *ServiceSpecFetcher) FetchSpec(ctx context.Context, k8sClient client.Client, namespace, componentName string) (interface{}, error) {
	// List all Services in the namespace and filter by component owner
	serviceList := &openchoreov1alpha1.ServiceList{}
	if err := k8sClient.List(ctx, serviceList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Find the service that belongs to this component
	for i := range serviceList.Items {
		service := &serviceList.Items[i]
		if service.Spec.Owner.ComponentName == componentName {
			return &service.Spec, nil
		}
	}

	return nil, fmt.Errorf("service not found for component: %s", componentName)
}

// WebApplicationSpecFetcher fetches WebApplication specifications
type WebApplicationSpecFetcher struct{}

func (f *WebApplicationSpecFetcher) GetTypeName() string {
	return "WebApplication"
}

func (f *WebApplicationSpecFetcher) FetchSpec(ctx context.Context, k8sClient client.Client, namespace, componentName string) (interface{}, error) {
	// List all WebApplications in the namespace and filter by component owner
	webAppList := &openchoreov1alpha1.WebApplicationList{}
	if err := k8sClient.List(ctx, webAppList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list web applications: %w", err)
	}

	// Find the web application that belongs to this component
	for i := range webAppList.Items {
		webApp := &webAppList.Items[i]
		if webApp.Spec.Owner.ComponentName == componentName {
			return &webApp.Spec, nil
		}
	}

	return nil, fmt.Errorf("web application not found for component: %s", componentName)
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
