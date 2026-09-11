// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// ErrAuditLogsNotSupported is returned when the configured logs adapter answers
// 501: it does not serve the audit trail. Expected for modules that have not
// adopted it and a different condition from a failure.
var ErrAuditLogsNotSupported = errors.New("audit logs are not supported by the configured logs adapter")

// ErrAuditLogFilterValuesNotSupported is separate from ErrAuditLogsNotSupported
// because the two are separately declinable: an adapter may serve audit records
// while being unable to aggregate them, and a caller told only "audit logs are
// unsupported" would give up on records that in fact work.
var ErrAuditLogFilterValuesNotSupported = errors.New(
	"audit log filter values are not supported by the configured logs adapter")

// ErrAuditLogsCursorExpired is returned when the adapter answers 410: the
// cursor's point-in-time has lapsed, or the indices it was minted against have
// rolled. The caller must restart the query from the first page — never read as
// the end of the trail.
var ErrAuditLogsCursorExpired = errors.New("audit logs cursor has expired")

var _ observability.AuditLogsAdapter = (*LogsAdapter)(nil)

// GetAuditLogs implements observability.AuditLogsAdapter.
func (p *LogsAdapter) GetAuditLogs(
	ctx context.Context,
	params observability.AuditLogsParams,
) (*observability.AuditLogsResult, error) {
	client, err := logsadapterclientgen.NewClientWithResponses(
		p.baseURL, logsadapterclientgen.WithHTTPClient(p.httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logs client: %w", err)
	}

	resp, err := client.QueryAuditLogsWithResponse(ctx, auditLogsRequestBody(params))
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	switch resp.StatusCode() {
	case http.StatusNotImplemented:
		return nil, ErrAuditLogsNotSupported
	case http.StatusGone:
		return nil, ErrAuditLogsCursorExpired
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	records := make([]observability.AuditLogRecord, 0, len(resp.JSON200.Records))
	for _, r := range resp.JSON200.Records {
		records = append(records, toAuditLogRecord(r))
	}

	return &observability.AuditLogsResult{
		Records:       records,
		TotalCount:    resp.JSON200.Total,
		TotalRelation: observability.AuditLogsTotalRelation(resp.JSON200.TotalRelation),
		Took:          resp.JSON200.TookMs,
		NextCursor:    deref(resp.JSON200.NextCursor),
		Timeline:      toAuditLogTimeline(resp.JSON200.Timeline),
	}, nil
}

// GetAuditLogFilterValues implements observability.AuditLogsAdapter.
func (p *LogsAdapter) GetAuditLogFilterValues(
	ctx context.Context,
	params observability.AuditLogFilterValuesParams,
) (*observability.AuditLogFilterValuesResult, error) {
	client, err := logsadapterclientgen.NewClientWithResponses(
		p.baseURL, logsadapterclientgen.WithHTTPClient(p.httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create audit log filter values client: %w", err)
	}

	body := logsadapterclientgen.QueryAuditLogFilterValuesJSONRequestBody{
		Query:  auditLogsRequestBody(params.Query),
		Filter: logsadapterclientgen.AuditLogFilterValuesRequestFilter(params.Filter),
	}
	if params.ValueSearch != "" {
		body.ValueSearch = &params.ValueSearch
	}
	if params.MaxValues > 0 {
		body.MaxValues = &params.MaxValues
	}

	resp, err := client.QueryAuditLogFilterValuesWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	switch resp.StatusCode() {
	case http.StatusNotImplemented:
		return nil, ErrAuditLogFilterValuesNotSupported
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	values := make([]observability.AuditLogFilterValue, 0, len(resp.JSON200.Values))
	for _, v := range resp.JSON200.Values {
		values = append(values, observability.AuditLogFilterValue{Value: v.Value, Count: v.Count})
	}

	return &observability.AuditLogFilterValuesResult{
		Filter:        resp.JSON200.Filter,
		Values:        values,
		TotalValues:   resp.JSON200.TotalValues,
		TotalRelation: observability.AuditLogsTotalRelation(resp.JSON200.TotalRelation),
		Took:          resp.JSON200.TookMs,
	}, nil
}

// auditLogsRequestBody maps the internal params onto the adapter contract. The
// filter groups are nested on both sides so this stays a field-for-field copy,
// which is where a rename would otherwise go unnoticed.
func auditLogsRequestBody(params observability.AuditLogsParams) logsadapterclientgen.AuditLogsQueryRequest {
	body := logsadapterclientgen.AuditLogsQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
	}

	actor := logsadapterclientgen.AuditLogsActorFilter{}
	setIfNotEmpty(&actor.Id, params.Actor.IDs)
	setIfNotEmpty(&actor.Type, params.Actor.Types)
	setIfNotEmpty(&actor.Issuer, params.Actor.Issuers)
	setIfNotEmpty(&actor.SessionId, params.Actor.SessionIDs)
	setIfNotEmpty(&actor.Entitlements, params.Actor.Entitlements)
	if actor != (logsadapterclientgen.AuditLogsActorFilter{}) {
		body.Actor = &actor
	}

	resource := logsadapterclientgen.AuditLogsResourceFilter{}
	setIfNotEmpty(&resource.Type, params.Resource.Types)
	setIfNotEmpty(&resource.Namespace, params.Resource.Namespaces)
	setIfNotEmpty(&resource.Environment, params.Resource.Environments)
	setIfNotEmpty(&resource.Project, params.Resource.Projects)
	setIfNotEmpty(&resource.Component, params.Resource.Components)
	setIfNotEmpty(&resource.Name, params.Resource.Names)
	if resource != (logsadapterclientgen.AuditLogsResourceFilter{}) {
		body.Resource = &resource
	}

	setIfNotEmpty(&body.Action, params.Actions)
	setIfNotEmpty(&body.Producer, params.Producers)
	setIfNotEmpty(&body.OperationId, params.OperationIDs)
	setIfNotEmpty(&body.RequestId, params.RequestIDs)
	setIfNotEmpty(&body.EventId, params.EventIDs)
	setIfNotEmpty(&body.SourceIp, params.SourceIPs)
	setIfNotEmpty(&body.UserAgent, params.UserAgents)

	// The closed enums carry generated types rather than plain strings, so each
	// needs its own conversion; the observer has already rejected an unknown
	// value with a 400 by this point.
	if len(params.Categories) > 0 {
		categories := make([]logsadapterclientgen.AuditLogsQueryRequestCategory, 0, len(params.Categories))
		for _, c := range params.Categories {
			categories = append(categories, logsadapterclientgen.AuditLogsQueryRequestCategory(c))
		}
		body.Category = &categories
	}
	if len(params.Results) > 0 {
		results := make([]logsadapterclientgen.AuditLogsQueryRequestResult, 0, len(params.Results))
		for _, r := range params.Results {
			results = append(results, logsadapterclientgen.AuditLogsQueryRequestResult(r))
		}
		body.Result = &results
	}
	if len(params.Surfaces) > 0 {
		surfaces := make([]logsadapterclientgen.AuditLogsQueryRequestSurface, 0, len(params.Surfaces))
		for _, s := range params.Surfaces {
			surfaces = append(surfaces, logsadapterclientgen.AuditLogsQueryRequestSurface(s))
		}
		body.Surface = &surfaces
	}

	if params.SearchPhrase != "" {
		body.SearchPhrase = &params.SearchPhrase
	}
	if params.Limit > 0 {
		body.Limit = &params.Limit
	}
	if params.SortOrder != "" {
		sortOrder := logsadapterclientgen.AuditLogsQueryRequestSortOrder(params.SortOrder)
		body.SortOrder = &sortOrder
	}
	if params.Cursor != "" {
		body.Cursor = &params.Cursor
	}
	// Sent only when true. An adapter predating the field ignores it either
	// way, but an explicit false is still a field it has to tolerate.
	if params.IncludeTimeline {
		includeTimeline := true
		body.IncludeTimeline = &includeTimeline
	}
	if params.TimelineInterval != "" {
		body.TimelineInterval = &params.TimelineInterval
	}

	return body
}

func toAuditLogRecord(r logsadapterclientgen.AuditLogRecord) observability.AuditLogRecord {
	return observability.AuditLogRecord{
		SchemaVersion: r.SchemaVersion,
		EventID:       r.EventId,
		EventTime:     r.EventTime,
		Actor: observability.AuditLogActor{
			Type:         r.Actor.Type,
			ID:           r.Actor.Id,
			Issuer:       deref(r.Actor.Issuer),
			SessionID:    deref(r.Actor.SessionId),
			Entitlements: derefEntitlements(r.Actor.Entitlements),
		},
		Action:      r.Action,
		Category:    r.Category,
		Result:      r.Result,
		RequestID:   deref(r.RequestId),
		SourceIP:    deref(r.SourceIp),
		UserAgent:   deref(r.UserAgent),
		Producer:    deref(r.Producer),
		Surface:     deref(r.Surface),
		OperationID: deref(r.OperationId),
		HTTP:        toAuditLogHTTPInfo(r.Http),
		Resource:    toAuditLogResource(r.Resource),
		Metadata:    derefAnyMap(r.Metadata),
		Collector:   toAuditLogCollectorInfo(r.Collector),
		Log:         deref(r.Log),
	}
}

func toAuditLogHTTPInfo(src *logsadapterclientgen.AuditLogHTTPInfo) *observability.AuditLogHTTPInfo {
	if src == nil {
		return nil
	}
	return &observability.AuditLogHTTPInfo{Method: deref(src.Method), Path: deref(src.Path)}
}

func toAuditLogResource(src *logsadapterclientgen.AuditLogResource) *observability.AuditLogResource {
	if src == nil {
		return nil
	}
	return &observability.AuditLogResource{
		Type:        deref(src.Type),
		Namespace:   deref(src.Namespace),
		Environment: deref(src.Environment),
		Project:     deref(src.Project),
		Component:   deref(src.Component),
		Resource:    deref(src.Resource),
		UID:         deref(src.Uid),
		Name:        deref(src.Name),
		Metadata:    derefAnyMap(src.Metadata),
	}
}

func toAuditLogCollectorInfo(
	src *logsadapterclientgen.AuditLogCollectorInfo,
) *observability.AuditLogCollectorInfo {
	if src == nil {
		return nil
	}
	return &observability.AuditLogCollectorInfo{
		NamespaceName: deref(src.NamespaceName),
		PodName:       deref(src.PodName),
		ContainerName: deref(src.ContainerName),
	}
}

// toAuditLogTimeline preserves nil: an adapter that cannot compute a timeline
// omits it, and that is not the same answer as one reporting no activity.
func toAuditLogTimeline(src *logsadapterclientgen.AuditLogTimeline) *observability.AuditLogTimeline {
	if src == nil {
		return nil
	}
	buckets := make([]observability.AuditLogTimelineBucket, 0, len(src.Buckets))
	for _, b := range src.Buckets {
		buckets = append(buckets, observability.AuditLogTimelineBucket{
			StartTime: b.StartTime,
			Total:     b.Total,
			Counts:    derefCounts(b.Counts),
		})
	}
	return &observability.AuditLogTimeline{Interval: src.Interval, Buckets: buckets}
}

func derefEntitlements(ptr *map[string][]string) map[string][]string {
	if ptr == nil {
		return nil
	}
	return *ptr
}

func derefAnyMap(ptr *map[string]interface{}) map[string]any {
	if ptr == nil {
		return nil
	}
	return *ptr
}

func derefCounts(ptr *map[string]int64) map[string]int64 {
	if ptr == nil {
		return nil
	}
	return *ptr
}
