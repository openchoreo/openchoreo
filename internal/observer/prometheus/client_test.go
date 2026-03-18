// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, statusSuccess, result.Status)
	assert.Equal(t, "vector", result.Data.ResultType)
	require.Len(t, result.Data.Result, 2)

	series1 := result.Data.Result[0]
	assert.Equal(t, "up", series1.Metric["__name__"])
	assert.Equal(t, "prometheus", series1.Metric["job"])

	require.Len(t, series1.Values, 1)

	expectedTimestamp := 1699876543.0
	assert.Equal(t, expectedTimestamp, series1.Values[0].Timestamp)
	assert.Equal(t, "1", series1.Values[0].Value)
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

	assert.Equal(t, statusSuccess, result.Status)
	assert.Equal(t, "matrix", result.Data.ResultType)
	require.Len(t, result.Data.Result, 1)

	series := result.Data.Result[0]
	assert.Equal(t, "cpu_usage", series.Metric["__name__"])
	require.Len(t, series.Values, 3)

	// Verify first data point
	assert.Equal(t, 1699876540.0, series.Values[0].Timestamp)
	assert.Equal(t, "0.5", series.Values[0].Value)

	// Verify last data point
	assert.Equal(t, 1699876560.0, series.Values[2].Timestamp)
	assert.Equal(t, "0.7", series.Values[2].Value)
}

func TestConvertToTimeSeriesResponse_Scalar(t *testing.T) {
	scalar := &model.Scalar{
		Value:     42.5,
		Timestamp: model.Time(1699876543000),
	}

	result := convertToTimeSeriesResponse(scalar)

	// Verify response structure
	assert.Equal(t, statusSuccess, result.Status)
	assert.Equal(t, "scalar", result.Data.ResultType)
	require.Len(t, result.Data.Result, 1)

	series := result.Data.Result[0]
	assert.Empty(t, series.Metric)
	require.Len(t, series.Values, 1)
	assert.Equal(t, 1699876543.0, series.Values[0].Timestamp)
	assert.Equal(t, "42.5", series.Values[0].Value)
}

func TestConvertToTimeSeriesResponse_EmptyVector(t *testing.T) {
	vector := model.Vector{}

	result := convertToTimeSeriesResponse(vector)

	assert.Equal(t, statusSuccess, result.Status)
	assert.Empty(t, result.Data.Result)
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

			require.Len(t, result, len(tt.expected))

			for i, point := range result {
				assert.Equal(t, tt.expected[i].Time, point.Time, "Point %d time mismatch", i)
				assert.Equal(t, tt.expected[i].Value, point.Value, "Point %d value mismatch", i)
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

	require.Len(t, result, 1)

	expectedTime := "2023-11-13T11:55:43Z"
	assert.Equal(t, expectedTime, result[0].Time)

	parsedTime, err := time.Parse(time.RFC3339, result[0].Time)
	require.NoError(t, err, "Failed to parse time as RFC3339")

	expectedParsed := time.Unix(1699876543, 0).UTC()
	assert.True(t, parsedTime.Equal(expectedParsed), "Parsed time %v doesn't match expected %v", parsedTime, expectedParsed)
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

	assert.Equal(t, statusSuccess, resp.Status)
	assert.Equal(t, "matrix", resp.Data.ResultType)
	assert.Len(t, resp.Data.Result, 1)
}

func TestTimeSeriesResponse_WithError(t *testing.T) {
	resp := &TimeSeriesResponse{
		Status:    "error",
		Error:     "query timeout",
		ErrorType: "timeout",
		Data:      TimeSeriesData{},
	}

	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "query timeout", resp.Error)
	assert.Equal(t, "timeout", resp.ErrorType)
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

			assert.Equal(t, tt.expectedType, result.Data.ResultType)
			assert.Len(t, result.Data.Result, tt.expectedSeries)
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

	require.Len(t, result, 1)
	assert.Equal(t, float64(0), result[0].Value)
}
