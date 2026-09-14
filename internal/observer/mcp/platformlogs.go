// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/openchoreo/openchoreo/internal/observer/api/handlers"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// defaultMaxSources caps the breakdown returned per coordinate. Lower than the
// REST endpoint's 100: values come back count-descending, so the head is the
// useful part and a full page of pod names is a large answer for a triage
// question. The REST cap of 1000 still applies above this.
const defaultMaxSources = 20

// platformLogSourceFields maps the values include_sources accepts onto the
// filter names the platform logs API uses. The tool surface stays snake_case
// like its own parameters; the API keeps its camelCase filter names.
var platformLogSourceFields = map[string]string{
	"cluster_instance":     "clusterInstance",
	"kubernetes_namespace": "namespace",
	"pod_name":             "podName",
	"container_name":       "containerName",
}

// platformLogSourceFieldNames lists the include_sources values in a stable order,
// for schema enums and error messages.
var platformLogSourceFieldNames = slices.Sorted(maps.Keys(platformLogSourceFields))

// PlatformLogsResult is the query_platform_logs response: the log records, plus
// an optional breakdown of which coordinates produced them.
type PlatformLogsResult struct {
	Logs   []types.PlatformLog `json:"logs"`
	Total  int                 `json:"total"`
	TookMs int                 `json:"tookMs"`

	// Sources is present only when include_sources named fields, keyed by those
	// same values. Each list is ordered by count descending.
	Sources map[string][]types.PlatformLogFilterValue `json:"sources,omitempty"`

	// SourcesError says why a requested breakdown is missing. The records are
	// still returned: an adapter can serve platform logs and yet be unable to
	// aggregate them, and failing the call would throw away logs the caller
	// could have had.
	SourcesError string `json:"sourcesError,omitempty"`
}

// QueryPlatformLogs queries platform logs and, when include_sources names
// coordinates, the distinct values each takes across the matching records.
//
// The records query and one aggregation per requested coordinate run
// concurrently; each is authorized on its own by the service decorator.
func (h *MCPHandler) QueryPlatformLogs(ctx context.Context,
	clusterInstances, namespaces, podNames, containerNames []string,
	labels, startTime, endTime, searchPhrase string, logLevels []string,
	limit int, sortOrder string, includeSources []string, maxSources int,
) (any, error) {
	parsedLabels, err := handlers.ParseLabelSelector(labels)
	if err != nil {
		return nil, err
	}

	req := &types.PlatformLogsQueryRequest{
		ClusterInstances: clusterInstances,
		Namespaces:       namespaces,
		PodNames:         podNames,
		ContainerNames:   containerNames,
		Labels:           parsedLabels,
		StartTime:        startTime,
		EndTime:          endTime,
		SearchPhrase:     searchPhrase,
		LogLevels:        logLevels,
		Limit:            limit,
		SortOrder:        sortOrder,
	}
	// Applies the same caps and defaults the REST endpoint applies, so a query
	// the API would reject is not accepted here instead.
	if err := handlers.ValidatePlatformLogsQueryRequest(req); err != nil {
		return nil, err
	}

	// An unknown coordinate is the caller's mistake, so it fails the call rather
	// than degrading to a missing breakdown.
	fields, err := resolvePlatformLogSourceFields(includeSources)
	if err != nil {
		return nil, err
	}

	if maxSources == 0 {
		maxSources = defaultMaxSources
	}

	// Every aggregation is built and validated before any of them is launched: an
	// out-of-range max_sources is the caller's mistake like an unknown coordinate,
	// and must fail the call rather than arrive as SourcesError, which reports what
	// the backend could not do.
	valuesReqs := make(map[string]*types.PlatformLogFilterValuesRequest, len(fields))
	for name, filter := range fields {
		// The filter's own selections stay in the query: the adapter drops them,
		// so that counting podName under a selected pod still offers the others.
		valuesReq := &types.PlatformLogFilterValuesRequest{
			Query:     *req,
			Filter:    filter,
			MaxValues: maxSources,
		}
		if err := handlers.ValidatePlatformLogFilterValuesRequest(valuesReq); err != nil {
			return nil, err
		}
		valuesReqs[name] = valuesReq
	}

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		logsResp   *types.PlatformLogsResponse
		logsErr    error
		sources    = make(map[string][]types.PlatformLogFilterValue, len(fields))
		sourcesErr error
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := h.platformLogsService.QueryPlatformLogs(ctx, req)
		mu.Lock()
		defer mu.Unlock()
		logsResp, logsErr = resp, err
	}()

	for name, valuesReq := range valuesReqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := h.platformLogsService.QueryPlatformLogFilterValues(ctx, valuesReq)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if sourcesErr == nil {
					sourcesErr = err
				}
				return
			}
			sources[name] = resp.Values
		}()
	}

	wg.Wait()

	if logsErr != nil {
		return nil, logsErr
	}

	result := &PlatformLogsResult{
		Logs:   logsResp.Logs,
		Total:  logsResp.Total,
		TookMs: logsResp.TookMs,
	}
	if len(sources) > 0 {
		result.Sources = sources
	}
	if sourcesErr != nil {
		// Whatever did come back is kept - those counts are correct, and
		// SourcesError says the breakdown is short of what was asked for.
		result.SourcesError = sourcesErr.Error()
	}
	return result, nil
}

// resolvePlatformLogSourceFields maps the requested include_sources values onto
// API filter names, rejecting duplicates and unknown values.
func resolvePlatformLogSourceFields(includeSources []string) (map[string]string, error) {
	fields := make(map[string]string, len(includeSources))
	for _, name := range includeSources {
		filter, ok := platformLogSourceFields[name]
		if !ok {
			return nil, fmt.Errorf("include_sources value %q is not a coordinate; expected one of %s",
				name, strings.Join(platformLogSourceFieldNames, ", "))
		}
		if _, dup := fields[name]; dup {
			return nil, fmt.Errorf("duplicate include_sources value %q is not allowed", name)
		}
		fields[name] = filter
	}
	return fields, nil
}
