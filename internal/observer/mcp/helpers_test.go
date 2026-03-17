// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"
	"time"
)

func TestStrPtr(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		if got := strPtr(""); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("non-empty string returns pointer", func(t *testing.T) {
		s := "hello"
		got := strPtr(s)
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		if *got != s {
			t.Errorf("expected %q, got %q", s, *got)
		}
	})
}

func TestParseRFC3339Time(t *testing.T) {
	t.Run("valid RFC3339 parses correctly", func(t *testing.T) {
		got, err := parseRFC3339Time("2025-01-01T00:00:00Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("valid RFC3339 with timezone offset parses correctly", func(t *testing.T) {
		got, err := parseRFC3339Time("2025-06-15T10:30:00+05:30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Normalize to UTC for comparison
		wantUTC := time.Date(2025, 6, 15, 5, 0, 0, 0, time.UTC)
		if !got.UTC().Equal(wantUTC) {
			t.Errorf("expected %v, got %v", wantUTC, got.UTC())
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		_, err := parseRFC3339Time("2025-01-01 00:00:00")
		if err == nil {
			t.Error("expected error for non-RFC3339 format, got nil")
		}
	})

	t.Run("empty string returns error", func(t *testing.T) {
		_, err := parseRFC3339Time("")
		if err == nil {
			t.Error("expected error for empty string, got nil")
		}
	})
}

func TestSetDefaults(t *testing.T) {
	t.Run("zero limit defaults to 100", func(t *testing.T) {
		limit, _, _ := setDefaults(0, "desc", []string{})
		if limit != 100 {
			t.Errorf("expected limit 100, got %d", limit)
		}
	})

	t.Run("non-zero limit is unchanged", func(t *testing.T) {
		limit, _, _ := setDefaults(50, "desc", []string{})
		if limit != 50 {
			t.Errorf("expected limit 50, got %d", limit)
		}
	})

	t.Run("empty sortOrder defaults to desc", func(t *testing.T) {
		_, sortOrder, _ := setDefaults(10, "", []string{})
		if sortOrder != "desc" {
			t.Errorf("expected sortOrder %q, got %q", "desc", sortOrder)
		}
	})

	t.Run("non-empty sortOrder is unchanged", func(t *testing.T) {
		_, sortOrder, _ := setDefaults(10, "asc", []string{})
		if sortOrder != "asc" {
			t.Errorf("expected sortOrder %q, got %q", "asc", sortOrder)
		}
	})

	t.Run("nil logLevels defaults to empty slice", func(t *testing.T) {
		_, _, logLevels := setDefaults(10, "desc", nil)
		if logLevels == nil {
			t.Error("expected non-nil empty slice, got nil")
		}
		if len(logLevels) != 0 {
			t.Errorf("expected empty slice, got %v", logLevels)
		}
	})

	t.Run("non-nil logLevels is unchanged", func(t *testing.T) {
		input := []string{"ERROR", "WARN"}
		_, _, logLevels := setDefaults(10, "desc", input)
		if len(logLevels) != 2 || logLevels[0] != "ERROR" || logLevels[1] != "WARN" {
			t.Errorf("expected %v, got %v", input, logLevels)
		}
	})
}

func TestValidateComponentScope(t *testing.T) {
	t.Run("empty namespace returns error", func(t *testing.T) {
		if err := validateComponentScope("", "proj", "comp"); err == nil {
			t.Error("expected error for empty namespace, got nil")
		}
	})

	t.Run("component without project returns error", func(t *testing.T) {
		if err := validateComponentScope("ns", "", "comp"); err == nil {
			t.Error("expected error when component is set but project is empty, got nil")
		}
	})

	t.Run("namespace only is valid", func(t *testing.T) {
		if err := validateComponentScope("ns", "", ""); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("namespace and project without component is valid", func(t *testing.T) {
		if err := validateComponentScope("ns", "proj", ""); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("all fields set is valid", func(t *testing.T) {
		if err := validateComponentScope("ns", "proj", "comp"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
