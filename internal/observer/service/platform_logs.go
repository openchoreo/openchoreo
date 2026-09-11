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

// ErrPlatformLogsRetrieval wraps a failure to reach or read from the logs adapter.
var ErrPlatformLogsRetrieval = errors.New("platform logs retrieval failed")

// PlatformLogsService serves platform logs queries from /api/v1alpha1/platform-logs.
type PlatformLogsService struct {
	adapter observability.PlatformLogsAdapter
	logger  *slog.Logger
}

var _ PlatformLogsQuerier = (*PlatformLogsService)(nil)

// NewPlatformLogsService creates a PlatformLogsService.
func NewPlatformLogsService(adapter observability.PlatformLogsAdapter, logger *slog.Logger) *PlatformLogsService {
	return &PlatformLogsService{adapter: adapter, logger: logger}
}

// QueryPlatformLogs retrieves platform logs matching the request.
func (s *PlatformLogsService) QueryPlatformLogs(
	ctx context.Context,
	req *types.PlatformLogsQueryRequest,
) (*types.PlatformLogsResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	result, err := s.adapter.GetPlatformLogs(ctx, observability.PlatformLogsParams{
		ClusterInstances: req.ClusterInstances,
		Namespaces:       req.Namespaces,
		PodNames:         req.PodNames,
		ContainerNames:   req.ContainerNames,
		Labels:           req.Labels,
		StartTime:        startTime,
		EndTime:          endTime,
		SearchPhrase:     req.SearchPhrase,
		LogLevels:        req.LogLevels,
		Limit:            req.Limit,
		SortOrder:        req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, ErrPlatformLogsNotSupported) {
			return nil, err
		}
		s.logger.Error("Failed to retrieve platform logs", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrPlatformLogsRetrieval, err)
	}

	logs := make([]types.PlatformLog, 0, len(result.Logs))
	for _, l := range result.Logs {
		logs = append(logs, types.PlatformLog{
			Timestamp:       l.Timestamp.UTC().Format(time.RFC3339Nano),
			Log:             l.Log,
			Level:           l.LogLevel,
			ClusterInstance: l.ClusterInstance,
			NamespaceName:   l.NamespaceName,
			PodName:         l.PodName,
			ContainerName:   l.ContainerName,
			PodIP:           l.PodIP,
			NodeName:        l.NodeName,
			ContainerImage:  l.ContainerImage,
			Labels:          l.Labels,
		})
	}

	return &types.PlatformLogsResponse{
		Logs:   logs,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}, nil
}

// --- filter values ---

// ErrPlatformLogFilterValuesRetrieval wraps a failure to reach or read from the logs
// adapter.
var ErrPlatformLogFilterValuesRetrieval = errors.New("platform log filter values retrieval failed")

// QueryPlatformLogFilterValues lists the values one coordinate filter can take.
func (s *PlatformLogsService) QueryPlatformLogFilterValues(
	ctx context.Context,
	req *types.PlatformLogFilterValuesRequest,
) (*types.PlatformLogFilterValuesResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.Query.StartTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.Query.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	result, err := s.adapter.GetPlatformLogFilterValues(ctx, observability.PlatformLogFilterValuesParams{
		Query: observability.PlatformLogsParams{
			ClusterInstances: req.Query.ClusterInstances,
			Namespaces:       req.Query.Namespaces,
			PodNames:         req.Query.PodNames,
			ContainerNames:   req.Query.ContainerNames,
			Labels:           req.Query.Labels,
			StartTime:        startTime,
			EndTime:          endTime,
			SearchPhrase:     req.Query.SearchPhrase,
			LogLevels:        req.Query.LogLevels,
		},
		Filter:      req.Filter,
		ValueSearch: req.ValueSearch,
		MaxValues:   req.MaxValues,
	})
	if err != nil {
		// Passed through unwrapped so the handler can answer 501 rather than
		// reporting a failure.
		if errors.Is(err, ErrPlatformLogFilterValuesNotSupported) {
			return nil, err
		}
		s.logger.Error("Failed to retrieve platform log filter values", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrPlatformLogFilterValuesRetrieval, err)
	}

	values := make([]types.PlatformLogFilterValue, 0, len(result.Values))
	for _, v := range result.Values {
		values = append(values, types.PlatformLogFilterValue{Value: v.Value, Count: v.Count})
	}

	return &types.PlatformLogFilterValuesResponse{
		Filter:        result.Filter,
		Values:        values,
		TotalValues:   result.TotalValues,
		TotalRelation: result.TotalRelation,
		TookMs:        result.Took,
	}, nil
}
