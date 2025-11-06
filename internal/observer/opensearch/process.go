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

	// Convert durationInNanos - handle both int64 and float64 cases
	var durationInNanos int64
	if val, ok := source["durationInNanos"]; ok && val != nil {
		switch v := val.(type) {
		case int64:
			durationInNanos = v
		case float64:
			durationInNanos = int64(v)
		case int:
			durationInNanos = int64(v)
		}
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

	// Safe string extraction helper
	getString := func(key string) string {
		if val, ok := source[key]; ok && val != nil {
			if str, ok := val.(string); ok {
				return str
			}
		}
		return ""
	}

	entry := Span{
		DurationInNanos: durationInNanos,
		EndTime:         endTime,
		Name:            getString("name"),
		StartTime:       startTime,
		SpanID:          getString("spanId"),
		TraceID:         getString("traceId"),
	}

	return entry
}
