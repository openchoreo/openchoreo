// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// NewTestServiceWithAuthz constructs a k8sResourcesServiceWithAuthz for testing,
// allowing callers to inject a mock internal Service and PDP.
func NewTestServiceWithAuthz(internal Service, k8sClient client.Client, pdp authz.PDP, logger *slog.Logger) Service {
	return &k8sResourcesServiceWithAuthz{
		internal:  internal,
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(pdp, logger),
	}
}
