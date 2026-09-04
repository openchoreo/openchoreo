// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// ErrClusterLogsNotSupported is returned when the configured logs adapter answers
// 501: it does not implement the cluster logs endpoint. This is expected for
// modules that have not adopted it yet and is a different condition from a failure.
var ErrClusterLogsNotSupported = errors.New("cluster logs are not supported by the configured logs adapter")

var _ observability.ClusterLogsAdapter = (*LogsAdapter)(nil)

// GetClusterLogs implements observability.ClusterLogsAdapter.
func (p *LogsAdapter) GetClusterLogs(
	ctx context.Context,
	params observability.ClusterLogsParams,
) (*observability.ClusterLogsResult, error) {
	client, err := logsadapterclientgen.NewClientWithResponses(
		p.baseURL, logsadapterclientgen.WithHTTPClient(p.httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster logs client: %w", err)
	}

	body := logsadapterclientgen.QueryClusterLogsJSONRequestBody{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
	}
	setIfNotEmpty(&body.ClusterInstance, params.ClusterInstances)
	setIfNotEmpty(&body.Namespace, params.Namespaces)
	setIfNotEmpty(&body.PodName, params.PodNames)
	setIfNotEmpty(&body.ContainerName, params.ContainerNames)
	if len(params.Labels) > 0 {
		labels := params.Labels
		body.Labels = &labels
	}
	if len(params.LogLevels) > 0 {
		levels := make([]logsadapterclientgen.ClusterLogsQueryRequestLogLevels, 0, len(params.LogLevels))
		for _, l := range params.LogLevels {
			levels = append(levels, logsadapterclientgen.ClusterLogsQueryRequestLogLevels(l))
		}
		body.LogLevels = &levels
	}
	if params.SearchPhrase != "" {
		body.SearchPhrase = &params.SearchPhrase
	}
	if params.Limit > 0 {
		body.Limit = &params.Limit
	}
	if params.SortOrder != "" {
		sortOrder := logsadapterclientgen.ClusterLogsQueryRequestSortOrder(params.SortOrder)
		body.SortOrder = &sortOrder
	}

	resp, err := client.QueryClusterLogsWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotImplemented {
		return nil, ErrClusterLogsNotSupported
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	logs := make([]observability.ClusterLogEntry, 0, len(resp.JSON200.Logs))
	for _, l := range resp.JSON200.Logs {
		logs = append(logs, observability.ClusterLogEntry{
			Timestamp:       derefTime(l.Timestamp),
			Log:             deref(l.Log),
			LogLevel:        deref(l.Level),
			ClusterInstance: deref(l.ClusterInstance),
			NamespaceName:   deref(l.NamespaceName),
			PodName:         deref(l.PodName),
			ContainerName:   deref(l.ContainerName),
			PodIP:           deref(l.PodIp),
			NodeName:        deref(l.NodeName),
			ContainerImage:  deref(l.ContainerImage),
			Labels:          derefMap(l.Labels),
		})
	}

	return &observability.ClusterLogsResult{
		Logs:       logs,
		TotalCount: resp.JSON200.Total,
		Took:       resp.JSON200.TookMs,
	}, nil
}

func setIfNotEmpty(dst **[]string, values []string) {
	if len(values) > 0 {
		v := values
		*dst = &v
	}
}

func deref(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func derefTime(ptr *time.Time) time.Time {
	if ptr == nil {
		return time.Time{}
	}
	return *ptr
}

func derefMap(ptr *map[string]string) map[string]string {
	if ptr == nil {
		return nil
	}
	return *ptr
}
