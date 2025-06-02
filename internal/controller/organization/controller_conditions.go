// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package organization

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const ConditionDeleting controller.ConditionType = "Deleting"

// ReasonOrganizationFinalizing the organization is being deleted
const ReasonOrganizationFinalizing controller.ConditionReason = "OrganizationFinalizing"

func NewOrganizationFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionDeleting,
		metav1.ConditionFalse,
		ReasonOrganizationFinalizing,
		"Organization is being deleted",
		generation,
	)
}
