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

// ErrClusterLogsRetrieval wraps a failure to reach or read from the logs adapter.
var ErrClusterLogsRetrieval = errors.New("cluster logs retrieval failed")

// ClusterLogsService serves cluster logs queries from /api/v1alpha1/cluster-logs.
type ClusterLogsService struct {
	adapter observability.ClusterLogsAdapter
	logger  *slog.Logger
}

var _ ClusterLogsQuerier = (*ClusterLogsService)(nil)

// NewClusterLogsService creates a ClusterLogsService.
func NewClusterLogsService(adapter observability.ClusterLogsAdapter, logger *slog.Logger) *ClusterLogsService {
	return &ClusterLogsService{adapter: adapter, logger: logger}
}

// QueryClusterLogs retrieves cluster logs matching the request.
func (s *ClusterLogsService) QueryClusterLogs(
	ctx context.Context,
	req *types.ClusterLogsQueryRequest,
) (*types.ClusterLogsResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	result, err := s.adapter.GetClusterLogs(ctx, observability.ClusterLogsParams{
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
		if errors.Is(err, ErrClusterLogsNotSupported) {
			return nil, err
		}
		s.logger.Error("Failed to retrieve cluster logs", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrClusterLogsRetrieval, err)
	}

	logs := make([]types.ClusterLog, 0, len(result.Logs))
	for _, l := range result.Logs {
		logs = append(logs, types.ClusterLog{
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

	return &types.ClusterLogsResponse{
		Logs:   logs,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}, nil
}
