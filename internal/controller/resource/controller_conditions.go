// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates the Resource has been successfully reconciled.
	// Failure modes are encoded in the condition's Reason; this mirrors the
	// minimal Component condition set (`internal/controller/component/controller_conditions.go:14-21`).
	ConditionReady controller.ConditionType = "Ready"

	// ConditionFinalizing indicates the Resource is being finalized (deleted).
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

// Constants for condition reasons

const (
	// ReasonReconciled indicates the Resource has been validated and the
	// latest ResourceRelease is in place.
	ReasonReconciled controller.ConditionReason = "Reconciled"

	// ReasonResourceTypeNotFound indicates the referenced ResourceType or
	// ClusterResourceType does not exist in the cluster yet.
	ReasonResourceTypeNotFound controller.ConditionReason = "ResourceTypeNotFound"

	// ReasonFinalizing indicates the Resource is being finalized.
	ReasonFinalizing controller.ConditionReason = "Finalizing"
)

// NewFinalizingCondition returns a Finalizing=True condition observed at the
// given generation, used while the Resource is being torn down.
func NewFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonFinalizing,
		"Resource is finalizing",
		generation,
	)
}
