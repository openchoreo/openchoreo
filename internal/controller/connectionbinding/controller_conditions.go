// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package connectionbinding

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types.
const (
	// ConditionAllResolved indicates whether all connections have been resolved.
	ConditionAllResolved controller.ConditionType = "AllResolved"
)

// Constants for condition reasons.
const (
	// ReasonAllResolved indicates all connections have resolved URLs.
	ReasonAllResolved controller.ConditionReason = "AllResolved"

	// ReasonConnectionsPending indicates some connections are waiting for resolution.
	ReasonConnectionsPending controller.ConditionReason = "ConnectionsPending"

	// ReasonNoConnections indicates no connections to resolve.
	ReasonNoConnections controller.ConditionReason = "NoConnections"
)
