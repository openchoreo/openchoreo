// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the buildplane is created
	ConditionCreated controller.ConditionType = "Created"
)

const (
	// ReasonBuildPlaneCreated is the reason used when a buildplane is created/ready
	ReasonBuildPlaneCreated controller.ConditionReason = "BuildPlaneCreated"
)

// NewBuildPlaneCreatedCondition creates a condition to indicate the buildplane is created/ready
func NewBuildPlaneCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonBuildPlaneCreated,
		"Build plane is created",
		generation,
	)
}
