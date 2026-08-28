// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

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
