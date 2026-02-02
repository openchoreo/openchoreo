// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the clusterdataplane is created
	ConditionCreated controller.ConditionType = "Created"

	// ConditionFinalizing represents whether the clusterdataplane is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonClusterDataPlaneCreated is the reason used when a clusterdataplane is created/ready
	ReasonClusterDataPlaneCreated controller.ConditionReason = "ClusterDataPlaneCreated"

	// ReasonClusterDataplaneFinalizing is the reason used when a clusterdataplane's dependents are being deleted
	ReasonClusterDataplaneFinalizing controller.ConditionReason = "ClusterDataplaneFinalizing"
)

// NewClusterDataPlaneCreatedCondition creates a condition to indicate the clusterdataplane is created/ready
func NewClusterDataPlaneCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonClusterDataPlaneCreated,
		"ClusterDataplane is created",
		generation,
	)
}

// NewClusterDataPlaneFinalizingCondition creates a condition to indicate the clusterdataplane is finalizing
func NewClusterDataPlaneFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonClusterDataplaneFinalizing,
		"ClusterDataplane is finalizing",
		generation,
	)
}
