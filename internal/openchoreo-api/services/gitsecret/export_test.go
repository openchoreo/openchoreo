// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import (
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// NewAuthzServiceForTest creates a gitSecretServiceWithAuthz with the given
// Service and PDP, allowing external test packages to inject mockery-generated
// mocks for both dependencies.
func NewAuthzServiceForTest(internal Service, pdp authzcore.PDP, logger *slog.Logger) Service {
	return &gitSecretServiceWithAuthz{
		internal: internal,
		authz:    services.NewAuthzChecker(pdp, logger),
	}
}
