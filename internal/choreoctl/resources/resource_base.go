// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	choreoctlClient "github.com/openchoreo/openchoreo/internal/choreoctl/resources/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// ResourceBase provides common fields and functionality for all resources
type ResourceBase struct {
	namespace string
	labels    map[string]string
	config    constants.CRDConfig
	apiClient *choreoctlClient.APIClient
}

// CommonResource interface defines operations that make sense for all resources
type CommonResource interface {
	GetNamespace() string
	GetConfig() constants.CRDConfig
	SetNamespace(namespace string)
}

// Printable interface for resources that support printing/formatting
type Printable interface {
	Print(format OutputFormat, filter *ResourceFilter) error
}

// Applicable interface for resources that support applying via API
type Applicable interface {
	ApplyResource(resource map[string]interface{}) error
}

// ResourceBaseOption configures a ResourceBase
type ResourceBaseOption func(*ResourceBase)

// NewResourceBase creates a new ResourceBase with options
func NewResourceBase(opts ...ResourceBaseOption) *ResourceBase {
	base := &ResourceBase{}
	for _, opt := range opts {
		opt(base)
	}
	return base
}

// WithResourceNamespace sets the namespace for the resource
func WithResourceNamespace(namespace string) ResourceBaseOption {
	return func(base *ResourceBase) {
		base.namespace = namespace
	}
}

// WithResourceLabel sets a label for the resource
func WithResourceLabel(key, value string) ResourceBaseOption {
	return func(base *ResourceBase) {
		if base.labels == nil {
			base.labels = make(map[string]string)
		}
		base.labels[key] = value
	}
}

// WithResourceConfig sets the CRD config for the resource
func WithResourceConfig(config constants.CRDConfig) ResourceBaseOption {
	return func(base *ResourceBase) {
		base.config = config
	}
}

// WithResourceAPIClient sets the API client for the resource
func WithResourceAPIClient(apiClient *choreoctlClient.APIClient) ResourceBaseOption {
	return func(base *ResourceBase) {
		base.apiClient = apiClient
	}
}

// CommonResource interface implementations
func (base *ResourceBase) GetNamespace() string {
	return base.namespace
}

func (base *ResourceBase) GetConfig() constants.CRDConfig {
	return base.config
}

func (base *ResourceBase) SetNamespace(namespace string) {
	base.namespace = namespace
}

// GetAPIClient returns the API client for use by resource implementations
func (base *ResourceBase) GetAPIClient() *choreoctlClient.APIClient {
	return base.apiClient
}

// ApplyTypedResource applies a typed resource using the OpenChoreo API
func (base *ResourceBase) ApplyTypedResource(obj interface{}) error {
	// Convert to unstructured map for API call
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert resource to unstructured: %w", err)
	}

	// Use stored API client if available, otherwise create new one
	apiClient := base.apiClient
	if apiClient == nil {
		apiClient, err = choreoctlClient.NewAPIClient()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}
	}

	// Check API server connectivity and apply
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := apiClient.HealthCheck(ctx); err != nil {
		return fmt.Errorf("OpenChoreo API server not accessible: %w", err)
	}

	// Apply the resource using the API
	_, err = apiClient.Apply(ctx, data)
	return err
}
