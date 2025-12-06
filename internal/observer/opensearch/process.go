// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"time"
)

// ParseSpanEntry converts a search hit of a span to a SpanEntry struct
func ParseSpanEntry(hit Hit) Span {
	source := hit.Source

	// Handle nil source map
	if source == nil {
		return Span{}
	}

	// Parse startTime
	var startTime time.Time
	if val, ok := source["startTime"]; ok && val != nil {
		if timeStr, ok := val.(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
				startTime = parsed
			}
		}
	}

	// Parse endTime
	var endTime time.Time
	if val, ok := source["endTime"]; ok && val != nil {
		if timeStr, ok := val.(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
				endTime = parsed
			}
		}
	}

	// Calculate DurationNanoseconds from endTime - startTime
	var durationNanoseconds int64
	if !startTime.IsZero() && !endTime.IsZero() {
		durationNanoseconds = endTime.Sub(startTime).Nanoseconds()
	}

	// Safe string extraction helper
	getString := func(key string) string {
		if val, ok := source[key]; ok && val != nil {
			if str, ok := val.(string); ok {
				return str
			}
		}
		return ""
	}

	// Extract resource fields (component-uid, project-uid, etc.)
	var componentUID, projectUID string
	if resource, ok := source["resource"].(map[string]interface{}); ok {
		if val, ok := resource["openchoreo.dev/component-uid"]; ok {
			if str, ok := val.(string); ok {
				componentUID = str
			}
		}
		if val, ok := resource["openchoreo.dev/project-uid"]; ok {
			if str, ok := val.(string); ok {
				projectUID = str
			}
		}
	}

	entry := Span{
		DurationNanoseconds:    durationNanoseconds,
		EndTime:                endTime,
		Name:                   getString("name"),
		OpenChoreoComponentUID: componentUID,
		OpenChoreoProjectUID:   projectUID,
		ParentSpanID:           getString("parentSpanId"),
		StartTime:              startTime,
		SpanID:                 getString("spanId"),
	}

	return entry
}

// GetTraceID extracts the traceId from a span hit
func GetTraceID(hit Hit) string {
	if hit.Source == nil {
		return ""
	}
	if val, ok := hit.Source["traceId"]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
