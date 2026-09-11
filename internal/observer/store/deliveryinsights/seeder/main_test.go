// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestParseEnvironments(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		wantNames     []string
		wantFactors   []float64
		wantErrSubstr string
	}{
		{
			name:        "dev-like to production-like ordering sets the cadence ladder",
			value:       "dev,staging,production",
			wantNames:   []string{"dev", "staging", "production"},
			wantFactors: []float64{3.0, 1.5, 1.0},
		},
		{
			name:        "single environment deploys at the base cadence",
			value:       "production",
			wantNames:   []string{"production"},
			wantFactors: []float64{1.0},
		},
		{
			name:        "surrounding whitespace and empty entries are ignored",
			value:       " dev , , production ",
			wantNames:   []string{"dev", "production"},
			wantFactors: []float64{3.0, 1.0},
		},
		{
			// A repeated name generates a second history with colliding release
			// UIDs, and seedIncidentEntries keys facts by release UID -- so
			// incidents would attach to the wrong fact.
			name:          "duplicate environment names are rejected",
			value:         "dev,dev",
			wantErrSubstr: `duplicate environment name "dev"`,
		},
		{
			name:          "duplicates are rejected after trimming",
			value:         "dev, dev ,production",
			wantErrSubstr: `duplicate environment name "dev"`,
		},
		{
			name:          "no usable names is an error",
			value:         " , ",
			wantErrSubstr: "at least one environment name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envs, err := parseEnvironments(tt.value)

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("parseEnvironments(%q) = %v, want error containing %q",
						tt.value, envs, tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("parseEnvironments(%q) error = %q, want it to contain %q",
						tt.value, err, tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseEnvironments(%q) returned unexpected error: %v", tt.value, err)
			}
			if len(envs) != len(tt.wantNames) {
				t.Fatalf("parseEnvironments(%q) returned %d environments, want %d",
					tt.value, len(envs), len(tt.wantNames))
			}
			for i, env := range envs {
				if env.name != tt.wantNames[i] {
					t.Errorf("environment %d name = %q, want %q", i, env.name, tt.wantNames[i])
				}
				if env.rateFactor != tt.wantFactors[i] {
					t.Errorf("environment %d (%s) rateFactor = %v, want %v",
						i, env.name, env.rateFactor, tt.wantFactors[i])
				}
			}
		})
	}
}

// TestParseEnvironmentsReleaseUIDCollision pins the reason duplicates are rejected:
// release UIDs embed the environment name, so two same-named environments would
// produce identical UID sequences.
func TestParseEnvironmentsReleaseUIDCollision(t *testing.T) {
	if _, err := parseEnvironments("dev,dev"); err == nil {
		t.Fatal("parseEnvironments(\"dev,dev\") succeeded; duplicate environments " +
			"generate colliding release UIDs and must be rejected")
	}
}
