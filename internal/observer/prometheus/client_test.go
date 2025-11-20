// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

const (
	statusSuccess = "success"
)

func TestConvertToTimeSeriesResponse_Vector(t *testing.T) {
	vector := model.Vector{
		&model.Sample{
			Metric: model.Metric{
				"__name__": "up",
				"job":      "prometheus",
				"instance": "localhost:9090",
			},
			Value:     1,
			Timestamp: model.Time(1699876543000), // milliseconds
		},
		&model.Sample{
			Metric: model.Metric{
				"__name__": "up",
				"job":      "node-exporter",
				"instance": "localhost:9100",
			},
			Value:     1,
			Timestamp: model.Time(1699876543000),
		},
	}

	result := convertToTimeSeriesResponse(vector)

	if result.Status != statusSuccess {
		t.Errorf("Expected status 'success', got %q", result.Status)
	}

	if result.Data.ResultType != "vector" {
		t.Errorf("Expected resultType 'vector', got %q", result.Data.ResultType)
	}

	if len(result.Data.Result) != 2 {
		t.Fatalf("Expected 2 time series, got %d", len(result.Data.Result))
	}

	series1 := result.Data.Result[0]
	if series1.Metric["__name__"] != "up" {
		t.Errorf("Expected metric name 'up', got %q", series1.Metric["__name__"])
	}
	if series1.Metric["job"] != "prometheus" {
		t.Errorf("Expected job 'prometheus', got %q", series1.Metric["job"])
	}

	if len(series1.Values) != 1 {
		t.Fatalf("Expected 1 data point, got %d", len(series1.Values))
	}

	expectedTimestamp := 1699876543.0
	if series1.Values[0].Timestamp != expectedTimestamp {
		t.Errorf("Expected timestamp %f, got %f", expectedTimestamp, series1.Values[0].Timestamp)
	}

	if series1.Values[0].Value != "1" {
		t.Errorf("Expected value '1', got %q", series1.Values[0].Value)
	}
}

func TestConvertToTimeSeriesResponse_Matrix(t *testing.T) {
	matrix := model.Matrix{
		&model.SampleStream{
			Metric: model.Metric{
				"__name__":             "cpu_usage",
				"label_component_name": "test-component",
			},
			Values: []model.SamplePair{
				{
					Timestamp: model.Time(1699876540000),
					Value:     0.5,
				},
				{
					Timestamp: model.Time(1699876550000),
					Value:     0.6,
				},
				{
					Timestamp: model.Time(1699876560000),
					Value:     0.7,
				},
			},
		},
	}

	result := convertToTimeSeriesResponse(matrix)

	if result.Status != statusSuccess {
		t.Errorf("Expected status 'success', got %q", result.Status)
	}

	if result.Data.ResultType != "matrix" {
		t.Errorf("Expected resultType 'matrix', got %q", result.Data.ResultType)
	}

	if len(result.Data.Result) != 1 {
		t.Fatalf("Expected 1 time series, got %d", len(result.Data.Result))
	}

	series := result.Data.Result[0]
	if series.Metric["__name__"] != "cpu_usage" {
		t.Errorf("Expected metric name 'cpu_usage', got %q", series.Metric["__name__"])
	}

	if len(series.Values) != 3 {
		t.Fatalf("Expected 3 data points, got %d", len(series.Values))
	}

	// Verify first data point
	if series.Values[0].Timestamp != 1699876540.0 {
		t.Errorf("Expected timestamp 1699876540.0, got %f", series.Values[0].Timestamp)
	}
	if series.Values[0].Value != "0.5" {
		t.Errorf("Expected value '0.5', got %q", series.Values[0].Value)
	}

	// Verify last data point
	if series.Values[2].Timestamp != 1699876560.0 {
		t.Errorf("Expected timestamp 1699876560.0, got %f", series.Values[2].Timestamp)
	}
	if series.Values[2].Value != "0.7" {
		t.Errorf("Expected value '0.7', got %q", series.Values[2].Value)
	}
}

func TestConvertToTimeSeriesResponse_Scalar(t *testing.T) {
	scalar := &model.Scalar{
		Value:     42.5,
		Timestamp: model.Time(1699876543000),
	}

	result := convertToTimeSeriesResponse(scalar)

	// Verify response structure
	if result.Status != statusSuccess {
		t.Errorf("Expected status 'success', got %q", result.Status)
	}

	if result.Data.ResultType != "scalar" {
		t.Errorf("Expected resultType 'scalar', got %q", result.Data.ResultType)
	}

	if len(result.Data.Result) != 1 {
		t.Fatalf("Expected 1 time series, got %d", len(result.Data.Result))
	}

	series := result.Data.Result[0]
	if len(series.Metric) != 0 {
		t.Errorf("Expected empty metric map for scalar, got %d entries", len(series.Metric))
	}

	if len(series.Values) != 1 {
		t.Fatalf("Expected 1 data point, got %d", len(series.Values))
	}

	if series.Values[0].Timestamp != 1699876543.0 {
		t.Errorf("Expected timestamp 1699876543.0, got %f", series.Values[0].Timestamp)
	}

	if series.Values[0].Value != "42.5" {
		t.Errorf("Expected value '42.5', got %q", series.Values[0].Value)
	}
}

func TestConvertToTimeSeriesResponse_EmptyVector(t *testing.T) {
	vector := model.Vector{}

	result := convertToTimeSeriesResponse(vector)

	if result.Status != statusSuccess {
		t.Errorf("Expected status 'success', got %q", result.Status)
	}

	if len(result.Data.Result) != 0 {
		t.Errorf("Expected 0 time series for empty vector, got %d", len(result.Data.Result))
	}
}

func TestConvertTimeSeriesToTimeValuePoints(t *testing.T) {
	tests := []struct {
		name     string
		input    TimeSeries
		expected []TimeValuePoint
	}{
		{
			name: "single data point",
			input: TimeSeries{
				Metric: map[string]string{"label": "value"},
				Values: []DataPoint{
					{
						Timestamp: 1699876543.0,
						Value:     "100.5",
					},
				},
			},
			expected: []TimeValuePoint{
				{
					Time:  "2023-11-13T11:55:43Z",
					Value: 100.5,
				},
			},
		},
		{
			name: "multiple data points",
			input: TimeSeries{
				Metric: map[string]string{"component": "test"},
				Values: []DataPoint{
					{Timestamp: 1699876540.0, Value: "10.1"},
					{Timestamp: 1699876550.0, Value: "20.2"},
					{Timestamp: 1699876560.0, Value: "30.3"},
				},
			},
			expected: []TimeValuePoint{
				{Time: "2023-11-13T11:55:40Z", Value: 10.1},
				{Time: "2023-11-13T11:55:50Z", Value: 20.2},
				{Time: "2023-11-13T11:56:00Z", Value: 30.3},
			},
		},
		{
			name: "zero value",
			input: TimeSeries{
				Values: []DataPoint{
					{Timestamp: 1699876543.0, Value: "0"},
				},
			},
			expected: []TimeValuePoint{
				{Time: "2023-11-13T11:55:43Z", Value: 0},
			},
		},
		{
			name: "negative value",
			input: TimeSeries{
				Values: []DataPoint{
					{Timestamp: 1699876543.0, Value: "-15.75"},
				},
			},
			expected: []TimeValuePoint{
				{Time: "2023-11-13T11:55:43Z", Value: -15.75},
			},
		},
		{
			name: "scientific notation",
			input: TimeSeries{
				Values: []DataPoint{
					{Timestamp: 1699876543.0, Value: "1.5e+06"},
				},
			},
			expected: []TimeValuePoint{
				{Time: "2023-11-13T11:55:43Z", Value: 1500000},
			},
		},
		{
			name: "empty values",
			input: TimeSeries{
				Metric: map[string]string{},
				Values: []DataPoint{},
			},
			expected: []TimeValuePoint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertTimeSeriesToTimeValuePoints(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d points, got %d", len(tt.expected), len(result))
			}

			for i, point := range result {
				if point.Time != tt.expected[i].Time {
					t.Errorf("Point %d: expected time %q, got %q", i, tt.expected[i].Time, point.Time)
				}
				if point.Value != tt.expected[i].Value {
					t.Errorf("Point %d: expected value %f, got %f", i, tt.expected[i].Value, point.Value)
				}
			}
		})
	}
}

func TestConvertTimeSeriesToTimeValuePoints_TimeFormat(t *testing.T) {
	// Test that time format is ISO 8601 (RFC3339)
	ts := TimeSeries{
		Values: []DataPoint{
			{Timestamp: 1699876543.123, Value: "100"}, // With fractional seconds (should be truncated)
		},
	}

	result := ConvertTimeSeriesToTimeValuePoints(ts)

	if len(result) != 1 {
		t.Fatalf("Expected 1 point, got %d", len(result))
	}

	expectedTime := "2023-11-13T11:55:43Z"
	if result[0].Time != expectedTime {
		t.Errorf("Expected ISO 8601 time %q, got %q", expectedTime, result[0].Time)
	}

	parsedTime, err := time.Parse(time.RFC3339, result[0].Time)
	if err != nil {
		t.Errorf("Failed to parse time as RFC3339: %v", err)
	}

	expectedParsed := time.Unix(1699876543, 0).UTC()
	if !parsedTime.Equal(expectedParsed) {
		t.Errorf("Parsed time %v doesn't match expected %v", parsedTime, expectedParsed)
	}
}

func TestTimeSeriesResponse_Structure(t *testing.T) {
	resp := &TimeSeriesResponse{
		Status: "success",
		Data: TimeSeriesData{
			ResultType: "matrix",
			Result: []TimeSeries{
				{
					Metric: map[string]string{"label": "value"},
					Values: []DataPoint{
						{Timestamp: 1234567890.0, Value: "42"},
					},
				},
			},
		},
	}

	if resp.Status != statusSuccess {
		t.Errorf("Expected status 'success', got %q", resp.Status)
	}

	if resp.Data.ResultType != "matrix" {
		t.Errorf("Expected resultType 'matrix', got %q", resp.Data.ResultType)
	}

	if len(resp.Data.Result) != 1 {
		t.Errorf("Expected 1 result, got %d", len(resp.Data.Result))
	}
}

func TestTimeSeriesResponse_WithError(t *testing.T) {
	resp := &TimeSeriesResponse{
		Status:    "error",
		Error:     "query timeout",
		ErrorType: "timeout",
		Data:      TimeSeriesData{},
	}

	if resp.Status != "error" {
		t.Errorf("Expected status 'error', got %q", resp.Status)
	}

	if resp.Error != "query timeout" {
		t.Errorf("Expected error 'query timeout', got %q", resp.Error)
	}

	if resp.ErrorType != "timeout" {
		t.Errorf("Expected errorType 'timeout', got %q", resp.ErrorType)
	}
}

func TestConvertToTimeSeriesResponse_VariousMetrics(t *testing.T) {
	tests := []struct {
		name           string
		input          model.Value
		expectedType   string
		expectedSeries int
	}{
		{
			name: "vector with multiple series",
			input: model.Vector{
				&model.Sample{Metric: model.Metric{"job": "a"}, Value: 1, Timestamp: 1000000},
				&model.Sample{Metric: model.Metric{"job": "b"}, Value: 2, Timestamp: 1000000},
				&model.Sample{Metric: model.Metric{"job": "c"}, Value: 3, Timestamp: 1000000},
			},
			expectedType:   "vector",
			expectedSeries: 3,
		},
		{
			name: "matrix with multiple streams",
			input: model.Matrix{
				&model.SampleStream{
					Metric: model.Metric{"job": "a"},
					Values: []model.SamplePair{{Timestamp: 1000000, Value: 1}},
				},
				&model.SampleStream{
					Metric: model.Metric{"job": "b"},
					Values: []model.SamplePair{{Timestamp: 1000000, Value: 2}},
				},
			},
			expectedType:   "matrix",
			expectedSeries: 2,
		},
		{
			name:           "scalar",
			input:          &model.Scalar{Value: 100, Timestamp: 1000000},
			expectedType:   "scalar",
			expectedSeries: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToTimeSeriesResponse(tt.input)

			if result.Data.ResultType != tt.expectedType {
				t.Errorf("Expected type %q, got %q", tt.expectedType, result.Data.ResultType)
			}

			if len(result.Data.Result) != tt.expectedSeries {
				t.Errorf("Expected %d series, got %d", tt.expectedSeries, len(result.Data.Result))
			}
		})
	}
}

// Test error scenarios
func TestConvertTimeSeriesToTimeValuePoints_InvalidValue(t *testing.T) {
	ts := TimeSeries{
		Values: []DataPoint{
			{Timestamp: 1699876543.0, Value: "not-a-number"},
		},
	}

	result := ConvertTimeSeriesToTimeValuePoints(ts)

	if len(result) != 1 {
		t.Fatalf("Expected 1 point, got %d", len(result))
	}

	if result[0].Value != 0 {
		t.Errorf("Expected value 0 for invalid input, got %f", result[0].Value)
	}
}
