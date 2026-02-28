// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"
)

// HealthService provides health check functionality
type HealthService struct {
	logger *slog.Logger
}

// NewHealthService creates a new HealthService instance
func NewHealthService(logger *slog.Logger) *HealthService {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthService{
		logger: logger,
	}
}

// Check performs a health check on the observer service
func (s *HealthService) Check(ctx context.Context) error {
	s.logger.Debug("Health check passed")
	return nil
}
