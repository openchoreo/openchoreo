// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// EventsService provides Kubernetes event query functionality.
type EventsService struct {
	eventsAdapter observability.EventsAdapter
	config        *config.Config
	resolver      *ResourceUIDResolver
	logger        *slog.Logger
}

var (
	// ErrEventsResolveSearchScope indicates a failure while resolving scope/resource identifiers.
	ErrEventsResolveSearchScope = errors.New("events search scope resolution failed")
	// ErrEventsRetrieval indicates a failure while retrieving events from the adapter.
	ErrEventsRetrieval = errors.New("events retrieval failed")
)

// NewEventsService creates a new EventsService instance backed by the HTTP logs adapter.
// The resolver is passed in as it's shared across multiple services.
func NewEventsService(
	eventsAdapter observability.EventsAdapter,
	resolver *ResourceUIDResolver,
	cfg *config.Config,
	logger *slog.Logger,
) (*EventsService, error) {
	if eventsAdapter == nil {
		return nil, fmt.Errorf("events adapter is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EventsService{
		eventsAdapter: eventsAdapter,
		config:        cfg,
		resolver:      resolver,
		logger:        logger,
	}, nil
}

// QueryEvents queries Kubernetes events based on the provided request, forwarding to the events adapter.
func (s *EventsService) QueryEvents(ctx context.Context, req *types.EventsQueryRequest) (*types.EventsQueryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	s.logger.Info("QueryEvents called",
		"startTime", req.StartTime,
		"endTime", req.EndTime,
		"limit", req.Limit)

	// Convert request to internal representation with resolved UIDs
	scope, err := resolveSearchScope(ctx, s.resolver, req.SearchScope)
	if err != nil {
		s.logger.Error("Failed to resolve search scope", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrEventsResolveSearchScope, err)
	}

	// Parse time parameters
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		s.logger.Error("Failed to parse start time", "error", err)
		return nil, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		s.logger.Error("Failed to parse end time", "error", err)
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	// Route to appropriate handler based on scope type
	if scope.IsWorkflowScope {
		return s.queryWorkflowEvents(ctx, scope, startTime, endTime, req)
	}
	return s.queryComponentEvents(ctx, scope, startTime, endTime, req)
}

// queryComponentEvents handles component event queries.
func (s *EventsService) queryComponentEvents(
	ctx context.Context,
	scope *internalSearchScope,
	startTime, endTime time.Time,
	req *types.EventsQueryRequest,
) (*types.EventsQueryResponse, error) {
	s.logger.Debug("Component search scope",
		"namespaceName", scope.NamespaceName,
		"projectUid", scope.ProjectUID,
		"componentUid", scope.ComponentUID,
		"environmentUid", scope.EnvironmentUID)

	params := observability.ComponentEventsParams{
		ComponentID:   scope.ComponentUID,
		EnvironmentID: scope.EnvironmentUID,
		ProjectID:     scope.ProjectUID,
		Namespace:     scope.NamespaceName,
		StartTime:     startTime,
		EndTime:       endTime,
		Limit:         req.Limit,
		SortOrder:     req.SortOrder,
	}

	result, err := s.eventsAdapter.GetComponentEvents(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get component events from adapter", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrEventsRetrieval, err)
	}
	if result == nil {
		return nil, fmt.Errorf("%w: component events adapter returned nil result", ErrEventsRetrieval)
	}

	s.logger.Debug("Component events retrieved from adapter",
		"count", len(result.Events),
		"total", result.TotalCount)

	return convertComponentEventsToResponse(result.Events, result.TotalCount, result.Took), nil
}

// queryWorkflowEvents handles workflow run event queries.
func (s *EventsService) queryWorkflowEvents(
	ctx context.Context,
	scope *internalSearchScope,
	startTime, endTime time.Time,
	req *types.EventsQueryRequest,
) (*types.EventsQueryResponse, error) {
	s.logger.Debug("Workflow search scope",
		"namespaceName", scope.NamespaceName,
		"workflowRunName", scope.WorkflowRunName)

	params := observability.WorkflowEventsParams{
		Namespace:       scope.NamespaceName,
		WorkflowRunName: scope.WorkflowRunName,
		TaskName:        scope.TaskName,
		StartTime:       startTime,
		EndTime:         endTime,
		Limit:           req.Limit,
		SortOrder:       req.SortOrder,
	}

	result, err := s.eventsAdapter.GetWorkflowEvents(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get workflow events from adapter", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrEventsRetrieval, err)
	}
	if result == nil {
		return nil, fmt.Errorf("%w: workflow events adapter returned nil result", ErrEventsRetrieval)
	}

	s.logger.Debug("Workflow events retrieved from adapter",
		"count", len(result.Events),
		"total", result.TotalCount)

	return convertWorkflowEventsToResponse(result.Events, result.TotalCount, result.Took), nil
}

// convertComponentEventsToResponse converts component-scoped adapter events into the
// API response type, including the OpenChoreo resource metadata block.
func convertComponentEventsToResponse(in []observability.EventEntry, total, took int) *types.EventsQueryResponse {
	events := make([]types.EventEntry, 0, len(in))
	for _, e := range in {
		events = append(events, types.EventEntry{
			Timestamp: e.Timestamp.Format(time.RFC3339),
			Message:   e.Message,
			Type:      e.Type,
			Reason:    e.Reason,
			Metadata: &types.EventMetadata{
				ComponentName:   e.ComponentName,
				ProjectName:     e.ProjectName,
				EnvironmentName: e.EnvironmentName,
				NamespaceName:   e.NamespaceName,
				ComponentUID:    e.ComponentID,
				ProjectUID:      e.ProjectID,
				EnvironmentUID:  e.EnvironmentID,
				ObjectKind:      e.ObjectKind,
				ObjectName:      e.ObjectName,
				ObjectNamespace: e.ObjectNamespace,
			},
		})
	}

	return &types.EventsQueryResponse{
		Events: events,
		Total:  total,
		TookMs: took,
	}
}

// convertWorkflowEventsToResponse converts workflow-scoped adapter events into the
// API response type. The metadata block is omitted as it does not apply to workflow runs.
func convertWorkflowEventsToResponse(in []observability.EventEntry, total, took int) *types.EventsQueryResponse {
	events := make([]types.EventEntry, 0, len(in))
	for _, e := range in {
		events = append(events, types.EventEntry{
			Timestamp: e.Timestamp.Format(time.RFC3339),
			Message:   e.Message,
			Type:      e.Type,
			Reason:    e.Reason,
		})
	}

	return &types.EventsQueryResponse{
		Events: events,
		Total:  total,
		TookMs: took,
	}
}
