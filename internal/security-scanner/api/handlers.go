// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
)

type Handler struct {
	querier backend.Querier
	logger  *slog.Logger
}

func NewHandler(querier backend.Querier, logger *slog.Logger) *Handler {
	return &Handler{
		querier: querier,
		logger:  logger,
	}
}

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("GET /api/v1/health", handler.healthHandler)
	mux.HandleFunc("GET /api/v1/posture/findings", handler.listPostureFindingsHandler)
}

func (h *Handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "v1.0.0",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) listPostureFindingsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 50
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 1000 {
			pageSize = ps
		}
	}

	offset := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	// Get total count of resources with findings
	totalCount, err := h.querier.CountResourcesWithPostureFindings(ctx)
	if err != nil {
		h.logger.Error("Failed to count resources", "error", err)
		http.Error(w, "Failed to count resources", http.StatusInternalServerError)
		return
	}

	// Get paginated list of resources with findings
	resources, err := h.querier.ListResourcesWithPostureFindings(ctx, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list resources", "error", err)
		http.Error(w, "Failed to list resources", http.StatusInternalServerError)
		return
	}

	// Build response with resource details, labels, and findings
	resourcesWithFindings := make([]ResourceWithFindings, 0, len(resources))
	for _, resource := range resources {
		// Get labels for this resource
		labels, err := h.querier.GetResourceLabels(ctx, resource.ID)
		if err != nil {
			h.logger.Error("Failed to get resource labels", "resource_id", resource.ID, "error", err)
			labels = make(map[string]string)
		}

		// Get findings for this resource
		findings, err := h.querier.GetPostureFindingsByResourceID(ctx, resource.ID)
		if err != nil {
			h.logger.Error("Failed to get resource findings", "resource_id", resource.ID, "error", err)
			continue
		}

		// Convert backend.PostureFinding to api.PostureFinding
		apiFindings := make([]PostureFinding, len(findings))
		for i, f := range findings {
			apiFindings[i] = PostureFinding{
				ID:          f.ID,
				CheckID:     f.CheckID,
				CheckName:   f.CheckName,
				Severity:    f.Severity,
				Category:    f.Category,
				Description: f.Description,
				Remediation: f.Remediation,
				CreatedAt:   f.CreatedAt,
			}
		}

		resourceWithFindings := ResourceWithFindings{
			Type:            resource.ResourceType,
			Namespace:       resource.ResourceNamespace,
			Name:            resource.ResourceName,
			UID:             resource.ResourceUID,
			ResourceVersion: resource.ResourceVersion,
			CreatedAt:       resource.CreatedAt,
			UpdatedAt:       resource.UpdatedAt,
			Labels:          labels,
			Findings:        apiFindings,
		}
		resourcesWithFindings = append(resourcesWithFindings, resourceWithFindings)
	}

	// Calculate pagination info
	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	pagination := PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: int(totalCount),
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}

	response := PostureFindingsResponse{
		Resources:  resourcesWithFindings,
		Pagination: pagination,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
