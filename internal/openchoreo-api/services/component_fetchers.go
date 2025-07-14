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
	FetchSpec(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (interface{}, error)
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

func (f *ServiceSpecFetcher) FetchSpec(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (interface{}, error) {
	service := &openchoreov1alpha1.Service{}
	if err := k8sClient.Get(ctx, key, service); err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	return &service.Spec, nil
}

// WebApplicationSpecFetcher fetches WebApplication specifications
type WebApplicationSpecFetcher struct{}

func (f *WebApplicationSpecFetcher) GetTypeName() string {
	return "WebApplication"
}

func (f *WebApplicationSpecFetcher) FetchSpec(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (interface{}, error) {
	webApp := &openchoreov1alpha1.WebApplication{}
	if err := k8sClient.Get(ctx, key, webApp); err != nil {
		return nil, fmt.Errorf("failed to get web application: %w", err)
	}
	return &webApp.Spec, nil
}

type WorkloadSpecFetcher struct{}

func (f *WorkloadSpecFetcher) GetTypeName() string {
	return "Workload"
}

func (f *WorkloadSpecFetcher) FetchSpec(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (interface{}, error) {
	workload := &openchoreov1alpha1.Workload{}
	if err := k8sClient.Get(ctx, key, workload); err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}
	return &workload.Spec, nil
}
