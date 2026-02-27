// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"
	"testing"
)

func TestHealthService_Check(t *testing.T) {
	logger := slog.Default()
	svc := NewHealthService(logger)

	err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("expected nil error from Check(), got %v", err)
	}
}

func TestNewHealthService(t *testing.T) {
	logger := slog.Default()
	svc := NewHealthService(logger)

	if svc == nil {
		t.Fatal("expected non-nil HealthService")
	}
}
