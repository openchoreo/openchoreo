// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the project is created
	ConditionCreated controller.ConditionType = "Created"
	// ConditionFinalizing represents whether the project is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"

	// ConditionReady reflects the project release lifecycle health: True
	// when spec.type is set and the inlined ProjectType snapshot resolves
	// (later commits also gate on the latest ProjectRelease being in
	// place). For Projects with no spec.type, Ready stays Unknown to
	// signal the legacy / opt-out mode.
	ConditionReady controller.ConditionType = "Ready"
)

const (
	// ReasonProjectCreated is the reason used when a project is created/ready
	ReasonProjectCreated controller.ConditionReason = "ProjectCreated"

	// ReasonProjectFinalizing is the reason used when a projects's dependents are being deleted'
	ReasonProjectFinalizing controller.ConditionReason = "ProjectFinalizing"

	// ReasonNoProjectType indicates the Project has no spec.type set; the
	// release-management reconcile path is skipped. Set spec.type on the
	// Project to opt into automatic ProjectRelease creation.
	ReasonNoProjectType controller.ConditionReason = "NoProjectType"

	// ReasonProjectTypeNotFound indicates the referenced ProjectType or
	// ClusterProjectType does not exist in the cluster yet.
	ReasonProjectTypeNotFound controller.ConditionReason = "ProjectTypeNotFound"

	// ReasonReconciled indicates the Project has been validated against
	// its referenced (Cluster)ProjectType. Later commits also gate on
	// status.latestRelease pointing at a current ProjectRelease.
	ReasonReconciled controller.ConditionReason = "Reconciled"
)

// NewProjectCreatedCondition creates a condition to indicate the project is created/ready
func NewProjectCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonProjectCreated,
		"Project is created",
		generation,
	)
}

func NewProjectFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonProjectFinalizing,
		"Project is finalizing",
		generation,
	)
}
