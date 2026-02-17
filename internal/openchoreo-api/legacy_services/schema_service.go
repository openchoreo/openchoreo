// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const objectType = "Object"

// SchemaService handles Kubernetes schema explanation operations
type SchemaService struct {
	k8sClient       client.Client
	discoveryClient *discovery.DiscoveryClient
	logger          *slog.Logger
}

// NewSchemaService creates a new schema service
func NewSchemaService(k8sClient client.Client, logger *slog.Logger) *SchemaService {
	// Get REST config
	cfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error("Failed to get kubernetes config", "error", err)
		panic(fmt.Sprintf("failed to get kubernetes config: %v", err))
	}

	// Create discovery client once
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		logger.Error("Failed to create discovery client", "error", err)
		panic(fmt.Sprintf("failed to create discovery client: %v", err))
	}

	return &SchemaService{
		k8sClient:       k8sClient,
		discoveryClient: discoveryClient,
		logger:          logger,
	}
}

// SchemaExplanation represents the structured schema information
type SchemaExplanation struct {
	Group       string                `json:"group"`
	Kind        string                `json:"kind"`
	Version     string                `json:"version"`
	Field       string                `json:"field,omitempty"`
	Type        string                `json:"type"`
	Description string                `json:"description,omitempty"`
	Properties  []PropertyDescription `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
}

// PropertyDescription represents a single field/property in the schema
type PropertyDescription struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

// ExplainSchema explains the schema of a Kubernetes resource kind
func (s *SchemaService) ExplainSchema(ctx context.Context, kind, path string) (*SchemaExplanation, error) {
	s.logger.Debug("Explaining schema", "kind", kind, "path", path)

	// Find the GVK for the given kind
	gvk, err := s.findGVKForKind(kind)
	if err != nil {
		s.logger.Warn("Failed to find resource kind", "kind", kind, "error", err)
		return nil, fmt.Errorf("failed to find resource for kind %s: %w", kind, err)
	}

	// Get OpenAPI v3 client
	openAPIv3 := s.discoveryClient.OpenAPIV3()

	// Get the group version paths
	paths, err := openAPIv3.Paths()
	if err != nil {
		s.logger.Error("Failed to get OpenAPI paths", "error", err)
		return nil, fmt.Errorf("failed to get OpenAPI paths: %w", err)
	}

	// Find the group version
	gv := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}

	// Try multiple possible path formats
	var groupVersion openapi.GroupVersion
	var ok bool

	// Try the standard format first
	groupVersion, ok = paths[gv.String()]
	if !ok {
		// Try with apis/ prefix
		groupVersion, ok = paths["apis/"+gv.String()]
	}
	if !ok {
		// Try just the version for core API
		if gvk.Group == "" {
			groupVersion, ok = paths[gvk.Version]
			if !ok {
				groupVersion, ok = paths["api/"+gvk.Version]
			}
		}
	}

	if !ok {
		return nil, fmt.Errorf("group version %s not found in OpenAPI spec", gv.String())
	}

	// Get the OpenAPI spec bytes
	openAPIBytes, err := groupVersion.Schema("application/json")
	if err != nil {
		s.logger.Error("Failed to get OpenAPI spec", "error", err)
		return nil, fmt.Errorf("failed to get OpenAPI spec: %w", err)
	}

	// Parse the OpenAPI spec
	var openAPISpec spec3.OpenAPI
	if err := json.Unmarshal(openAPIBytes, &openAPISpec); err != nil {
		s.logger.Error("Failed to parse OpenAPI spec", "error", err)
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Find the schema for this kind
	schemaRef, err := s.findSchemaForKind(&openAPISpec, gvk)
	if err != nil {
		s.logger.Warn("Failed to find schema", "kind", gvk.Kind, "error", err)
		return nil, fmt.Errorf("failed to find schema for %s: %w", gvk.Kind, err)
	}

	// Navigate to the field if path is provided
	fieldSchema := schemaRef
	fieldPath := ""
	if path != "" {
		fieldPath = path
		parts := strings.Split(path, ".")
		for _, part := range parts {
			if len(fieldSchema.Properties) == 0 {
				return nil, fmt.Errorf("field %q not found - no properties in schema", part)
			}

			// Try exact match first
			propSchema, ok := fieldSchema.Properties[part]
			if !ok {
				// Try case-insensitive match
				for key, val := range fieldSchema.Properties {
					if strings.EqualFold(key, part) {
						propSchema = val
						ok = true
						break
					}
				}
			}

			if !ok {
				return nil, fmt.Errorf("field %q not found in path", part)
			}
			fieldSchema = &propSchema
		}
	}

	// Build the explanation
	explanation := s.buildSchemaExplanation(gvk, fieldPath, fieldSchema)

	s.logger.Debug("Schema explanation completed", "kind", kind, "path", path)
	return explanation, nil
}

// findGVKForKind finds the GroupVersionKind for a given kind name
func (s *SchemaService) findGVKForKind(kind string) (schema.GroupVersionKind, error) {
	// Get all API resources
	_, apiResourceLists, err := s.discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	// Normalize kind for comparison (case-insensitive)
	normalizedKind := strings.ToLower(kind)

	// Search through all resources
	for _, apiResourceList := range apiResourceLists {
		gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			continue
		}

		for _, resource := range apiResourceList.APIResources {
			if strings.ToLower(resource.Kind) == normalizedKind {
				return schema.GroupVersionKind{
					Group:   gv.Group,
					Version: gv.Version,
					Kind:    resource.Kind,
				}, nil
			}
		}
	}

	return schema.GroupVersionKind{}, fmt.Errorf("resource kind %q not found", kind)
}

// findSchemaForKind locates the schema definition for a specific kind in the OpenAPI spec
func (s *SchemaService) findSchemaForKind(openAPISpec *spec3.OpenAPI, gvk schema.GroupVersionKind) (*spec.Schema, error) {
	if openAPISpec.Components == nil || openAPISpec.Components.Schemas == nil {
		return nil, fmt.Errorf("no schemas found in OpenAPI spec")
	}

	// Convert group to match Kubernetes OpenAPI schema naming convention
	// e.g., "openchoreo.dev" becomes "dev.openchoreo"
	groupParts := strings.Split(gvk.Group, ".")
	reversedGroup := ""
	if len(groupParts) > 0 {
		// Reverse the parts: openchoreo.dev -> dev.openchoreo
		for i := len(groupParts) - 1; i >= 0; i-- {
			if reversedGroup != "" {
				reversedGroup += "."
			}
			reversedGroup += groupParts[i]
		}
	}

	// Common schema naming patterns in Kubernetes OpenAPI V3
	possibleNames := []string{
		// Standard k8s format: dev.openchoreo.v1alpha1.Component
		fmt.Sprintf("%s.%s.%s", reversedGroup, gvk.Version, gvk.Kind),
		// Alternative formats
		fmt.Sprintf("%s.%s.%s.%s", gvk.Group, gvk.Version, gvk.Kind, gvk.Kind),
		fmt.Sprintf("%s.%s.%s", gvk.Group, gvk.Version, gvk.Kind),
		gvk.Kind,
	}

	for _, name := range possibleNames {
		if schemaRef, ok := openAPISpec.Components.Schemas[name]; ok {
			return schemaRef, nil
		}
	}

	// If exact match failed, try partial match as fallback
	for schemaName, schemaRef := range openAPISpec.Components.Schemas {
		if strings.HasSuffix(schemaName, "."+gvk.Kind) {
			return schemaRef, nil
		}
	}

	return nil, fmt.Errorf("schema for kind %s not found", gvk.Kind)
}

// buildSchemaExplanation builds the schema explanation from the schema
func (s *SchemaService) buildSchemaExplanation(gvk schema.GroupVersionKind, fieldPath string, fieldSchema *spec.Schema) *SchemaExplanation {
	explanation := &SchemaExplanation{
		Group:       gvk.Group,
		Kind:        gvk.Kind,
		Version:     gvk.Version,
		Field:       fieldPath,
		Type:        getSchemaType(fieldSchema),
		Description: fieldSchema.Description,
		Required:    fieldSchema.Required,
	}

	// Add properties if this is an object
	if len(fieldSchema.Properties) > 0 {
		explanation.Properties = s.extractProperties(fieldSchema)
	}

	return explanation
}

// extractProperties extracts property information from a schema
func (s *SchemaService) extractProperties(schema *spec.Schema) []PropertyDescription {
	if schema.Properties == nil {
		return nil
	}

	// Sort field names for consistent output
	fieldNames := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)

	properties := make([]PropertyDescription, 0, len(fieldNames))
	for _, name := range fieldNames {
		propSchema := schema.Properties[name]

		// Check if required
		isRequired := false
		if schema.Required != nil {
			for _, req := range schema.Required {
				if req == name {
					isRequired = true
					break
				}
			}
		}

		properties = append(properties, PropertyDescription{
			Name:        name,
			Type:        getSchemaType(&propSchema),
			Description: propSchema.Description,
			Required:    isRequired,
		})
	}

	return properties
}

// getSchemaType determines the type string for a schema
func getSchemaType(schema *spec.Schema) string {
	if schema == nil {
		return objectType
	}

	// Check for arrays
	if schema.Type.Contains("array") {
		if schema.Items != nil && schema.Items.Schema != nil {
			itemType := getSchemaType(schema.Items.Schema)
			return fmt.Sprintf("[]%s", itemType)
		}
		return "[]" + objectType
	}

	// Check for maps (additionalProperties)
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		valueType := getSchemaType(schema.AdditionalProperties.Schema)
		return fmt.Sprintf("map[string]%s", valueType)
	}

	// Return the type or default to Object
	if len(schema.Type) > 0 {
		return schema.Type[0]
	}

	// If it has properties, it's an object
	if len(schema.Properties) > 0 {
		return objectType
	}

	return objectType
}
