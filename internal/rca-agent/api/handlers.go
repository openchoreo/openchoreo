// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	rcaAuthz "github.com/openchoreo/openchoreo/internal/rca-agent/authz"
	"github.com/openchoreo/openchoreo/internal/rca-agent/prompts"
	"github.com/openchoreo/openchoreo/internal/rca-agent/service"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

// AgentService defines the service methods that the HTTP handlers depend on.
type AgentService interface {
	ResolveComponentScope(ctx context.Context, namespace, project, component, environment string) (*service.Scope, error)
	ResolveProjectScope(ctx context.Context, namespace, project, environment string) (*service.Scope, error)
	RunAnalysis(params *service.AnalysisParams)
	StreamChat(ctx context.Context, bearerToken string, params *service.ChatParams) <-chan service.ChatEvent
}

// Handler implements the RCA agent HTTP API.
type Handler struct {
	logger            *slog.Logger
	reportStore       store.ReportStore
	authzClient       authzcore.PDP
	service           AgentService
	streamWriteTimeout time.Duration
}

// NewHandler creates a new API handler.
func NewHandler(logger *slog.Logger, reportStore store.ReportStore, authzClient authzcore.PDP, svc AgentService, streamWriteTimeout time.Duration) *Handler {
	return &Handler{
		logger:            logger,
		reportStore:       reportStore,
		authzClient:       authzClient,
		service:           svc,
		streamWriteTimeout: streamWriteTimeout,
	}
}

// Analyze handles POST /api/v1alpha1/rca-agent/analyze.
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Namespace == "" || req.Project == "" || req.Component == "" || req.Environment == "" {
		writeError(w, http.StatusBadRequest, "namespace, project, component, and environment are required")
		return
	}

	if req.Alert.ID == "" {
		writeError(w, http.StatusBadRequest, "alert.id is required")
		return
	}

	h.logger.Info("analyze request received",
		"namespace", req.Namespace,
		"project", req.Project,
		"component", req.Component,
		"alert_id", req.Alert.ID,
	)

	// Resolve scope (names → UIDs) before creating the pending record.
	// Scope must be resolved before creating the pending report.
	scope, err := h.service.ResolveComponentScope(r.Context(),
		req.Namespace, req.Project, req.Component, req.Environment,
	)
	if err != nil {
		h.logger.Error("failed to resolve scope", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve component scope")
		return
	}

	// Generate report ID from alert ID + timestamp.
	reportID := fmt.Sprintf("%s_%d", req.Alert.ID, time.Now().Unix())

	if err := h.reportStore.UpsertReport(r.Context(), &store.ReportEntry{
		ReportID:        reportID,
		AlertID:         req.Alert.ID,
		Status:          "pending",
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		NamespaceName:   req.Namespace,
		ProjectName:     req.Project,
		EnvironmentName: req.Environment,
		ComponentName:   req.Component,
		EnvironmentUID:  scope.EnvironmentUID,
		ProjectUID:      scope.ProjectUID,
	}); err != nil {
		h.logger.Error("failed to create pending report", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create report")
		return
	}

	// Launch background analysis with scope already resolved.
	// No authz check: this endpoint is served on the internal port only.
	go h.service.RunAnalysis(toAnalysisParams(reportID, &req, scope))

	writeJSON(w, http.StatusOK, RCAResponse{
		ReportID: reportID,
		Status:   "pending",
	})
}

// Chat handles POST /api/v1alpha1/rca-agent/chat.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.ReportID == "" {
		writeError(w, http.StatusBadRequest, "reportId is required")
		return
	}

	if req.Namespace == "" || req.Project == "" {
		writeError(w, http.StatusBadRequest, "namespace and project are required")
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages must not be empty")
		return
	}

	if len(req.Messages) > 50 {
		writeError(w, http.StatusBadRequest, "messages must not exceed 50")
		return
	}

	for i, msg := range req.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("message %d has invalid role %q: must be \"user\" or \"assistant\"", i, msg.Role))
			return
		}
		if len(msg.Content) > 10000 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("message %d content exceeds 10000 characters", i))
			return
		}
	}

	// Authorize against caller-supplied scope (names, not UIDs).
	resourceType, resourceID, hierarchy := rcaAuthz.ComponentScopeAuthz(req.Namespace, req.Project, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionViewRCAReport, resourceType, resourceID, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	h.logger.Info("chat request received",
		"report_id", req.ReportID,
		"namespace", req.Namespace,
		"message_count", len(req.Messages),
	)

	entry, err := h.reportStore.GetReport(r.Context(), req.ReportID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		h.logger.Error("failed to get report for chat", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get report")
		return
	}

	if entry.NamespaceName != "" && entry.NamespaceName != req.Namespace {
		writeError(w, http.StatusForbidden, "report does not belong to the specified namespace")
		return
	}
	if entry.ProjectName != "" && entry.ProjectName != req.Project {
		writeError(w, http.StatusForbidden, "report does not belong to the specified project")
		return
	}

	// Parse report context if available.
	var reportContext any
	if entry.Report != nil {
		var parsed any
		if err := json.Unmarshal([]byte(*entry.Report), &parsed); err == nil {
			reportContext = parsed
		}
	}

	scope := &prompts.Scope{
		Namespace:   req.Namespace,
		Environment: req.Environment,
		Project:     req.Project,
	}

	// Stream NDJSON events.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(h.streamWriteTimeout))

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	// Extract user's bearer token from JWT middleware for API auth.
	bearerToken, _ := jwt.GetToken(r)

	chatParams := toChatParams(&req, reportContext, scope)
	chatEvents := h.service.StreamChat(r.Context(), bearerToken, chatParams)

	enc := json.NewEncoder(w)
	for ev := range chatEvents {
		if err := enc.Encode(ev); err != nil {
			break
		}
		_ = rc.Flush()
	}
}

// ListReports handles GET /api/v1/rca-agent/reports.
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	project := q.Get("project")
	environment := q.Get("environment")
	namespace := q.Get("namespace")
	startTime := q.Get("startTime")
	endTime := q.Get("endTime")

	if project == "" || environment == "" || namespace == "" || startTime == "" || endTime == "" {
		writeError(w, http.StatusUnprocessableEntity, "project, environment, namespace, startTime, and endTime are required")
		return
	}

	// Validate time range.
	startParsed, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid startTime format, expected RFC3339")
		return
	}
	endParsed, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endTime format, expected RFC3339")
		return
	}
	if !startParsed.Before(endParsed) {
		writeError(w, http.StatusBadRequest, "startTime must be before endTime")
		return
	}

	// Authorize
	resourceType, resourceID, hierarchy := rcaAuthz.ComponentScopeAuthz(namespace, project, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionViewRCAReport, resourceType, resourceID, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	// Resolve scope (names → UIDs) for querying.
	scope, err := h.service.ResolveProjectScope(r.Context(), namespace, project, environment)
	if err != nil {
		h.logger.Error("failed to resolve scope for list", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve project scope")
		return
	}

	limit := 100
	if l := q.Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 || parsed > 10000 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 10000")
			return
		}
		limit = parsed
	}

	sort := "DESC"
	if s := q.Get("sort"); s != "" {
		switch s {
		case "asc":
			sort = "ASC"
		case "desc":
			sort = "DESC"
		default:
			writeError(w, http.StatusBadRequest, "sort must be asc or desc")
			return
		}
	}

	status := q.Get("status")
	if status != "" && status != "pending" && status != "completed" && status != "failed" {
		writeError(w, http.StatusBadRequest, "status must be pending, completed, or failed")
		return
	}

	entries, total, err := h.reportStore.ListReports(r.Context(), store.QueryParams{
		ProjectUID:     scope.ProjectUID,
		EnvironmentUID: scope.EnvironmentUID,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		SortOrder:      sort,
		Status:         status,
	})
	if err != nil {
		h.logger.Error("failed to list reports", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list reports")
		return
	}

	reports := make([]RCAReportSummary, 0, len(entries))
	for _, e := range entries {
		reports = append(reports, RCAReportSummary{
			AlertID:   e.AlertID,
			ReportID:  e.ReportID,
			Timestamp: e.Timestamp,
			Summary:   e.Summary,
			Status:    e.Status,
		})
	}

	writeJSON(w, http.StatusOK, RCAReportsResponse{
		Reports:    reports,
		TotalCount: total,
	})
}

// GetReport handles GET /api/v1/rca-agent/reports/{report_id}.
func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("report_id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "report_id is required")
		return
	}

	entry, err := h.reportStore.GetReport(r.Context(), reportID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		h.logger.Error("failed to get report", "report_id", reportID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get report")
		return
	}

	// Authorize using the report's stored project
	resourceType, resourceID2, hierarchy := rcaAuthz.ComponentScopeAuthz(entry.NamespaceName, entry.ProjectName, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionViewRCAReport, resourceType, resourceID2, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	resp := RCAReportDetailed{
		AlertID:   entry.AlertID,
		ReportID:  entry.ReportID,
		Timestamp: entry.Timestamp,
		Status:    entry.Status,
	}

	if entry.Report != nil {
		var report RCAReport
		if err := json.Unmarshal([]byte(*entry.Report), &report); err != nil {
			h.logger.Error("failed to unmarshal report JSON", "report_id", reportID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to parse stored report")
			return
		}
		resp.Report = &report
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateReport handles PUT /api/v1/rca-agent/reports/{report_id}.
func (h *Handler) UpdateReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("report_id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "report_id is required")
		return
	}

	var req ReportUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if hasOverlap(req.AppliedIndices, req.DismissedIndices) {
		writeError(w, http.StatusBadRequest, "appliedIndices and dismissedIndices must not overlap")
		return
	}

	// Get existing report
	entry, err := h.reportStore.GetReport(r.Context(), reportID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		h.logger.Error("failed to get report for update", "report_id", reportID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get report")
		return
	}

	// Authorize using the report's stored project
	resourceType, resourceID2, hierarchy := rcaAuthz.ComponentScopeAuthz(entry.NamespaceName, entry.ProjectName, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionUpdateRCAReport, resourceType, resourceID2, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	if entry.Report == nil {
		writeError(w, http.StatusBadRequest, "report has no content to update")
		return
	}

	// Parse the stored report JSON, apply status changes, re-serialize
	var reportData map[string]any
	if err := json.Unmarshal([]byte(*entry.Report), &reportData); err != nil {
		h.logger.Error("failed to unmarshal report for update", "report_id", reportID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to parse stored report")
		return
	}

	// Update action statuses in the report (only valid transitions)
	changed := applyActionStatusUpdates(reportData, req.AppliedIndices, req.DismissedIndices)

	if changed {
		updatedJSON, err := json.Marshal(reportData)
		if err != nil {
			h.logger.Error("failed to marshal updated report", "report_id", reportID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to serialize updated report")
			return
		}

		if err := h.reportStore.UpdateActionStatuses(r.Context(), reportID, string(updatedJSON)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "report not found")
				return
			}
			h.logger.Error("failed to update report", "report_id", reportID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update report")
			return
		}
	}

	writeJSON(w, http.StatusOK, StatusResponse{Status: "ok"})
}

// Health handles GET /health.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, StatusResponse{Status: "healthy"})
}

// applyActionStatusUpdates modifies the recommended_actions in the report JSON.
// Returns true if any changes were made. Only performs valid transitions:
//   - applied: revised → applied
//   - dismissed: revised|suggested → dismissed
func applyActionStatusUpdates(report map[string]any, applied, dismissed []int) bool {
	result, ok := report["result"].(map[string]any)
	if !ok {
		return false
	}

	recommendations, ok := result["recommendations"].(map[string]any)
	if !ok {
		return false
	}

	actions, ok := recommendations["recommended_actions"].([]any)
	if !ok {
		return false
	}

	appliedSet := make(map[int]struct{}, len(applied))
	for _, idx := range applied {
		appliedSet[idx] = struct{}{}
	}

	dismissedSet := make(map[int]struct{}, len(dismissed))
	for _, idx := range dismissed {
		dismissedSet[idx] = struct{}{}
	}

	changed := false
	for i, raw := range actions {
		action, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		current, _ := action["status"].(string)

		if _, inApplied := appliedSet[i]; inApplied && current == "revised" {
			action["status"] = "applied"
			changed = true
		} else if _, inDismissed := dismissedSet[i]; inDismissed && (current == "revised" || current == "suggested") {
			action["status"] = "dismissed"
			changed = true
		}
	}

	return changed
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, ErrorResponse{Detail: detail})
}

func handleAuthzError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rcaAuthz.ErrAuthzUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, rcaAuthz.ErrAuthzForbidden):
		writeError(w, http.StatusForbidden, "insufficient permissions")
	case errors.Is(err, rcaAuthz.ErrAuthzServiceUnavailable):
		writeError(w, http.StatusServiceUnavailable, "authorization service unavailable")
	default:
		writeError(w, http.StatusInternalServerError, "authorization check failed")
	}
}

func hasOverlap(a, b []int) bool {
	set := make(map[int]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; ok {
			return true
		}
	}
	return false
}

func toAnalysisParams(reportID string, req *AnalyzeRequest, scope *service.Scope) *service.AnalysisParams {
	params := &service.AnalysisParams{
		ReportID: reportID,
		AlertID:  req.Alert.ID,
		Scope:    scope,
		Alert: service.AlertParams{
			ID:        req.Alert.ID,
			Timestamp: req.Alert.Timestamp,
			Rule: service.AlertRuleParams{
				Name: req.Alert.Rule.Name,
			},
		},
		Meta: req.Meta,
	}

	if v, ok := req.Alert.Value.(float64); ok {
		params.Alert.Value = v
	}
	if req.Alert.Rule.Description != nil {
		params.Alert.Rule.Description = *req.Alert.Rule.Description
	}
	if req.Alert.Rule.Severity != nil {
		params.Alert.Rule.Severity = *req.Alert.Rule.Severity
	}
	if req.Alert.Rule.Source != nil {
		params.Alert.Rule.Source = &service.AlertSourceParams{Type: req.Alert.Rule.Source.Type}
		if req.Alert.Rule.Source.Query != nil {
			params.Alert.Rule.Source.Query = *req.Alert.Rule.Source.Query
		}
		if req.Alert.Rule.Source.Metric != nil {
			params.Alert.Rule.Source.Metric = *req.Alert.Rule.Source.Metric
		}
	}
	if req.Alert.Rule.Condition != nil {
		params.Alert.Rule.Condition = &service.AlertConditionParams{
			Window:    req.Alert.Rule.Condition.Window,
			Interval:  req.Alert.Rule.Condition.Interval,
			Operator:  req.Alert.Rule.Condition.Operator,
			Threshold: req.Alert.Rule.Condition.Threshold,
		}
	}

	return params
}

func toChatParams(req *ChatRequest, reportContext any, scope *prompts.Scope) *service.ChatParams {
	msgs := make([]service.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = service.ChatMessage{Role: m.Role, Content: m.Content}
	}
	return &service.ChatParams{
		Messages:      msgs,
		ReportContext: reportContext,
		Scope:         scope,
	}
}
