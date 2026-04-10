// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"strings"
	"time"
)

const (
	SpanStatusOK    = "ok"
	SpanStatusError = "error"
	SpanStatusUnset = "unset"
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

	// Extract attributes
	var attributes map[string]interface{}
	if attrs, ok := source["attributes"].(map[string]interface{}); ok {
		attributes = attrs
	}

	// Extract resource attributes
	var resourceAttributes map[string]interface{}
	if resource, ok := source["resource"].(map[string]interface{}); ok {
		resourceAttributes = resource
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
		SpanKind:               getString("kind"),
		Status:                 DetermineSpanStatus(source),
		Attributes:             attributes,
		ResourceAttributes:     resourceAttributes,
	}

	return entry
}

// DetermineSpanStatus derives a span's execution status
// Returns one of "ok", "error", or "unset".
func DetermineSpanStatus(spanHit map[string]interface{}) string {
	if spanHit == nil {
		return SpanStatusUnset
	}

	// Check OpenTelemetry status code from nested status map
	if statusMap, ok := spanHit["status"].(map[string]interface{}); ok {
		if code, ok := statusMap["code"].(string); ok {
			switch strings.ToLower(code) {
			case SpanStatusError:
				return SpanStatusError
			case SpanStatusOK:
				return SpanStatusOK
			}
		}
	}

	return SpanStatusUnset
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
