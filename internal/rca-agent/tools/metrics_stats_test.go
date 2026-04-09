// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateStats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		values     []float64
		timestamps []string
		wantNil    bool
		wantMean   float64
		wantMedian float64
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "basic sequence",
			values:     []float64{1, 2, 3, 4, 5},
			timestamps: []string{"t0", "t1", "t2", "t3", "t4"},
			wantMean:   3.0,
			wantMedian: 3.0,
			wantMin:    1.0,
			wantMax:    5.0,
		},
		{
			name:    "nil values",
			values:  nil,
			wantNil: true,
		},
		{
			name:    "empty values",
			values:  []float64{},
			wantNil: true,
		},
		{
			name:       "single value",
			values:     []float64{42},
			timestamps: []string{"t0"},
			wantMean:   42.0,
			wantMedian: 42.0,
			wantMin:    42.0,
			wantMax:    42.0,
		},
		{
			name:       "even count median",
			values:     []float64{1, 2, 3, 4},
			wantMean:   2.5,
			wantMedian: 2.5,
			wantMin:    1.0,
			wantMax:    4.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := calculateStats(tt.values, tt.timestamps)
			if tt.wantNil {
				assert.Nil(t, s)
				return
			}
			require.NotNil(t, s)
			assert.InDelta(t, tt.wantMean, s.Mean, 1e-10)
			assert.InDelta(t, tt.wantMedian, s.Median, 1e-10)
			assert.InDelta(t, tt.wantMin, s.Min, 1e-10)
			assert.InDelta(t, tt.wantMax, s.Max, 1e-10)
		})
	}
}

func TestCalculateStats_TimeBounds(t *testing.T) {
	t.Parallel()
	s := calculateStats([]float64{1, 2, 3}, []string{"t0", "t1", "t2"})
	require.NotNil(t, s)
	assert.Equal(t, "t0", s.StartTime)
	assert.Equal(t, "t2", s.EndTime)
}

func TestDetectAnomalies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		values          []float64
		wantSpikes      int
		wantMinSpikes   int // use when exact count is hard to predict
		wantMaxZ        float64
		wantLargestDrop float64
	}{
		{
			name:       "stable values - no anomalies",
			values:     []float64{10, 10, 10, 10, 10},
			wantSpikes: 0,
		},
		{
			name:          "extreme outlier triggers z-score",
			values:        []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 500},
			wantMinSpikes: 1,
			wantMaxZ:      3.0,
		},
		{
			name:          "100% change triggers percentage spike",
			values:        []float64{100, 200},
			wantMinSpikes: 1,
		},
		{
			name:       "dual trigger counts once",
			values:     []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 200},
			wantSpikes: 1,
		},
		{
			name:            "largest drop detected",
			values:          []float64{100, 50, 80, 20},
			wantLargestDrop: -60.0,
		},
		{
			name:       "single value - no anomalies",
			values:     []float64{42},
			wantSpikes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := detectAnomalies(tt.values)
			if tt.wantSpikes > 0 {
				assert.Equal(t, tt.wantSpikes, a.SpikeCount)
			}
			if tt.wantMinSpikes > 0 {
				assert.GreaterOrEqual(t, a.SpikeCount, tt.wantMinSpikes)
			}
			if tt.wantMaxZ > 0 {
				assert.Greater(t, a.MaxSpikeMagnitude, tt.wantMaxZ)
			}
			if tt.wantLargestDrop != 0 {
				assert.InDelta(t, tt.wantLargestDrop, a.LargestDrop, 1e-10)
			}
		})
	}
}

func TestCalculateResourcePressure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		usage, requests, limits []float64
		wantNil                 bool
		wantReqRatio            float64
		wantLimRatio            float64
		wantExceededReqs        bool
		wantExceededLims        bool
	}{
		{
			name:    "nil usage",
			usage:   nil,
			wantNil: true,
		},
		{
			name:    "empty usage",
			usage:   []float64{},
			wantNil: true,
		},
		{
			name:             "normal - under limits",
			usage:            []float64{50, 60},
			requests:         []float64{100, 100},
			limits:           []float64{200, 200},
			wantReqRatio:     0.55,
			wantLimRatio:     0.275,
			wantExceededReqs: false,
			wantExceededLims: false,
		},
		{
			name:             "exceeded both",
			usage:            []float64{150},
			requests:         []float64{100},
			limits:           []float64{120},
			wantReqRatio:     1.5,
			wantLimRatio:     1.25,
			wantExceededReqs: true,
			wantExceededLims: true,
		},
		{
			name:         "zero denominators skipped",
			usage:        []float64{50, 60},
			requests:     []float64{0, 100},
			wantReqRatio: 0.6, // only 60/100
		},
		{
			name:             "all zero denominators",
			usage:            []float64{50, 60},
			requests:         []float64{0, 0},
			wantReqRatio:     0.0,
			wantExceededReqs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := calculateResourcePressure(tt.usage, tt.requests, tt.limits)
			if tt.wantNil {
				assert.Nil(t, p)
				return
			}
			require.NotNil(t, p)
			if tt.wantReqRatio != 0 || len(tt.requests) > 0 {
				assert.InDelta(t, tt.wantReqRatio, p.AvgUsageToRequestRatio, 1e-10)
			}
			if tt.wantLimRatio != 0 || len(tt.limits) > 0 {
				assert.InDelta(t, tt.wantLimRatio, p.AvgUsageToLimitRatio, 1e-10)
			}
			assert.Equal(t, tt.wantExceededReqs, p.ExceededRequests)
			assert.Equal(t, tt.wantExceededLims, p.ExceededLimits)
		})
	}
}

func TestCorrelation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b []float64
		want float64
	}{
		{
			name: "perfect positive",
			a:    []float64{1, 2, 3, 4, 5},
			b:    []float64{10, 20, 30, 40, 50},
			want: 1.0,
		},
		{
			name: "perfect negative",
			a:    []float64{1, 2, 3, 4, 5},
			b:    []float64{50, 40, 30, 20, 10},
			want: -1.0,
		},
		{
			name: "no correlation - constant a",
			a:    []float64{1, 1, 1, 1},
			b:    []float64{1, 2, 3, 4},
			want: 0.0,
		},
		{
			name: "too few values",
			a:    []float64{1},
			b:    []float64{2},
			want: 0.0,
		},
		{
			name: "nil slices",
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, correlation(tt.a, tt.b), 1e-10)
		})
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sorted []float64
		p      float64
		want   float64
	}{
		{
			name:   "p50 of 1-10",
			sorted: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:      50,
			want:   5.5,
		},
		{
			name: "empty slice",
			p:    50,
			want: 0.0,
		},
		{
			name:   "single value",
			sorted: []float64{42},
			p:      99,
			want:   42.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, percentile(tt.sorted, tt.p), 1e-10)
		})
	}
}
