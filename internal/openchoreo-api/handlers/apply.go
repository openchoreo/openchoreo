// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ApplyResourceResponse represents the response for apply operations
type ApplyResourceResponse struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Operation  string `json:"operation"` // "created" or "updated" or "unchanged"
}

// ApplyResource handles POST /api/v1/apply - forwards resource to Kubernetes API like kubectl apply
func (h *Handler) ApplyResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse the raw resource payload
	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		h.logger.Error("Failed to decode apply request", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Validate resource using shared validation
	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Convert to unstructured object
	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}

	// Handle namespace logic for the resource
	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		h.logger.Error("Failed to handle resource namespace",
			"kind", kind, "name", name, "error", err)
		writeErrorResponse(w, http.StatusBadRequest,
			"Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	// Apply the resource to Kubernetes
	operation, err := h.applyToKubernetes(ctx, unstructuredObj)
	if err != nil {
		h.logger.Error("Failed to apply resource to Kubernetes",
			"kind", kind, "name", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError,
			"Failed to apply resource: "+err.Error(), services.CodeInternalError)
		return
	}

	// Return success response
	response := ApplyResourceResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  unstructuredObj.GetNamespace(), // Use the actual namespace set on the object
		Operation:  operation,
	}

	h.logger.Info("Resource applied successfully",
		"kind", kind, "name", name, "namespace", unstructuredObj.GetNamespace(), "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// applyToKubernetes applies the resource to Kubernetes cluster using server-side apply
func (h *Handler) applyToKubernetes(ctx context.Context, obj *unstructured.Unstructured) (string, error) {
	// Get the Kubernetes client from services
	k8sClient := h.services.GetKubernetesClient()

	// Create a unique field manager for choreoctl
	fieldManager := "choreoctl"

	// Check if the resource already exists using shared helper
	_, err := h.getExistingResource(ctx, obj)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return "", err
		}
		// Resource doesn't exist, create it
		if err := k8sClient.Create(ctx, obj); err != nil {
			return "", err
		}
		return "created", nil
	}

	// Resource exists, perform server-side apply (patch)
	patch := client.Apply
	patchOptions := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(fieldManager),
	}

	if err := k8sClient.Patch(ctx, obj, patch, patchOptions...); err != nil {
		return "", err
	}

	return "updated", nil
}

// DeleteResourceResponse represents the response for delete operations
type DeleteResourceResponse struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Operation  string `json:"operation"` // "deleted" or "not_found"
}

// DeleteResource handles DELETE /api/v1/delete - forwards resource deletion to Kubernetes API like kubectl delete
func (h *Handler) DeleteResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse the JSON payload
	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload", services.CodeInvalidInput)
		return
	}

	// Validate resource using shared validation
	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Convert to unstructured object
	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}

	// Handle namespace logic for the resource
	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		h.logger.Error("Failed to handle resource namespace",
			"kind", kind, "name", name, "error", err)
		writeErrorResponse(w, http.StatusBadRequest,
			"Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	// Delete the resource from Kubernetes
	operation, err := h.deleteFromKubernetes(ctx, unstructuredObj)
	if err != nil {
		h.logger.Error("Failed to delete resource from Kubernetes",
			"kind", kind, "name", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError,
			"Failed to delete resource: "+err.Error(), services.CodeInternalError)
		return
	}

	// Return success response
	response := DeleteResourceResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  unstructuredObj.GetNamespace(),
		Operation:  operation,
	}

	h.logger.Info("Resource deleted successfully",
		"kind", kind, "name", name, "namespace", unstructuredObj.GetNamespace(), "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// deleteFromKubernetes deletes the resource from Kubernetes cluster
func (h *Handler) deleteFromKubernetes(ctx context.Context, obj *unstructured.Unstructured) (string, error) {
	k8sClient := h.services.GetKubernetesClient()

	// Check if the resource exists using shared helper
	existing, err := h.getExistingResource(ctx, obj)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return "", err
		}
		// Resource doesn't exist
		return "not_found", nil
	}

	// Delete the resource
	if err := k8sClient.Delete(ctx, existing); err != nil {
		return "", err
	}

	return "deleted", nil
}

// handleResourceNamespace handles namespace logic for both cluster-scoped and namespaced resources
func (h *Handler) handleResourceNamespace(obj *unstructured.Unstructured, apiVersion, kind string) error {
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
			h.logger.Warn("Namespace specified for cluster-scoped resource, ignoring",
				"kind", kind, "name", obj.GetName(), "namespace", obj.GetNamespace())
			obj.SetNamespace("")
		}
		return nil
	}

	// For namespaced resources, apply namespace defaulting logic
	return h.handleNamespacedResource(obj, gvk)
}

// isClusterScopedResource determines if a resource is cluster-scoped
func (h *Handler) isClusterScopedResource(gvk schema.GroupVersionKind) bool {
	// List of known cluster-scoped OpenChoreo resources
	clusterScopedResources := map[string]bool{
		"Organization": true,
	}

	return clusterScopedResources[gvk.Kind]
}

// handleNamespacedResource handles namespace defaulting for namespaced resources
func (h *Handler) handleNamespacedResource(obj *unstructured.Unstructured, gvk schema.GroupVersionKind) error {
	// If namespace is already set, keep it
	if obj.GetNamespace() != "" {
		return nil
	}

	// Apply default namespace based on resource type and context
	defaultNamespace := "default"
	obj.SetNamespace(defaultNamespace)
	h.logger.Info("Applied default namespace to resource",
		"kind", gvk.Kind, "name", obj.GetName(), "namespace", defaultNamespace)

	return nil
}

// Common helper functions to eliminate duplication

// validateResourceRequest validates common fields required for both apply and delete
func validateResourceRequest(resourceObj map[string]interface{}) (string, string, string, error) {
	// Validate required fields
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

// createNamespacedName creates a NamespacedName for Kubernetes operations
func createNamespacedName(obj *unstructured.Unstructured) types.NamespacedName {
	namespacedName := types.NamespacedName{
		Name: obj.GetName(),
	}
	// Only set namespace for namespaced resources
	if obj.GetNamespace() != "" {
		namespacedName.Namespace = obj.GetNamespace()
	}
	return namespacedName
}

// getExistingResource checks if a resource exists and returns it
func (h *Handler) getExistingResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	k8sClient := h.services.GetKubernetesClient()
	
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	
	namespacedName := createNamespacedName(obj)
	err := k8sClient.Get(ctx, namespacedName, existing)
	
	return existing, err
}
