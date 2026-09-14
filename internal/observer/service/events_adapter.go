// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// GetComponentEvents implements observability.EventsAdapter.
// It forwards a component-scoped Kubernetes event query to the logs adapter service.
func (p *LogsAdapter) GetComponentEvents(
	ctx context.Context,
	params observability.ComponentEventsParams,
) (*observability.ComponentEventsResult, error) {
	var scope logsadapterclientgen.EventsQueryRequest_SearchScope
	if err := scope.FromComponentSearchScope(logsadapterclientgen.ComponentSearchScope{
		Namespace:      params.Namespace,
		ProjectUid:     nonEmptyStrPtr(params.ProjectID),
		ComponentUid:   nonEmptyStrPtr(params.ComponentID),
		EnvironmentUid: nonEmptyStrPtr(params.EnvironmentID),
	}); err != nil {
		return nil, fmt.Errorf("failed to build component events search scope: %w", err)
	}

	adapterResp, err := p.queryEvents(ctx, scope, params.StartTime, params.EndTime, params.Limit, params.SortOrder)
	if err != nil {
		return nil, err
	}

	return &observability.ComponentEventsResult{
		Events:     toObservabilityEvents(adapterResp.Events),
		TotalCount: intPtrVal(adapterResp.Total),
		Took:       intPtrVal(adapterResp.TookMs),
	}, nil
}

// GetWorkflowEvents implements observability.EventsAdapter.
// It forwards a workflow-scoped Kubernetes event query to the logs adapter service.
func (p *LogsAdapter) GetWorkflowEvents(
	ctx context.Context,
	params observability.WorkflowEventsParams,
) (*observability.WorkflowEventsResult, error) {
	var scope logsadapterclientgen.EventsQueryRequest_SearchScope
	if err := scope.FromWorkflowSearchScope(logsadapterclientgen.WorkflowSearchScope{
		Namespace:       params.Namespace,
		WorkflowRunName: nonEmptyStrPtr(params.WorkflowRunName),
		TaskName:        nonEmptyStrPtr(params.TaskName),
	}); err != nil {
		return nil, fmt.Errorf("failed to build workflow events search scope: %w", err)
	}

	adapterResp, err := p.queryEvents(ctx, scope, params.StartTime, params.EndTime, params.Limit, params.SortOrder)
	if err != nil {
		return nil, err
	}

	return &observability.WorkflowEventsResult{
		Events:     toObservabilityEvents(adapterResp.Events),
		TotalCount: intPtrVal(adapterResp.Total),
		Took:       intPtrVal(adapterResp.TookMs),
	}, nil
}

// queryEvents builds and executes a QueryEvents call against the logs adapter and decodes the response.
func (p *LogsAdapter) queryEvents(
	ctx context.Context,
	scope logsadapterclientgen.EventsQueryRequest_SearchScope,
	startTime, endTime time.Time,
	limit int,
	sortOrder string,
) (*logsadapterclientgen.EventsQueryResponse, error) {
	adapterReq := logsadapterclientgen.EventsQueryRequest{
		StartTime:   startTime,
		EndTime:     endTime,
		SearchScope: &scope,
	}
	if limit > 0 {
		adapterReq.Limit = &limit
	}
	if sortOrder != "" {
		so := logsadapterclientgen.EventsQueryRequestSortOrder(sortOrder)
		adapterReq.SortOrder = &so
	}

	resp, err := p.adapterClient.QueryEvents(ctx, adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call logs adapter query events: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
		return nil, err
	}

	return decodeEventsResponse(resp)
}

// decodeEventsResponse decodes the adapter's HTTP response body into an EventsQueryResponse.
func decodeEventsResponse(resp *http.Response) (*logsadapterclientgen.EventsQueryResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read adapter events response: %w", err)
	}
	var result logsadapterclientgen.EventsQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode adapter events response: %w", err)
	}
	return &result, nil
}

// toObservabilityEvents converts generated client event entries to observability.EventEntry.
func toObservabilityEvents(entries *[]logsadapterclientgen.EventEntry) []observability.EventEntry {
	if entries == nil {
		return []observability.EventEntry{}
	}
	events := make([]observability.EventEntry, 0, len(*entries))
	for _, e := range *entries {
		entry := observability.EventEntry{
			Message: stringPtrVal(e.Message),
			Type:    stringPtrVal(e.Type),
			Reason:  stringPtrVal(e.Reason),
		}
		if e.Timestamp != nil {
			entry.Timestamp = *e.Timestamp
		}
		if e.Metadata != nil {
			m := e.Metadata
			entry.ComponentName = stringPtrVal(m.ComponentName)
			entry.ProjectName = stringPtrVal(m.ProjectName)
			entry.EnvironmentName = stringPtrVal(m.EnvironmentName)
			entry.NamespaceName = stringPtrVal(m.NamespaceName)
			entry.ComponentID = uuidPtrVal(m.ComponentUid)
			entry.ProjectID = uuidPtrVal(m.ProjectUid)
			entry.EnvironmentID = uuidPtrVal(m.EnvironmentUid)
			entry.ObjectKind = stringPtrVal(m.ObjectKind)
			entry.ObjectName = stringPtrVal(m.ObjectName)
			entry.ObjectNamespace = stringPtrVal(m.ObjectNamespace)
		}
		events = append(events, entry)
	}
	return events
}

// nonEmptyStrPtr returns a pointer to s, or nil if s is empty.
func nonEmptyStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// intPtrVal safely dereferences an *int, returning 0 for nil.
func intPtrVal(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// uuidPtrVal returns the string form of a *UUID, or "" for nil.
func uuidPtrVal(u *openapi_types.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}
