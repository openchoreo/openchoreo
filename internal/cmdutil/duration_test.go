// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Valid cases with days
		{
			name:     "days only",
			input:    "90d",
			expected: 90 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "days and hours with space",
			input:    "10d 1h",
			expected: (10*24 + 1) * time.Hour,
			wantErr:  false,
		},
		{
			name:     "days and hours without space",
			input:    "10d1h",
			expected: (10*24 + 1) * time.Hour,
			wantErr:  false,
		},
		{
			name:     "all units with spaces",
			input:    "10d 1h 10m 100s",
			expected: 10*24*time.Hour + 1*time.Hour + 10*time.Minute + 100*time.Second,
			wantErr:  false,
		},
		{
			name:     "all units without spaces",
			input:    "10d1h10m100s",
			expected: 10*24*time.Hour + 1*time.Hour + 10*time.Minute + 100*time.Second,
			wantErr:  false,
		},
		{
			name:     "days and minutes",
			input:    "5d 30m",
			expected: 5*24*time.Hour + 30*time.Minute,
			wantErr:  false,
		},
		{
			name:     "days and seconds",
			input:    "1d 500s",
			expected: 24*time.Hour + 500*time.Second,
			wantErr:  false,
		},

		// Valid cases without days (standard Go format)
		{
			name:     "hours and minutes",
			input:    "1h30m",
			expected: 1*time.Hour + 30*time.Minute,
			wantErr:  false,
		},
		{
			name:     "minutes only",
			input:    "45m",
			expected: 45 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "seconds only",
			input:    "1000s",
			expected: 1000 * time.Second,
			wantErr:  false,
		},
		{
			name:     "hours only",
			input:    "2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "all units except days",
			input:    "2h30m45s",
			expected: 2*time.Hour + 30*time.Minute + 45*time.Second,
			wantErr:  false,
		},

		// Valid cases with extra spaces
		{
			name:     "extra spaces between units",
			input:    "10d  1h  30m",
			expected: 10*24*time.Hour + 1*time.Hour + 30*time.Minute,
			wantErr:  false,
		},
		{
			name:     "leading and trailing spaces",
			input:    "  5d 2h  ",
			expected: 5*24*time.Hour + 2*time.Hour,
			wantErr:  false,
		},

		// Invalid cases
		{
			name:     "empty string",
			input:    "",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "invalid format",
			input:    "invalid",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "invalid unit",
			input:    "10w",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "missing number",
			input:    "d",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "negative duration (Go format)",
			input:    "-5h",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "decimal numbers",
			input:    "1.5d",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDuration(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParseDuration(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid duration with days",
			input:   "90d",
			wantErr: false,
		},
		{
			name:    "valid duration mixed units",
			input:   "10d 1h 30m",
			wantErr: false,
		},
		{
			name:    "valid duration standard format",
			input:   "1h30m",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "negative duration",
			input:   "-5h",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDuration(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateDuration(%q) expected error, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateDuration(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestParseDurationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "very large duration",
			input:    "365d",
			expected: 365 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "one of each unit",
			input:    "1d 1h 1m 1s",
			expected: 24*time.Hour + 1*time.Hour + 1*time.Minute + 1*time.Second,
			wantErr:  false,
		},
		{
			name:     "zero duration",
			input:    "0s",
			expected: 0,
			wantErr:  false, // Zero is a valid duration value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDuration(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParseDuration(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
				}
			}
		})
	}
}
