// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates the Endpoint has been validated and its latest
	// EndpointRelease is in place. Failure modes are encoded in the Reason.
	ConditionReady controller.ConditionType = "Ready"

	// ConditionFinalizing indicates the Endpoint is being finalized (deleted).
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

// Constants for condition reasons

const (
	// ReasonReconciled indicates the Endpoint has been validated and the
	// latest EndpointRelease is in place.
	ReasonReconciled controller.ConditionReason = "Reconciled"

	// ReasonEndpointTypeNotFound indicates the referenced EndpointType or
	// ClusterEndpointType does not exist in the cluster yet.
	ReasonEndpointTypeNotFound controller.ConditionReason = "EndpointTypeNotFound"

	// ReasonInvalidParameters indicates spec.parameters does not satisfy the
	// referenced type's parameters schema.
	ReasonInvalidParameters controller.ConditionReason = "InvalidParameters"

	// ReasonBackendNotFound indicates a backendRef names a component that has no
	// Workload, or a workload endpoint the component's Workload does not declare.
	ReasonBackendNotFound controller.ConditionReason = "BackendNotFound"

	// ReasonBackendNotExposed indicates a backendRef names a workload endpoint
	// that is not reachable through the gateway: its visibility is neither
	// internal nor external, so the component's network policy would drop the
	// Endpoint's traffic.
	ReasonBackendNotExposed controller.ConditionReason = "BackendNotExposed"

	// ReasonFinalizing indicates the Endpoint is being finalized.
	ReasonFinalizing controller.ConditionReason = "Finalizing"
)

// NewFinalizingCondition returns a Finalizing=True condition observed at the
// given generation, used while the Endpoint is being torn down.
func NewFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonFinalizing,
		"Endpoint is finalizing",
		generation,
	)
}
