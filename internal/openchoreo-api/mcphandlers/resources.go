// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *MCPHandler) ApplyResource(ctx context.Context, resource map[string]interface{}) (any, error) {
	// Validate resource
	kind, apiVersion, name, err := validateResourceRequest(resource)
	if err != nil {
		return nil, fmt.Errorf("invalid resource: %w", err)
	}

	// Convert to unstructured object
	unstructuredObj := &unstructured.Unstructured{Object: resource}

	// Parse GroupVersion
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %s: %w", apiVersion, err)
	}

	// Create GroupVersionKind
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kind,
	}
	unstructuredObj.SetGroupVersionKind(gvk)

	// Handle namespace logic
	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		return nil, fmt.Errorf("failed to handle resource namespace: %w", err)
	}

	// Apply the resource
	k8sClient := h.Services.GetKubernetesClient()
	fieldManager := "mcp-server"

	// Check if resource exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	namespacedName := client.ObjectKey{
		Name:      name,
		Namespace: unstructuredObj.GetNamespace(),
	}

	err = k8sClient.Get(ctx, namespacedName, existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("failed to check if resource exists: %w", err)
		}
		// Resource doesn't exist, create it
		if err := k8sClient.Create(ctx, unstructuredObj); err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}
		return map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"name":       name,
			"namespace":  unstructuredObj.GetNamespace(),
			"operation":  "created",
		}, nil
	}

	// Resource exists, perform server-side apply
	patch := client.Apply
	patchOptions := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(fieldManager),
	}

	if err := k8sClient.Patch(ctx, unstructuredObj, patch, patchOptions...); err != nil {
		return nil, fmt.Errorf("failed to update resource: %w", err)
	}

	return map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
		"namespace":  unstructuredObj.GetNamespace(),
		"operation":  "updated",
	}, nil
}

func (h *MCPHandler) DeleteResource(ctx context.Context, resource map[string]interface{}) (any, error) {
	// Validate resource
	kind, apiVersion, name, err := validateResourceRequest(resource)
	if err != nil {
		return nil, fmt.Errorf("invalid resource: %w", err)
	}

	// Convert to unstructured object
	unstructuredObj := &unstructured.Unstructured{Object: resource}

	// Parse GroupVersion
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %s: %w", apiVersion, err)
	}

	// Create GroupVersionKind
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kind,
	}
	unstructuredObj.SetGroupVersionKind(gvk)

	// Handle namespace logic
	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		return nil, fmt.Errorf("failed to handle resource namespace: %w", err)
	}

	// Delete the resource
	k8sClient := h.Services.GetKubernetesClient()

	// Check if resource exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	namespacedName := client.ObjectKey{
		Name:      name,
		Namespace: unstructuredObj.GetNamespace(),
	}

	err = k8sClient.Get(ctx, namespacedName, existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("failed to check if resource exists: %w", err)
		}
		// Resource doesn't exist
		return map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"name":       name,
			"namespace":  unstructuredObj.GetNamespace(),
			"operation":  "not_found",
		}, nil
	}

	// Delete the resource
	if err := k8sClient.Delete(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to delete resource: %w", err)
	}

	return map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
		"namespace":  unstructuredObj.GetNamespace(),
		"operation":  "deleted",
	}, nil
}

// validateResourceRequest validates common fields required for both apply and delete
func validateResourceRequest(resourceObj map[string]interface{}) (string, string, string, error) {
	kind, ok := resourceObj["kind"].(string)
	if !ok || kind == "" {
		return "", "", "", fmt.Errorf("missing or invalid 'kind' field")
	}

	apiVersion, ok := resourceObj["apiVersion"].(string)
	if !ok || apiVersion == "" {
		return "", "", "", fmt.Errorf("missing or invalid 'apiVersion' field")
	}

	// Parse and validate the group from apiVersion
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid apiVersion format '%s': %w", apiVersion, err)
	}

	// Check if the resource belongs to openchoreo.dev group
	if gv.Group != "openchoreo.dev" {
		return "", "", "", fmt.Errorf("only resources with 'openchoreo.dev' group are supported, got '%s'", gv.Group)
	}

	metadata, ok := resourceObj["metadata"].(map[string]interface{})
	if !ok {
		return "", "", "", fmt.Errorf("missing or invalid 'metadata' field")
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return "", "", "", fmt.Errorf("missing or invalid 'metadata.name' field")
	}

	return kind, apiVersion, name, nil
}

// handleResourceNamespace handles namespace logic for both cluster-scoped and namespaced resources
func (h *MCPHandler) handleResourceNamespace(obj *unstructured.Unstructured, apiVersion, kind string) error {
	// Parse the GroupVersion from apiVersion
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return fmt.Errorf("invalid apiVersion %s: %w", apiVersion, err)
	}

	// Create GroupVersionKind
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kind,
	}

	// Set the GVK on the object
	obj.SetGroupVersionKind(gvk)

	// Check if this is a cluster-scoped resource
	if h.isClusterScopedResource(gvk) {
		// For cluster-scoped resources, ensure namespace is empty
		if obj.GetNamespace() != "" {
			obj.SetNamespace("")
		}
		return nil
	}

	// For namespaced resources, apply namespace defaulting logic
	return h.handleNamespacedResource(obj)
}

// isClusterScopedResource determines if a resource is cluster-scoped
func (h *MCPHandler) isClusterScopedResource(gvk schema.GroupVersionKind) bool {
	// List of known cluster-scoped OpenChoreo resources
	clusterScopedResources := map[string]bool{
		"Namespace": true,
	}

	return clusterScopedResources[gvk.Kind]
}

// handleNamespacedResource handles namespace defaulting for namespaced resources
func (h *MCPHandler) handleNamespacedResource(obj *unstructured.Unstructured) error {
	// If namespace is already set, keep it
	if obj.GetNamespace() != "" {
		return nil
	}

	// Apply default namespace
	defaultNamespace := "default"
	obj.SetNamespace(defaultNamespace)

	return nil
}
