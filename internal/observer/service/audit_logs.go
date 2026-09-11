// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// ErrAuditLogsRetrieval wraps a failure to reach or read from the logs adapter.
var ErrAuditLogsRetrieval = errors.New("audit logs retrieval failed")

// AuditLogsService serves the audit trail's two reads: the records themselves
// and the distinct values a filter over them can take.
type AuditLogsService struct {
	adapter observability.AuditLogsAdapter
	logger  *slog.Logger
}

var _ AuditLogsQuerier = (*AuditLogsService)(nil)

// NewAuditLogsService creates an AuditLogsService.
func NewAuditLogsService(adapter observability.AuditLogsAdapter, logger *slog.Logger) *AuditLogsService {
	return &AuditLogsService{adapter: adapter, logger: logger}
}

// QueryAuditLogs retrieves audit records matching the request.
func (s *AuditLogsService) QueryAuditLogs(
	ctx context.Context,
	req *types.AuditLogsQueryRequest,
) (*types.AuditLogsResponse, error) {
	params, err := toAuditLogsParams(req)
	if err != nil {
		return nil, err
	}

	result, err := s.adapter.GetAuditLogs(ctx, params)
	if err != nil {
		return nil, s.wrapRetrieval("Failed to retrieve audit logs", err,
			ErrAuditLogsNotSupported, ErrAuditLogsCursorExpired)
	}

	records := make([]types.AuditLogRecord, 0, len(result.Records))
	for _, r := range result.Records {
		records = append(records, toTypesAuditLogRecord(r))
	}

	resp := &types.AuditLogsResponse{
		Records:       records,
		Total:         result.TotalCount,
		TotalRelation: string(result.TotalRelation),
		TookMs:        result.Took,
		NextCursor:    result.NextCursor,
	}
	// Only ever answered when asked for. An adapter that computes a timeline
	// unconditionally must not make every caller carry it.
	if req.IncludeTimeline {
		resp.Timeline = toTypesAuditLogTimeline(result.Timeline)
	}
	return resp, nil
}

// QueryAuditLogFilterValues retrieves the distinct values one filter takes.
func (s *AuditLogsService) QueryAuditLogFilterValues(
	ctx context.Context,
	req *types.AuditLogFilterValuesRequest,
) (*types.AuditLogFilterValuesResponse, error) {
	query, err := toAuditLogsParams(&req.Query)
	if err != nil {
		return nil, err
	}

	result, err := s.adapter.GetAuditLogFilterValues(ctx, observability.AuditLogFilterValuesParams{
		Query:       query,
		Filter:      req.Filter,
		ValueSearch: req.ValueSearch,
		MaxValues:   req.MaxValues,
	})
	if err != nil {
		return nil, s.wrapRetrieval("Failed to retrieve audit log filter values", err,
			ErrAuditLogFilterValuesNotSupported)
	}

	values := make([]types.AuditLogFilterValue, 0, len(result.Values))
	for _, v := range result.Values {
		values = append(values, types.AuditLogFilterValue{Value: v.Value, Count: v.Count})
	}

	return &types.AuditLogFilterValuesResponse{
		Filter:        result.Filter,
		Values:        values,
		TotalValues:   result.TotalValues,
		TotalRelation: string(result.TotalRelation),
		TookMs:        result.Took,
	}, nil
}

// wrapRetrieval passes the sentinels the handler maps onto specific statuses
// straight through, and wraps anything else as a retrieval failure. Which
// sentinels are pass-through differs per operation — a cursor cannot expire on
// a filter-values query — so each caller names its own.
func (s *AuditLogsService) wrapRetrieval(msg string, err error, passThrough ...error) error {
	for _, sentinel := range passThrough {
		if errors.Is(err, sentinel) {
			return err
		}
	}
	s.logger.Error(msg, "error", err)
	return fmt.Errorf("%w: %w", ErrAuditLogsRetrieval, err)
}

// toAuditLogsParams maps the decoded request onto the adapter params. The
// filter groups stay grouped so this is a field-for-field copy.
func toAuditLogsParams(req *types.AuditLogsQueryRequest) (observability.AuditLogsParams, error) {
	if req == nil {
		return observability.AuditLogsParams{}, fmt.Errorf("request is required")
	}
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return observability.AuditLogsParams{}, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return observability.AuditLogsParams{}, fmt.Errorf("failed to parse end time: %w", err)
	}

	return observability.AuditLogsParams{
		StartTime: startTime,
		EndTime:   endTime,
		Actor: observability.AuditLogsActorFilter{
			IDs:          req.Actor.IDs,
			Types:        req.Actor.Types,
			Issuers:      req.Actor.Issuers,
			SessionIDs:   req.Actor.SessionIDs,
			Entitlements: req.Actor.Entitlements,
		},
		Resource: observability.AuditLogsResourceFilter{
			Types:        req.Resource.Types,
			Namespaces:   req.Resource.Namespaces,
			Environments: req.Resource.Environments,
			Projects:     req.Resource.Projects,
			Components:   req.Resource.Components,
			Names:        req.Resource.Names,
		},
		Actions:          req.Actions,
		Categories:       req.Categories,
		Results:          req.Results,
		Producers:        req.Producers,
		Surfaces:         req.Surfaces,
		OperationIDs:     req.OperationIDs,
		RequestIDs:       req.RequestIDs,
		EventIDs:         req.EventIDs,
		SourceIPs:        req.SourceIPs,
		UserAgents:       req.UserAgents,
		SearchPhrase:     req.SearchPhrase,
		Limit:            req.Limit,
		SortOrder:        req.SortOrder,
		Cursor:           req.Cursor,
		IncludeTimeline:  req.IncludeTimeline,
		TimelineInterval: req.TimelineInterval,
	}, nil
}

func toTypesAuditLogRecord(r observability.AuditLogRecord) types.AuditLogRecord {
	return types.AuditLogRecord{
		SchemaVersion: r.SchemaVersion,
		EventID:       r.EventID,
		EventTime:     r.EventTime.UTC().Format(time.RFC3339Nano),
		Actor: types.AuditLogActor{
			Type:         r.Actor.Type,
			ID:           r.Actor.ID,
			Issuer:       r.Actor.Issuer,
			SessionID:    r.Actor.SessionID,
			Entitlements: r.Actor.Entitlements,
		},
		Action:      r.Action,
		Category:    r.Category,
		Result:      r.Result,
		RequestID:   r.RequestID,
		SourceIP:    r.SourceIP,
		UserAgent:   r.UserAgent,
		Producer:    r.Producer,
		Surface:     r.Surface,
		OperationID: r.OperationID,
		HTTP:        toTypesAuditLogHTTPInfo(r.HTTP),
		Resource:    toTypesAuditLogResource(r.Resource),
		Metadata:    r.Metadata,
		Collector:   toTypesAuditLogCollectorInfo(r.Collector),
		Log:         r.Log,
	}
}

func toTypesAuditLogHTTPInfo(src *observability.AuditLogHTTPInfo) *types.AuditLogHTTPInfo {
	if src == nil {
		return nil
	}
	return &types.AuditLogHTTPInfo{Method: src.Method, Path: src.Path}
}

func toTypesAuditLogResource(src *observability.AuditLogResource) *types.AuditLogResource {
	if src == nil {
		return nil
	}
	return &types.AuditLogResource{
		Type:        src.Type,
		Namespace:   src.Namespace,
		Environment: src.Environment,
		Project:     src.Project,
		Component:   src.Component,
		Resource:    src.Resource,
		UID:         src.UID,
		Name:        src.Name,
		Metadata:    src.Metadata,
	}
}

func toTypesAuditLogCollectorInfo(
	src *observability.AuditLogCollectorInfo,
) *types.AuditLogCollectorInfo {
	if src == nil {
		return nil
	}
	return &types.AuditLogCollectorInfo{
		NamespaceName: src.NamespaceName,
		PodName:       src.PodName,
		ContainerName: src.ContainerName,
	}
}

// toTypesAuditLogTimeline preserves nil: the caller has to tell "the adapter
// cannot compute this" apart from "no activity in the window", and only nil
// says the former.
func toTypesAuditLogTimeline(src *observability.AuditLogTimeline) *types.AuditLogTimeline {
	if src == nil {
		return nil
	}
	buckets := make([]types.AuditLogTimelineBucket, 0, len(src.Buckets))
	for _, b := range src.Buckets {
		buckets = append(buckets, types.AuditLogTimelineBucket{
			StartTime: b.StartTime.UTC().Format(time.RFC3339Nano),
			Total:     b.Total,
			Counts:    b.Counts,
		})
	}
	return &types.AuditLogTimeline{Interval: src.Interval, Buckets: buckets}
}
