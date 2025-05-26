// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the component is created
	ConditionCreated controller.ConditionType = "Created"
	// ConditionFinalizing represents whether the component is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonComponentCreated is the reason used when a component is created/ready
	ReasonComponentCreated controller.ConditionReason = "ComponentCreated"

	// ReasonComponentFinalizing is the reason used when a component's dependents are being deleted'
	ReasonComponentFinalizing controller.ConditionReason = "ComponentFinalizing"
)

// NewComponentCreatedCondition creates a condition to indicate the component is created/ready
func NewComponentCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonComponentCreated,
		"Component is created",
		generation,
	)
}

// NewComponentFinalizingCondition creates a condition to indicate the component is being finalized
func NewComponentFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonComponentFinalizing,
		"Component is finalizing",
		generation,
	)
}
