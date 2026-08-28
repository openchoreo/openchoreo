// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestGetEnvDuration(t *testing.T) {
	t.Run("unset uses default", func(t *testing.T) {
		t.Setenv("TEST_RENDER_TIMEOUT", "")
		got, err := getEnvDuration("TEST_RENDER_TIMEOUT", 5*time.Second)
		if err != nil || got != 5*time.Second {
			t.Fatalf("getEnvDuration() = %s, %v; want 5s, nil", got, err)
		}
	})

	t.Run("valid duration", func(t *testing.T) {
		t.Setenv("TEST_RENDER_TIMEOUT", "1m30s")
		got, err := getEnvDuration("TEST_RENDER_TIMEOUT", 0)
		if err != nil || got != 90*time.Second {
			t.Fatalf("getEnvDuration() = %s, %v; want 1m30s, nil", got, err)
		}
	})

	t.Run("malformed duration is rejected", func(t *testing.T) {
		t.Setenv("TEST_RENDER_TIMEOUT", "eventually")
		if _, err := getEnvDuration("TEST_RENDER_TIMEOUT", 0); err == nil {
			t.Fatal("getEnvDuration() accepted a malformed duration")
		}
	})
}

func TestGetEnvUint(t *testing.T) {
	t.Run("unset uses default", func(t *testing.T) {
		t.Setenv("TEST_CEL_COST_LIMIT", "")
		got, err := getEnvUint("TEST_CEL_COST_LIMIT", 42)
		if err != nil || got != 42 {
			t.Fatalf("getEnvUint() = %d, %v; want 42, nil", got, err)
		}
	})

	t.Run("valid value", func(t *testing.T) {
		t.Setenv("TEST_CEL_COST_LIMIT", "2000000")
		got, err := getEnvUint("TEST_CEL_COST_LIMIT", 0)
		if err != nil || got != 2_000_000 {
			t.Fatalf("getEnvUint() = %d, %v; want 2000000, nil", got, err)
		}
	})

	// Silently falling back to the default here would leave an operator running with a cost
	// limit they did not choose and no signal that their setting was discarded.
	t.Run("malformed value is rejected", func(t *testing.T) {
		t.Setenv("TEST_CEL_COST_LIMIT", "two million")
		if _, err := getEnvUint("TEST_CEL_COST_LIMIT", 0); err == nil {
			t.Fatal("getEnvUint() accepted a malformed value")
		}
	})
}

func TestValidateRenderTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Nanosecond, time.Minute} {
		if err := validateRenderTimeout(timeout); err != nil {
			t.Fatalf("validateRenderTimeout(%s) returned %v", timeout, err)
		}
	}
	if err := validateRenderTimeout(-time.Second); err == nil {
		t.Fatal("validateRenderTimeout() accepted a negative duration")
	}
}
