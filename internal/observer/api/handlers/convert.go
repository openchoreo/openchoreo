// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// remarshalJSON round-trips between two types describing the same schema.
//
// Not a field-by-field mapper: re-decoding runs types.SearchScope.UnmarshalJSON,
// whose oneOf discrimination the generated union wrapper does not perform.
func remarshalJSON(src, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decoding request: %w", err)
	}
	return nil
}

// rfc3339OrEmpty formats a generated time as the string the internal types take.
//
// Zero maps to "" so ValidateTimeRange reports "startTime is required" — the
// generated fields have no omitempty, and a formatted zero time parses fine.
// Nano precision, because RFC3339 would truncate a query window boundary.
func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func toTypesLogsQuery(src gen.LogsQueryRequest) (*types.LogsQueryRequest, error) {
	var dst types.LogsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

func toTypesEventsQuery(src gen.EventsQueryRequest) (*types.EventsQueryRequest, error) {
	var dst types.EventsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

func toTypesMetricsQuery(src gen.MetricsQueryRequest) (*types.MetricsQueryRequest, error) {
	var dst types.MetricsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

func toTypesRuntimeTopology(src gen.RuntimeTopologyRequest) (*types.RuntimeTopologyRequest, error) {
	var dst types.RuntimeTopologyRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

// Field-mapped because types.CostQueryRequest has no JSON tags to round-trip
// through. A query parameter added to the spec arrives here as a zero value
// rather than a build failure, so add it below when you add it there.
func toTypesCostQuery(
	namespace, environment string,
	params gen.GetComponentCostsParams,
) *types.CostQueryRequest {
	return &types.CostQueryRequest{
		Namespace:   namespace,
		Environment: environment,
		Project:     derefString(params.Project),
		Component:   derefString(params.Component),
		StartTime:   rfc3339OrEmpty(params.StartTime),
		EndTime:     rfc3339OrEmpty(params.EndTime),
		Granularity: derefString(params.Granularity),
	}
}

// Field-mapped for the same reason as toTypesCostQuery.
func toTypesRecommendationQuery(
	namespace, environment string,
	params gen.GetRecommendationsParams,
) *types.RecommendationQueryRequest {
	return &types.RecommendationQueryRequest{
		Namespace:   namespace,
		Environment: environment,
		Project:     derefString(params.Project),
		Component:   derefString(params.Component),
		StartTime:   rfc3339OrEmpty(params.StartTime),
		EndTime:     rfc3339OrEmpty(params.EndTime),
	}
}

// toTypesAuditLogsQuery maps the generated request onto the internal one.
//
// A round-trip rather than a field-by-field copy: the two types describe the
// same schema with the same JSON keys — that is the point of naming the filters
// after the record fields they match — so re-decoding is exact across all
// twenty-odd filters and immune to a mapping typo in either nested group.
//
// The window is the exception and must be set afterwards. The generated fields
// are time.Time with no omitempty, so an absent startTime round-trips as
// "0001-01-01T00:00:00Z" — which parses, and would slip past the required-window
// check. rfc3339OrEmpty maps zero to "" instead; see its doc comment.
func toTypesAuditLogsQuery(src gen.AuditLogsQueryRequest) (*types.AuditLogsQueryRequest, error) {
	var dst types.AuditLogsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

// toTypesAuditLogFilterValues maps the generated request onto the internal one,
// for the same reason as toTypesAuditLogsQuery — including the same window
// correction, on the nested query.
func toTypesAuditLogFilterValues(
	src gen.AuditLogFilterValuesRequest,
) (*types.AuditLogFilterValuesRequest, error) {
	var dst types.AuditLogFilterValuesRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.Query.StartTime = rfc3339OrEmpty(src.Query.StartTime)
	dst.Query.EndTime = rfc3339OrEmpty(src.Query.EndTime)
	return &dst, nil
}

// toTypesPlatformLogsQuery maps the generated query parameters onto the internal request.
func toTypesPlatformLogsQuery(src gen.GetPlatformLogsParams) (*types.PlatformLogsQueryRequest, error) {
	dst := &types.PlatformLogsQueryRequest{
		StartTime: rfc3339OrEmpty(src.StartTime),
		EndTime:   rfc3339OrEmpty(src.EndTime),
	}
	if src.ClusterInstance != nil {
		dst.ClusterInstances = *src.ClusterInstance
	}
	if src.Namespace != nil {
		dst.Namespaces = *src.Namespace
	}
	if src.PodName != nil {
		dst.PodNames = *src.PodName
	}
	if src.ContainerName != nil {
		dst.ContainerNames = *src.ContainerName
	}
	if src.LogLevels != nil {
		dst.LogLevels = *src.LogLevels
	}
	if src.SearchPhrase != nil {
		dst.SearchPhrase = *src.SearchPhrase
	}
	if src.Limit != nil {
		dst.Limit = *src.Limit
	}
	if src.SortOrder != nil {
		dst.SortOrder = string(*src.SortOrder)
	}
	if src.Labels != nil {
		labels, err := ParseLabelSelector(*src.Labels)
		if err != nil {
			return nil, err
		}
		dst.Labels = labels
	}
	return dst, nil
}
