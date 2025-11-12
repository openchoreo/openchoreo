// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

type Client struct {
	api     v1.API
	baseURL string
	logger  *slog.Logger
}

// TimeSeriesResponse represents a Prometheus range query response with time series data
type TimeSeriesResponse struct {
	Status    string         `json:"status"`
	Data      TimeSeriesData `json:"data"`
	Error     string         `json:"error,omitempty"`
	ErrorType string         `json:"errorType,omitempty"`
}

// TimeSeriesData contains the result type and time series results
type TimeSeriesData struct {
	ResultType string       `json:"resultType"`
	Result     []TimeSeries `json:"result"`
}

// TimeSeries represents a single time series with metric labels and data points
type TimeSeries struct {
	Metric map[string]string `json:"metric"`
	Values []DataPoint       `json:"values"`
}

// DataPoint represents a single timestamp-value pair in a time series
type DataPoint struct {
	Timestamp float64 `json:"timestamp"` // Unix timestamp in seconds
	Value     string  `json:"value"`     // Metric value as string for precision
}

// TimeValuePoint represents a simplified data point with ISO 8601 time format
type TimeValuePoint struct {
	Time  string  `json:"time"`  // ISO 8601 formatted timestamp
	Value float64 `json:"value"` // Numeric value
}

func NewClient(cfg *config.PrometheusConfig, logger *slog.Logger) (*Client, error) {
	client, err := api.NewClient(api.Config{
		Address: cfg.Address,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to create prometheus client: %w", err)
	}

	v1api := v1.NewAPI(client)

	return &Client{
		baseURL: cfg.Address,
		api:     v1api,
		logger:  logger,
	}, nil
}

// HealthCheck performs a health check on Prometheus using the official API
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	c.logger.Debug("Performing Prometheus health check")

	_, _, err := c.api.Query(ctx, "up", time.Now())
	if err != nil {
		c.logger.Error("Prometheus health check failed", "error", err)
		return fmt.Errorf("prometheus health check failed: %w", err)
	}

	c.logger.Debug("Successfully connected to Prometheus")
	return nil
}

// Executes a PromQL range query and returns full time series data
// This method returns all data points in the time range, suitable for charting and visualization
func (c *Client) QueryRangeTimeSeries(ctx context.Context, query string, start, end time.Time, step time.Duration) (*TimeSeriesResponse, error) {
	c.logger.Debug("Executing Prometheus range query for time series",
		"query", query,
		"start", start,
		"end", end,
		"step", step)

	r := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	result, warnings, err := c.api.QueryRange(ctx, query, r)
	if err != nil {
		return nil, fmt.Errorf("failed to execute range query: %w", err)
	}

	if len(warnings) > 0 {
		c.logger.Warn("Prometheus range query returned warnings", "warnings", warnings)
	}

	tsResp := convertToTimeSeriesResponse(result)

	c.logger.Debug("Prometheus range query for time series executed successfully",
		"series_count", len(tsResp.Data.Result))

	return tsResp, nil
}

// Converts Prometheus model.Value to TimeSeriesResponse format. This properly handles Matrix results with all 
// data points
func convertToTimeSeriesResponse(result model.Value) *TimeSeriesResponse {
	tsResp := &TimeSeriesResponse{
		Status: "success",
		Data: TimeSeriesData{
			ResultType: result.Type().String(),
			Result:     make([]TimeSeries, 0),
		},
	}

	switch v := result.(type) {
	case model.Vector:
		for _, sample := range v {
			metric := make(map[string]string)
			for k, val := range sample.Metric {
				metric[string(k)] = string(val)
			}

			ts := TimeSeries{
				Metric: metric,
				Values: []DataPoint{
					{
						Timestamp: float64(sample.Timestamp) / 1000, // Convert to seconds
						Value:     fmt.Sprintf("%v", sample.Value),
					},
				},
			}
			tsResp.Data.Result = append(tsResp.Data.Result, ts)
		}

	case model.Matrix:
		for _, stream := range v {
			metric := make(map[string]string)
			for k, val := range stream.Metric {
				metric[string(k)] = string(val)
			}

			values := make([]DataPoint, 0, len(stream.Values))
			for _, samplePair := range stream.Values {
				values = append(values, DataPoint{
					Timestamp: float64(samplePair.Timestamp) / 1000, // Convert to seconds
					Value:     fmt.Sprintf("%v", samplePair.Value),
				})
			}

			ts := TimeSeries{
				Metric: metric,
				Values: values,
			}
			tsResp.Data.Result = append(tsResp.Data.Result, ts)
		}

	case *model.Scalar:
		ts := TimeSeries{
			Metric: map[string]string{},
			Values: []DataPoint{
				{
					Timestamp: float64(v.Timestamp) / 1000,
					Value:     fmt.Sprintf("%v", v.Value),
				},
			},
		}
		tsResp.Data.Result = append(tsResp.Data.Result, ts)
	}

	return tsResp
}

// Converts a TimeSeries to an array of TimeValuePoint with ISO 8601 formatted timestamps and float64 values
func ConvertTimeSeriesToTimeValuePoints(ts TimeSeries) []TimeValuePoint {
	points := make([]TimeValuePoint, 0, len(ts.Values))

	for _, dp := range ts.Values {
		t := time.Unix(int64(dp.Timestamp), 0).UTC()

		var value float64
		fmt.Sscanf(dp.Value, "%f", &value)

		points = append(points, TimeValuePoint{
			Time:  t.Format(time.RFC3339), // ISO 8601 format
			Value: value,
		})
	}

	return points
}
