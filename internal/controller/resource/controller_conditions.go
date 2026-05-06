// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates the Resource has been successfully reconciled.
	// Failure modes are encoded in the condition's Reason; this mirrors the
	// minimal Component condition set (`internal/controller/component/controller_conditions.go:14-21`).
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// ReasonResourceTypeNotFound indicates the referenced ResourceType or
	// ClusterResourceType does not exist in the cluster yet.
	ReasonResourceTypeNotFound controller.ConditionReason = "ResourceTypeNotFound"
)
