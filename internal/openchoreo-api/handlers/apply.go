// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	// Validate required fields
	kind, ok := resourceObj["kind"].(string)
	if !ok || kind == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid 'kind' field", services.CodeInvalidInput)
		return
	}

	apiVersion, ok := resourceObj["apiVersion"].(string)
	if !ok || apiVersion == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid 'apiVersion' field", services.CodeInvalidInput)
		return
	}

	metadata, ok := resourceObj["metadata"].(map[string]interface{})
	if !ok {
		writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid 'metadata' field", services.CodeInvalidInput)
		return
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid 'metadata.name' field", services.CodeInvalidInput)
		return
	}

	// Convert to unstructured object
	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}

	// Handle null spec fields - convert to empty object
	if spec, exists := resourceObj["spec"]; exists && spec == nil {
		resourceObj["spec"] = map[string]interface{}{}
		unstructuredObj = &unstructured.Unstructured{Object: resourceObj}
	}

	// Extract namespace (may be empty for cluster-scoped resources)
	namespace, _ := metadata["namespace"].(string)
	
	// If no namespace is specified but this is a namespaced resource, set default namespace
	if namespace == "" && unstructuredObj.GetNamespace() == "" {
		// Check if this resource type is namespaced by trying to determine from GVK
		// For OpenChoreo CRDs, most are namespaced except Organization which is cluster-scoped
		gvk := unstructuredObj.GetObjectKind().GroupVersionKind()
		if gvk.Group == "openchoreo.dev" && gvk.Kind != "Organization" && gvk.Kind != "DeploymentPipeline" {
			unstructuredObj.SetNamespace("default")
		}
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
		"kind", kind, "name", name, "namespace", namespace, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// applyToKubernetes applies the resource to Kubernetes cluster using server-side apply
func (h *Handler) applyToKubernetes(ctx context.Context, obj *unstructured.Unstructured) (string, error) {
	// Get the Kubernetes client from services
	k8sClient := h.services.GetKubernetesClient()

	// Create a unique field manager for choreoctl
	fieldManager := "choreoctl"
	
	// Check if the resource already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	
	namespacedName := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	err := k8sClient.Get(ctx, namespacedName, existing)
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