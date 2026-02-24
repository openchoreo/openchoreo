// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	svcerrors "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
)

// ListWorkflows returns a list of generic workflows
func (h *Handler) ListWorkflows(
	ctx context.Context,
	request gen.ListWorkflowsRequestObject,
) (gen.ListWorkflowsResponseObject, error) {
	h.logger.Debug("ListWorkflows called", "namespaceName", request.NamespaceName)

	workflows, err := h.legacyServices.WorkflowService.ListWorkflows(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list workflows", "error", err)
		return gen.ListWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Workflow, 0, len(workflows))
	for _, wf := range workflows {
		items = append(items, toGenWorkflow(wf))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListWorkflows200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenWorkflow converts models.WorkflowResponse to gen.Workflow
func toGenWorkflow(wf *models.WorkflowResponse) gen.Workflow {
	return gen.Workflow{
		Name:        wf.Name,
		DisplayName: ptr.To(wf.DisplayName),
		Description: ptr.To(wf.Description),
		CreatedAt:   wf.CreatedAt,
	}
}

// GetWorkflowSchema returns the parameter schema for a workflow
func (h *Handler) GetWorkflowSchema(
	ctx context.Context,
	request gen.GetWorkflowSchemaRequestObject,
) (gen.GetWorkflowSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// ListWorkflowRuns returns a list of workflow runs
func (h *Handler) ListWorkflowRuns(
	ctx context.Context,
	request gen.ListWorkflowRunsRequestObject,
) (gen.ListWorkflowRunsResponseObject, error) {
	h.logger.Info("ListWorkflowRuns called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, request.NamespaceName, "", "", opts)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.ListWorkflowRuns403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list workflow runs", "error", err)
		return gen.ListWorkflowRuns500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items := make([]gen.WorkflowRun, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, toGenWorkflowRunFromCR(&result.Items[i]))
	}

	return gen.ListWorkflowRuns200JSONResponse(gen.WorkflowRunList{
		Items:      items,
		Pagination: ToPagination(result),
	}), nil
}

// CreateWorkflowRun creates a new workflow run
func (h *Handler) CreateWorkflowRun(
	ctx context.Context,
	request gen.CreateWorkflowRunRequestObject,
) (gen.CreateWorkflowRunResponseObject, error) {
	h.logger.Info("CreateWorkflowRun called",
		"namespace", request.NamespaceName,
		"workflow", request.Body.WorkflowName)

	// Convert parameters to runtime.RawExtension
	var parametersRaw *runtime.RawExtension
	if request.Body.Parameters != nil {
		rawBytes, err := json.Marshal(request.Body.Parameters)
		if err != nil {
			h.logger.Error("Failed to marshal parameters", "error", err)
			return gen.CreateWorkflowRun400JSONResponse{BadRequestJSONResponse: badRequest("Invalid parameters")}, nil
		}
		parametersRaw = &runtime.RawExtension{Raw: rawBytes}
	}

	// Generate a unique name for the workflow run
	runName, err := generateWorkflowRunName(request.Body.WorkflowName)
	if err != nil {
		h.logger.Error("Failed to generate workflow run name", "error", err)
		return gen.CreateWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	wfRun := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: runName,
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name:       request.Body.WorkflowName,
				Parameters: parametersRaw,
			},
		},
	}

	created, err := h.services.WorkflowRunService.CreateWorkflowRun(ctx, request.NamespaceName, wfRun)
	if err != nil {
		if errors.Is(err, workflowrunsvc.ErrWorkflowNotFound) {
			return gen.CreateWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.CreateWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to create workflow run", "error", err)
		return gen.CreateWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.CreateWorkflowRun201JSONResponse(toGenWorkflowRunFromCR(created)), nil
}

// GetWorkflowRun returns a specific workflow run
func (h *Handler) GetWorkflowRun(
	ctx context.Context,
	request gen.GetWorkflowRunRequestObject,
) (gen.GetWorkflowRunResponseObject, error) {
	h.logger.Info("GetWorkflowRun called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	wfRun, err := h.services.WorkflowRunService.GetWorkflowRun(ctx, request.NamespaceName, request.RunName)
	if err != nil {
		if errors.Is(err, workflowrunsvc.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GetWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get workflow run", "error", err)
		return gen.GetWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkflowRun200JSONResponse(toGenWorkflowRunFromCR(wfRun)), nil
}

// GetWorkflowRunStatus returns the status and per-step details of a specific workflow run
func (h *Handler) GetWorkflowRunStatus(
	ctx context.Context,
	request gen.GetWorkflowRunStatusRequestObject,
) (gen.GetWorkflowRunStatusResponseObject, error) {
	h.logger.Info("GetWorkflowRunStatus called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	status, err := h.legacyServices.WorkflowRunService.GetWorkflowRunStatus(ctx, request.NamespaceName, request.RunName, h.Config.ClusterGateway.URL)
	if err != nil {
		if errors.Is(err, legacyservices.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunStatus404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, legacyservices.ErrForbidden) {
			return gen.GetWorkflowRunStatus403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get workflow run status", "error", err)
		return gen.GetWorkflowRunStatus500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	steps := make([]gen.WorkflowStepStatus, 0, len(status.Steps))
	for _, s := range status.Steps {
		step := gen.WorkflowStepStatus{
			Name:       s.Name,
			Phase:      normalizeStepPhase(s.Phase),
			StartedAt:  s.StartedAt,
			FinishedAt: s.FinishedAt,
		}
		steps = append(steps, step)
	}

	return gen.GetWorkflowRunStatus200JSONResponse{
		Status:               gen.WorkflowRunStatusResponseStatus(status.Status),
		Steps:                steps,
		HasLiveObservability: status.HasLiveObservability,
	}, nil
}

// GetWorkflowRunLogs returns logs for a specific workflow run
func (h *Handler) GetWorkflowRunLogs(
	ctx context.Context,
	request gen.GetWorkflowRunLogsRequestObject,
) (gen.GetWorkflowRunLogsResponseObject, error) {
	h.logger.Info("GetWorkflowRunLogs called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	step := ""
	if request.Params.Step != nil {
		step = *request.Params.Step
	}

	logs, err := h.legacyServices.WorkflowRunService.GetWorkflowRunLogs(ctx, request.NamespaceName, request.RunName,
		step, h.Config.ClusterGateway.URL, request.Params.SinceSeconds)
	if err != nil {
		if errors.Is(err, legacyservices.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunLogs404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, legacyservices.ErrForbidden) {
			return gen.GetWorkflowRunLogs403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, legacyservices.ErrWorkflowRunReferenceNotFound) {
			return gen.GetWorkflowRunLogs404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		h.logger.Error("Failed to get workflow run logs", "error", err)
		return gen.GetWorkflowRunLogs500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := make(gen.GetWorkflowRunLogs200JSONResponse, 0, len(logs))
	for _, entry := range logs {
		logEntry := gen.WorkflowRunLogEntry{Log: entry.Log}
		if entry.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
				logEntry.Timestamp = &ts
			} else if ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
				logEntry.Timestamp = &ts
			}
		}
		result = append(result, logEntry)
	}
	return result, nil
}

// GetWorkflowRunEvents returns Kubernetes events for a specific workflow run
func (h *Handler) GetWorkflowRunEvents(
	ctx context.Context,
	request gen.GetWorkflowRunEventsRequestObject,
) (gen.GetWorkflowRunEventsResponseObject, error) {
	h.logger.Info("GetWorkflowRunEvents called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	step := ""
	if request.Params.Step != nil {
		step = *request.Params.Step
	}

	events, err := h.legacyServices.WorkflowRunService.GetWorkflowRunEvents(ctx, request.NamespaceName, request.RunName,
		step, h.Config.ClusterGateway.URL)
	if err != nil {
		if errors.Is(err, legacyservices.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunEvents404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, legacyservices.ErrForbidden) {
			return gen.GetWorkflowRunEvents403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, legacyservices.ErrWorkflowRunReferenceNotFound) {
			return gen.GetWorkflowRunEvents404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRunReference")}, nil
		}
		h.logger.Error("Failed to get workflow run events", "error", err)
		return gen.GetWorkflowRunEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := make(gen.GetWorkflowRunEvents200JSONResponse, 0, len(events))
	for _, entry := range events {
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			ts, err = time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err != nil {
				h.logger.Warn("Failed to parse event timestamp", "timestamp", entry.Timestamp, "error", err)
			}
		}
		result = append(result, gen.WorkflowRunEventEntry{
			Timestamp: ts,
			Type:      entry.Type,
			Reason:    entry.Reason,
			Message:   entry.Message,
		})
	}
	return result, nil
}

// toGenWorkflowRunFromCR converts a WorkflowRun CRD to the OpenAPI gen.WorkflowRun type.
func toGenWorkflowRunFromCR(wfRun *openchoreov1alpha1.WorkflowRun) gen.WorkflowRun {
	if wfRun == nil {
		return gen.WorkflowRun{}
	}

	status := getWorkflowRunStatus(wfRun.Status.Conditions)

	result := gen.WorkflowRun{
		Name:         wfRun.Name,
		OrgName:      wfRun.Namespace,
		WorkflowName: wfRun.Spec.Workflow.Name,
		Status:       gen.WorkflowRunStatus(status),
		CreatedAt:    wfRun.CreationTimestamp.Time,
	}

	if wfRun.UID != "" {
		uid := string(wfRun.UID)
		result.Uuid = &uid
	}

	result.Phase = &status

	// Set FinishedAt from WorkflowCompleted condition
	for _, condition := range wfRun.Status.Conditions {
		if condition.Type == "WorkflowCompleted" && condition.Status == metav1.ConditionTrue {
			t := condition.LastTransitionTime.Time
			result.FinishedAt = &t
			break
		}
	}

	// Extract parameters if available
	if wfRun.Spec.Workflow.Parameters != nil && wfRun.Spec.Workflow.Parameters.Raw != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(wfRun.Spec.Workflow.Parameters.Raw, &params); err == nil {
			result.Parameters = &params
		}
	}

	return result
}

const workflowRunStatusPending = "Pending"

// getWorkflowRunStatus determines the user-friendly status from workflow run conditions.
func getWorkflowRunStatus(conditions []metav1.Condition) string {
	if len(conditions) == 0 {
		return workflowRunStatusPending
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowFailed" && condition.Status == metav1.ConditionTrue {
			return "Failed"
		}
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowSucceeded" && condition.Status == metav1.ConditionTrue {
			return "Succeeded"
		}
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowRunning" && condition.Status == metav1.ConditionTrue {
			return "Running"
		}
	}

	return workflowRunStatusPending
}

// generateWorkflowRunName generates a unique name for the workflow run.
func generateWorkflowRunName(workflowName string) (string, error) {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random suffix: %w", err)
	}
	suffix := hex.EncodeToString(bytes)

	runName := fmt.Sprintf("%s-run-%s", workflowName, suffix)

	if len(runName) > 63 {
		maxWorkflowNameLen := 63 - len("-run-") - 8
		if maxWorkflowNameLen > 0 {
			runName = fmt.Sprintf("%s-run-%s", workflowName[:maxWorkflowNameLen], suffix)
		} else {
			return "", fmt.Errorf("workflow name is too long to generate valid run name")
		}
	}

	return runName, nil
}

// normalizeStepPhase maps a raw phase string from the WorkflowTask CRD to a
// valid OpenAPI enum value. Argo's "Omitted" phase is mapped to "Skipped";
// any other unrecognized value falls back to "Error".
func normalizeStepPhase(phase string) gen.WorkflowStepStatusPhase {
	switch phase {
	case "Pending", "Running", "Succeeded", "Failed", "Skipped", "Error":
		return gen.WorkflowStepStatusPhase(phase)
	case "Omitted":
		return gen.Skipped
	default:
		return gen.Error
	}
}
