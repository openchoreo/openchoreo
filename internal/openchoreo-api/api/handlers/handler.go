// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// errNotImplemented is returned for stub methods that are not yet implemented.
// TODO: Remove this error once all handler stubs are fully implemented.
var errNotImplemented = errors.New("not implemented")

// Handler implements gen.StrictServerInterface
type Handler struct {
	legacyServices *legacyservices.Services
	services       *handlerservices.Services
	logger         *slog.Logger
	Config         *config.Config
}

// Compile-time check that Handler implements StrictServerInterface
var _ gen.StrictServerInterface = (*Handler)(nil)

// New creates a new Handler
func New(legacyServices *legacyservices.Services, svc *handlerservices.Services, logger *slog.Logger, cfg *config.Config) *Handler {
	return &Handler{
		legacyServices: legacyServices,
		services:       svc,
		logger:         logger,
		Config:         cfg,
	}
}
