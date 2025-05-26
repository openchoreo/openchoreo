// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the dataplane is created
	ConditionCreated controller.ConditionType = "Created"

	// ConditionFinalizing represents whether the dataplane is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonDataPlaneCreated is the reason used when a dataplane is created/ready
	ReasonDataPlaneCreated controller.ConditionReason = "DataPlaneCreated"

	// ReasonDataplaneFinalizing is the reason used when a dataplane's dependents are being deleted
	ReasonDataplaneFinalizing controller.ConditionReason = "DataplaneFinalizing"
)

// NewDataPlaneCreatedCondition creates a condition to indicate the dataplane is created/ready
func NewDataPlaneCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonDataPlaneCreated,
		"Dataplane is created",
		generation,
	)
}

// NewDataPlaneFinalizingCondition creates a condition to indicate the dataplane is finalizing
func NewDataPlaneFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDataplaneFinalizing,
		"Dataplane is finalizing",
		generation,
	)
}
