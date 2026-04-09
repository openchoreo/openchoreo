// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"math"
	"sort"
)

// MetricStats holds computed statistics for a time series.
type MetricStats struct {
	Mean                   float64 `json:"mean"`
	Median                 float64 `json:"median"`
	Min                    float64 `json:"min"`
	Max                    float64 `json:"max"`
	StdDev                 float64 `json:"std_dev"`
	CoefficientOfVariation float64 `json:"coefficient_of_variation"`
	P90                    float64 `json:"p90"`
	P95                    float64 `json:"p95"`
	StartTime              string  `json:"start_time,omitempty"`
	EndTime                string  `json:"end_time,omitempty"`
}

// AnomalyInfo holds anomaly detection results.
type AnomalyInfo struct {
	SpikeCount        int     `json:"spike_count"`
	MaxSpikeMagnitude float64 `json:"max_spike_magnitude"`
	LargestDrop       float64 `json:"largest_drop"`
}

// ResourcePressure holds usage-to-request/limit ratios.
type ResourcePressure struct {
	AvgUsageToRequestRatio float64 `json:"avg_usage_to_request_ratio"`
	AvgUsageToLimitRatio   float64 `json:"avg_usage_to_limit_ratio"`
	ExceededRequests       bool    `json:"exceeded_requests"`
	ExceededLimits         bool    `json:"exceeded_limits"`
}

func calculateStats(values []float64, timestamps []string) *MetricStats {
	if len(values) == 0 {
		return nil
	}

	n := float64(len(values))
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}

	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= n
	stdDev := math.Sqrt(variance)

	cv := 0.0
	if mean != 0 {
		cv = stdDev / mean
	}

	stats := &MetricStats{
		Mean:                   mean,
		Median:                 median,
		Min:                    sorted[0],
		Max:                    sorted[len(sorted)-1],
		StdDev:                 stdDev,
		CoefficientOfVariation: cv,
		P90:                    percentile(sorted, 90),
		P95:                    percentile(sorted, 95),
	}

	if len(timestamps) > 0 {
		stats.StartTime = timestamps[0]
		stats.EndTime = timestamps[len(timestamps)-1]
	}

	return stats
}

func detectAnomalies(values []float64) *AnomalyInfo {
	if len(values) < 2 {
		return &AnomalyInfo{}
	}

	n := float64(len(values))
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	std := math.Sqrt(variance / n)

	// Z-score anomalies (threshold: 3.0 sigma).
	maxZ := 0.0
	flagged := make([]bool, len(values))
	for i, v := range values {
		z := math.Abs((v - mean) / (std + 1e-10))
		if z > 3.0 {
			flagged[i] = true
		}
		if z > maxZ {
			maxZ = z
		}
	}

	// Percentage change anomalies (threshold: 50%).
	for i := 1; i < len(values); i++ {
		pctChange := math.Abs((values[i]-values[i-1])/(values[i-1]+1e-10)) * 100
		if pctChange > 50 {
			flagged[i] = true
		}
	}

	spikeCount := 0
	for _, f := range flagged {
		if f {
			spikeCount++
		}
	}

	// Largest drop.
	largestDrop := 0.0
	for i := 1; i < len(values); i++ {
		diff := values[i] - values[i-1]
		if diff < largestDrop {
			largestDrop = diff
		}
	}

	return &AnomalyInfo{
		SpikeCount:        spikeCount,
		MaxSpikeMagnitude: maxZ,
		LargestDrop:       largestDrop,
	}
}

func calculateResourcePressure(usage, requests, limits []float64) *ResourcePressure {
	if len(usage) == 0 {
		return nil
	}

	pressure := &ResourcePressure{}

	if len(requests) > 0 {
		n := min(len(usage), len(requests))
		sum := 0.0
		validCount := 0
		for i := 0; i < n; i++ {
			if requests[i] == 0 {
				continue
			}
			sum += usage[i] / requests[i]
			validCount++
			if usage[i] > requests[i] {
				pressure.ExceededRequests = true
			}
		}
		if validCount > 0 {
			pressure.AvgUsageToRequestRatio = sum / float64(validCount)
		}
	}

	if len(limits) > 0 {
		n := min(len(usage), len(limits))
		sum := 0.0
		validCount := 0
		for i := 0; i < n; i++ {
			if limits[i] == 0 {
				continue
			}
			sum += usage[i] / limits[i]
			validCount++
			if usage[i] > limits[i] {
				pressure.ExceededLimits = true
			}
		}
		if validCount > 0 {
			pressure.AvgUsageToLimitRatio = sum / float64(validCount)
		}
	}

	return pressure
}

func correlation(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n < 2 {
		return 0
	}

	sumA, sumB := 0.0, 0.0
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)

	cov, varA, varB := 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}

	denom := math.Sqrt(varA * varB)
	if denom == 0 {
		return 0
	}
	return cov / denom
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
