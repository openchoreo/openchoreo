// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"testing"
	"time"
)

// Every one of these helpers backs a command-line flag default, so a silent
// misparse becomes a silently wrong deployment: a drain window that never
// drains, TLS that never turns on. The contract that matters is that anything
// unset, empty or unparseable falls back to the caller's default rather than a
// zero value.

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  string
	}{
		{name: "unset falls back", set: false, want: "fallback"},
		{name: "empty falls back", set: true, value: "", want: "fallback"},
		{name: "set wins", set: true, value: "override", want: "override"},
		{name: "whitespace is a value", set: true, value: " ", want: " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("OPENCHOREO_TEST_STR", tt.value)
			}
			if got := GetEnv("OPENCHOREO_TEST_STR", "fallback"); got != tt.want {
				t.Fatalf("GetEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		def   bool
		want  bool
	}{
		{name: "unset keeps default true", set: false, def: true, want: true},
		{name: "unset keeps default false", set: false, def: false, want: false},
		{name: "empty keeps default", set: true, value: "", def: true, want: true},
		{name: "true", set: true, value: "true", def: false, want: true},
		{name: "one", set: true, value: "1", def: false, want: true},
		{name: "false", set: true, value: "false", def: true, want: false},
		{name: "zero", set: true, value: "0", def: true, want: false},
		// Anything unrecognized reads as false rather than falling back to the
		// default: "TRUE" or "yes" silently disables a flag the operator meant
		// to enable.
		{name: "uppercase TRUE is not recognized", set: true, value: "TRUE", def: true, want: false},
		{name: "yes is not recognized", set: true, value: "yes", def: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("OPENCHOREO_TEST_BOOL", tt.value)
			}
			if got := GetEnvBool("OPENCHOREO_TEST_BOOL", tt.def); got != tt.want {
				t.Fatalf("GetEnvBool(%q, %v) = %v, want %v", tt.value, tt.def, got, tt.want)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  int
	}{
		{name: "unset falls back", set: false, want: 8443},
		{name: "empty falls back", set: true, value: "", want: 8443},
		{name: "parses", set: true, value: "9000", want: 9000},
		{name: "negative parses", set: true, value: "-1", want: -1},
		{name: "unparseable falls back", set: true, value: "not-a-number", want: 8443},
		// Sscanf stops at the first non-digit, so a trailing unit is silently
		// dropped rather than rejected.
		{name: "trailing junk keeps the leading digits", set: true, value: "80port", want: 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("OPENCHOREO_TEST_INT", tt.value)
			}
			if got := GetEnvInt("OPENCHOREO_TEST_INT", 8443); got != tt.want {
				t.Fatalf("GetEnvInt(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  time.Duration
	}{
		{name: "unset falls back", set: false, want: 30 * time.Second},
		{name: "empty falls back", set: true, value: "", want: 30 * time.Second},
		{name: "parses seconds", set: true, value: "10s", want: 10 * time.Second},
		{name: "parses compound", set: true, value: "1m30s", want: 90 * time.Second},
		{name: "zero is honored", set: true, value: "0s", want: 0},
		// A bare number is not Go duration syntax: it must fall back rather
		// than be read as nanoseconds.
		{name: "unitless falls back", set: true, value: "10", want: 30 * time.Second},
		{name: "unparseable falls back", set: true, value: "soon", want: 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("OPENCHOREO_TEST_DUR", tt.value)
			}
			if got := GetEnvDuration("OPENCHOREO_TEST_DUR", 30*time.Second); got != tt.want {
				t.Fatalf("GetEnvDuration(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
