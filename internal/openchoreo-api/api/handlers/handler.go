// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Handler implements gen.StrictServerInterface
type Handler struct {
	services *services.Services
	logger   *slog.Logger
}

// Compile-time check that Handler implements StrictServerInterface
var _ gen.StrictServerInterface = (*Handler)(nil)

// New creates a new Handler
func New(services *services.Services, logger *slog.Logger) *Handler {
	return &Handler{
		services: services,
		logger:   logger,
	}
}
