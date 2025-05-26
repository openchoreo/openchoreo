// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady represents whether the endpoint is ready
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// ReasonEndpointReady the endpoint is ready
	ReasonEndpointReady controller.ConditionReason = "EndpointReady"
)

func EndpointReadyCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		controller.TypeReady,
		metav1.ConditionTrue,
		ReasonEndpointReady,
		"Endpoint is ready",
		generation,
	)
}

func EndpointFailedExternalReconcileCondition(generation int64, message string) metav1.Condition {
	return controller.NewCondition(
		controller.TypeReady,
		metav1.ConditionFalse,
		ReasonEndpointReady,
		message,
		generation,
	)
}

func EndpointTerminatingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		controller.TypeReady,
		metav1.ConditionFalse,
		ReasonEndpointReady,
		"Endpoint is terminating",
		generation,
	)
}
