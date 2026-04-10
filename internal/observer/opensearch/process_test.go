// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"testing"
	"time"
)

func TestParseSpanEntry(t *testing.T) {
	tests := []struct {
		name     string
		hit      Hit
		expected Span
	}{
		{
			name: "complete span entry",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "b72e731db5edfd1df2658bd78f751862",
					"spanId":          "614f55c7ccbfffdc",
					"name":            "database-query",
					"kind":            "SERVER",
					"durationInNanos": int64(101018208),
					"startTime":       "2025-10-28T11:13:56.484388Z",
					"endTime":         "2025-10-28T11:13:56.585406208Z",
				},
			},
			expected: Span{
				SpanID:              "614f55c7ccbfffdc",
				Name:                "database-query",
				SpanKind:            "SERVER",
				DurationNanoseconds: 101018208,
				StartTime:           mustParseTime("2025-10-28T11:13:56.484388Z"),
				EndTime:             mustParseTime("2025-10-28T11:13:56.585406208Z"),
			},
		},
		{
			name: "duration as float64",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace123",
					"spanId":          "a1b2c3d4e5f67890",
					"name":            "api-call",
					"durationInNanos": float64(200084125),
					"startTime":       "2025-10-28T11:13:56.585424Z",
					"endTime":         "2025-10-28T11:13:56.785508125Z",
				},
			},
			expected: Span{
				SpanID:              "a1b2c3d4e5f67890",
				Name:                "api-call",
				DurationNanoseconds: 200084125,
				StartTime:           mustParseTime("2025-10-28T11:13:56.585424Z"),
				EndTime:             mustParseTime("2025-10-28T11:13:56.785508125Z"),
			},
		},
		{
			name: "duration as int",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace789",
					"spanId":          "b2c3d4e5f6789abc",
					"name":            "processing",
					"durationInNanos": int(150000000),
					"startTime":       "2025-10-28T12:00:00Z",
					"endTime":         "2025-10-28T12:00:00.15Z",
				},
			},
			expected: Span{
				SpanID:              "b2c3d4e5f6789abc",
				Name:                "processing",
				DurationNanoseconds: 150000000,
				StartTime:           mustParseTime("2025-10-28T12:00:00Z"),
				EndTime:             mustParseTime("2025-10-28T12:00:00.15Z"),
			},
		},
		{
			name: "missing optional fields",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId": "trace-minimal",
					"spanId":  "c3d4e5f67890abcd",
					"name":    "minimal-span",
					// Missing durationInNanos, startTime, endTime
				},
			},
			expected: Span{
				SpanID:              "c3d4e5f67890abcd",
				Name:                "minimal-span",
				DurationNanoseconds: 0,
				StartTime:           time.Time{},
				EndTime:             time.Time{},
			},
		},
		{
			name: "null values",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-null",
					"spanId":          "d4e5f67890abcdef",
					"name":            "null-span",
					"durationInNanos": nil,
					"startTime":       nil,
					"endTime":         nil,
				},
			},
			expected: Span{
				SpanID:              "d4e5f67890abcdef",
				Name:                "null-span",
				DurationNanoseconds: 0,
				StartTime:           time.Time{},
				EndTime:             time.Time{},
			},
		},
		{
			name: "invalid time formats",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":   "trace-invalid-time",
					"spanId":    "e5f67890abcdef12",
					"name":      "invalid-time-span",
					"startTime": "invalid-time-format",
					"endTime":   "2025-13-45T25:70:70Z",
				},
			},
			expected: Span{
				SpanID:              "e5f67890abcdef12",
				Name:                "invalid-time-span",
				DurationNanoseconds: 0, // Duration is 0 because times are invalid
				StartTime:           time.Time{},
				EndTime:             time.Time{},
			},
		},
		{
			name: "non-string time values",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":   "trace-non-string-time",
					"spanId":    "f67890abcdef1234",
					"name":      "non-string-time-span",
					"startTime": 123456789,
					"endTime":   true,
				},
			},
			expected: Span{
				SpanID:              "f67890abcdef1234",
				Name:                "non-string-time-span",
				DurationNanoseconds: 0, // Duration is 0 because times are non-string
				StartTime:           time.Time{},
				EndTime:             time.Time{},
			},
		},
		{
			name: "zero duration",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-zero",
					"spanId":          "1234567890abcdef",
					"name":            "zero-duration-span",
					"durationInNanos": int64(0),
					"startTime":       "2025-10-28T15:00:00Z",
					"endTime":         "2025-10-28T15:00:00Z",
				},
			},
			expected: Span{
				SpanID:              "1234567890abcdef",
				Name:                "zero-duration-span",
				DurationNanoseconds: 0,
				StartTime:           mustParseTime("2025-10-28T15:00:00Z"),
				EndTime:             mustParseTime("2025-10-28T15:00:00Z"),
			},
		},
		{
			name: "large duration",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":   "trace-large",
					"spanId":    "234567890abcdef1",
					"name":      "large-duration-span",
					"startTime": "2025-10-28T16:00:00Z",
					"endTime":   "2025-10-28T16:00:09.223372036Z",
				},
			},
			expected: Span{
				SpanID:              "234567890abcdef1",
				Name:                "large-duration-span",
				DurationNanoseconds: 9223372036, // Calculated from endTime - startTime
				StartTime:           mustParseTime("2025-10-28T16:00:00Z"),
				EndTime:             mustParseTime("2025-10-28T16:00:09.223372036Z"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSpanEntry(tt.hit)

			// Compare all fields
			if result.SpanID != tt.expected.SpanID {
				t.Errorf("SpanId: expected '%s', got '%s'", tt.expected.SpanID, result.SpanID)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("Name: expected '%s', got '%s'", tt.expected.Name, result.Name)
			}

			if result.DurationNanoseconds != tt.expected.DurationNanoseconds {
				t.Errorf("DurationInNanos: expected %d, got %d", tt.expected.DurationNanoseconds, result.DurationNanoseconds)
			}

			if !result.StartTime.Equal(tt.expected.StartTime) {
				t.Errorf("StartTime: expected '%v', got '%v'", tt.expected.StartTime, result.StartTime)
			}

			if !result.EndTime.Equal(tt.expected.EndTime) {
				t.Errorf("EndTime: expected '%v', got '%v'", tt.expected.EndTime, result.EndTime)
			}
		})
	}
}

func TestParseSpanEntry_SafeHandling(t *testing.T) {
	// Test cases that should be handled safely without panics
	safeTests := []struct {
		name     string
		hit      Hit
		expected Span
	}{
		{
			name: "missing required string fields",
			hit: Hit{
				Source: map[string]interface{}{
					// Missing traceId, spanId, name, and times
				},
			},
			expected: Span{
				SpanID:              "",
				Name:                "",
				DurationNanoseconds: 0, // Duration is 0 because no times provided
				StartTime:           time.Time{},
				EndTime:             time.Time{},
			},
		},
		{
			name: "non-string required fields",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId": 123,
					"spanId":  456,
					"name":    true,
				},
			},
			expected: Span{
				SpanID:              "",
				Name:                "",
				DurationNanoseconds: 0,
				StartTime:           time.Time{},
				EndTime:             time.Time{},
			},
		},
	}

	for _, tt := range safeTests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSpanEntry(tt.hit)

			if result.SpanID != tt.expected.SpanID {
				t.Errorf("SpanId: expected '%s', got '%s'", tt.expected.SpanID, result.SpanID)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("Name: expected '%s', got '%s'", tt.expected.Name, result.Name)
			}

			if result.DurationNanoseconds != tt.expected.DurationNanoseconds {
				t.Errorf("DurationInNanos: expected %d, got %d", tt.expected.DurationNanoseconds, result.DurationNanoseconds)
			}
		})
	}
}

func TestParseSpanEntry_EdgeCases(t *testing.T) {
	t.Run("empty source map", func(t *testing.T) {
		hit := Hit{
			Source: map[string]interface{}{},
		}

		result := ParseSpanEntry(hit)

		// All fields should have zero/empty values
		if result.SpanID != "" {
			t.Errorf("Expected empty SpanId, got '%s'", result.SpanID)
		}
		if result.Name != "" {
			t.Errorf("Expected empty Name, got '%s'", result.Name)
		}
		if result.DurationNanoseconds != 0 {
			t.Errorf("Expected zero DurationInNanos, got %d", result.DurationNanoseconds)
		}
	})

	t.Run("nil source map", func(t *testing.T) {
		hit := Hit{
			Source: nil,
		}

		result := ParseSpanEntry(hit)

		// All fields should have zero/empty values
		if result.SpanID != "" {
			t.Errorf("Expected empty SpanId, got '%s'", result.SpanID)
		}
		if result.Name != "" {
			t.Errorf("Expected empty Name, got '%s'", result.Name)
		}
		if result.DurationNanoseconds != 0 {
			t.Errorf("Expected zero DurationInNanos, got %d", result.DurationNanoseconds)
		}
	})
}

// Helper function to parse time strings for test data
func mustParseTime(timeStr string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		panic("Failed to parse time in test: " + timeStr)
	}
	return parsed
}

func TestDetermineSpanStatus(t *testing.T) {
	tests := []struct {
		name     string
		spanHit  map[string]interface{}
		expected string
	}{
		{
			name:     "nil input",
			spanHit:  nil,
			expected: SpanStatusUnset,
		},
		{
			name:     "missing status key",
			spanHit:  map[string]interface{}{"name": "some-span"},
			expected: SpanStatusUnset,
		},
		{
			name: "status key is not a map",
			spanHit: map[string]interface{}{
				"status": "ok",
			},
			expected: SpanStatusUnset,
		},
		{
			name: "status map missing code",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"message": "all good"},
			},
			expected: SpanStatusUnset,
		},
		{
			name: "string code ok lowercase",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": "ok"},
			},
			expected: SpanStatusOK,
		},
		{
			name: "string code ok uppercase",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": "OK"},
			},
			expected: SpanStatusOK,
		},
		{
			name: "string code ok mixed case",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": "Ok"},
			},
			expected: SpanStatusOK,
		},
		{
			name: "string code error lowercase",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": "error"},
			},
			expected: SpanStatusError,
		},
		{
			name: "string code error uppercase",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": "ERROR"},
			},
			expected: SpanStatusError,
		},
		{
			name: "string code unknown",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": "unknown"},
			},
			expected: SpanStatusUnset,
		},
		{
			name: "non-string code type",
			spanHit: map[string]interface{}{
				"status": map[string]interface{}{"code": true},
			},
			expected: SpanStatusUnset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineSpanStatus(tt.spanHit)
			if result != tt.expected {
				t.Errorf("DetermineSpanStatus: expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseSpanEntry_StatusPropagation(t *testing.T) {
	tests := []struct {
		name           string
		hit            Hit
		expectedStatus string
	}{
		{
			name: "span with string ok status",
			hit: Hit{
				Source: map[string]interface{}{
					"spanId": "span1",
					"name":   "test-span",
					"status": map[string]interface{}{"code": "ok"},
				},
			},
			expectedStatus: SpanStatusOK,
		},
		{
			name: "span with string error status",
			hit: Hit{
				Source: map[string]interface{}{
					"spanId": "span2",
					"name":   "error-span",
					"status": map[string]interface{}{"code": "ERROR"},
				},
			},
			expectedStatus: SpanStatusError,
		},
		{
			name: "span with missing status",
			hit: Hit{
				Source: map[string]interface{}{
					"spanId": "span5",
					"name":   "no-status-span",
				},
			},
			expectedStatus: SpanStatusUnset,
		},
		{
			name: "span with invalid status payload",
			hit: Hit{
				Source: map[string]interface{}{
					"spanId": "span6",
					"name":   "invalid-status-span",
					"status": "not-a-map",
				},
			},
			expectedStatus: SpanStatusUnset,
		},
		{
			// nil source returns early with zero-value Span before DetermineSpanStatus is called
			name:           "nil source",
			hit:            Hit{Source: nil},
			expectedStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSpanEntry(tt.hit)
			if result.Status != tt.expectedStatus {
				t.Errorf("ParseSpanEntry status: expected %q, got %q", tt.expectedStatus, result.Status)
			}
		})
	}
}

func TestGetTraceID(t *testing.T) {
	tests := []struct {
		name     string
		hit      Hit
		expected string
	}{
		{
			name: "valid traceId",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId": "b72e731db5edfd1df2658bd78f751862",
				},
			},
			expected: "b72e731db5edfd1df2658bd78f751862",
		},
		{
			name: "missing traceId",
			hit: Hit{
				Source: map[string]interface{}{
					"spanId": "614f55c7ccbfffdc",
				},
			},
			expected: "",
		},
		{
			name: "nil traceId",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId": nil,
				},
			},
			expected: "",
		},
		{
			name: "non-string traceId",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId": 12345,
				},
			},
			expected: "",
		},
		{
			name: "nil source",
			hit: Hit{
				Source: nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTraceID(tt.hit)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
