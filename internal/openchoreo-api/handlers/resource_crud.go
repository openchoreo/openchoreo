// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

// ResourceCRUDResponse represents the response for resource CRUD operations
type ResourceCRUDResponse struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Operation  string `json:"operation,omitempty"` // "created", "updated", "deleted", "not_found"
}

// openChoreoGVK creates a GroupVersionKind for an OpenChoreo resource
func openChoreoGVK(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "openchoreo.dev",
		Version: "v1alpha1",
		Kind:    kind,
	}
}

// buildUnstructuredRef creates an unstructured object with the specified GVK, namespace, and name
func buildUnstructuredRef(gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

// getResourceByGVK fetches a resource from Kubernetes using the specified GVK, namespace, and name
func (h *Handler) getResourceByGVK(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	k8sClient := h.services.GetKubernetesClient()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	err := k8sClient.Get(ctx, namespacedName, obj)
	return obj, err
}

// ========== ComponentType Definition Handlers ==========

// GetComponentTypeDefinition handles GET /api/v1/namespaces/{namespaceName}/component-types/{ctName}/definition
func (h *Handler) GetComponentTypeDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	ctName := r.PathValue("ctName")

	if namespaceName == "" || ctName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "ctName", ctName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and ctName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("ComponentType")
	obj, err := h.getResourceByGVK(ctx, gvk, namespaceName, ctName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Warn("ComponentType not found", "namespace", namespaceName, "name", ctName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentType not found", services.CodeComponentTypeNotFound)
			return
		}
		log.Error("Failed to get ComponentType", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get ComponentType", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved ComponentType definition", "namespace", namespaceName, "name", ctName)
	writeSuccessResponse(w, http.StatusOK, obj.Object)
}

// UpdateComponentTypeDefinition handles PUT /api/v1/namespaces/{namespaceName}/component-types/{ctName}/definition
func (h *Handler) UpdateComponentTypeDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	ctName := r.PathValue("ctName")

	if namespaceName == "" || ctName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "ctName", ctName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and ctName are required", services.CodeInvalidInput)
		return
	}

	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		log.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Validate the resource
	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Validate kind matches
	if kind != "ComponentType" {
		writeErrorResponse(w, http.StatusBadRequest, "Kind must be ComponentType", services.CodeInvalidInput)
		return
	}

	// Ensure namespace and name in URL match the resource
	if name != ctName {
		writeErrorResponse(w, http.StatusBadRequest, "Resource name does not match URL", services.CodeInvalidInput)
		return
	}

	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}

	// Set namespace from URL
	unstructuredObj.SetNamespace(namespaceName)

	// Handle namespace logic
	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		log.Error("Failed to handle resource namespace", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	// Apply the resource
	operation, err := h.applyToKubernetes(ctx, unstructuredObj)
	if err != nil {
		log.Error("Failed to apply ComponentType", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to apply ComponentType: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("ComponentType applied successfully", "namespace", namespaceName, "name", ctName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// DeleteComponentTypeDefinition handles DELETE /api/v1/namespaces/{namespaceName}/component-types/{ctName}/definition
func (h *Handler) DeleteComponentTypeDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	ctName := r.PathValue("ctName")

	if namespaceName == "" || ctName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "ctName", ctName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and ctName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("ComponentType")
	obj := buildUnstructuredRef(gvk, namespaceName, ctName)

	operation, err := h.deleteFromKubernetes(ctx, obj)
	if err != nil {
		log.Error("Failed to delete ComponentType", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete ComponentType: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       ctName,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("ComponentType deleted", "namespace", namespaceName, "name", ctName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// ========== Trait Definition Handlers ==========

// GetTraitDefinition handles GET /api/v1/namespaces/{namespaceName}/traits/{traitName}/definition
func (h *Handler) GetTraitDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	traitName := r.PathValue("traitName")

	if namespaceName == "" || traitName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "traitName", traitName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and traitName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("Trait")
	obj, err := h.getResourceByGVK(ctx, gvk, namespaceName, traitName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Warn("Trait not found", "namespace", namespaceName, "name", traitName)
			writeErrorResponse(w, http.StatusNotFound, "Trait not found", services.CodeNotFound)
			return
		}
		log.Error("Failed to get Trait", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get Trait", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved Trait definition", "namespace", namespaceName, "name", traitName)
	writeSuccessResponse(w, http.StatusOK, obj.Object)
}

// UpdateTraitDefinition handles PUT /api/v1/namespaces/{namespaceName}/traits/{traitName}/definition
func (h *Handler) UpdateTraitDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	traitName := r.PathValue("traitName")

	if namespaceName == "" || traitName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "traitName", traitName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and traitName are required", services.CodeInvalidInput)
		return
	}

	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		log.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	if kind != "Trait" {
		writeErrorResponse(w, http.StatusBadRequest, "Kind must be Trait", services.CodeInvalidInput)
		return
	}

	if name != traitName {
		writeErrorResponse(w, http.StatusBadRequest, "Resource name does not match URL", services.CodeInvalidInput)
		return
	}

	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}
	unstructuredObj.SetNamespace(namespaceName)

	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		log.Error("Failed to handle resource namespace", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	operation, err := h.applyToKubernetes(ctx, unstructuredObj)
	if err != nil {
		log.Error("Failed to apply Trait", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to apply Trait: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("Trait applied successfully", "namespace", namespaceName, "name", traitName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// DeleteTraitDefinition handles DELETE /api/v1/namespaces/{namespaceName}/traits/{traitName}/definition
func (h *Handler) DeleteTraitDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	traitName := r.PathValue("traitName")

	if namespaceName == "" || traitName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "traitName", traitName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and traitName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("Trait")
	obj := buildUnstructuredRef(gvk, namespaceName, traitName)

	operation, err := h.deleteFromKubernetes(ctx, obj)
	if err != nil {
		log.Error("Failed to delete Trait", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete Trait: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       traitName,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("Trait deleted", "namespace", namespaceName, "name", traitName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// ========== Workflow Definition Handlers ==========

// GetWorkflowDefinition handles GET /api/v1/namespaces/{namespaceName}/workflows/{workflowName}/definition
func (h *Handler) GetWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	workflowName := r.PathValue("workflowName")

	if namespaceName == "" || workflowName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "workflowName", workflowName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and workflowName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("Workflow")
	obj, err := h.getResourceByGVK(ctx, gvk, namespaceName, workflowName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Warn("Workflow not found", "namespace", namespaceName, "name", workflowName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow not found", services.CodeNotFound)
			return
		}
		log.Error("Failed to get Workflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get Workflow", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved Workflow definition", "namespace", namespaceName, "name", workflowName)
	writeSuccessResponse(w, http.StatusOK, obj.Object)
}

// UpdateWorkflowDefinition handles PUT /api/v1/namespaces/{namespaceName}/workflows/{workflowName}/definition
func (h *Handler) UpdateWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	workflowName := r.PathValue("workflowName")

	if namespaceName == "" || workflowName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "workflowName", workflowName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and workflowName are required", services.CodeInvalidInput)
		return
	}

	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		log.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	if kind != "Workflow" {
		writeErrorResponse(w, http.StatusBadRequest, "Kind must be Workflow", services.CodeInvalidInput)
		return
	}

	if name != workflowName {
		writeErrorResponse(w, http.StatusBadRequest, "Resource name does not match URL", services.CodeInvalidInput)
		return
	}

	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}
	unstructuredObj.SetNamespace(namespaceName)

	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		log.Error("Failed to handle resource namespace", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	operation, err := h.applyToKubernetes(ctx, unstructuredObj)
	if err != nil {
		log.Error("Failed to apply Workflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to apply Workflow: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("Workflow applied successfully", "namespace", namespaceName, "name", workflowName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// DeleteWorkflowDefinition handles DELETE /api/v1/namespaces/{namespaceName}/workflows/{workflowName}/definition
func (h *Handler) DeleteWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	workflowName := r.PathValue("workflowName")

	if namespaceName == "" || workflowName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "workflowName", workflowName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and workflowName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("Workflow")
	obj := buildUnstructuredRef(gvk, namespaceName, workflowName)

	operation, err := h.deleteFromKubernetes(ctx, obj)
	if err != nil {
		log.Error("Failed to delete Workflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete Workflow: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       workflowName,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("Workflow deleted", "namespace", namespaceName, "name", workflowName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// ========== ComponentWorkflow Definition Handlers ==========

// GetComponentWorkflowDefinition handles GET /api/v1/namespaces/{namespaceName}/component-workflows/{cwName}/definition
func (h *Handler) GetComponentWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	cwName := r.PathValue("cwName")

	if namespaceName == "" || cwName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "cwName", cwName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and cwName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("ComponentWorkflow")
	obj, err := h.getResourceByGVK(ctx, gvk, namespaceName, cwName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Warn("ComponentWorkflow not found", "namespace", namespaceName, "name", cwName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentWorkflow not found", services.CodeNotFound)
			return
		}
		log.Error("Failed to get ComponentWorkflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get ComponentWorkflow", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved ComponentWorkflow definition", "namespace", namespaceName, "name", cwName)
	writeSuccessResponse(w, http.StatusOK, obj.Object)
}

// UpdateComponentWorkflowDefinition handles PUT /api/v1/namespaces/{namespaceName}/component-workflows/{cwName}/definition
func (h *Handler) UpdateComponentWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	cwName := r.PathValue("cwName")

	if namespaceName == "" || cwName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "cwName", cwName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and cwName are required", services.CodeInvalidInput)
		return
	}

	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		log.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	if kind != "ComponentWorkflow" {
		writeErrorResponse(w, http.StatusBadRequest, "Kind must be ComponentWorkflow", services.CodeInvalidInput)
		return
	}

	if name != cwName {
		writeErrorResponse(w, http.StatusBadRequest, "Resource name does not match URL", services.CodeInvalidInput)
		return
	}

	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}
	unstructuredObj.SetNamespace(namespaceName)

	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		log.Error("Failed to handle resource namespace", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	operation, err := h.applyToKubernetes(ctx, unstructuredObj)
	if err != nil {
		log.Error("Failed to apply ComponentWorkflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to apply ComponentWorkflow: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("ComponentWorkflow applied successfully", "namespace", namespaceName, "name", cwName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// DeleteComponentWorkflowDefinition handles DELETE /api/v1/namespaces/{namespaceName}/component-workflows/{cwName}/definition
func (h *Handler) DeleteComponentWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	cwName := r.PathValue("cwName")

	if namespaceName == "" || cwName == "" {
		log.Warn("Missing required path parameters", "namespaceName", namespaceName, "cwName", cwName)
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName and cwName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK("ComponentWorkflow")
	obj := buildUnstructuredRef(gvk, namespaceName, cwName)

	operation, err := h.deleteFromKubernetes(ctx, obj)
	if err != nil {
		log.Error("Failed to delete ComponentWorkflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete ComponentWorkflow: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       cwName,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("ComponentWorkflow deleted", "namespace", namespaceName, "name", cwName, "operation", operation)
	writeSuccessResponse(w, http.StatusOK, response)
}

// ========== Generic Resource Handlers ==========

// GetResource handles GET /api/v1/namespaces/{namespaceName}/resources/{kind}/{resourceName}
func (h *Handler) GetResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	namespaceName := r.PathValue("namespaceName")
	kind := r.PathValue("kind")
	resourceName := r.PathValue("resourceName")

	// Add common fields to logger context
	log = log.With("namespace", namespaceName, "kind", kind, "name", resourceName)

	if namespaceName == "" || kind == "" || resourceName == "" {
		log.Error("Missing required path parameters")
		writeErrorResponse(w, http.StatusBadRequest, "namespaceName, kind and resourceName are required", services.CodeInvalidInput)
		return
	}

	gvk := openChoreoGVK(kind)
	obj, err := h.getResourceByGVK(ctx, gvk, namespaceName, resourceName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Error("Resource not found")
			writeErrorResponse(w, http.StatusNotFound, "Resource not found", services.CodeNotFound)
			return
		}
		// Check if this is a RESTMapper error (unsupported/unknown kind)
		if meta.IsNoMatchError(err) {
			log.Error("Unsupported or unknown resource kind", "error", err)
			writeErrorResponse(w, http.StatusBadRequest, "Unsupported or unknown resource kind: "+kind, services.CodeInvalidInput)
			return
		}
		log.Error("Failed to get resource", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get resource", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved resource")
	writeSuccessResponse(w, http.StatusOK, obj.Object)
}
