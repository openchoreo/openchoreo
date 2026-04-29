// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// NewAuthzServiceForTest builds a secretServiceWithAuthz with the given
// Service and PDP, allowing external tests to inject mocks for both.
func NewAuthzServiceForTest(internal Service, pdp authzcore.PDP, logger *slog.Logger) Service {
	return &secretServiceWithAuthz{
		internal: internal,
		authz:    services.NewAuthzChecker(pdp, logger),
	}
}
