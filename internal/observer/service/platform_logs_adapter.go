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

// ErrPlatformLogsNotSupported is returned when the configured logs adapter answers
// 501: it does not implement the platform logs endpoint. This is expected for
// modules that have not adopted it yet and is a different condition from a failure.
var ErrPlatformLogsNotSupported = errors.New("platform logs are not supported by the configured logs adapter")

var _ observability.PlatformLogsAdapter = (*LogsAdapter)(nil)

// GetPlatformLogs implements observability.PlatformLogsAdapter.
func (p *LogsAdapter) GetPlatformLogs(
	ctx context.Context,
	params observability.PlatformLogsParams,
) (*observability.PlatformLogsResult, error) {
	client, err := logsadapterclientgen.NewClientWithResponses(
		p.baseURL, logsadapterclientgen.WithHTTPClient(p.httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create platform logs client: %w", err)
	}

	body := logsadapterclientgen.QueryPlatformLogsJSONRequestBody{
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
		levels := make([]logsadapterclientgen.PlatformLogsQueryRequestLogLevels, 0, len(params.LogLevels))
		for _, l := range params.LogLevels {
			levels = append(levels, logsadapterclientgen.PlatformLogsQueryRequestLogLevels(l))
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
		sortOrder := logsadapterclientgen.PlatformLogsQueryRequestSortOrder(params.SortOrder)
		body.SortOrder = &sortOrder
	}

	resp, err := client.QueryPlatformLogsWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotImplemented {
		return nil, ErrPlatformLogsNotSupported
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	logs := make([]observability.PlatformLogEntry, 0, len(resp.JSON200.Logs))
	for _, l := range resp.JSON200.Logs {
		logs = append(logs, observability.PlatformLogEntry{
			Timestamp:       l.Timestamp,
			Log:             l.Log,
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

	return &observability.PlatformLogsResult{
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

func derefMap(ptr *map[string]string) map[string]string {
	if ptr == nil {
		return nil
	}
	return *ptr
}

// --- filter values ---

// ErrPlatformLogFilterValuesNotSupported is returned when the configured logs adapter
// answers 501. Aggregating a field's values is work an adapter may not be able to do
// even though it serves platform logs, so this is a distinct condition from
// ErrPlatformLogsNotSupported and from a failure.
var ErrPlatformLogFilterValuesNotSupported = errors.New(
	"platform log filter values are not supported by the configured logs adapter")

// GetPlatformLogFilterValues implements observability.PlatformLogsAdapter.
func (p *LogsAdapter) GetPlatformLogFilterValues(
	ctx context.Context,
	params observability.PlatformLogFilterValuesParams,
) (*observability.PlatformLogFilterValuesResult, error) {
	client, err := logsadapterclientgen.NewClientWithResponses(
		p.baseURL, logsadapterclientgen.WithHTTPClient(p.httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create platform log filter values client: %w", err)
	}

	// The record query goes over the wire whole, including the named filter's own
	// selections: excluding them is the adapter's job, and sending a doctored query
	// would leave it unable to tell "not selected" from "excluded for this call".
	query := logsadapterclientgen.PlatformLogsQueryRequest{
		StartTime: params.Query.StartTime,
		EndTime:   params.Query.EndTime,
	}
	setIfNotEmpty(&query.ClusterInstance, params.Query.ClusterInstances)
	setIfNotEmpty(&query.Namespace, params.Query.Namespaces)
	setIfNotEmpty(&query.PodName, params.Query.PodNames)
	setIfNotEmpty(&query.ContainerName, params.Query.ContainerNames)
	if len(params.Query.Labels) > 0 {
		labels := params.Query.Labels
		query.Labels = &labels
	}
	if len(params.Query.LogLevels) > 0 {
		levels := make([]logsadapterclientgen.PlatformLogsQueryRequestLogLevels, 0, len(params.Query.LogLevels))
		for _, l := range params.Query.LogLevels {
			levels = append(levels, logsadapterclientgen.PlatformLogsQueryRequestLogLevels(l))
		}
		query.LogLevels = &levels
	}
	if params.Query.SearchPhrase != "" {
		query.SearchPhrase = &params.Query.SearchPhrase
	}

	body := logsadapterclientgen.QueryPlatformLogFilterValuesJSONRequestBody{
		Query:  query,
		Filter: logsadapterclientgen.PlatformLogFilterValuesRequestFilter(params.Filter),
	}
	if params.ValueSearch != "" {
		body.ValueSearch = &params.ValueSearch
	}
	if params.MaxValues > 0 {
		body.MaxValues = &params.MaxValues
	}

	resp, err := client.QueryPlatformLogFilterValuesWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotImplemented {
		return nil, ErrPlatformLogFilterValuesNotSupported
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	values := make([]observability.PlatformLogFilterValue, 0, len(resp.JSON200.Values))
	for _, v := range resp.JSON200.Values {
		values = append(values, observability.PlatformLogFilterValue{Value: v.Value, Count: v.Count})
	}

	return &observability.PlatformLogFilterValuesResult{
		Filter:        resp.JSON200.Filter,
		Values:        values,
		TotalValues:   resp.JSON200.TotalValues,
		TotalRelation: string(resp.JSON200.TotalRelation),
		Took:          resp.JSON200.TookMs,
	}, nil
}
