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
					"durationInNanos": int64(101018208),
					"startTime":       "2025-10-28T11:13:56.484388Z",
					"endTime":         "2025-10-28T11:13:56.585406208Z",
				},
			},
			expected: Span{
				TraceID:         "b72e731db5edfd1df2658bd78f751862",
				SpanID:          "614f55c7ccbfffdc",
				Name:            "database-query",
				DurationInNanos: 101018208,
				StartTime:       mustParseTime("2025-10-28T11:13:56.484388Z"),
				EndTime:         mustParseTime("2025-10-28T11:13:56.585406208Z"),
			},
		},
		{
			name: "duration as float64",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace123",
					"spanId":          "span456",
					"name":            "api-call",
					"durationInNanos": float64(200084125),
					"startTime":       "2025-10-28T11:13:56.585424Z",
					"endTime":         "2025-10-28T11:13:56.785508125Z",
				},
			},
			expected: Span{
				TraceID:         "trace123",
				SpanID:          "span456",
				Name:            "api-call",
				DurationInNanos: 200084125,
				StartTime:       mustParseTime("2025-10-28T11:13:56.585424Z"),
				EndTime:         mustParseTime("2025-10-28T11:13:56.785508125Z"),
			},
		},
		{
			name: "duration as int",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace789",
					"spanId":          "span012",
					"name":            "processing",
					"durationInNanos": int(150000000),
					"startTime":       "2025-10-28T12:00:00Z",
					"endTime":         "2025-10-28T12:00:00.15Z",
				},
			},
			expected: Span{
				TraceID:         "trace789",
				SpanID:          "span012",
				Name:            "processing",
				DurationInNanos: 150000000,
				StartTime:       mustParseTime("2025-10-28T12:00:00Z"),
				EndTime:         mustParseTime("2025-10-28T12:00:00.15Z"),
			},
		},
		{
			name: "missing optional fields",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId": "trace-minimal",
					"spanId":  "span-minimal",
					"name":    "minimal-span",
					// Missing durationInNanos, startTime, endTime
				},
			},
			expected: Span{
				TraceID:         "trace-minimal",
				SpanID:          "span-minimal",
				Name:            "minimal-span",
				DurationInNanos: 0,
				StartTime:       time.Time{},
				EndTime:         time.Time{},
			},
		},
		{
			name: "null values",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-null",
					"spanId":          "span-null",
					"name":            "null-span",
					"durationInNanos": nil,
					"startTime":       nil,
					"endTime":         nil,
				},
			},
			expected: Span{
				TraceID:         "trace-null",
				SpanID:          "span-null",
				Name:            "null-span",
				DurationInNanos: 0,
				StartTime:       time.Time{},
				EndTime:         time.Time{},
			},
		},
		{
			name: "invalid time formats",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-invalid-time",
					"spanId":          "span-invalid-time",
					"name":            "invalid-time-span",
					"durationInNanos": int64(50000000),
					"startTime":       "invalid-time-format",
					"endTime":         "2025-13-45T25:70:70Z",
				},
			},
			expected: Span{
				TraceID:         "trace-invalid-time",
				SpanID:          "span-invalid-time",
				Name:            "invalid-time-span",
				DurationInNanos: 50000000,
				StartTime:       time.Time{},
				EndTime:         time.Time{},
			},
		},
		{
			name: "non-string time values",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-non-string-time",
					"spanId":          "span-non-string-time",
					"name":            "non-string-time-span",
					"durationInNanos": int64(75000000),
					"startTime":       123456789,
					"endTime":         true,
				},
			},
			expected: Span{
				TraceID:         "trace-non-string-time",
				SpanID:          "span-non-string-time",
				Name:            "non-string-time-span",
				DurationInNanos: 75000000,
				StartTime:       time.Time{},
				EndTime:         time.Time{},
			},
		},
		{
			name: "zero duration",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-zero",
					"spanId":          "span-zero",
					"name":            "zero-duration-span",
					"durationInNanos": int64(0),
					"startTime":       "2025-10-28T15:00:00Z",
					"endTime":         "2025-10-28T15:00:00Z",
				},
			},
			expected: Span{
				TraceID:         "trace-zero",
				SpanID:          "span-zero",
				Name:            "zero-duration-span",
				DurationInNanos: 0,
				StartTime:       mustParseTime("2025-10-28T15:00:00Z"),
				EndTime:         mustParseTime("2025-10-28T15:00:00Z"),
			},
		},
		{
			name: "large duration",
			hit: Hit{
				Source: map[string]interface{}{
					"traceId":         "trace-large",
					"spanId":          "span-large",
					"name":            "large-duration-span",
					"durationInNanos": int64(9223372036854775807), // Max int64
					"startTime":       "2025-10-28T16:00:00Z",
					"endTime":         "2025-10-28T16:00:09.223372036Z",
				},
			},
			expected: Span{
				TraceID:         "trace-large",
				SpanID:          "span-large",
				Name:            "large-duration-span",
				DurationInNanos: 9223372036854775807,
				StartTime:       mustParseTime("2025-10-28T16:00:00Z"),
				EndTime:         mustParseTime("2025-10-28T16:00:09.223372036Z"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSpanEntry(tt.hit)

			// Compare all fields
			if result.TraceID != tt.expected.TraceID {
				t.Errorf("TraceID: expected '%s', got '%s'", tt.expected.TraceID, result.TraceID)
			}

			if result.SpanID != tt.expected.SpanID {
				t.Errorf("SpanId: expected '%s', got '%s'", tt.expected.SpanID, result.SpanID)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("Name: expected '%s', got '%s'", tt.expected.Name, result.Name)
			}

			if result.DurationInNanos != tt.expected.DurationInNanos {
				t.Errorf("DurationInNanos: expected %d, got %d", tt.expected.DurationInNanos, result.DurationInNanos)
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
					// Missing traceId, spanId, name
					"durationInNanos": int64(100000),
				},
			},
			expected: Span{
				TraceID:         "",
				SpanID:          "",
				Name:            "",
				DurationInNanos: 100000,
				StartTime:       time.Time{},
				EndTime:         time.Time{},
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
				TraceID:         "",
				SpanID:          "",
				Name:            "",
				DurationInNanos: 0,
				StartTime:       time.Time{},
				EndTime:         time.Time{},
			},
		},
	}

	for _, tt := range safeTests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSpanEntry(tt.hit)

			if result.TraceID != tt.expected.TraceID {
				t.Errorf("TraceID: expected '%s', got '%s'", tt.expected.TraceID, result.TraceID)
			}

			if result.SpanID != tt.expected.SpanID {
				t.Errorf("SpanId: expected '%s', got '%s'", tt.expected.SpanID, result.SpanID)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("Name: expected '%s', got '%s'", tt.expected.Name, result.Name)
			}

			if result.DurationInNanos != tt.expected.DurationInNanos {
				t.Errorf("DurationInNanos: expected %d, got %d", tt.expected.DurationInNanos, result.DurationInNanos)
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
		if result.TraceID != "" {
			t.Errorf("Expected empty TraceID, got '%s'", result.TraceID)
		}
		if result.SpanID != "" {
			t.Errorf("Expected empty SpanId, got '%s'", result.SpanID)
		}
		if result.Name != "" {
			t.Errorf("Expected empty Name, got '%s'", result.Name)
		}
		if result.DurationInNanos != 0 {
			t.Errorf("Expected zero DurationInNanos, got %d", result.DurationInNanos)
		}
	})

	t.Run("nil source map", func(t *testing.T) {
		hit := Hit{
			Source: nil,
		}

		result := ParseSpanEntry(hit)

		// All fields should have zero/empty values
		if result.TraceID != "" {
			t.Errorf("Expected empty TraceID, got '%s'", result.TraceID)
		}
		if result.SpanID != "" {
			t.Errorf("Expected empty SpanId, got '%s'", result.SpanID)
		}
		if result.Name != "" {
			t.Errorf("Expected empty Name, got '%s'", result.Name)
		}
		if result.DurationInNanos != 0 {
			t.Errorf("Expected zero DurationInNanos, got %d", result.DurationInNanos)
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
